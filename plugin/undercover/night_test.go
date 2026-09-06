package undercover

import (
	"slices"
	"strings"
	"testing"
)

func TestCaptureNightOutcomeIncludesAllDeadAndAlivePlayers(t *testing.T) {
	g := makeStartedGame(t, 5)
	firstDead := g.JoinOrder[0]
	secondDead := g.JoinOrder[1]
	g.eliminate(firstDead)
	g.eliminate(secondDead)

	outcome := nightOutcome{}
	captureNightOutcome(g, &outcome)

	if len(outcome.DeadPlayers) != 2 {
		t.Fatalf("dead players = %v, want 2", outcome.DeadPlayers)
	}
	for i, id := range []int64{firstDead, secondDead} {
		got := outcome.DeadPlayers[i]
		want := g.Players[id]
		if got.Name != want.Name || got.Role != want.Role {
			t.Fatalf("dead player %d = %+v, want name=%q role=%s", i, got, want.Name, want.Role)
		}
	}

	wantAlive := make([]string, 0, len(g.Order))
	for _, id := range g.JoinOrder {
		if g.Players[id].Alive {
			wantAlive = append(wantAlive, g.Players[id].Name)
		}
	}
	if !slices.Equal(outcome.AlivePlayers, wantAlive) {
		t.Fatalf("alive players = %v, want %v", outcome.AlivePlayers, wantAlive)
	}
}

func TestFormatRoundPlayersOnlyShowsRolesForDeadPlayers(t *testing.T) {
	text := formatRoundPlayers(
		[]namedElimination{{Name: "玩家1", Role: roleWolf}, {Name: "玩家2", Role: roleBlank}},
		[]string{"玩家3", "玩家4"},
	)
	want := "死亡玩家：玩家1（狼人）、玩家2（白板）\n存活玩家：玩家3、玩家4"
	if text != want {
		t.Fatalf("round players = %q, want %q", text, want)
	}
	if strings.Contains(text, "玩家3（") || strings.Contains(text, "玩家4（") {
		t.Fatalf("alive player role was exposed: %q", text)
	}
}
