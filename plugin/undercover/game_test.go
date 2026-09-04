package undercover

import (
	"errors"
	"fmt"
	"regexp"
	"testing"
)

func makeStartedGame(t *testing.T, playerCount int) *game {
	t.Helper()
	g := newGame(1, "玩家1")
	for i := 2; i <= playerCount; i++ {
		if err := g.join(int64(i), fmt.Sprintf("玩家%d", i)); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := g.begin(1, wordPair{Civilian: "牛奶", Undercover: "豆浆"}); err != nil {
		t.Fatal(err)
	}
	if err := g.completeDeal(); err != nil {
		t.Fatal(err)
	}
	return g
}

func finishDescriptions(t *testing.T, g *game) {
	t.Helper()
	order := append([]int64(nil), g.Order...)
	for i, id := range order {
		_, voting, err := g.describe(id, fmt.Sprintf("这是第%d条安全描述", i+1))
		if err != nil {
			t.Fatalf("player %d describes: %v", id, err)
		}
		if voting != (i == len(order)-1) {
			t.Fatalf("unexpected voting state after clue %d", i+1)
		}
	}
}

func TestRoleSetupByPlayerCount(t *testing.T) {
	tests := []struct {
		players, wolves, blanks, angels int
	}{
		{3, 1, 0, 0},
		{5, 1, 1, 0},
		{7, 1, 1, 0},
		{8, 2, 1, 1},
		{12, 2, 1, 1},
	}
	for _, tt := range tests {
		g := makeStartedGame(t, tt.players)
		counts := map[playerRole]int{}
		for _, p := range g.Players {
			counts[p.Role]++
		}
		if counts[roleWolf] != tt.wolves || counts[roleBlank] != tt.blanks || counts[roleAngel] != tt.angels {
			t.Errorf("%d players: roles=%v", tt.players, counts)
		}
		if counts[roleCivilian]+counts[roleWolf]+counts[roleBlank]+counts[roleAngel] != tt.players {
			t.Errorf("%d players: role total mismatch", tt.players)
		}
	}
}

func TestSecretsForSpecialAndUnknownRoles(t *testing.T) {
	g := newGame(1, "玩家1")
	for i := 2; i <= 8; i++ {
		if err := g.join(int64(i), fmt.Sprintf("玩家%d", i)); err != nil {
			t.Fatal(err)
		}
	}
	secrets, err := g.begin(1, wordPair{Civilian: "牛奶", Undercover: "豆浆"})
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range secrets {
		switch item.Role {
		case roleCivilian, roleWolf:
			if len(item.Words) != 1 {
				t.Errorf("%s got %d words", item.Role, len(item.Words))
			}
		case roleBlank:
			if len(item.Words) != 0 {
				t.Errorf("blank got words: %v", item.Words)
			}
		case roleAngel:
			if len(item.Words) != 2 {
				t.Errorf("angel got %d words", len(item.Words))
			}
		}
	}
}

func TestDescriptionTurnAndSecretProtection(t *testing.T) {
	g := makeStartedGame(t, 8)
	first := g.Order[0]
	second := g.Order[1]
	if _, _, err := g.describe(second, "还没轮到我"); !errors.Is(err, errNotYourTurn) {
		t.Fatalf("got %v, want errNotYourTurn", err)
	}
	if words := g.Players[first].Words; len(words) > 0 {
		if _, _, err := g.describe(first, "这句话包含"+words[0]); err == nil {
			t.Fatal("description containing a visible word was accepted")
		}
	}
	if next, voting, err := g.describe(first, "一种日常可见的东西"); err != nil || voting || next != second {
		t.Fatalf("next=%d voting=%v err=%v", next, voting, err)
	}
}

func TestCivilianWinsWhenLastWolfIsVotedOut(t *testing.T) {
	g := makeStartedGame(t, 3)
	finishDescriptions(t, g)
	wolf := g.WolfIDs[0]
	var last voteResult
	for _, voter := range append([]int64(nil), g.Order...) {
		target := wolf
		if voter == wolf {
			for _, id := range g.Order {
				if id != wolf {
					target = id
					break
				}
			}
		}
		var err error
		last, err = g.vote(voter, target)
		if err != nil {
			t.Fatal(err)
		}
	}
	if last.Winner != "平民" || last.Eliminated.ID != wolf || last.Eliminated.Role != roleWolf {
		t.Fatalf("unexpected result: %+v", last)
	}
}

func TestWolfWinsWhenParityReachedAfterVote(t *testing.T) {
	g := makeStartedGame(t, 3)
	finishDescriptions(t, g)
	var civilian int64
	for _, id := range g.Order {
		if g.Players[id].Role == roleCivilian {
			civilian = id
			break
		}
	}
	var last voteResult
	for _, voter := range append([]int64(nil), g.Order...) {
		target := civilian
		if voter == civilian {
			target = g.WolfIDs[0]
		}
		var err error
		last, err = g.vote(voter, target)
		if err != nil {
			t.Fatal(err)
		}
	}
	if last.Winner != "狼人" || last.Eliminated.ID != civilian {
		t.Fatalf("unexpected result: %+v", last)
	}
}

func TestTieRequiresRestrictedRevote(t *testing.T) {
	g := makeStartedGame(t, 4)
	finishDescriptions(t, g)
	a, b, c, d := g.Order[0], g.Order[1], g.Order[2], g.Order[3]
	votes := [][2]int64{{a, c}, {b, d}, {c, d}, {d, c}}
	var last voteResult
	for _, vote := range votes {
		var err error
		last, err = g.vote(vote[0], vote[1])
		if err != nil {
			t.Fatal(err)
		}
	}
	if len(last.Tie) != 2 || len(g.VoteTargets) != 2 || len(g.Votes) != 0 {
		t.Fatalf("tie was not prepared correctly: result=%+v targets=%v", last, g.VoteTargets)
	}
	if _, err := g.vote(b, a); err == nil {
		t.Fatal("vote outside tied candidates was accepted")
	}
}

func TestVoteResultTracksVotedAndPendingPlayers(t *testing.T) {
	g := makeStartedGame(t, 4)
	finishDescriptions(t, g)
	voter := g.Order[0]
	target := g.Order[1]
	result, err := g.vote(voter, target)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Voted) != 1 || result.Voted[0] != voter {
		t.Fatalf("voted = %v, want [%d]", result.Voted, voter)
	}
	if len(result.Pending) != len(g.Order)-1 {
		t.Fatalf("pending = %v, want %d players", result.Pending, len(g.Order)-1)
	}
	for _, id := range result.Pending {
		if id == voter {
			t.Fatalf("voter %d was also listed as pending", voter)
		}
	}
}

func TestNextRoundArchivesAndClearsPreviousClues(t *testing.T) {
	g := makeStartedGame(t, 5)
	finishDescriptions(t, g)
	want := append([]clueRecord(nil), g.RoundClues...)
	if len(want) != len(g.Order) {
		t.Fatalf("recorded %d clues, want %d", len(want), len(g.Order))
	}

	g.Phase = phaseNight
	g.NightActions = make(map[int64]int64)
	result, err := g.resolveNight(true)
	if err != nil {
		t.Fatal(err)
	}
	if result.Winner != "" {
		t.Skip("random role assignment reached a terminal state")
	}
	if result.ClueRound != 1 || len(result.Clues) != len(want) {
		t.Fatalf("archive round=%d clues=%v, want round=1 clues=%v", result.ClueRound, result.Clues, want)
	}
	for i := range want {
		if result.Clues[i] != want[i] {
			t.Fatalf("archive clue %d = %+v, want %+v", i, result.Clues[i], want[i])
		}
	}
	if len(g.RoundClues) != 0 {
		t.Fatalf("current round still contains previous clues: %v", g.RoundClues)
	}
}

func TestNextRoundStartingMidOrderWaitsForEveryDescription(t *testing.T) {
	g := makeStartedGame(t, 6)
	finishDescriptions(t, g)

	oldOrder := append([]int64(nil), g.Order...)
	wolf := g.WolfIDs[0]
	var target int64
	for i := 2; i < len(oldOrder)-1; i++ {
		if oldOrder[i] != wolf {
			target = oldOrder[i]
			break
		}
	}
	if target == 0 {
		t.Fatal("could not find a non-wolf night target in the middle of the order")
	}

	g.Phase = phaseNight
	g.NightActions = map[int64]int64{wolf: target}
	result, err := g.resolveNight(true)
	if err != nil {
		t.Fatal(err)
	}
	if result.Winner != "" {
		t.Fatalf("night unexpectedly ended the game: %+v", result)
	}

	start := -1
	for i, id := range g.Order {
		if id == result.NextDescriber {
			start = i
			break
		}
	}
	if start <= 0 {
		t.Fatalf("next describer index = %d, want a position after the start of order", start)
	}
	expected := append(append([]int64(nil), g.Order[start:]...), g.Order[:start]...)
	for i, id := range expected {
		next, voting, describeErr := g.describe(id, fmt.Sprintf("第二轮安全描述%d", i+1))
		if describeErr != nil {
			t.Fatalf("player %d describes: %v", id, describeErr)
		}
		if voting != (i == len(expected)-1) {
			t.Fatalf("voting=%v after %d/%d descriptions", voting, i+1, len(expected))
		}
		if !voting && next != expected[i+1] {
			t.Fatalf("next=%d, want %d", next, expected[i+1])
		}
	}
	if len(g.RoundClues) != len(g.Order) {
		t.Fatalf("recorded %d second-round clues, want %d", len(g.RoundClues), len(g.Order))
	}
}

func TestNightActorsExcludeBlankAndAngel(t *testing.T) {
	g := makeStartedGame(t, 8)
	g.Phase = phaseNight
	g.NightActions = make(map[int64]int64)
	actors := g.nightActors()
	if len(actors) != 6 {
		t.Fatalf("got %d actors, want 6", len(actors))
	}
	for _, id := range actors {
		if g.Players[id].Role == roleBlank || g.Players[id].Role == roleAngel {
			t.Fatalf("%s received night action", g.Players[id].Role)
		}
	}
	if _, err := g.nightAction(g.BlankID, 0); !errors.Is(err, errNoNightAction) {
		t.Fatalf("blank action returned %v", err)
	}
	if _, err := g.nightAction(g.AngelID, 0); !errors.Is(err, errNoNightAction) {
		t.Fatalf("angel action returned %v", err)
	}
}

func TestCivilianAttackCausesSuicide(t *testing.T) {
	g := makeStartedGame(t, 5)
	g.Phase = phaseNight
	g.NightActions = make(map[int64]int64)
	if _, err := g.blankGuess(g.BlankID, "", "", true); err != nil {
		t.Fatal(err)
	}
	actors := g.nightActors()
	var attackingCivilian, target int64
	for _, id := range actors {
		if g.Players[id].Role == roleCivilian && attackingCivilian == 0 {
			attackingCivilian = id
		} else if target == 0 {
			target = id
		}
	}
	var last nightResult
	for _, actor := range actors {
		actionTarget := int64(0)
		if actor == attackingCivilian {
			actionTarget = target
		}
		var err error
		last, err = g.nightAction(actor, actionTarget)
		if err != nil {
			t.Fatal(err)
		}
	}
	if len(last.Killed) != 1 || last.Killed[0].ID != attackingCivilian {
		t.Fatalf("night killed %+v, want civilian %d", last.Killed, attackingCivilian)
	}
	if !g.Players[target].Alive {
		t.Fatal("civilian attack killed its target")
	}
}

func TestEachWolfAttackSucceedsSimultaneously(t *testing.T) {
	g := makeStartedGame(t, 8)
	g.Phase = phaseNight
	g.NightActions = make(map[int64]int64)
	if _, err := g.blankGuess(g.BlankID, "", "", true); err != nil {
		t.Fatal(err)
	}
	actors := g.nightActors()
	targets := make([]int64, 0, 2)
	for _, id := range g.Order {
		if g.Players[id].Role != roleWolf && g.Players[id].Role != roleCivilian {
			targets = append(targets, id)
		}
	}
	var last nightResult
	wolfIndex := 0
	for _, actor := range actors {
		target := int64(0)
		if g.Players[actor].Role == roleWolf {
			target = targets[wolfIndex]
			wolfIndex++
		}
		var err error
		last, err = g.nightAction(actor, target)
		if err != nil {
			t.Fatal(err)
		}
	}
	killed := map[int64]bool{}
	for _, player := range last.Killed {
		killed[player.ID] = true
	}
	if !killed[targets[0]] || !killed[targets[1]] {
		t.Fatalf("night killed %+v, want both wolf targets %v", last.Killed, targets)
	}
}

func TestBlankWinsByGuessingBothWordsInEitherOrder(t *testing.T) {
	g := makeStartedGame(t, 5)
	g.Phase = phaseNight
	g.NightActions = make(map[int64]int64)
	result, err := g.blankGuess(g.BlankID, g.UndercoverWord, g.CivilianWord, false)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Complete || result.Winner != "白板" || g.Phase != phaseFinished {
		t.Fatalf("unexpected result: %+v phase=%v", result, g.Phase)
	}
}

func TestBlankGetsOnlyOneGuessPerNight(t *testing.T) {
	g := makeStartedGame(t, 5)
	g.Phase = phaseNight
	g.NightActions = make(map[int64]int64)
	result, err := g.blankGuess(g.BlankID, "错误词一", "错误词二", false)
	if err != nil {
		t.Fatal(err)
	}
	if result.Complete || result.Winner != "" || !g.BlankActed {
		t.Fatalf("unexpected wrong-guess result: %+v", result)
	}
	if _, err := g.blankGuess(g.BlankID, g.CivilianWord, g.UndercoverWord, false); !errors.Is(err, errNightActionUsed) {
		t.Fatalf("second guess returned %v", err)
	}
}

func TestForcedNightTreatsMissingActionsAsNoAttack(t *testing.T) {
	g := makeStartedGame(t, 5)
	g.Phase = phaseNight
	g.NightActions = make(map[int64]int64)
	result, err := g.resolveNight(true)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Complete || len(result.Killed) != 0 || g.Phase != phaseDescribing {
		t.Fatalf("unexpected forced result: %+v phase=%v", result, g.Phase)
	}
}

func TestWolfWinsAfterNightReachesParity(t *testing.T) {
	g := makeStartedGame(t, 4)
	var civilians []int64
	for _, id := range g.Order {
		if g.Players[id].Role == roleCivilian {
			civilians = append(civilians, id)
		}
	}
	g.eliminate(civilians[0]) // 模拟白天已有一名平民出局。
	g.Phase = phaseNight
	g.NightActions = make(map[int64]int64)
	actors := g.nightActors()
	var last nightResult
	for _, actor := range actors {
		target := int64(0)
		if g.Players[actor].Role == roleWolf {
			target = civilians[1]
		}
		var err error
		last, err = g.nightAction(actor, target)
		if err != nil {
			t.Fatal(err)
		}
	}
	if last.Winner != "狼人" || g.Phase != phaseFinished {
		t.Fatalf("unexpected result: %+v phase=%v", last, g.Phase)
	}
}

func TestWordPairsAreUsableAndUnique(t *testing.T) {
	pairs := builtinPairs()
	seen := make(map[string]struct{}, len(pairs))
	for i, pair := range pairs {
		if pair.Civilian == "" || pair.Undercover == "" || pair.Civilian == pair.Undercover {
			t.Fatalf("invalid pair at %d: %+v", i, pair)
		}
		key := pair.Civilian + "\x00" + pair.Undercover
		reverse := pair.Undercover + "\x00" + pair.Civilian
		if _, ok := seen[key]; ok {
			t.Fatalf("duplicate pair: %+v", pair)
		}
		if _, ok := seen[reverse]; ok {
			t.Fatalf("reverse duplicate pair: %+v", pair)
		}
		seen[key] = struct{}{}
	}
}

func TestVotePattern(t *testing.T) {
	re := regexp.MustCompile(votePattern)
	tests := []struct {
		message string
		want    int64
	}{
		{"卧底投票 [CQ:at,qq=123456]", 123456},
		{"卧底投票[CQ:at,name=某人,qq=42]", 42},
		{"卧底投票 98765", 98765},
	}
	for _, tt := range tests {
		matches := re.FindStringSubmatch(tt.message)
		got, err := voteTarget(matches)
		if err != nil || got != tt.want {
			t.Errorf("voteTarget(%q) = %d, %v; want %d", tt.message, got, err, tt.want)
		}
	}
}

func TestNightActionPattern(t *testing.T) {
	re := regexp.MustCompile(nightActionPattern)
	tests := []string{
		"卧底刀人 不刀",
		"卧底刀人 123456",
		"卧底刀人 987654321 123456",
	}
	for _, input := range tests {
		if !re.MatchString(input) {
			t.Errorf("night action pattern rejected %q", input)
		}
	}
	for _, input := range []string{"卧底刀人", "卧底刀人 张三", "卧底刀人 1 2 3", "卧底刀人 987654321 不刀"} {
		if re.MatchString(input) {
			t.Errorf("night action pattern accepted %q", input)
		}
	}
}

func TestBlankGuessPattern(t *testing.T) {
	re := regexp.MustCompile(blankGuessPattern)
	for _, input := range []string{
		"卧底猜词 牛奶|豆浆",
		"卧底猜词 牛奶｜豆浆",
		"卧底猜词 987654321 牛奶|豆浆",
	} {
		if !re.MatchString(input) {
			t.Errorf("blank guess pattern rejected %q", input)
		}
	}
	for _, input := range []string{"卧底猜词", "卧底猜词 放弃", "卧底猜词 牛奶"} {
		if re.MatchString(input) {
			t.Errorf("blank guess pattern accepted %q", input)
		}
	}
}
