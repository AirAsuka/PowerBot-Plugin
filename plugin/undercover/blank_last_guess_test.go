package undercover

import (
	"errors"
	"regexp"
	"slices"
	"testing"
	"time"

	zero "github.com/wdvxdr1123/ZeroBot"
)

func voteOutBlank(t *testing.T, g *game, timeout bool) {
	t.Helper()
	finishDescriptions(t, g)
	var result voteResult
	var err error
	if timeout {
		if _, err := g.vote(g.WolfIDs[0], g.BlankID); err != nil {
			t.Fatal(err)
		}
		result, err = g.expireVoting(g.VoteDeadline, g.VoteDeadline)
	} else {
		for _, id := range slices.Clone(g.Order) {
			result, err = g.vote(id, g.BlankID)
			if err != nil {
				t.Fatal(err)
			}
		}
	}
	if err != nil || !result.Complete || !result.BlankLastGuess || result.Eliminated.ID != g.BlankID || result.Winner != "" || len(result.NightActors) != 0 {
		t.Fatalf("blank vote result = %+v, err=%v", result, err)
	}
	if g.Phase != phaseBlankLastGuess || g.Players[g.BlankID].Alive || slices.Contains(g.Order, g.BlankID) || g.BlankGuessDeadline.IsZero() || !g.VoteDeadline.IsZero() {
		t.Fatal("voting did not pause for the eliminated blank's last guess")
	}
}

func TestVotedOutBlankCanWinBeforeParitySettlement(t *testing.T) {
	for _, timeout := range []bool{false, true} {
		g := makeStartedGame(t, 5)
		// Leave one wolf, one civilian and the blank; voting out the blank reaches parity.
		for _, id := range slices.Clone(g.Order) {
			if len(g.Order) > 3 && g.Players[id].Role == roleCivilian {
				g.eliminate(id)
			}
		}
		g.BlankActed = true // A prior night guess does not consume the last guess.
		voteOutBlank(t, g, timeout)
		if g.winner() != "狼人" {
			t.Fatal("test must reach wolf parity before the guess")
		}
		deadline := g.BlankGuessDeadline
		result, err := g.blankLastGuess(g.BlankID, g.UndercoverWord, g.CivilianWord, false, deadline.Add(-time.Second))
		if err != nil || result.Winner != "白板" || g.Phase != phaseFinished || result.Reveal.CivilianWord != g.CivilianWord {
			t.Fatalf("last guess = %+v, err=%v", result, err)
		}
		if _, err := g.expireBlankLastGuess(deadline, deadline); !errors.Is(err, errBlankLastGuessIgnored) {
			t.Fatalf("old timer after win = %v", err)
		}
	}
}

func TestBlankLastGuessFailureResumesSettlement(t *testing.T) {
	for _, parity := range []bool{false, true} {
		for _, action := range []string{"wrong", "give up", "timeout"} {
			g := makeStartedGame(t, 5)
			if parity {
				for _, id := range slices.Clone(g.Order) {
					if len(g.Order) > 3 && g.Players[id].Role == roleCivilian {
						g.eliminate(id)
					}
				}
			}
			voteOutBlank(t, g, false)
			deadline := g.BlankGuessDeadline
			var result voteResult
			var err error
			if action == "timeout" {
				result, err = g.expireBlankLastGuess(deadline, deadline)
			} else {
				result, err = g.blankLastGuess(g.BlankID, g.CivilianWord, "错误词", action == "give up", time.Now())
			}
			if err != nil || !result.Complete || !g.BlankGuessDeadline.IsZero() {
				t.Fatalf("%s parity=%v: result=%+v err=%v", action, parity, result, err)
			}
			if parity {
				if result.Winner != "狼人" || g.Phase != phaseFinished {
					t.Fatalf("%s: wolf victory was not settled: %+v", action, result)
				}
			} else {
				if result.Winner != "" || g.Phase != phaseNight || len(result.NightActors) != 4 || g.nightActionsNeeded() != 4 {
					t.Fatalf("%s: night did not resume: %+v", action, result)
				}
				if _, err := g.blankGuess(g.BlankID, g.CivilianWord, g.UndercoverWord, false); !errors.Is(err, errPlayerOut) {
					t.Fatalf("eliminated blank private guess = %v", err)
				}
			}
			if _, err := g.blankLastGuess(g.BlankID, g.CivilianWord, g.UndercoverWord, false, time.Now()); !errors.Is(err, errBlankLastGuessIgnored) {
				t.Fatalf("repeat guess accepted: %v", err)
			}
			if _, err := g.expireBlankLastGuess(deadline, deadline); !errors.Is(err, errBlankLastGuessIgnored) {
				t.Fatalf("repeat timeout accepted: %v", err)
			}
		}
	}
}

func TestBlankLastGuessRejectsOtherPlayersAndLateGuesses(t *testing.T) {
	g := makeStartedGame(t, 5)
	voteOutBlank(t, g, false)
	deadline := g.BlankGuessDeadline
	for _, actor := range []int64{g.WolfIDs[0], 999} {
		if _, err := g.blankLastGuess(actor, g.CivilianWord, g.UndercoverWord, false, time.Now()); err == nil {
			t.Fatalf("actor %d was allowed to guess", actor)
		}
	}
	if _, err := g.blankLastGuess(g.BlankID, g.CivilianWord, g.UndercoverWord, false, deadline); !errors.Is(err, errBlankLastGuessIgnored) {
		t.Fatalf("late guess accepted: %v", err)
	}
	for _, times := range [][2]time.Time{{deadline, deadline.Add(-time.Nanosecond)}, {deadline.Add(-time.Second), deadline}} {
		if _, err := g.expireBlankLastGuess(times[0], times[1]); !errors.Is(err, errBlankLastGuessIgnored) {
			t.Fatalf("early or stale timer accepted: %v", err)
		}
	}
	if _, err := g.blankGuess(g.BlankID, g.CivilianWord, g.UndercoverWord, false); !errors.Is(err, errNotNight) {
		t.Fatalf("private guess accepted during public guess phase: %v", err)
	}
	if _, err := g.nightAction(g.WolfIDs[0], 0); !errors.Is(err, errNotNight) {
		t.Fatalf("night action accepted before guess: %v", err)
	}
	if !g.BlankGuessDeadline.Equal(deadline) || g.Phase != phaseBlankLastGuess {
		t.Fatal("invalid actions consumed or extended the last guess")
	}
}

func TestBlankKilledAtNightHasNoLastGuess(t *testing.T) {
	g := makeStartedGame(t, 5)
	g.Phase = phaseNight
	g.NightActions = map[int64]int64{g.WolfIDs[0]: g.BlankID}
	result, err := g.resolveNight(true)
	if err != nil || !result.Complete || g.Players[g.BlankID].Alive || g.Phase != phaseDescribing {
		t.Fatalf("night kill = %+v, err=%v", result, err)
	}
	if _, err := g.blankLastGuess(g.BlankID, g.CivilianWord, g.UndercoverWord, false, time.Now()); !errors.Is(err, errBlankLastGuessIgnored) {
		t.Fatalf("night death granted last guess: %v", err)
	}
}

func TestBlankLastGuessOldRoomTimerIsSilent(t *testing.T) {
	const groupID = int64(-3002)
	g := makeStartedGame(t, 5)
	voteOutBlank(t, g, false)
	rooms.mu.Lock()
	rooms.rooms[groupID] = g
	rooms.mu.Unlock()
	t.Cleanup(func() { rooms.removeIfSame(groupID, g) })
	// With no API caller, any announcement from a stale callback fails the test.
	ctx := &zero.Ctx{Event: &zero.Event{GroupID: groupID}}
	oldRoom := makeStartedGame(t, 5)
	processBlankLastGuess(ctx, "", "", true, oldRoom, g.BlankGuessDeadline)
	if g.Phase != phaseBlankLastGuess {
		t.Fatal("old room timer settled the new room")
	}
	rooms.removeIfSame(groupID, g)
	processBlankLastGuess(ctx, "", "", true, g, g.BlankGuessDeadline)
}

func TestGroupBlankGuessPattern(t *testing.T) {
	re := regexp.MustCompile(groupBlankGuessPattern)
	for _, input := range []string{"卧底猜词 牛奶 豆浆", "卧底猜词 123 456", "卧底猜词 放弃"} {
		if !re.MatchString(input) {
			t.Errorf("group guess rejected %q", input)
		}
	}
	for _, input := range []string{"卧底猜词 牛奶", "卧底猜词 123 牛奶 豆浆", "卧底猜词"} {
		if re.MatchString(input) {
			t.Errorf("group guess accepted %q", input)
		}
	}
}
