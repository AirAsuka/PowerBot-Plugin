package undercover

import (
	"errors"
	"time"

	zero "github.com/wdvxdr1123/ZeroBot"
	"github.com/wdvxdr1123/ZeroBot/message"
)

const blankLastGuessTimeout = 2 * time.Minute
const groupBlankGuessPattern = `^卧底猜词\s+(?:(\S+)\s+(\S+)|(放弃))\s*$`

var errBlankLastGuessIgnored = errors.New("现在没有白板出局猜词机会，或猜词已截止")

func (g *game) blankLastGuess(actor int64, first, second string, giveUp bool, now time.Time) (voteResult, error) {
	if g.Phase != phaseBlankLastGuess || !now.Before(g.BlankGuessDeadline) {
		return voteResult{}, errBlankLastGuessIgnored
	}
	if actor != g.BlankID {
		return voteResult{}, errors.New("只有本轮被投出的白板可以猜词")
	}
	return g.resolveBlankLastGuess(first, second, giveUp), nil
}

func (g *game) expireBlankLastGuess(deadline, now time.Time) (voteResult, error) {
	if g.Phase != phaseBlankLastGuess || deadline.IsZero() || !g.BlankGuessDeadline.Equal(deadline) || now.Before(deadline) {
		return voteResult{}, errBlankLastGuessIgnored
	}
	return g.resolveBlankLastGuess("", "", true), nil
}

func (g *game) resolveBlankLastGuess(first, second string, giveUp bool) voteResult {
	g.BlankGuessDeadline = time.Time{}
	g.touch()
	result := voteResult{Complete: true}
	if !giveUp && g.blankGuessCorrect(first, second) {
		result.Winner = "白板"
		g.finish(&result.Reveal)
		return result
	}
	return g.continueAfterElimination(result)
}

func registerBlankLastGuessCommands() {
	engine.OnRegex(groupBlankGuessPattern, zero.OnlyGroup).SetBlock(true).Handle(func(ctx *zero.Ctx) {
		matches := ctx.State["regex_matched"].([]string)
		processBlankLastGuess(ctx, matches[1], matches[2], matches[3] != "", nil, time.Time{})
	})
}

func scheduleBlankLastGuessTimeout(ctx *zero.Ctx, expected *game) {
	var deadline time.Time
	_ = rooms.withRoom(ctx.Event.GroupID, func(g *game) error {
		if g == expected && g.Phase == phaseBlankLastGuess {
			deadline = g.BlankGuessDeadline
		}
		return nil
	})
	if deadline.IsZero() {
		return
	}
	time.AfterFunc(time.Until(deadline), func() {
		processBlankLastGuess(ctx, "", "", true, expected, deadline)
	})
}

func processBlankLastGuess(ctx *zero.Ctx, first, second string, giveUp bool, expected *game, deadline time.Time) {
	var (
		room         *game
		result       voteResult
		finalSummary string
	)
	err := rooms.withRoom(ctx.Event.GroupID, func(g *game) error {
		room = g
		var err error
		if expected != nil {
			if g != expected {
				return errBlankLastGuessIgnored
			}
			result, err = g.expireBlankLastGuess(deadline, time.Now())
		} else {
			result, err = g.blankLastGuess(ctx.Event.UserID, first, second, giveUp, time.Now())
		}
		if err == nil && result.Winner != "" {
			finalSummary = revealSummary(g, result.Reveal)
		}
		return err
	})
	if err != nil {
		if expected == nil {
			sendError(ctx, err)
		}
		return
	}
	if result.Winner == "白板" {
		rooms.removeIfSame(ctx.Event.GroupID, room)
		ctx.SendChain(message.Text("白板成功猜出了平民词和狼人词，白板单独获胜！\n", finalSummary))
		return
	}
	text := "白板猜词错误，本次出局猜词机会已用完。"
	if expected != nil {
		text = "白板出局猜词限时2分钟已到，视为放弃。"
	} else if giveUp {
		text = "白板已放弃本次出局猜词机会。"
	}
	if result.Winner != "" {
		rooms.removeIfSame(ctx.Event.GroupID, room)
		ctx.SendChain(message.Text(text, "\n", result.Winner, "阵营获胜！\n", finalSummary))
		return
	}
	ctx.SendChain(message.Text(text, "\n天黑请闭眼，机器人正在私聊本夜可行动的玩家。"))
	startNight(ctx, ctx.Event.GroupID, room, result.NightActors)
}
