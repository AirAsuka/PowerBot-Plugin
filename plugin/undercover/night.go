package undercover

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	zero "github.com/wdvxdr1123/ZeroBot"
	"github.com/wdvxdr1123/ZeroBot/message"
)

const (
	nightActionTimeout = 2 * time.Minute
)

type namedElimination struct {
	Name string
	Role playerRole
}

type nightOutcome struct {
	Result       nightResult
	Killed       []namedElimination
	DeadPlayers  []namedElimination
	AlivePlayers []string
	NextName     string
	ClueRound    int
	Clues        []clueRecord
	FinalSummary string
	Room         *game
}

func registerNightCommands() {
	engine.OnRegex(nightActionPattern, zero.OnlyPrivate).SetBlock(true).Handle(func(ctx *zero.Ctx) {
		matches := ctx.State["regex_matched"].([]string)
		if matches[1] == "" {
			submitNoAttack(ctx)
			return
		}

		groupInput := ""
		targetInput := matches[1]
		if matches[2] != "" {
			groupInput = matches[1]
			targetInput = matches[2]
		}
		groupID, err := nightGroupID(ctx.Event.UserID, groupInput)
		if err != nil {
			sendError(ctx, err)
			return
		}
		targetID, err := strconv.ParseInt(targetInput, 10, 64)
		if err != nil || targetID <= 0 {
			sendError(ctx, errors.New("目标QQ号格式错误"))
			return
		}

		outcome, err := submitNightAction(groupID, ctx.Event.UserID, targetID)
		if err != nil {
			sendError(ctx, err)
			return
		}
		if !outcome.Result.Complete {
			action := "夜间选择已记录"
			if outcome.Result.Changed {
				action = "夜间选择已修改"
			}
			ctx.SendChain(message.Text(action, "（", outcome.Result.ActionsCast, "/", outcome.Result.ActionsNeeded, "）"))
			return
		}
		ctx.SendChain(message.Text("夜间选择已记录，所有玩家均已行动，正在结算。"))
		announceNight(ctx, groupID, outcome)
	})

	engine.OnRegex(blankGuessPattern, zero.OnlyPrivate).SetBlock(true).Handle(func(ctx *zero.Ctx) {
		matches := ctx.State["regex_matched"].([]string)
		groupID, err := blankGroupID(ctx.Event.UserID, matches[1])
		if err != nil {
			sendError(ctx, err)
			return
		}
		outcome, err := submitBlankGuess(groupID, ctx.Event.UserID, matches[2], matches[3], false)
		if err != nil {
			sendError(ctx, err)
			return
		}
		if outcome.Result.Winner == "白板" {
			ctx.SendChain(message.Text("两词全部猜中，白板获胜！"))
			announceNight(ctx, groupID, outcome)
			return
		}
		ctx.SendChain(message.Text("猜词错误，本夜机会已用完（", outcome.Result.ActionsCast, "/", outcome.Result.ActionsNeeded, "）。"))
		if outcome.Result.Complete {
			announceNight(ctx, groupID, outcome)
		}
	})

	engine.OnFullMatch("卧底猜词 放弃", zero.OnlyPrivate).SetBlock(true).Handle(func(ctx *zero.Ctx) {
		groups := rooms.pendingBlankGuessGroups(ctx.Event.UserID)
		if len(groups) == 0 {
			sendError(ctx, errors.New("没有找到你当前可猜词的夜晚房间"))
			return
		}
		completed := make(map[int64]nightOutcome)
		for _, groupID := range groups {
			outcome, err := submitBlankGuess(groupID, ctx.Event.UserID, "", "", true)
			if err != nil {
				continue
			}
			if outcome.Result.Complete {
				completed[groupID] = outcome
			}
		}
		ctx.SendChain(message.Text("已放弃本夜猜词机会。"))
		for groupID, outcome := range completed {
			announceNight(ctx, groupID, outcome)
		}
	})
}

func submitNoAttack(ctx *zero.Ctx) {
	groups := rooms.pendingNightGroups(ctx.Event.UserID)
	if len(groups) == 0 {
		sendError(ctx, errors.New("没有找到你当前可行动的夜晚房间"))
		return
	}
	completed := make(map[int64]nightOutcome)
	recorded := 0
	for _, groupID := range groups {
		outcome, err := submitNightAction(groupID, ctx.Event.UserID, 0)
		if err != nil {
			continue
		}
		recorded++
		if outcome.Result.Complete {
			completed[groupID] = outcome
		}
	}
	if recorded == 0 {
		sendError(ctx, errors.New("夜间状态已经变化，请重新查看游戏状态"))
		return
	}
	ctx.SendChain(message.Text("已记录不刀，无需填写QQ号。"))
	for groupID, outcome := range completed {
		announceNight(ctx, groupID, outcome)
	}
}

func nightGroupID(userID int64, input string) (int64, error) {
	if input != "" {
		groupID, err := strconv.ParseInt(input, 10, 64)
		if err != nil || groupID <= 0 {
			return 0, errors.New("群号格式错误")
		}
		return groupID, nil
	}
	groups := rooms.pendingNightGroups(userID)
	switch len(groups) {
	case 0:
		return 0, errors.New("没有找到你当前可行动的夜晚房间")
	case 1:
		return groups[0], nil
	default:
		return 0, errors.New("你在多个群有夜间行动，刀人时请使用“卧底刀人 群号 目标QQ号”")
	}
}

func blankGroupID(userID int64, input string) (int64, error) {
	if input != "" {
		groupID, err := strconv.ParseInt(input, 10, 64)
		if err != nil || groupID <= 0 {
			return 0, errors.New("群号格式错误")
		}
		return groupID, nil
	}
	groups := rooms.pendingBlankGuessGroups(userID)
	switch len(groups) {
	case 0:
		return 0, errors.New("没有找到你当前可猜词的夜晚房间")
	case 1:
		return groups[0], nil
	default:
		return 0, errors.New("你在多个群有白板猜词机会，请使用“卧底猜词 群号 词语1|词语2”")
	}
}

func startNight(ctx *zero.Ctx, groupID int64, expected *game, actors []int64) {
	prompts := make(map[int64]string, len(actors))
	blankID := int64(0)
	blankPrompt := ""
	round := 0
	err := rooms.withRoom(groupID, func(g *game) error {
		if g != expected || g.Phase != phaseNight {
			return errNotNight
		}
		round = g.Round
		for _, actor := range actors {
			var targets strings.Builder
			for _, id := range g.Order {
				if id != actor {
					fmt.Fprintf(&targets, "\n%s：%d", g.Players[id].Name, id)
				}
			}
			prompts[actor] = fmt.Sprintf(
				"【谁是卧底】第%d轮夜晚\n你不知道自己是平民还是狼人。请选择是否开刀：\n不刀：卧底刀人 不刀\n刀人：卧底刀人 目标QQ号\n可选目标：%s\n注意：狼人开刀会杀死目标；平民开刀会导致自己出局。",
				g.Round, targets.String())
		}
		if g.blankCanGuess() {
			blankID = g.BlankID
			blankPrompt = fmt.Sprintf(
				"【谁是卧底】第%d轮夜晚\n你是白板，本夜有一次猜词机会。\n猜词：卧底猜词 词语1|词语2（两个词顺序不限）\n放弃：卧底猜词 放弃\n同时猜中平民词和狼人词，你将立即获胜；猜错后本夜不能重猜。",
				g.Round)
		}
		return nil
	})
	if err != nil {
		return
	}

	failed := make([]int64, 0)
	for _, actor := range actors {
		if ctx.SendPrivateMessage(actor, message.Text(prompts[actor])) == 0 {
			failed = append(failed, actor)
		}
	}
	blankFailed := blankID != 0 && ctx.SendPrivateMessage(blankID, message.Text(blankPrompt)) == 0
	for _, actor := range failed {
		outcome, actionErr := submitNightAction(groupID, actor, 0)
		if actionErr == nil && outcome.Result.Complete {
			announceNight(ctx, groupID, outcome)
			return
		}
	}
	if blankFailed {
		outcome, actionErr := submitBlankGuess(groupID, blankID, "", "", true)
		if actionErr == nil && outcome.Result.Complete {
			announceNight(ctx, groupID, outcome)
			return
		}
	}

	time.AfterFunc(nightActionTimeout, func() {
		outcome, ok := forceNightResolution(groupID, expected, round)
		if ok {
			announceNight(ctx, groupID, outcome)
		}
	})
}

func submitNightAction(groupID, actorID, targetID int64) (nightOutcome, error) {
	var outcome nightOutcome
	err := rooms.withRoom(groupID, func(g *game) error {
		outcome.Room = g
		result, err := g.nightAction(actorID, targetID)
		if err != nil {
			return err
		}
		outcome.Result = result
		if result.Complete {
			captureNightOutcome(g, &outcome)
		}
		return nil
	})
	return outcome, err
}

func submitBlankGuess(groupID, actorID int64, first, second string, giveUp bool) (nightOutcome, error) {
	var outcome nightOutcome
	err := rooms.withRoom(groupID, func(g *game) error {
		outcome.Room = g
		result, err := g.blankGuess(actorID, first, second, giveUp)
		if err != nil {
			return err
		}
		outcome.Result = result
		if result.Complete {
			captureNightOutcome(g, &outcome)
		}
		return nil
	})
	return outcome, err
}

func forceNightResolution(groupID int64, expected *game, round int) (nightOutcome, bool) {
	var outcome nightOutcome
	err := rooms.withRoom(groupID, func(g *game) error {
		if g != expected || g.Phase != phaseNight || g.Round != round {
			return errNotNight
		}
		outcome.Room = g
		result, err := g.resolveNight(true)
		if err != nil {
			return err
		}
		outcome.Result = result
		captureNightOutcome(g, &outcome)
		return nil
	})
	return outcome, err == nil && outcome.Result.Complete
}

func captureNightOutcome(g *game, outcome *nightOutcome) {
	for _, killed := range outcome.Result.Killed {
		outcome.Killed = append(outcome.Killed, namedElimination{
			Name: g.Players[killed.ID].Name,
			Role: killed.Role,
		})
	}
	for _, id := range g.JoinOrder {
		p := g.Players[id]
		if p.Alive {
			outcome.AlivePlayers = append(outcome.AlivePlayers, p.Name)
			continue
		}
		outcome.DeadPlayers = append(outcome.DeadPlayers, namedElimination{
			Name: p.Name,
			Role: p.Role,
		})
	}
	if outcome.Result.NextDescriber != 0 {
		outcome.NextName = g.Players[outcome.Result.NextDescriber].Name
	}
	outcome.ClueRound = outcome.Result.ClueRound
	outcome.Clues = append([]clueRecord(nil), outcome.Result.Clues...)
	if outcome.Result.Winner != "" {
		outcome.FinalSummary = revealSummary(g, outcome.Result.Reveal)
	}
}

func announceNight(ctx *zero.Ctx, groupID int64, outcome nightOutcome) {
	if outcome.Result.Winner == "白板" {
		rooms.removeIfSame(groupID, outcome.Room)
		ctx.SendGroupMessage(groupID, message.Text(
			"白板成功猜出了平民词和狼人词，白板获胜！\n", outcome.FinalSummary,
		))
		return
	}
	var b strings.Builder
	if len(outcome.Killed) == 0 {
		b.WriteString("天亮了，昨夜平安无事。")
	} else {
		b.WriteString("天亮了，昨夜出局：")
		for i, killed := range outcome.Killed {
			if i > 0 {
				b.WriteString("、")
			}
			fmt.Fprintf(&b, "%s（%s）", killed.Name, killed.Role)
		}
		b.WriteString("。")
	}
	if outcome.Result.Winner != "" {
		rooms.removeIfSame(groupID, outcome.Room)
		fmt.Fprintf(&b, "\n%s阵营获胜！\n%s", outcome.Result.Winner, outcome.FinalSummary)
		ctx.SendGroupMessage(groupID, message.Text(b.String()))
		return
	}
	sendClueArchive(ctx, groupID, outcome.ClueRound, outcome.Clues)
	b.WriteString("\n")
	b.WriteString(formatRoundPlayers(outcome.DeadPlayers, outcome.AlivePlayers))
	b.WriteString("\n进入下一轮，请 ")
	ctx.SendGroupMessage(groupID, message.Message{
		message.Text(b.String()),
		message.At(outcome.Result.NextDescriber),
		message.Text("（", outcome.NextName, "）先描述。"),
	})
}

func formatRoundPlayers(dead []namedElimination, alive []string) string {
	var b strings.Builder
	b.WriteString("死亡玩家：")
	if len(dead) == 0 {
		b.WriteString("暂无")
	} else {
		for i, p := range dead {
			if i > 0 {
				b.WriteString("、")
			}
			fmt.Fprintf(&b, "%s（%s）", p.Name, p.Role)
		}
	}
	b.WriteString("\n存活玩家：")
	if len(alive) == 0 {
		b.WriteString("暂无")
	} else {
		b.WriteString(strings.Join(alive, "、"))
	}
	return b.String()
}

// sendClueArchive 在第 2 轮及以后开始前，把上一轮描述复刻为合并转发记录。
// 优先使用原玩家昵称和 QQ 作为节点发送者；平台不允许伪造其他发送者时，
// 再由机器人作为统一发送者复刻一遍，确保记录仍能发出。
func sendClueArchive(ctx *zero.Ctx, groupID int64, round int, clues []clueRecord) {
	if round <= 0 || len(clues) == 0 {
		return
	}
	nodes := make(message.Message, 0, len(clues)+1)
	nodes = append(nodes, message.CustomNode("谁是卧底", ctx.Event.SelfID, fmt.Sprintf("第%d轮发言记录", round)))
	for _, clue := range clues {
		nodes = append(nodes, message.CustomNode(clue.PlayerName, clue.PlayerID, clue.Text))
	}
	if ctx.SendGroupForwardMessage(groupID, nodes).Get("message_id").Int() != 0 {
		return
	}

	fallback := make(message.Message, 0, len(clues)+1)
	fallback = append(fallback, message.CustomNode("谁是卧底", ctx.Event.SelfID, fmt.Sprintf("第%d轮发言记录", round)))
	for _, clue := range clues {
		fallback = append(fallback, message.CustomNode(
			"谁是卧底",
			ctx.Event.SelfID,
			fmt.Sprintf("%s（%d）：%s", clue.PlayerName, clue.PlayerID, clue.Text),
		))
	}
	if ctx.SendGroupForwardMessage(groupID, fallback).Get("message_id").Int() == 0 {
		ctx.SendGroupMessage(groupID, message.Text("第", round, "轮发言记录发送失败，请稍后通过“卧底状态”确认游戏进度。"))
	}
}

func secretText(item secret) string {
	switch item.Role {
	case roleBlank:
		return "【谁是卧底】游戏已开始\n你的身份是：白板\n你没有词语，请根据其他人的描述隐藏身份。每轮夜晚你有一次机会猜出平民词和狼人词，两词全部正确即可单独获胜。\n回到群内，轮到你时发送：卧底描述 你的描述"
	case roleAngel:
		return fmt.Sprintf("【谁是卧底】游戏已开始\n你的身份是：天使\n你看到的两个词是：%s / %s\n你不知道哪个属于平民、哪个属于狼人；你没有夜间刀人行动。\n回到群内，轮到你时发送：卧底描述 你的描述", item.Words[0], item.Words[1])
	default:
		return fmt.Sprintf("【谁是卧底】游戏已开始\n你的词语是：%s\n你不知道自己是平民还是狼人。请保密，不要截图或直接说出词语。\n回到群内，轮到你时发送：卧底描述 你的描述", item.Words[0])
	}
}

func roleSetupText(playerCount int) string {
	switch {
	case playerCount >= 8:
		return "本局配置：2狼、1白板、1天使，其余为平民。"
	case playerCount >= 5:
		return "本局配置：1狼、1白板，其余为平民。"
	default:
		return "本局配置：1狼，其余为平民。"
	}
}

func revealSummary(g *game, reveal gameReveal) string {
	wolves := make([]string, 0, len(reveal.WolfIDs))
	for _, id := range reveal.WolfIDs {
		wolves = append(wolves, g.Players[id].Name)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "平民词：%s\n狼人词：%s\n狼人：%s", reveal.CivilianWord, reveal.UndercoverWord, strings.Join(wolves, "、"))
	if reveal.BlankID != 0 {
		fmt.Fprintf(&b, "\n白板：%s", g.Players[reveal.BlankID].Name)
	}
	if reveal.AngelID != 0 {
		fmt.Fprintf(&b, "\n天使：%s", g.Players[reveal.AngelID].Name)
	}
	return b.String()
}
