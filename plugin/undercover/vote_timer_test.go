package undercover

import (
	"errors"
	"testing"
	"time"

	zero "github.com/wdvxdr1123/ZeroBot"
)

func newVotingGame(t *testing.T) *game {
	t.Helper()
	g := makeStartedGame(t, 6)
	finishDescriptions(t, g)
	return g
}

func TestVotingDeadlineAndLateVotes(t *testing.T) {
	before := time.Now()
	g := newVotingGame(t)
	deadline := g.VoteDeadline
	if deadline.Before(before.Add(3*time.Minute)) || deadline.After(time.Now().Add(3*time.Minute)) {
		t.Fatalf("deadline = %v, want three minutes after voting begins", deadline)
	}
	if !g.acceptsVote(deadline.Add(-time.Nanosecond)) || g.acceptsVote(deadline) || g.acceptsVote(deadline.Add(time.Nanosecond)) {
		t.Fatal("votes must only be accepted strictly before the deadline")
	}
	if _, err := g.vote(1, 2); err != nil {
		t.Fatal(err)
	}
	if _, err := g.vote(1, 3); err != nil {
		t.Fatal(err)
	}
	if !g.VoteDeadline.Equal(deadline) {
		t.Fatal("changing a vote extended the deadline")
	}
	g.VoteDeadline = time.Now().Add(-time.Second)
	updated := g.UpdatedAt
	for _, v := range [][2]int64{{1, 0}, {2, 1}} {
		if _, err := g.vote(v[0], v[1]); !errors.Is(err, errVoteIgnored) {
			t.Fatalf("late vote = %v, want ignored", err)
		}
	}
	if len(g.Votes) != 1 || g.Votes[1] != 3 || !g.UpdatedAt.Equal(updated) {
		t.Fatalf("late votes changed game: votes=%v updated=%v", g.Votes, g.UpdatedAt)
	}
}

func TestVotingTimeoutSettlesOnlyReceivedVotes(t *testing.T) {
	g := newVotingGame(t)
	if _, err := g.vote(1, 6); err != nil {
		t.Fatal(err)
	}
	deadline := g.VoteDeadline
	if _, err := g.expireVoting(deadline, deadline.Add(-time.Nanosecond)); !errors.Is(err, errVoteIgnored) {
		t.Fatalf("early timeout = %v", err)
	}
	r, err := g.expireVoting(deadline, deadline)
	if err != nil || !r.Complete || r.Eliminated.ID != 6 || g.Players[6].Alive {
		t.Fatalf("timeout settlement = %+v, %v", r, err)
	}
	if _, err := g.expireVoting(deadline, deadline.Add(time.Second)); !errors.Is(err, errVoteIgnored) {
		t.Fatalf("duplicate timeout = %v", err)
	}
	if _, err := g.vote(2, 1); !errors.Is(err, errVoteIgnored) {
		t.Fatalf("vote after settlement = %v", err)
	}
}

func TestVotingTimeoutWithNoCandidateVotes(t *testing.T) {
	for _, abstain := range []bool{false, true} {
		g := newVotingGame(t)
		if abstain {
			if _, err := g.vote(1, 0); err != nil {
				t.Fatal(err)
			}
		}
		r, err := g.expireVoting(g.VoteDeadline, g.VoteDeadline)
		if err != nil || !r.Complete || !r.NoElimination || g.Phase == phaseVoting {
			t.Fatalf("abstain=%v timeout = %+v, phase=%v, err=%v", abstain, r, g.Phase, err)
		}
	}
}

func TestVotingTimeoutTieStartsFreshDeadline(t *testing.T) {
	g := newVotingGame(t)
	for _, v := range [][2]int64{{1, 3}, {2, 4}} {
		if _, err := g.vote(v[0], v[1]); err != nil {
			t.Fatal(err)
		}
	}
	// Expire this ballot without waiting for three minutes in the test.
	g.VoteDeadline = time.Now().Add(-time.Second)
	deadline := g.VoteDeadline
	before := time.Now()
	r, err := g.expireVoting(deadline, time.Now())
	if err != nil || len(r.Tie) != 2 || g.Phase != phaseVoting || len(g.Votes) != 0 {
		t.Fatalf("tie = %+v, phase=%v, err=%v", r, g.Phase, err)
	}
	fresh := g.VoteDeadline
	if fresh.Before(before.Add(3*time.Minute)) || fresh.After(time.Now().Add(3*time.Minute)) {
		t.Fatalf("revote deadline = %v", fresh)
	}
	if _, err := g.expireVoting(deadline, fresh); !errors.Is(err, errVoteIgnored) {
		t.Fatalf("stale timeout accepted: %v", err)
	}
	if !g.VoteDeadline.Equal(fresh) {
		t.Fatal("stale timeout changed revote deadline")
	}
	if _, err := g.vote(1, 5); err == nil {
		t.Fatal("revote accepted non-candidate")
	}
	if _, err := g.vote(1, 3); err != nil {
		t.Fatalf("revote rejected valid vote: %v", err)
	}
	r, err = g.expireVoting(fresh, fresh)
	if err != nil || !r.Complete || r.Eliminated.ID != 3 {
		t.Fatalf("revote timeout = %+v, %v", r, err)
	}
}

func TestCompletedVotingIgnoresOldTimer(t *testing.T) {
	g := newVotingGame(t)
	deadline := g.VoteDeadline
	var r voteResult
	for id := int64(1); id <= 6; id++ {
		var err error
		r, err = g.vote(id, 0)
		if err != nil {
			t.Fatal(err)
		}
	}
	if !r.Complete || !r.NoElimination {
		t.Fatalf("early completion = %+v", r)
	}
	if _, err := g.expireVoting(deadline, deadline); !errors.Is(err, errVoteIgnored) {
		t.Fatalf("old timeout = %v", err)
	}
}

func TestExpiredVoteHandlerAndOldRoomTimerAreSilent(t *testing.T) {
	const groupID = int64(-3001)
	g := newVotingGame(t)
	g.VoteDeadline = time.Now().Add(-time.Second)
	rooms.mu.Lock()
	rooms.rooms[groupID] = g
	rooms.mu.Unlock()
	t.Cleanup(func() { rooms.removeIfSame(groupID, g) })
	// No API caller is installed: an attempted reply or archive send fails the test.
	ctx := &zero.Ctx{Event: &zero.Event{GroupID: groupID, UserID: 1}}
	handleVote(ctx)
	if len(g.Votes) != 0 || g.Phase != phaseVoting {
		t.Fatal("late handler changed the ballot")
	}
	oldRoom := newVotingGame(t)
	oldRoom.VoteDeadline = g.VoteDeadline
	processVote(ctx, 0, oldRoom, oldRoom.VoteDeadline)
	if g.Phase != phaseVoting || len(g.Votes) != 0 {
		t.Fatal("old room timer settled the replacement room")
	}
	rooms.removeIfSame(groupID, g)
	processVote(ctx, 0, g, g.VoteDeadline)
	handleVote(ctx)
}
