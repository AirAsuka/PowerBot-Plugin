package werewolf

import (
	"fmt"
	"testing"
)

func gameWithRoles(t *testing.T, roles ...role) *game {
	t.Helper()
	g := newGame(1, "玩家1")
	for i := 2; i <= len(roles); i++ {
		if err := g.join(int64(i), fmt.Sprintf("玩家%d", i)); err != nil {
			t.Fatal(err)
		}
	}
	for i, r := range roles {
		g.Players[int64(i+1)].Role = r
	}
	g.Round = 1
	g.startNight()
	return g
}

func TestRoleCounts(t *testing.T) {
	wantWolves := map[int]int{6: 2, 7: 2, 8: 2, 9: 3, 10: 3, 11: 3, 12: 4}
	for n := minPlayers; n <= maxPlayers; n++ {
		counts := roleCounts(n)
		total := 0
		for _, count := range counts {
			total += count
		}
		if total != n || counts[roleWolf] != wantWolves[n] || counts[roleSeer] != 1 || counts[roleWitch] != 1 {
			t.Fatalf("%d players: %v", n, counts)
		}
		if (n >= 8) != (counts[roleHunter] == 1) {
			t.Fatalf("%d players: hunter count %d", n, counts[roleHunter])
		}
	}
}

func TestBeginCreatesConfiguredRoles(t *testing.T) {
	for n := minPlayers; n <= maxPlayers; n++ {
		g := newGame(1, "玩家1")
		for i := 2; i <= n; i++ {
			if err := g.join(int64(i), fmt.Sprintf("玩家%d", i)); err != nil {
				t.Fatal(err)
			}
		}
		secrets, err := g.begin(1)
		if err != nil {
			t.Fatal(err)
		}
		if len(secrets) != n || g.Phase != phaseDealing {
			t.Fatalf("n=%d secrets=%d phase=%v", n, len(secrets), g.Phase)
		}
		got := map[role]int{}
		for _, p := range g.Players {
			got[p.Role]++
		}
		for r, count := range roleCounts(n) {
			if got[r] != count {
				t.Fatalf("n=%d role=%v got=%d want=%d", n, r, got[r], count)
			}
		}
	}
}

func TestWolfConsensusThenSpecialActionsResolveNight(t *testing.T) {
	g := gameWithRoles(t, roleWolf, roleWolf, roleSeer, roleWitch, roleVillager, roleVillager)
	if r, err := g.wolfVote(1, 5); err != nil || r.Ready {
		t.Fatalf("first wolf vote: %+v %v", r, err)
	}
	r, err := g.wolfVote(2, 5)
	if err != nil || !r.Ready || r.Victim != 5 || g.Phase != phaseNightSpecial {
		t.Fatalf("second wolf vote: %+v phase=%v err=%v", r, g.Phase, err)
	}
	inspection, err := g.inspect(3, 1)
	if err != nil || !inspection.IsWolf || inspection.Outcome.Complete {
		t.Fatalf("inspection=%+v err=%v", inspection, err)
	}
	outcome, err := g.witchAct(4, "跳过", 0)
	if err != nil || !outcome.Complete || len(outcome.Deaths) != 1 || outcome.Deaths[0].ID != 5 {
		t.Fatalf("outcome=%+v err=%v", outcome, err)
	}
	if g.Phase != phaseDay || g.Players[5].Alive {
		t.Fatalf("phase=%v victim alive=%v", g.Phase, g.Players[5].Alive)
	}
}

func TestWitchPotionsAreSingleUse(t *testing.T) {
	g := gameWithRoles(t, roleWolf, roleWolf, roleSeer, roleWitch, roleVillager, roleVillager, roleVillager)
	_, _ = g.wolfVote(1, 5)
	_, _ = g.wolfVote(2, 5)
	_, _ = g.inspect(3, 1)
	outcome, err := g.witchAct(4, "救", 0)
	if err != nil || len(outcome.Deaths) != 0 || !g.Players[5].Alive || !g.AntidoteUsed {
		t.Fatalf("heal outcome=%+v err=%v", outcome, err)
	}
	g.Round++
	g.startNight()
	_, _ = g.wolfVote(1, 6)
	_, _ = g.wolfVote(2, 6)
	_, _ = g.inspect(3, 2)
	if _, err := g.witchAct(4, "救", 0); err == nil {
		t.Fatal("second antidote was accepted")
	}
	outcome, err = g.witchAct(4, "毒", 1)
	if err != nil || len(outcome.Deaths) != 2 || !g.PoisonUsed {
		t.Fatalf("poison outcome=%+v err=%v", outcome, err)
	}
}

func TestTieRevote(t *testing.T) {
	g := gameWithRoles(t, roleWolf, roleWolf, roleSeer, roleWitch, roleVillager, roleVillager, roleVillager, roleHunter)
	g.startDay(nil)
	for g.Phase == phaseDay {
		if _, _, err := g.speak(g.currentSpeaker(), "测试发言"); err != nil {
			t.Fatal(err)
		}
	}
	votes := [][2]int64{{1, 3}, {2, 4}, {3, 4}, {4, 3}, {5, 3}, {6, 4}, {7, 3}, {8, 4}}
	var result voteResult
	for _, v := range votes {
		var err error
		result, err = g.vote(v[0], v[1])
		if err != nil {
			t.Fatal(err)
		}
	}
	if len(result.Tie) != 2 || len(g.VoteTargets) != 2 || len(g.Votes) != 0 {
		t.Fatalf("tie result=%+v", result)
	}
	if _, err := g.vote(1, 5); err == nil {
		t.Fatal("vote outside tied candidates was accepted")
	}
}

func TestHunterMayShootBeforeParityVictory(t *testing.T) {
	g := gameWithRoles(t, roleWolf, roleHunter, roleVillager, roleVillager)
	g.Phase, g.Votes = phaseVoting, map[int64]int64{}
	votes := [][2]int64{{1, 2}, {2, 1}, {3, 2}, {4, 2}}
	var result voteResult
	for _, v := range votes {
		var err error
		result, err = g.vote(v[0], v[1])
		if err != nil {
			t.Fatal(err)
		}
	}
	if !result.NeedHunter || result.Winner != "" || g.Phase != phaseHunter {
		t.Fatalf("vote result=%+v phase=%v", result, g.Phase)
	}
	hunter, err := g.hunterShoot(2, 1)
	if err != nil || hunter.Winner != "好人" {
		t.Fatalf("hunter result=%+v err=%v", hunter, err)
	}
}

func TestPoisonedHunterCannotShoot(t *testing.T) {
	g := gameWithRoles(t, roleWolf, roleWolf, roleSeer, roleWitch, roleHunter, roleVillager, roleVillager)
	_, _ = g.wolfVote(1, 6)
	_, _ = g.wolfVote(2, 6)
	_, _ = g.inspect(3, 1)
	outcome, err := g.witchAct(4, "毒", 5)
	if err != nil {
		t.Fatal(err)
	}
	if outcome.NeedHunter || g.Phase == phaseHunter {
		t.Fatalf("poisoned hunter was allowed to shoot: %+v", outcome)
	}
}
