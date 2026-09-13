package undercover

import (
	"time"

	zero "github.com/wdvxdr1123/ZeroBot"
	"github.com/wdvxdr1123/ZeroBot/message"
)

type descriptionTimeoutOutcome struct {
	Room         *game
	Round        int
	SkippedID    int64
	SkippedName  string
	NextID       int64
	NextName     string
	Voting       bool
	VoteProgress string
	Archives     []clueArchive
}

// scheduleCurrentDescriptionTimeout starts a timer for the current describer.
// Timers are intentionally not cancelled: the round/player checks in
// forceDescriptionTimeout make an old callback a harmless no-op.
func scheduleCurrentDescriptionTimeout(ctx *zero.Ctx, groupID int64, expected *game) {
	var (
		round    int
		playerID int64
		deadline time.Time
	)
	err := rooms.withRoom(groupID, func(g *game) error {
		if g != expected || g.Phase != phaseDescribing {
			return errNotDescribing
		}
		round = g.Round
		playerID = g.currentDescriber()
		deadline = g.DescriptionDeadline
		return nil
	})
	if err != nil || playerID == 0 {
		return
	}

	delay := time.Until(deadline)
	if delay < 0 {
		delay = 0
	}
	time.AfterFunc(delay, func() {
		outcome, ok := forceDescriptionTimeout(groupID, expected, round, playerID)
		if !ok {
			return
		}
		announceDescriptionTimeout(ctx, groupID, outcome)
		if !outcome.Voting {
			scheduleCurrentDescriptionTimeout(ctx, groupID, expected)
		}
	})
}

func forceDescriptionTimeout(groupID int64, expected *game, round int, playerID int64) (descriptionTimeoutOutcome, bool) {
	var outcome descriptionTimeoutOutcome
	err := rooms.withRoom(groupID, func(g *game) error {
		if g != expected {
			return errNotDescribing
		}
		next, voting, skipped, ok := g.skipDescription(round, playerID)
		if !ok {
			return errNotDescribing
		}
		outcome.Room = g
		outcome.Round = round
		outcome.SkippedID = skipped
		outcome.SkippedName = g.Players[skipped].Name
		outcome.NextID = next
		outcome.Voting = voting
		if next != 0 {
			outcome.NextName = g.Players[next].Name
		}
		if voting {
			voted, pending := g.voteProgress()
			outcome.VoteProgress = formatVoteProgress(g, voted, pending)
			outcome.Archives = g.clueArchives(true)
		}
		return nil
	})
	return outcome, err == nil
}

func announceDescriptionTimeout(ctx *zero.Ctx, groupID int64, outcome descriptionTimeoutOutcome) {
	if outcome.Voting {
		scheduleVotingTimeout(ctx, outcome.Room)
		sendClueArchives(ctx, groupID, outcome.Archives)
		ctx.SendGroupMessage(groupID, message.Message{
			message.At(outcome.SkippedID),
			message.Text("（", outcome.SkippedName, "）2分钟内未描述，已自动跳过。\n本轮描述完毕，进入投票阶段（限时3分钟，超时未投视为弃票）。所有存活玩家请发送“卧底投票 @玩家”或“卧底投票 弃票”；可以改票，以最后一票为准。\n", outcome.VoteProgress),
		})
		return
	}
	ctx.SendGroupMessage(groupID, message.Message{
		message.At(outcome.SkippedID),
		message.Text("（", outcome.SkippedName, "）2分钟内未描述，已自动跳过。下一位请 "),
		message.At(outcome.NextID),
		message.Text("（", outcome.NextName, "）描述（限时2分钟）。"),
	})
}
