package werewolf

import (
	"time"

	zero "github.com/wdvxdr1123/ZeroBot"
)

func (g *game) acceptsVote(now time.Time) bool {
	return g.Phase == phaseVoting && now.Before(g.VoteDeadline)
}

// expireVoting uses the deadline to distinguish a revote from the original ballot.
func (g *game) expireVoting(deadline, now time.Time) (voteResult, error) {
	if g.Phase != phaseVoting || deadline.IsZero() || !g.VoteDeadline.Equal(deadline) || now.Before(deadline) {
		return voteResult{}, errVoteIgnored
	}
	result := voteResult{Cast: len(g.Votes), Needed: len(g.voterIDs())}
	result.Voted, result.Pending = g.voteProgress()
	return g.resolveVoting(result)
}

func scheduleVotingTimeout(ctx *zero.Ctx, expected *game) {
	scheduleVotingTimeoutForGroup(ctx, ctx.Event.GroupID, expected)
}

func scheduleVotingTimeoutForGroup(ctx *zero.Ctx, gid int64, expected *game) {
	var deadline time.Time
	_ = rooms.withRoom(gid, func(g *game) error {
		if g == expected && g.Phase == phaseVoting {
			deadline = g.VoteDeadline
		}
		return nil
	})
	if deadline.IsZero() {
		return
	}
	time.AfterFunc(time.Until(deadline), func() {
		processVoteInGroup(ctx, gid, 0, expected, deadline)
	})
}
