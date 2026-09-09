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
	votePattern   = `^狼人杀投票\s*(?:弃票|\[CQ:at,(?:[^\]]*,)?qq=(\d+)(?:,[^\]]*)?\]|(\d+))\s*$`
	hunterPattern = `^猎人开枪\s*(?:\[CQ:at,(?:[^\]]*,)?qq=(\d+)(?:,[^\]]*)?\]|(\d+))\s*$`
)

const helpText = `狼人杀（6—12人，机器人主持）
1. 创建狼人杀（创建者自动加入）
2. 其他玩家发送“加入狼人杀”
3. 房主发送“开始狼人杀”，机器人私聊身份
4. 夜晚按私聊提示行动；白天依次发送“狼人杀发言 内容”
5. 发言结束后发送“狼人杀投票 @玩家”或“狼人杀投票 弃票”（限时3分钟，超时未投视为弃票）

其他指令：狼人杀玩家、狼人杀状态、退出狼人杀、结束狼人杀
房主可发送“狼人杀开始投票”跳过剩余发言。
存活狼人在白天可发送“狼人自爆”，立即出局并结束白天，遗言后进入下一夜。
白天出局的玩家可在群里发送“狼人杀遗言 内容”或“狼人杀遗言 放弃”。

私聊指令：
狼人刀人 QQ号 / 狼人刀人 不刀 / 狼人查验 QQ号
狼队广播 内容（夜晚发送给所有存活狼队友）
女巫行动 救 / 女巫行动 毒 QQ号 / 女巫行动 跳过
狼人杀遗言 内容 / 狼人杀遗言 放弃（夜间出局）
同时参与多个群时，在动作后加群号，例如“狼人查验 群号 QQ号”“狼队广播 群号 内容”。

角色配置：6人局为预言家、猎人、2平民、2狼；7人加入女巫；8人3狼2民；9人3狼3民。
胜负规则：狼人全部出局则好人胜；平民全灭、神职全灭或狼人达到人数优势则狼人胜。
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
	engine.OnFullMatch("狼人自爆", zero.OnlyGroup).SetBlock(true).Handle(handleExplosion)
	engine.OnFullMatch("结束狼人杀", zero.OnlyGroup).SetBlock(true).Handle(endGame)
	engine.OnRegex(`^狼人杀发言\s+([\s\S]+)$`, zero.OnlyGroup).SetBlock(true).Handle(handleSpeech)
	engine.OnRegex(`^狼人杀遗言\s+([\s\S]+)$`, zero.OnlyGroup).SetBlock(true).Handle(handleDayLastWords)
	engine.OnRegex(votePattern, zero.OnlyGroup).SetBlock(true).Handle(handleVote)
	engine.OnRegex(hunterPattern, zero.OnlyGroup).SetBlock(true).Handle(handleHunter)
	engine.OnFullMatch("猎人不开枪", zero.OnlyGroup).SetBlock(true).Handle(func(ctx *zero.Ctx) { resolveHunter(ctx, 0) })

	engine.OnRegex(`^狼人刀人\s+(.+)$`, zero.OnlyPrivate).SetBlock(true).Handle(handleWolfAction)
	engine.OnRegex(`^狼队广播\s+([\s\S]+)$`, zero.OnlyPrivate).SetBlock(true).Handle(handleWolfBroadcast)
	engine.OnRegex(`^狼人查验\s+(.+)$`, zero.OnlyPrivate).SetBlock(true).Handle(handleInspect)
	engine.OnRegex(`^女巫行动\s+(.+)$`, zero.OnlyPrivate).SetBlock(true).Handle(handleWitch)
	engine.OnRegex(`^狼人杀遗言\s+([\s\S]+)$`, zero.OnlyPrivate).SetBlock(true).Handle(handleLastWords)
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
	var archives []speechArchive
	var room *game
	err := rooms.withRoom(ctx.Event.GroupID, func(g *game) error {
		room = g
		var e error
		next, voting, e = g.speak(ctx.Event.UserID, text, groupMessageID(ctx.Event.MessageID))
		if e == nil && voting {
			archives = g.speechArchives(true)
		}
		return e
	})
	if err != nil {
		if errors.Is(err, errNotYourTurn) && next != 0 {
			ctx.SendChain(message.Text("还没轮到你，请等待 "), message.At(next), message.Text(" 发言。"))
			return
		}
		sendError(ctx, err)
		return
	}
	if voting {
		scheduleVotingTimeout(ctx, room)
		if sendSpeechArchives(ctx, ctx.Event.GroupID, archives) {
			markVoteSummarySent(ctx.Event.GroupID, room)
		}
		ctx.SendChain(message.Text("所有存活玩家发言完毕，进入放逐投票（限时3分钟，超时未投视为弃票）。请发送“狼人杀投票 @玩家”或“狼人杀投票 弃票”，可在全员投完前改票。"))
		return
	}
	ctx.SendChain(message.Text("发言已记录，下一位请 "), message.At(next), message.Text(" 发言。"))
}

func skipToVote(ctx *zero.Ctx) {
	var archives []speechArchive
	var room *game
	err := rooms.withRoom(ctx.Event.GroupID, func(g *game) error {
		room = g
		if err := g.skipToVote(ctx.Event.UserID); err != nil {
			return err
		}
		archives = g.speechArchives(true)
		return nil
	})
	if err != nil {
		sendError(ctx, err)
		return
	}
	scheduleVotingTimeout(ctx, room)
	if sendSpeechArchives(ctx, ctx.Event.GroupID, archives) {
		markVoteSummarySent(ctx.Event.GroupID, room)
	}
	ctx.SendChain(message.Text("已进入放逐投票（限时3分钟，超时未投视为弃票），请发送“狼人杀投票 @玩家”或“狼人杀投票 弃票”。"))
}

func handleExplosion(ctx *zero.Ctx) {
	var result explosionResult
	var room *game
	var name string
	err := rooms.withRoom(ctx.Event.GroupID, func(g *game) error {
		room = g
		var actionErr error
		result, actionErr = g.explode(ctx.Event.UserID)
		if actionErr == nil {
			name = g.Players[ctx.Event.UserID].Name
		}
		return actionErr
	})
	if err != nil {
		sendError(ctx, err)
		return
	}
	if result.Winner != "" {
		finishAnnouncement(ctx, ctx.Event.GroupID, room, name+" 自爆并以狼人身份出局。\n"+result.Winner+"阵营获胜！", result.Reveal)
		return
	}
	if result.AwaitingLastWords {
		ctx.SendChain(message.Text(name, " 自爆并以狼人身份出局！白天发言及投票立即结束。"))
		promptDayLastWords(ctx, ctx.Event.GroupID, room)
		return
	}
	ctx.SendChain(message.Text(name, " 自爆并以狼人身份出局！白天发言及投票立即结束，天黑请闭眼。"))
	promptWolves(ctx, ctx.Event.GroupID, room)
}

func handleVote(ctx *zero.Ctx) {
	// Ignore closed ballots before parsing targets or sending any archive/reply.
	accepting := false
	_ = rooms.withRoom(ctx.Event.GroupID, func(g *game) error {
		accepting = g.acceptsVote(time.Now())
		return nil
	})
	if !accepting {
		return
	}
	matches := ctx.State["regex_matched"].([]string)
	target := int64(0)
	var err error
	if matches[1] != "" || matches[2] != "" {
		target, err = matchedTarget(ctx)
		if err != nil {
			sendError(ctx, err)
			return
		}
	}

	// 如果进入投票阶段时的转发被适配器明确拒绝，在第一票处理前补发。
	// 成功发送后会设置标记，因此正常路径不会重复刷屏。
	var pendingArchives []speechArchive
	var pendingRoom *game
	_ = rooms.withRoom(ctx.Event.GroupID, func(g *game) error {
		if g.acceptsVote(time.Now()) && !g.VoteSummarySent {
			pendingRoom = g
			pendingArchives = g.speechArchives(true)
		}
		return nil
	})
	if pendingRoom != nil && sendSpeechArchives(ctx, ctx.Event.GroupID, pendingArchives) {
		markVoteSummarySent(ctx.Event.GroupID, pendingRoom)
	}

	processVote(ctx, target, nil, time.Time{})
}

// processVote shares settlement and announcements between player votes and the timer.
func processVote(ctx *zero.Ctx, target int64, expected *game, deadline time.Time) {
	var r voteResult
	var room *game
	var eliminated string
	var ties []string
	var archives []speechArchive
	err := rooms.withRoom(ctx.Event.GroupID, func(g *game) error {
		room = g
		var e error
		if expected != nil {
			if g != expected {
				return errVoteIgnored
			}
			r, e = g.expireVoting(deadline, time.Now())
		} else {
			r, e = g.vote(ctx.Event.UserID, target)
		}
		if e != nil {
			return e
		}
		if r.Eliminated != 0 {
			eliminated = g.Players[r.Eliminated].Name
		}
		for _, id := range r.Tie {
			ties = append(ties, g.Players[id].Name)
		}
		if len(r.Tie) > 0 {
			archives = g.speechArchives(true)
		}
		return nil
	})
	if err != nil {
		if expected != nil || err == errVoteIgnored || err == errRoomNotFound {
			return
		}
		sendError(ctx, err)
		return
	}
	if expected != nil {
		ctx.SendChain(message.Text("投票限时3分钟已到，按已收到的票结算，未投票玩家视为弃票。"))
	}
	if !r.Complete {
		word := "投票已记录"
		if target == 0 {
			word = "弃票已记录"
		}
		if r.Changed && target == 0 {
			word = "已改为弃票"
		} else if r.Changed {
			word = "改票成功"
		}
		ctx.SendChain(message.Text(word, "（", r.Cast, "/", r.Needed, "）\n", progressText(room, r.Voted, r.Pending)))
		return
	}
	if len(r.Tie) > 0 {
		scheduleVotingTimeout(ctx, room)
		if sendSpeechArchives(ctx, ctx.Event.GroupID, archives) {
			markVoteSummarySent(ctx.Event.GroupID, room)
		}
		ctx.SendChain(message.Text("平票：", strings.Join(ties, "、"), "。请所有存活玩家重投（限时3分钟），且只能投给以上候选人或弃票。"))
		return
	}
	if r.TieLimitReached {
		ctx.SendChain(message.Text("连续两轮平票，本轮无人被放逐。天黑请闭眼，狼人请查看私聊。"))
		promptWolves(ctx, ctx.Event.GroupID, room)
		return
	}
	if r.NoElimination {
		ctx.SendChain(message.Text("本轮无有效候选票，无人被放逐。天黑请闭眼，狼人请查看私聊。"))
		promptWolves(ctx, ctx.Event.GroupID, room)
		return
	}
	if r.AwaitingLastWords {
		ctx.SendChain(message.Text(eliminated, " 被放逐。"))
		promptDayLastWords(ctx, ctx.Event.GroupID, room)
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
	if r.AwaitingLastWords {
		ctx.SendChain(message.Text(prefix))
		promptDayLastWords(ctx, ctx.Event.GroupID, room)
		return
	}
	if r.StartNight {
		ctx.SendChain(message.Text(prefix, "\n天黑请闭眼，狼人请查看私聊。"))
		promptWolves(ctx, ctx.Event.GroupID, room)
		return
	}
	sendSpeechArchives(ctx, ctx.Event.GroupID, r.Archives)
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
			fmt.Fprintf(&b, "\n轮次：第%d天", g.Round)
			if g.Phase == phaseNightLastWords {
				fmt.Fprintf(&b, "\n夜间结算中，遗言进度：%d/%d", len(g.LastWords), len(g.NightDeaths))
				text = b.String()
				return nil
			}
			if g.Phase == phaseDayLastWords {
				fmt.Fprintf(&b, "\n白天结算中，遗言进度：%d/%d", len(g.DayLastWords), len(g.DayDeaths))
				text = b.String()
				return nil
			}
			fmt.Fprintf(&b, "\n存活：%s", names(g, g.aliveIDs()))
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
	gid, target, err := privateWolfChoice(ctx.Event.UserID, fields)
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
	choice := "不刀"
	if target != 0 {
		choice = room.Players[target].Name
	}
	ctx.SendChain(message.Text("选择已记录：", choice, "（", r.Cast, "/", r.Needed, "）"))
	continueWolfActions(ctx, gid, room, r, ctx.Event.UserID, target)
}

func handleWolfBroadcast(ctx *zero.Ctx) {
	raw := strings.TrimSpace(ctx.State["regex_matched"].([]string)[1])
	gids := rooms.pendingWolfBroadcasts(ctx.Event.UserID)
	gid, text, err := resolveWolfBroadcast(gids, raw)
	if err != nil {
		sendError(ctx, err)
		return
	}
	var broadcast wolfBroadcast
	err = rooms.withRoom(gid, func(g *game) error {
		var actionErr error
		broadcast, actionErr = g.broadcastToWolves(ctx.Event.UserID, text)
		return actionErr
	})
	if err != nil {
		sendError(ctx, err)
		return
	}
	notice := broadcast.messageText()
	delivered := 0
	for _, teammate := range broadcast.Recipients {
		if ctx.SendPrivateMessage(teammate, message.Text(notice)) != 0 {
			delivered++
		}
	}
	if delivered != len(broadcast.Recipients) {
		ctx.SendChain(message.Text("广播发送完成：", delivered, "/", len(broadcast.Recipients), " 名狼队友收到。未收到的玩家可能尚未添加机器人好友。"))
		return
	}
	ctx.SendChain(message.Text("广播已发送给", delivered, "名狼队友。"))
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
		processNightOutcome(ctx, gid, room, r.Outcome)
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
		processNightOutcome(ctx, gid, room, r)
	}
}

func handleLastWords(ctx *zero.Ctx) {
	raw := strings.TrimSpace(ctx.State["regex_matched"].([]string)[1])
	gids := rooms.pendingLastWords(ctx.Event.UserID)
	if len(gids) == 0 {
		sendError(ctx, errors.New("没有找到你可以提交遗言的房间"))
		return
	}
	gid := gids[0]
	text := raw
	parts := strings.SplitN(raw, " ", 2)
	if len(gids) > 1 {
		if len(parts) != 2 {
			sendError(ctx, errors.New("你在多个群有遗言资格，请使用“狼人杀遗言 群号 内容”"))
			return
		}
		var err error
		gid, err = strconv.ParseInt(parts[0], 10, 64)
		if err != nil {
			sendError(ctx, errors.New("群号格式错误"))
			return
		}
		text = parts[1]
	} else if len(parts) == 2 && parts[0] == strconv.FormatInt(gid, 10) {
		text = parts[1]
	}
	var result nightResult
	var room *game
	err := rooms.withRoom(gid, func(g *game) error {
		room = g
		var actionErr error
		result, actionErr = g.submitLastWords(ctx.Event.UserID, text)
		return actionErr
	})
	if err != nil {
		sendError(ctx, err)
		return
	}
	ctx.SendChain(message.Text("遗言已记录。"))
	if !result.AwaitingLastWords {
		announceNight(ctx, gid, room, result)
	}
}

func handleDayLastWords(ctx *zero.Ctx) {
	text := strings.TrimSpace(ctx.State["regex_matched"].([]string)[1])
	var result dayLastWordsResult
	var room *game
	err := rooms.withRoom(ctx.Event.GroupID, func(g *game) error {
		room = g
		var actionErr error
		result, actionErr = g.submitDayLastWords(ctx.Event.UserID, text)
		return actionErr
	})
	if err != nil {
		sendError(ctx, err)
		return
	}
	ctx.SendChain(message.Text("遗言已记录。"))
	if !result.AwaitingLastWords {
		announceDayLastWords(ctx, ctx.Event.GroupID, room, result, false)
	}
}

func promptWolves(ctx *zero.Ctx, gid int64, expected *game) {
	var wolf int64
	var roundPlayers string
	_ = rooms.withRoom(gid, func(g *game) error {
		if g != expected || g.Phase != phaseNightWolf {
			return errors.New("阶段已变化")
		}
		wolf = g.currentWolf()
		roundPlayers = formatRoundPlayers(g)
		return nil
	})
	if roundPlayers != "" {
		ctx.SendGroupMessage(gid, message.Text(roundPlayers))
	}
	if wolf != 0 {
		promptWolf(ctx, gid, expected, wolf, 0, 0)
	}
}

func formatRoundPlayers(g *game) string {
	dead := make([]int64, 0, len(g.JoinOrder))
	alive := make([]int64, 0, len(g.JoinOrder))
	for _, id := range g.JoinOrder {
		if g.Players[id].Alive {
			alive = append(alive, id)
		} else {
			dead = append(dead, id)
		}
	}
	return fmt.Sprintf("第%d轮开始\n已死亡玩家：%s\n现存活玩家：%s", g.Round, names(g, dead), names(g, alive))
}

func promptWolf(ctx *zero.Ctx, gid int64, expected *game, wolf, previousWolf, previousTarget int64) {
	var prompt string
	round := 0
	timeoutTarget := int64(0)
	deciding := false
	valid := false
	_ = rooms.withRoom(gid, func(g *game) error {
		if g != expected || g.Phase != phaseNightWolf || g.currentWolf() != wolf {
			return errors.New("阶段已变化")
		}
		round = g.Round
		if g.WolfDeciding {
			deciding = true
			firstWolf, secondWolf := g.WolfOrder[0], g.WolfOrder[1]
			firstTarget, secondTarget := g.WolfVotes[firstWolf], g.WolfVotes[secondWolf]
			timeoutTarget = firstTarget
			prompt = fmt.Sprintf("【狼人杀】第%d夜最终裁决\n两名狼人的选择不一致：\n%s：%s\n%s：%s\n请1号狼从以上两个结果中给出最终选择。\n发送：狼人刀人 目标QQ号，或：狼人刀人 不刀\n若超时，将沿用你第一次提交的选择。", round, g.Players[firstWolf].Name, wolfChoiceText(g, firstTarget), g.Players[secondWolf].Name, wolfChoiceText(g, secondTarget))
			valid = true
			return nil
		}
		candidates := make([]int64, 0)
		for _, id := range g.aliveIDs() {
			candidates = append(candidates, id)
		}
		previous := ""
		if previousWolf != 0 {
			choice := "不刀"
			if previousTarget != 0 {
				choice = g.Players[previousTarget].Name + "（" + strconv.FormatInt(previousTarget, 10) + "）"
			}
			previous = "\n上一名狼队友 " + g.Players[previousWolf].Name + " 投给了：" + choice + "。"
		}
		prompt = fmt.Sprintf("【狼人杀】第%d夜\n轮到你刀人。可私聊发送“狼队广播 内容”与存活狼队友讨论。%s\n发送：狼人刀人 目标QQ号，或：狼人刀人 不刀\n可以选择任意存活玩家，包括自己和狼队友。\n可选目标：\n%s", round, previous, targetList(g, candidates))
		valid = true
		return nil
	})
	if !valid {
		return
	}
	ctx.SendPrivateMessage(wolf, message.Text(prompt))
	time.AfterFunc(nightTimeout, func() {
		var r wolfVoteResult
		ok := false
		_ = rooms.withRoom(gid, func(g *game) error {
			if g != expected || g.Phase != phaseNightWolf || g.Round != round || g.currentWolf() != wolf || g.WolfDeciding != deciding {
				return errors.New("阶段已变化")
			}
			var err error
			r, err = g.wolfVote(wolf, timeoutTarget)
			ok = err == nil
			return err
		})
		if ok {
			continueWolfActions(ctx, gid, expected, r, wolf, 0)
		}
	})
}

func wolfChoiceText(g *game, target int64) string {
	if target == 0 {
		return "不刀"
	}
	p := g.Players[target]
	return p.Name + "（" + strconv.FormatInt(target, 10) + "）"
}

func continueWolfActions(ctx *zero.Ctx, gid int64, g *game, r wolfVoteResult, previousWolf, previousTarget int64) {
	if !r.Ready {
		promptWolf(ctx, gid, g, r.NextWolf, previousWolf, previousTarget)
		return
	}
	notifyWolvesOfFinalTarget(ctx, gid, g, r.Victim)
	ctx.SendGroupMessage(gid, message.Text("狼人已全部完成投票，进入神职行动阶段。"))
	if r.Outcome.Complete {
		processNightOutcome(ctx, gid, g, r.Outcome)
		return
	}
	promptSpecial(ctx, gid, g, r.Victim)
}

func notifyWolvesOfFinalTarget(ctx *zero.Ctx, gid int64, expected *game, victim int64) {
	var wolves []int64
	var notice string
	_ = rooms.withRoom(gid, func(g *game) error {
		if g != expected {
			return errors.New("房间已变化")
		}
		wolves = append(wolves, g.WolfOrder...)
		notice = fmt.Sprintf("【狼人杀】第%d夜狼人投票流程结束，最终决定：不刀。", g.Round)
		if victim != 0 {
			notice = fmt.Sprintf("【狼人杀】第%d夜狼人投票流程结束，最终刀人目标：%s。", g.Round, wolfChoiceText(g, victim))
		}
		return nil
	})
	for _, wolf := range wolves {
		ctx.SendPrivateMessage(wolf, message.Text(notice))
	}
}

func promptSpecial(ctx *zero.Ctx, gid int64, expected *game, victim int64) {
	round := 0
	var seer, witch int64
	var targets, witchTargets, victimName string
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
		candidates = candidates[:0]
		for _, id := range g.aliveIDs() {
			if id != witch {
				candidates = append(candidates, id)
			}
		}
		witchTargets = targetList(g, candidates)
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
		selfSaveTip := "首夜可以自救。"
		if round > 1 {
			selfSaveTip = "首夜已过，不能自救。"
		}
		ctx.SendPrivateMessage(witch, message.Text("【狼人杀】第", round, "夜\n", tip, "\n请发送：女巫行动 救 / 女巫行动 毒 QQ号 / 女巫行动 跳过\n解药和毒药每局各限用一次；每夜只能行动一次，救人和毒人只能选择一个。\n", selfSaveTip, "\n解药可用：", antidoteAvailable, "，毒药可用：", poisonAvailable, "\n毒药可选目标：\n", witchTargets))
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
			processNightOutcome(ctx, gid, expected, r)
		}
	})
}

func processNightOutcome(ctx *zero.Ctx, gid int64, g *game, result nightResult) {
	if result.AwaitingLastWords {
		promptLastWords(ctx, gid, g, result.Deaths)
		return
	}
	announceNight(ctx, gid, g, result)
}

func promptLastWords(ctx *zero.Ctx, gid int64, expected *game, deaths []death) {
	round := 0
	_ = rooms.withRoom(gid, func(g *game) error {
		if g != expected || g.Phase != phaseNightLastWords {
			return errors.New("阶段已变化")
		}
		round = g.Round
		return nil
	})
	if round == 0 {
		return
	}
	for _, d := range deaths {
		ctx.SendPrivateMessage(d.ID, message.Text("【狼人杀】你在第", round, "夜出局。请在2分钟内私聊发送一句遗言：\n狼人杀遗言 你的遗言\n也可以发送：狼人杀遗言 放弃"))
	}
	time.AfterFunc(nightTimeout, func() {
		var result nightResult
		resolved := false
		_ = rooms.withRoom(gid, func(g *game) error {
			if g != expected || g.Phase != phaseNightLastWords || g.Round != round {
				return errors.New("阶段已变化")
			}
			result = g.forceLastWords()
			resolved = true
			return nil
		})
		if resolved {
			announceNight(ctx, gid, expected, result)
		}
	})
}

func promptDayLastWords(ctx *zero.Ctx, gid int64, expected *game) {
	var eligible []string
	round := 0
	_ = rooms.withRoom(gid, func(g *game) error {
		if g != expected || g.Phase != phaseDayLastWords {
			return errors.New("阶段已变化")
		}
		round = g.Round
		for _, d := range g.DayDeaths {
			eligible = append(eligible, g.Players[d.ID].Name)
		}
		return nil
	})
	if round == 0 || len(eligible) == 0 {
		return
	}
	ctx.SendGroupMessage(gid, message.Text(strings.Join(eligible, "、"), " 可在2分钟内直接在群里发送遗言：\n狼人杀遗言 你的遗言\n也可以发送：狼人杀遗言 放弃"))
	time.AfterFunc(nightTimeout, func() {
		var result dayLastWordsResult
		resolved := false
		_ = rooms.withRoom(gid, func(g *game) error {
			if g != expected || g.Phase != phaseDayLastWords || g.Round != round {
				return errors.New("阶段已变化")
			}
			result = g.forceDayLastWords()
			resolved = true
			return nil
		})
		if resolved {
			announceDayLastWords(ctx, gid, expected, result, true)
		}
	})
}

func announceDayLastWords(ctx *zero.Ctx, gid int64, g *game, r dayLastWordsResult, timedOut bool) {
	prefix := "白天遗言阶段结束。"
	if timedOut {
		prefix = "白天遗言时间到。"
	}
	if r.Winner != "" {
		finishAnnouncement(ctx, gid, g, prefix+"\n"+r.Winner+"阵营获胜！", r.Reveal)
		return
	}
	if r.StartNight {
		ctx.SendGroupMessage(gid, message.Text(prefix+"天黑请闭眼，狼人请查看私聊。"))
		promptWolves(ctx, gid, g)
		return
	}
	sendSpeechArchives(ctx, gid, r.Archives)
	ctx.SendGroupMessage(gid, message.Message{message.Text(prefix + "进入白天，首先请 "), message.At(r.FirstSpeaker), message.Text(" 发言。")})
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
		for _, d := range r.Deaths {
			words := strings.TrimSpace(r.LastWords[d.ID])
			if words == "" {
				words = "未留遗言"
			}
			fmt.Fprintf(&b, "\n%s的遗言：%s", g.Players[d.ID].Name, words)
		}
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
	sendSpeechArchives(ctx, gid, r.Archives)
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
		if result.AwaitingLastWords {
			ctx.SendGroupMessage(gid, message.Text(prefix))
			promptDayLastWords(ctx, gid, expected)
			return
		}
		if result.StartNight {
			ctx.SendGroupMessage(gid, message.Text(prefix+"\n天黑请闭眼，狼人请查看私聊。"))
			promptWolves(ctx, gid, expected)
			return
		}
		sendSpeechArchives(ctx, gid, result.Archives)
		ctx.SendGroupMessage(gid, message.Message{message.Text(prefix + "\n进入白天，首先请 "), message.At(result.FirstSpeaker), message.Text(" 发言。")})
	})
}

// sendSpeechArchives 按天依次把整局已有发言复刻为独立的合并转发记录。
func sendSpeechArchives(ctx *zero.Ctx, groupID int64, archives []speechArchive) bool {
	sent := true
	for _, archive := range archives {
		if !sendSpeechArchive(ctx, groupID, archive.Round, archive.Speeches) {
			sent = false
		}
	}
	return sent
}

// sendSpeechArchive 优先使用原玩家昵称和 QQ 作为节点发送者；平台不允许
// 使用玩家身份创建转发节点时，再由机器人统一标注玩家信息后重试。
func sendSpeechArchive(ctx *zero.Ctx, groupID int64, round int, speeches []speech) bool {
	if round <= 0 || len(speeches) == 0 {
		return true
	}

	// 优先转发玩家实际发送的群消息。部分 OneBot 实现会对自定义玩家节点
	// 返回成功，但消息随后被平台风控吞掉；引用真实消息节点更可靠。
	sourceNodes := make(message.Message, 0, len(speeches))
	for _, item := range speeches {
		if item.SourceMessageID <= 0 {
			sourceNodes = nil
			break
		}
		sourceNodes = append(sourceNodes, message.Node(item.SourceMessageID))
	}
	if len(sourceNodes) > 0 {
		ctx.SendGroupMessage(groupID, message.Text("第", round, "天发言汇总："))
		if ctx.SendGroupForwardMessage(groupID, sourceNodes).Get("message_id").Int() != 0 {
			return true
		}
	}

	nodes := make(message.Message, 0, len(speeches)+1)
	nodes = append(nodes, message.CustomNode("狼人杀", ctx.Event.SelfID, fmt.Sprintf("第%d天发言记录", round)))
	for _, item := range speeches {
		nodes = append(nodes, message.CustomNode(item.PlayerName, item.PlayerID, item.Text))
	}
	if ctx.SendGroupForwardMessage(groupID, nodes).Get("message_id").Int() != 0 {
		return true
	}

	fallback := make(message.Message, 0, len(speeches)+1)
	fallback = append(fallback, message.CustomNode("狼人杀", ctx.Event.SelfID, fmt.Sprintf("第%d天发言记录", round)))
	for _, item := range speeches {
		fallback = append(fallback, message.CustomNode(
			"狼人杀",
			ctx.Event.SelfID,
			fmt.Sprintf("%s（%d）：%s", item.PlayerName, item.PlayerID, item.Text),
		))
	}
	if ctx.SendGroupForwardMessage(groupID, fallback).Get("message_id").Int() == 0 {
		ctx.SendGroupMessage(groupID, message.Text("第", round, "天发言记录发送失败，请稍后通过“狼人杀状态”确认游戏进度。"))
		return false
	}
	return true
}

func groupMessageID(value any) int64 {
	switch id := value.(type) {
	case int64:
		return id
	case int:
		return int64(id)
	case float64:
		return int64(id)
	}
	id, _ := strconv.ParseInt(fmt.Sprint(value), 10, 64)
	return id
}

func markVoteSummarySent(groupID int64, expected *game) {
	_ = rooms.withRoom(groupID, func(g *game) error {
		if g == expected && g.Phase == phaseVoting {
			g.VoteSummarySent = true
		}
		return nil
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

func privateWolfChoice(uid int64, fields []string) (int64, int64, error) {
	gids := rooms.pending(uid, phaseNightWolf, roleWolf)
	if len(gids) == 0 {
		return 0, 0, errors.New("没有找到你当前可行动的狼人房间")
	}
	var gid int64
	var choice string
	if len(gids) == 1 && len(fields) == 1 {
		gid, choice = gids[0], fields[0]
	} else if len(fields) == 2 {
		var err error
		gid, err = strconv.ParseInt(fields[0], 10, 64)
		if err != nil || gid <= 0 {
			return 0, 0, errors.New("群号格式错误")
		}
		choice = fields[1]
	} else {
		return 0, 0, errors.New("格式：狼人刀人 目标QQ号 / 狼人刀人 不刀；多房间时在目标前填写群号")
	}
	if choice == "不刀" || choice == "空刀" {
		return gid, 0, nil
	}
	target, err := strconv.ParseInt(choice, 10, 64)
	if err != nil || target <= 0 {
		return 0, 0, errors.New("目标QQ号格式错误")
	}
	return gid, target, nil
}

func resolveWolfBroadcast(gids []int64, raw string) (int64, string, error) {
	if len(gids) == 0 {
		return 0, "", errors.New("没有找到你当前可广播的狼人房间")
	}
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, "", errors.New("格式：狼队广播 内容；多房间时使用“狼队广播 群号 内容”")
	}
	first := raw
	rest := ""
	if fields := strings.Fields(raw); len(fields) > 0 {
		first = fields[0]
		rest = strings.TrimSpace(strings.TrimPrefix(raw, first))
	}
	if len(gids) == 1 {
		gid := gids[0]
		if first == strconv.FormatInt(gid, 10) {
			if rest == "" {
				return 0, "", errors.New("广播内容不能为空")
			}
			return gid, rest, nil
		}
		return gid, raw, nil
	}
	if rest == "" {
		return 0, "", errors.New("你在多个群是存活狼人，请使用“狼队广播 群号 内容”")
	}
	gid, err := strconv.ParseInt(first, 10, 64)
	if err != nil || gid <= 0 {
		return 0, "", errors.New("群号格式错误")
	}
	for _, candidate := range gids {
		if candidate == gid {
			return gid, rest, nil
		}
	}
	return 0, "", errors.New("没有找到你在该群的夜间狼人房间")
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
	return fmt.Sprintf("本局配置：%d狼、%d预言家、%d女巫、%d猎人、%d平民。", c[roleWolf], c[roleSeer], c[roleWitch], c[roleHunter], c[roleVillager])
}
func secretText(g *game, s secret) string {
	text := "【狼人杀】游戏开始\n你的身份是：" + s.Role.String()
	if s.Role == roleWolf {
		teammates := "暂无"
		if len(s.Teammates) > 0 {
			teammates = targetList(g, s.Teammates)
		}
		text += "\n你的狼人队友（昵称：QQ）：\n" + teammates + "\n夜晚可私聊发送“狼队广播 内容”与存活狼队友讨论；机器人会按顺序私聊每名狼人刀人。"
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
func sendError(ctx *zero.Ctx, err error) { ctx.SendChain(message.Text("[狼人杀] ", err.Error())) }
