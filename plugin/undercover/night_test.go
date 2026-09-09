package undercover

import (
	"slices"
	"strconv"
	"strings"
	"testing"
)

func TestClueArchiveNodesKeepTimeoutInPlayerOrder(t *testing.T) {
	clues := []clueRecord{
		{PlayerID: 1, PlayerName: "玩家1", Text: "第一条描述"},
		{PlayerID: 2, PlayerName: "玩家2", TimedOut: true},
		{PlayerID: 3, PlayerName: "玩家3", Text: "第三条描述"},
	}
	for _, fallback := range []bool{false, true} {
		nodes := clueArchiveNodes(1, clues, 999, fallback)
		if len(nodes) != 4 {
			t.Fatalf("fallback=%v: got %d nodes, want 4", fallback, len(nodes))
		}
		timeout := nodes[2]
		if timeout.Type != "node" || timeout.Data["uin"] != "999" || timeout.Data["name"] != "谁是卧底" ||
			timeout.Data["content"] != "系统提示：玩家2（2）超时未进行发言描述，已自动跳过。" {
			t.Fatalf("fallback=%v: incorrect timeout node: %+v", fallback, timeout)
		}
		for _, i := range []int{0, 2} {
			node := nodes[i+1]
			wantSender := strconv.FormatInt(clues[i].PlayerID, 10)
			if fallback {
				wantSender = "999"
			}
			if node.Data["uin"] != wantSender || !strings.Contains(node.Data["content"], clues[i].Text) {
				t.Fatalf("fallback=%v: description %d has wrong sender or content: %+v", fallback, i, node)
			}
		}
	}
}

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

func TestCaptureNightOutcomeIncludesAllClueArchives(t *testing.T) {
	g := makeStartedGame(t, 5)
	g.ClueHistory = []clueArchive{
		{Round: 1, Clues: []clueRecord{{PlayerID: 1, PlayerName: "玩家1", Text: "第一轮"}}},
		{Round: 2, Clues: []clueRecord{{PlayerID: 2, PlayerName: "玩家2", Text: "第二轮"}}},
	}
	outcome := nightOutcome{}
	captureNightOutcome(g, &outcome)

	if len(outcome.Archives) != 2 || outcome.Archives[0].Round != 1 || outcome.Archives[1].Round != 2 {
		t.Fatalf("archives = %+v, want rounds 1 and 2", outcome.Archives)
	}
}
