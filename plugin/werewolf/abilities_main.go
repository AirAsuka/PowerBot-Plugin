package werewolf

import (
	"errors"
	"fmt"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	zero "github.com/wdvxdr1123/ZeroBot"
	"github.com/wdvxdr1123/ZeroBot/message"
)

func handleBoard(ctx *zero.Ctx) {
	name := strings.TrimSpace(ctx.State["regex_matched"].([]string)[1])
	var text string
	err := rooms.withRoom(ctx.Event.GroupID, func(g *game) error {
		if err := g.selectBoard(ctx.Event.UserID, name); err != nil {
			return err
		}
		text = "已选择：" + g.setupDescription()
		if name == "自动" {
			text = "已切换为按人数自动选板；只支持8、9、10、12人。"
		}
		return nil
	})
	if err != nil {
		sendError(ctx, err)
		return
	}
	ctx.SendChain(message.Text(text))
}

func (s *roomStore) pendingSkills(uid int64) []int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	var ids []int64
	for gid := range s.rooms {
		g := s.room(gid)
		if g == nil {
			continue
		}
		p := g.Players[uid]
		if p == nil || !p.Alive || uid == g.FearTarget {
			continue
		}
		if g.Phase == phaseNightPrepare && g.currentPreparation().Actor == uid || g.Phase == phaseNightSpecial && (p.Role == roleWolfWizard && !g.ExtraActed[uid] || p.HasGift && !p.GiftUsed && !g.GiftActed[uid]) {
			ids = append(ids, gid)
		}
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids
}

func parseSkillChoice(gids []int64, fields []string) (int64, string, int64, error) {
	if len(gids) == 0 {
		return 0, "", 0, errors.New("没有找到你当前可使用特殊技能的房间")
	}
	gid := gids[0]
	if len(fields) >= 2 {
		candidate, _ := strconv.ParseInt(fields[0], 10, 64)
		if slices.Contains(gids, candidate) {
			gid = candidate
			fields = fields[1:]
		} else if len(gids) > 1 {
			return 0, "", 0, errors.New("多房间时请先填写正确群号")
		}
	} else if len(gids) > 1 {
		return 0, "", 0, errors.New("多房间时请在动作前填写群号")
	}
	action := "目标"
	if len(fields) == 1 && (fields[0] == "跳过" || fields[0] == "保留") {
		return gid, fields[0], 0, nil
	}
	if len(fields) == 2 {
		action, fields = fields[0], fields[1:]
	}
	if len(fields) != 1 {
		return 0, "", 0, errors.New("格式：狼人杀技能 [群号] [动作] 目标QQ号 / 跳过 / 保留")
	}
	if action != "目标" && action != "幸运" && action != "查验" && action != "毒药" && action != "守护" {
		return 0, "", 0, errors.New("未知技能动作，请按私聊提示操作")
	}
	target, err := strconv.ParseInt(fields[0], 10, 64)
	if err != nil || target <= 0 {
		return 0, "", 0, errors.New("目标QQ号格式错误")
	}
	return gid, action, target, nil
}

func handleSkill(ctx *zero.Ctx) {
	fields := strings.Fields(ctx.State["regex_matched"].([]string)[1])
	gid, action, target, err := parseSkillChoice(rooms.pendingSkills(ctx.Event.UserID), fields)
	if err != nil {
		sendError(ctx, err)
		return
	}
	var result skillResult
	var room *game
	var preparation bool
	err = rooms.withRoom(gid, func(g *game) error {
		room = g
		preparation = g.Phase == phaseNightPrepare
		var e error
		result, e = g.useSkill(ctx.Event.UserID, action, target)
		return e
	})
	if err != nil {
		sendError(ctx, err)
		return
	}
	ctx.SendChain(message.Text(result.Text))
	if result.Outcome.Complete {
		processNightOutcome(ctx, gid, room, result.Outcome)
	} else if preparation {
		promptWolves(ctx, gid, room)
	}
}

func handleDaySkill(ctx *zero.Ctx, want role) {
	target, err := matchedTarget(ctx)
	if err != nil {
		sendError(ctx, err)
		return
	}
	var r daySkillResult
	var room *game
	var text string
	err = rooms.withRoom(ctx.Event.GroupID, func(g *game) error {
		room = g
		var e error
		r, e = g.daySkill(ctx.Event.UserID, target, want)
		if e == nil {
			text = g.Players[ctx.Event.UserID].Name + " 发动" + want.String() + "技能。"
			for _, d := range r.Deaths {
				text += "\n" + deathLabel(g, d) + " 出局。"
			}
		}
		return e
	})
	if err != nil {
		sendError(ctx, err)
		return
	}
	announceShotOutcome(ctx, ctx.Event.GroupID, room, r.Outcome, text)
}

func deathLabel(g *game, d death) string {
	name := g.Players[d.ID].Name
	if g.boardRules().RevealDeaths {
		name += "（" + d.Role.String() + "）"
	}
	return name
}

func announceShotOutcome(ctx *zero.Ctx, gid int64, g *game, r hunterResult, prefix string) {
	if r.Winner != "" {
		finishAnnouncement(ctx, gid, g, prefix+"\n"+r.Winner+"阵营获胜！", r.Reveal)
		return
	}
	ctx.SendGroupMessage(gid, message.Text(prefix))
	if r.NeedHunter {
		promptShooter(ctx, gid, g)
		return
	}
	if r.AwaitingLastWords {
		promptDayLastWords(ctx, gid, g)
		return
	}
	if r.StartNight {
		promptWolves(ctx, gid, g)
		return
	}
	sendSpeechArchives(ctx, gid, r.Archives)
	if r.FirstSpeaker != 0 {
		ctx.SendGroupMessage(gid, message.Message{message.Text("进入白天，首先请 "), message.At(r.FirstSpeaker), message.Text(" 发言。")})
	}
}

func promptShooter(ctx *zero.Ctx, gid int64, expected *game) {
	var id int64
	_ = rooms.withRoom(gid, func(g *game) error {
		if g == expected && g.Phase == phaseHunter {
			id = g.PendingHunter
		}
		return nil
	})
	if id == 0 {
		return
	}
	// Do not label a wolf king as a hunter or reveal a dark-board shooter's card.
	ctx.SendGroupMessage(gid, message.Message{message.At(id), message.Text(" 请在2分钟内发送“猎人开枪 QQ号”（狼王也可用）或“猎人不开枪”。")})
	scheduleHunterTimeout(ctx, gid, expected)
}

func promptPreparation(ctx *zero.Ctx, gid int64, expected *game) {
	var turn skillTurn
	var round, index int
	var prompt string
	_ = rooms.withRoom(gid, func(g *game) error {
		if g != expected || g.Phase != phaseNightPrepare {
			return nil
		}
		turn = g.currentPreparation()
		round, index = g.Round, g.PreparationIndex
		if turn.Actor == 0 {
			return nil
		}
		usage := fmt.Sprintf("狼人杀技能 %d 目标QQ号", gid)
		if turn.Role == roleMerchant {
			usage = fmt.Sprintf("狼人杀技能 %d 查验/毒药/守护 目标QQ号", gid)
		}
		prompt = fmt.Sprintf("【狼人杀·群%d】第%d夜，%s行动\n%s\n发送：%s\n或：狼人杀技能 %d 跳过\n可选玩家：\n%s", gid, round, turn.Role, roleDescriptions[turn.Role], usage, gid, targetList(g, g.aliveIDs()))
		if turn.Role == roleDreamer {
			prompt += "\n摄梦人不能跳过，超时将随机选择一名其他存活玩家。"
		}
		return nil
	})
	if prompt == "" {
		return
	}
	ctx.SendPrivateMessage(turn.Actor, message.Text(prompt))
	time.AfterFunc(nightTimeout, func() {
		var r skillResult
		ok := false
		_ = rooms.withRoom(gid, func(g *game) error {
			if g != expected || g.Phase != phaseNightPrepare || g.Round != round || g.PreparationIndex != index {
				return nil
			}
			r = g.forcePreparation()
			ok = true
			return nil
		})
		if !ok {
			return
		}
		if r.Outcome.Complete {
			processNightOutcome(ctx, gid, expected, r.Outcome)
		} else {
			promptWolves(ctx, gid, expected)
		}
	})
}

func witchPrompt(g *game, gid int64) string {
	tip := "解药已用完，无法得知本夜狼刀目标。"
	if !g.AntidoteUsed {
		tip = "今夜无人被狼人选中。"
		if g.WolfVictim != 0 {
			tip = "狼人目标是 " + g.Players[g.WolfVictim].Name + "。"
		}
	}
	self := "本板子女巫全程不可自救。"
	if g.boardRules().WitchSelfSave {
		self = "只有首夜可以自救。"
	}
	return fmt.Sprintf("【狼人杀·群%d】第%d夜\n%s\n%s\n发送：女巫行动 %d 救 / 毒 QQ号 / 跳过\n每夜限用一瓶。解药可用：%t；毒药可用：%t\n可选玩家：\n%s", gid, g.Round, tip, self, gid, !g.AntidoteUsed, !g.PoisonUsed, targetList(g, g.aliveIDs()))
}

func parseWitchChoice(gids []int64, fields []string) (int64, string, int64, error) {
	if len(gids) == 0 {
		return 0, "", 0, errors.New("没有女巫行动房间")
	}
	gid := gids[0]
	if len(fields) > 0 {
		n, err := strconv.ParseInt(fields[0], 10, 64)
		if err == nil {
			if !slices.Contains(gids, n) {
				return 0, "", 0, errors.New("你不能在该群进行女巫行动")
			}
			gid, fields = n, fields[1:]
		} else if len(gids) > 1 {
			return 0, "", 0, errors.New("多房间时请填写群号")
		}
	}
	if len(fields) == 1 && (fields[0] == "救" || fields[0] == "跳过") {
		return gid, fields[0], 0, nil
	}
	if len(fields) == 2 && fields[0] == "毒" {
		target, err := strconv.ParseInt(fields[1], 10, 64)
		if err == nil && target > 0 {
			return gid, "毒", target, nil
		}
	}
	return 0, "", 0, errors.New("格式：女巫行动 [群号] 救 / 毒 QQ号 / 跳过")
}
