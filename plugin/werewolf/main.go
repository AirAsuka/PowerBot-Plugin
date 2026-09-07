// Package werewolf 提供由机器人主持的群聊狼人杀游戏。
package werewolf

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	ctrl "github.com/FloatTech/zbpctrl"
	"github.com/FloatTech/zbputils/control"
	"github.com/FloatTech/zbputils/ctxext"
	zero "github.com/wdvxdr1123/ZeroBot"
	"github.com/wdvxdr1123/ZeroBot/message"
)

const (
	nightTimeout  = 2 * time.Minute
	votePattern   = `^狼人杀投票\s*(?:\[CQ:at,(?:[^\]]*,)?qq=(\d+)(?:,[^\]]*)?\]|(\d+))\s*$`
	hunterPattern = `^猎人开枪\s*(?:\[CQ:at,(?:[^\]]*,)?qq=(\d+)(?:,[^\]]*)?\]|(\d+))\s*$`
)

const helpText = `狼人杀（6—12人，机器人主持）
1. 创建狼人杀（创建者自动加入）
2. 其他玩家发送“加入狼人杀”
3. 房主发送“开始狼人杀”，机器人私聊身份
4. 夜晚按私聊提示行动；白天依次发送“狼人杀发言 内容”
5. 发言结束后发送“狼人杀投票 @玩家”

其他指令：狼人杀玩家、狼人杀状态、退出狼人杀、结束狼人杀
房主可发送“狼人杀开始投票”跳过剩余发言。

私聊指令：
狼人刀人 QQ号 / 狼人查验 QQ号
女巫行动 救 / 女巫行动 毒 QQ号 / 女巫行动 跳过
同时参与多个群时，在动作后加群号，例如“狼人查验 群号 QQ号”。

角色配置：6—7人含狼人、预言家、女巫和平民；8人起加入猎人；9人起3狼；12人4狼。
胜负规则：狼人全部出局则好人胜；存活狼人数达到其他存活人数则狼人胜。
提示：所有玩家开局前应先添加机器人好友并私聊任意消息。`

var (
	engine = control.AutoRegister(&ctrl.Options[*zero.Ctx]{
		DisableOnDefault:  false,
		Brief:             "狼人杀",
		Help:              helpText,
		PrivateDataFolder: "werewolf",
	}).ApplySingle(ctxext.NewGroupSingle("本群上一条狼人杀指令还在处理中，请稍后再试"))
	rooms = newRoomStore()
)

func init() {
	engine.OnFullMatchGroup([]string{"狼人杀", "狼人杀帮助"}, zero.OnlyGroup).SetBlock(true).Handle(func(ctx *zero.Ctx) { ctx.SendChain(message.Text(helpText)) })
	engine.OnFullMatch("创建狼人杀", zero.OnlyGroup).SetBlock(true).Handle(createRoom)
	engine.OnFullMatch("加入狼人杀", zero.OnlyGroup).SetBlock(true).Handle(joinRoom)
	engine.OnFullMatch("退出狼人杀", zero.OnlyGroup).SetBlock(true).Handle(leaveRoom)
	engine.OnFullMatch("狼人杀玩家", zero.OnlyGroup).SetBlock(true).Handle(listPlayers)
	engine.OnFullMatch("开始狼人杀", zero.OnlyGroup).SetBlock(true).Handle(startGame)
	engine.OnFullMatch("狼人杀状态", zero.OnlyGroup).SetBlock(true).Handle(showStatus)
	engine.OnFullMatch("狼人杀开始投票", zero.OnlyGroup).SetBlock(true).Handle(skipToVote)
	engine.OnFullMatch("结束狼人杀", zero.OnlyGroup).SetBlock(true).Handle(endGame)
	engine.OnRegex(`^狼人杀发言\s+([\s\S]+)$`, zero.OnlyGroup).SetBlock(true).Handle(handleSpeech)
	engine.OnRegex(votePattern, zero.OnlyGroup).SetBlock(true).Handle(handleVote)
	engine.OnRegex(hunterPattern, zero.OnlyGroup).SetBlock(true).Handle(handleHunter)
	engine.OnFullMatch("猎人不开枪", zero.OnlyGroup).SetBlock(true).Handle(func(ctx *zero.Ctx) { resolveHunter(ctx, 0) })

	engine.OnRegex(`^狼人刀人\s+(.+)$`, zero.OnlyPrivate).SetBlock(true).Handle(handleWolfAction)
	engine.OnRegex(`^狼人查验\s+(.+)$`, zero.OnlyPrivate).SetBlock(true).Handle(handleInspect)
	engine.OnRegex(`^女巫行动\s+(.+)$`, zero.OnlyPrivate).SetBlock(true).Handle(handleWitch)
}

func createRoom(ctx *zero.Ctx) {
	_, err := rooms.create(ctx.Event.GroupID, ctx.Event.UserID, playerName(ctx, ctx.Event.UserID))
	if err != nil {
		sendError(ctx, err)
		return
	}
	ctx.SendChain(message.At(ctx.Event.UserID), message.Text(" 已创建狼人杀房间并自动加入。其他玩家发送“加入狼人杀”，至少6人后由房主开始。"))
}

func joinRoom(ctx *zero.Ctx) {
	count := 0
	err := rooms.withRoom(ctx.Event.GroupID, func(g *game) error {
		if err := g.join(ctx.Event.UserID, playerName(ctx, ctx.Event.UserID)); err != nil {
			return err
		}
		count = len(g.Players)
		return nil
	})
	if err != nil {
		sendError(ctx, err)
		return
	}
	ctx.SendChain(message.At(ctx.Event.UserID), message.Text(" 加入成功，当前玩家：", count, "/", maxPlayers))
}

func leaveRoom(ctx *zero.Ctx) {
	var room *game
	var count int
	var newHost int64
	err := rooms.withRoom(ctx.Event.GroupID, func(g *game) error {
		room = g
		id, err := g.leave(ctx.Event.UserID)
		newHost = id
		count = len(g.Players)
		return err
	})
	if err != nil {
		sendError(ctx, err)
		return
	}
	if count == 0 {
		rooms.removeIfSame(ctx.Event.GroupID, room)
		ctx.SendChain(message.Text("房间已解散。"))
		return
	}
	text := fmt.Sprintf("退出成功，当前还剩%d人。", count)
	if newHost != 0 {
		text += " 新房主是" + room.Players[newHost].Name + "。"
	}
	ctx.SendChain(message.Text(text))
}

func listPlayers(ctx *zero.Ctx) {
	var text string
	err := rooms.withRoom(ctx.Event.GroupID, func(g *game) error {
		var b strings.Builder
		fmt.Fprintf(&b, "狼人杀玩家（%d/%d）：", len(g.Players), maxPlayers)
		for i, id := range g.JoinOrder {
			p := g.Players[id]
			mark := ""
			if id == g.HostID {
				mark += "（房主）"
			}
			if !p.Alive && g.Phase != phaseLobby {
				mark += "（已出局）"
			}
			fmt.Fprintf(&b, "\n%d. %s%s", i+1, p.Name, mark)
		}
		text = b.String()
		return nil
	})
	if err != nil {
		sendError(ctx, err)
		return
	}
	ctx.SendChain(message.Text(text))
}

func startGame(ctx *zero.Ctx) {
	g, secrets, err := rooms.begin(ctx.Event.GroupID, ctx.Event.UserID)
	if err != nil {
		sendError(ctx, err)
		return
	}
	sent := make([]int64, 0, len(secrets))
	var failed int64
	for _, s := range secrets {
		if ctx.SendPrivateMessage(s.UserID, message.Text(secretText(g, s))) == 0 {
			failed = s.UserID
			break
		}
		sent = append(sent, s.UserID)
	}
	if failed != 0 {
		_ = rooms.finishDeal(ctx.Event.GroupID, g, false)
		for _, id := range sent {
			ctx.SendPrivateMessage(id, message.Text("【狼人杀】开局失败，刚才的身份作废。"))
		}
		ctx.SendChain(message.Text("开局失败：无法私聊 "), message.At(failed), message.Text("。请该玩家先私聊机器人后重试，房间名单已保留。"))
		return
	}
	if err = rooms.finishDeal(ctx.Event.GroupID, g, true); err != nil {
		sendError(ctx, err)
		return
	}
	ctx.SendChain(message.Text("身份发送完成！", setupText(len(secrets)), "\n第1夜开始，狼人请查看私聊并在2分钟内行动。"))
	promptWolves(ctx, ctx.Event.GroupID, g)
}

func handleSpeech(ctx *zero.Ctx) {
	text := ctx.State["regex_matched"].([]string)[1]
	var next int64
	var voting bool
	err := rooms.withRoom(ctx.Event.GroupID, func(g *game) error { var e error; next, voting, e = g.speak(ctx.Event.UserID, text); return e })
	if err != nil {
		if errors.Is(err, errNotYourTurn) && next != 0 {
			ctx.SendChain(message.Text("还没轮到你，请等待 "), message.At(next), message.Text(" 发言。"))
			return
		}
		sendError(ctx, err)
		return
	}
	if voting {
		ctx.SendChain(message.Text("所有存活玩家发言完毕，进入放逐投票。请发送“狼人杀投票 @玩家”，可在全员投完前改票。"))
		return
	}
	ctx.SendChain(message.Text("发言已记录，下一位请 "), message.At(next), message.Text(" 发言。"))
}

func skipToVote(ctx *zero.Ctx) {
	err := rooms.withRoom(ctx.Event.GroupID, func(g *game) error { return g.skipToVote(ctx.Event.UserID) })
	if err != nil {
		sendError(ctx, err)
		return
	}
	ctx.SendChain(message.Text("已进入放逐投票，请发送“狼人杀投票 @玩家”。"))
}

func handleVote(ctx *zero.Ctx) {
	target, err := matchedTarget(ctx)
	if err != nil {
		sendError(ctx, err)
		return
	}
	var r voteResult
	var room *game
	var eliminated string
	var ties []string
	err = rooms.withRoom(ctx.Event.GroupID, func(g *game) error {
		room = g
		var e error
		r, e = g.vote(ctx.Event.UserID, target)
		if e != nil {
			return e
		}
		if r.Eliminated != 0 {
			eliminated = g.Players[r.Eliminated].Name
		}
		for _, id := range r.Tie {
			ties = append(ties, g.Players[id].Name)
		}
		return nil
	})
	if err != nil {
		sendError(ctx, err)
		return
	}
	if !r.Complete {
		word := "投票已记录"
		if r.Changed {
			word = "改票成功"
		}
		ctx.SendChain(message.Text(word, "（", r.Cast, "/", r.Needed, "）\n", progressText(room, r.Voted, r.Pending)))
		return
	}
	if len(r.Tie) > 0 {
		ctx.SendChain(message.Text("平票：", strings.Join(ties, "、"), "。请所有存活玩家重投，且只能投给以上候选人。"))
		return
	}
	if r.Winner != "" {
		finishAnnouncement(ctx, ctx.Event.GroupID, room, eliminated+" 被放逐。\n"+r.Winner+"阵营获胜！", r.Reveal)
		return
	}
	if r.NeedHunter {
		ctx.SendChain(message.Text(eliminated, " 被放逐。猎人请发送“猎人开枪 @玩家”或“猎人不开枪”。"))
		scheduleHunterTimeout(ctx, ctx.Event.GroupID, room)
		return
	}
	ctx.SendChain(message.Text(eliminated, " 被放逐。天黑请闭眼，狼人请查看私聊。"))
	promptWolves(ctx, ctx.Event.GroupID, room)
}

func handleHunter(ctx *zero.Ctx) {
	target, err := matchedTarget(ctx)
	if err != nil {
		sendError(ctx, err)
		return
	}
	resolveHunter(ctx, target)
}

func resolveHunter(ctx *zero.Ctx, target int64) {
	var r hunterResult
	var room *game
	var shotName string
	err := rooms.withRoom(ctx.Event.GroupID, func(g *game) error {
		room = g
		var e error
		r, e = g.hunterShoot(ctx.Event.UserID, target)
		if r.Shot != 0 {
			shotName = g.Players[r.Shot].Name
		}
		return e
	})
	if err != nil {
		sendError(ctx, err)
		return
	}
	prefix := "猎人选择不开枪。"
	if r.Shot != 0 {
		prefix = "猎人开枪带走了 " + shotName + "。"
	}
	if r.Winner != "" {
		finishAnnouncement(ctx, ctx.Event.GroupID, room, prefix+"\n"+r.Winner+"阵营获胜！", r.Reveal)
		return
	}
	if r.StartNight {
		ctx.SendChain(message.Text(prefix, "\n天黑请闭眼，狼人请查看私聊。"))
		promptWolves(ctx, ctx.Event.GroupID, room)
		return
	}
	ctx.SendChain(message.Text(prefix, "\n进入白天，首先请 "), message.At(r.FirstSpeaker), message.Text(" 发言。"))
}

func showStatus(ctx *zero.Ctx) {
	var text string
	err := rooms.withRoom(ctx.Event.GroupID, func(g *game) error {
		var b strings.Builder
		fmt.Fprintf(&b, "狼人杀状态：%s\n房主：%s", g.Phase, g.Players[g.HostID].Name)
		if g.Phase == phaseLobby {
			fmt.Fprintf(&b, "\n玩家：%d/%d（至少%d人开局）", len(g.Players), maxPlayers, minPlayers)
		} else {
			fmt.Fprintf(&b, "\n轮次：第%d天\n存活：%s", g.Round, names(g, g.aliveIDs()))
			switch g.Phase {
			case phaseNightWolf:
				fmt.Fprintf(&b, "\n狼人行动进度：%d/%d", len(g.WolfVotes), g.aliveRoleCount(roleWolf))
			case phaseNightSpecial:
				n := 0
				if g.SeerActed {
					n++
				}
				if g.WitchActed {
					n++
				}
				fmt.Fprintf(&b, "\n神职行动进度：%d/2", n)
			case phaseDay:
				fmt.Fprintf(&b, "\n当前发言：%s", g.Players[g.currentSpeaker()].Name)
			case phaseVoting:
				v, p := g.voteProgress()
				fmt.Fprintf(&b, "\n%s", progressText(g, v, p))
			}
		}
		text = b.String()
		return nil
	})
	if err != nil {
		sendError(ctx, err)
		return
	}
	ctx.SendChain(message.Text(text))
}

func endGame(ctx *zero.Ctx) {
	g, err := rooms.end(ctx.Event.GroupID, ctx.Event.UserID, zero.AdminPermission(ctx))
	if err != nil {
		sendError(ctx, err)
		return
	}
	text := "狼人杀房间已结束。"
	if g.Phase != phaseLobby && g.Phase != phaseDealing {
		text += "\n" + revealText(g, g.makeReveal())
	}
	ctx.SendChain(message.Text(text))
}

func handleWolfAction(ctx *zero.Ctx) {
	fields := strings.Fields(ctx.State["regex_matched"].([]string)[1])
	gid, target, err := privateTarget(ctx.Event.UserID, phaseNightWolf, []role{roleWolf}, fields)
	if err != nil {
		sendError(ctx, err)
		return
	}
	var r wolfVoteResult
	var room *game
	err = rooms.withRoom(gid, func(g *game) error { room = g; var e error; r, e = g.wolfVote(ctx.Event.UserID, target); return e })
	if err != nil {
		sendError(ctx, err)
		return
	}
	if !r.Ready {
		word := "选择已记录"
		if r.Changed {
			word = "选择已修改"
		}
		ctx.SendChain(message.Text(word, "（", r.Cast, "/", r.Needed, "）"))
		return
	}
	ctx.SendChain(message.Text("选择已记录，狼人行动结束。"))
	if r.Outcome.Complete {
		announceNight(ctx, gid, room, r.Outcome)
		return
	}
	promptSpecial(ctx, gid, room, r.Victim)
}

func handleInspect(ctx *zero.Ctx) {
	fields := strings.Fields(ctx.State["regex_matched"].([]string)[1])
	gid, target, err := privateTarget(ctx.Event.UserID, phaseNightSpecial, []role{roleSeer}, fields)
	if err != nil {
		sendError(ctx, err)
		return
	}
	var r inspectResult
	var room *game
	err = rooms.withRoom(gid, func(g *game) error { room = g; var e error; r, e = g.inspect(ctx.Event.UserID, target); return e })
	if err != nil {
		sendError(ctx, err)
		return
	}
	identity := "好人"
	if r.IsWolf {
		identity = "狼人"
	}
	ctx.SendChain(message.Text("查验结果：", room.Players[target].Name, " 是", identity, "。"))
	if r.Outcome.Complete {
		announceNight(ctx, gid, room, r.Outcome)
	}
}

func handleWitch(ctx *zero.Ctx) {
	fields := strings.Fields(ctx.State["regex_matched"].([]string)[1])
	gids := rooms.pending(ctx.Event.UserID, phaseNightSpecial, roleWitch)
	if len(gids) == 0 {
		sendError(ctx, errors.New("没有找到你可进行女巫行动的房间"))
		return
	}
	gid := int64(0)
	action := ""
	target := int64(0)
	if len(gids) == 1 {
		gid = gids[0]
		if len(fields) > 0 {
			action = fields[0]
		}
		if len(fields) == 2 {
			target, _ = strconv.ParseInt(fields[1], 10, 64)
		}
		if len(fields) > 2 {
			sendError(ctx, errors.New("格式：女巫行动 救 / 毒 QQ号 / 跳过"))
			return
		}
	} else {
		if len(fields) < 2 {
			sendError(ctx, errors.New("你在多个群有行动，请在动作前填写群号"))
			return
		}
		gid, _ = strconv.ParseInt(fields[0], 10, 64)
		action = fields[1]
		if len(fields) == 3 {
			target, _ = strconv.ParseInt(fields[2], 10, 64)
		}
	}
	if action == "毒" && target <= 0 {
		sendError(ctx, errors.New("使用毒药时必须填写目标QQ号"))
		return
	}
	var r nightResult
	var room *game
	err := rooms.withRoom(gid, func(g *game) error {
		room = g
		var e error
		r, e = g.witchAct(ctx.Event.UserID, action, target)
		return e
	})
	if err != nil {
		sendError(ctx, err)
		return
	}
	ctx.SendChain(message.Text("女巫行动已记录。"))
	if r.Complete {
		announceNight(ctx, gid, room, r)
	}
}

func promptWolves(ctx *zero.Ctx, gid int64, expected *game) {
	var wolves []int64
	var targets string
	round := 0
	_ = rooms.withRoom(gid, func(g *game) error {
		if g != expected || g.Phase != phaseNightWolf {
			return errors.New("阶段已变化")
		}
		wolves = g.roleIDs(roleWolf, true)
		candidates := make([]int64, 0)
		for _, id := range g.aliveIDs() {
			if g.Players[id].Role != roleWolf {
				candidates = append(candidates, id)
			}
		}
		targets = targetList(g, candidates)
		round = g.Round
		return nil
	})
	for _, id := range wolves {
		ctx.SendPrivateMessage(id, message.Text("【狼人杀】第", round, "夜\n你是狼人，请在2分钟内发送：狼人刀人 目标QQ号\n可选目标：\n", targets))
	}
	time.AfterFunc(nightTimeout, func() {
		var r wolfVoteResult
		ok := false
		_ = rooms.withRoom(gid, func(g *game) error {
			if g != expected || g.Phase != phaseNightWolf || g.Round != round {
				return errors.New("阶段已变化")
			}
			r = g.forceWolfVotes()
			ok = true
			return nil
		})
		if !ok {
			return
		}
		if r.Outcome.Complete {
			announceNight(ctx, gid, expected, r.Outcome)
		} else {
			ctx.SendGroupMessage(gid, message.Text("狼人行动时间结束，进入神职行动阶段。"))
			promptSpecial(ctx, gid, expected, r.Victim)
		}
	})
}

func promptSpecial(ctx *zero.Ctx, gid int64, expected *game, victim int64) {
	round := 0
	var seer, witch int64
	var targets, victimName string
	var antidoteAvailable, poisonAvailable bool
	_ = rooms.withRoom(gid, func(g *game) error {
		if g != expected || g.Phase != phaseNightSpecial {
			return errors.New("阶段已变化")
		}
		ids := g.roleIDs(roleSeer, true)
		if len(ids) > 0 {
			seer = ids[0]
		}
		ids = g.roleIDs(roleWitch, true)
		if len(ids) > 0 && !g.WitchActed {
			witch = ids[0]
		}
		candidates := make([]int64, 0)
		for _, id := range g.aliveIDs() {
			if id != seer {
				candidates = append(candidates, id)
			}
		}
		targets = targetList(g, candidates)
		if victim != 0 {
			victimName = g.Players[victim].Name
		}
		round = g.Round
		antidoteAvailable, poisonAvailable = !g.AntidoteUsed, !g.PoisonUsed
		return nil
	})
	if seer != 0 {
		ctx.SendPrivateMessage(seer, message.Text("【狼人杀】第", round, "夜\n请发送：狼人查验 目标QQ号\n可选目标：\n", targets))
	}
	if witch != 0 {
		tip := "今夜无人被狼人选中。"
		if victim != 0 {
			tip = "狼人目标是 " + victimName + "。"
		}
		ctx.SendPrivateMessage(witch, message.Text("【狼人杀】第", round, "夜\n", tip, "\n请发送：女巫行动 救 / 女巫行动 毒 QQ号 / 女巫行动 跳过\n解药可用：", antidoteAvailable, "，毒药可用：", poisonAvailable))
	}
	time.AfterFunc(nightTimeout, func() {
		var r nightResult
		ok := false
		_ = rooms.withRoom(gid, func(g *game) error {
			if g != expected || g.Phase != phaseNightSpecial || g.Round != round {
				return errors.New("阶段已变化")
			}
			r = g.forceSpecial()
			ok = true
			return nil
		})
		if ok {
			announceNight(ctx, gid, expected, r)
		}
	})
}

func announceNight(ctx *zero.Ctx, gid int64, g *game, r nightResult) {
	var b strings.Builder
	if len(r.Deaths) == 0 {
		b.WriteString("天亮了，昨夜平安无事。")
	} else {
		b.WriteString("天亮了，昨夜出局：")
		for i, d := range r.Deaths {
			if i > 0 {
				b.WriteString("、")
			}
			b.WriteString(g.Players[d.ID].Name)
		}
		b.WriteString("。")
	}
	if r.Winner != "" {
		finishAnnouncement(ctx, gid, g, b.String()+"\n"+r.Winner+"阵营获胜！", r.Reveal)
		return
	}
	if r.NeedHunter {
		b.WriteString("\n猎人请发送“猎人开枪 @玩家”或“猎人不开枪”。")
		ctx.SendGroupMessage(gid, message.Text(b.String()))
		scheduleHunterTimeout(ctx, gid, g)
		return
	}
	ctx.SendGroupMessage(gid, message.Message{message.Text(b.String() + "\n进入白天，首先请 "), message.At(r.FirstSpeaker), message.Text(" 发言。")})
}

func scheduleHunterTimeout(ctx *zero.Ctx, gid int64, expected *game) {
	var hunterID int64
	_ = rooms.withRoom(gid, func(g *game) error {
		if g != expected || g.Phase != phaseHunter {
			return errors.New("阶段已变化")
		}
		hunterID = g.PendingHunter
		return nil
	})
	if hunterID == 0 {
		return
	}
	time.AfterFunc(nightTimeout, func() {
		var result hunterResult
		resolved := false
		_ = rooms.withRoom(gid, func(g *game) error {
			if g != expected || g.Phase != phaseHunter || g.PendingHunter != hunterID {
				return errors.New("阶段已变化")
			}
			var err error
			result, err = g.hunterShoot(hunterID, 0)
			resolved = err == nil
			return err
		})
		if !resolved {
			return
		}
		prefix := "猎人行动超时，视为不开枪。"
		if result.Winner != "" {
			finishAnnouncement(ctx, gid, expected, prefix+"\n"+result.Winner+"阵营获胜！", result.Reveal)
			return
		}
		if result.StartNight {
			ctx.SendGroupMessage(gid, message.Text(prefix+"\n天黑请闭眼，狼人请查看私聊。"))
			promptWolves(ctx, gid, expected)
			return
		}
		ctx.SendGroupMessage(gid, message.Message{message.Text(prefix + "\n进入白天，首先请 "), message.At(result.FirstSpeaker), message.Text(" 发言。")})
	})
}

func finishAnnouncement(ctx *zero.Ctx, gid int64, g *game, prefix string, r reveal) {
	rooms.removeIfSame(gid, g)
	ctx.SendGroupMessage(gid, message.Text(prefix, "\n", revealText(g, r)))
}

func privateTarget(uid int64, p phase, roles []role, fields []string) (int64, int64, error) {
	gids := rooms.pending(uid, p, roles...)
	if len(gids) == 0 {
		return 0, 0, errors.New("没有找到你当前可行动的房间")
	}
	var gid, target int64
	if len(gids) == 1 && len(fields) == 1 {
		gid = gids[0]
		target, _ = strconv.ParseInt(fields[0], 10, 64)
	} else if len(fields) == 2 {
		gid, _ = strconv.ParseInt(fields[0], 10, 64)
		target, _ = strconv.ParseInt(fields[1], 10, 64)
	} else {
		return 0, 0, errors.New("格式错误；多房间行动时请使用“指令 群号 目标QQ号”")
	}
	if gid <= 0 || target <= 0 {
		return 0, 0, errors.New("群号或目标QQ号格式错误")
	}
	return gid, target, nil
}

func matchedTarget(ctx *zero.Ctx) (int64, error) {
	m := ctx.State["regex_matched"].([]string)
	s := m[1]
	if s == "" {
		s = m[2]
	}
	id, err := strconv.ParseInt(s, 10, 64)
	if err != nil || id <= 0 {
		return 0, errors.New("无法识别目标，请@玩家或填写QQ号")
	}
	return id, nil
}

func playerName(ctx *zero.Ctx, id int64) string {
	name := strings.TrimSpace(ctx.CardOrNickName(id))
	if name == "" {
		return strconv.FormatInt(id, 10)
	}
	return name
}
func names(g *game, ids []int64) string {
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		out = append(out, g.Players[id].Name)
	}
	if len(out) == 0 {
		return "暂无"
	}
	return strings.Join(out, "、")
}
func progressText(g *game, voted, pending []int64) string {
	return "已投票：" + names(g, voted) + "\n未投票：" + names(g, pending)
}
func targetList(g *game, ids []int64) string {
	var b strings.Builder
	for _, id := range ids {
		fmt.Fprintf(&b, "%s：%d\n", g.Players[id].Name, id)
	}
	return strings.TrimSuffix(b.String(), "\n")
}
func setupText(n int) string {
	c := roleCounts(n)
	return fmt.Sprintf("本局配置：%d狼、1预言家、1女巫、%d猎人、%d平民。", c[roleWolf], c[roleHunter], c[roleVillager])
}
func secretText(g *game, s secret) string {
	text := "【狼人杀】游戏开始\n你的身份是：" + s.Role.String()
	if s.Role == roleWolf {
		text += "\n你的狼人队友：" + names(g, s.Teammates) + "\n夜晚请按提示私聊刀人。"
	} else if s.Role == roleSeer {
		text += "\n每夜可查验一名存活玩家是否为狼人。"
	} else if s.Role == roleWitch {
		text += "\n你有一瓶解药和一瓶毒药，每瓶全局限用一次，每夜限用一瓶。"
	} else if s.Role == roleHunter {
		text += "\n被狼人杀死或被放逐时可以开枪；被女巫毒死时不能开枪。"
	}
	return text + "\n请保密身份，回到群内等待主持。"
}
func revealText(g *game, r reveal) string {
	parts := make([]string, 0, len(g.JoinOrder))
	for _, id := range g.JoinOrder {
		parts = append(parts, fmt.Sprintf("%s：%s", g.Players[id].Name, r.Roles[id]))
	}
	return "身份揭晓：\n" + strings.Join(parts, "\n")
}
func (g *game) roleIDs(want role, alive bool) []int64 {
	var ids []int64
	for _, id := range g.JoinOrder {
		p := g.Players[id]
		if p.Role == want && (!alive || p.Alive) {
			ids = append(ids, id)
		}
	}
	return ids
}
func (g *game) makeReveal() reveal {
	r := reveal{Roles: make(map[int64]role, len(g.Players))}
	for id, p := range g.Players {
		r.Roles[id] = p.Role
	}
	return r
}
func sendError(ctx *zero.Ctx, err error) { ctx.SendChain(message.Text("[狼人杀] ", err.Error())) }
