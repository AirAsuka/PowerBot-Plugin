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

func TestBeginAssignsOneUndercover(t *testing.T) {
	g := makeStartedGame(t, 5)
	undercoverCount := 0
	for _, p := range g.Players {
		if p.IsUndercover {
			undercoverCount++
			if p.Word != g.UndercoverWord {
				t.Fatalf("undercover got %q, want %q", p.Word, g.UndercoverWord)
			}
		} else if p.Word != g.CivilianWord {
			t.Fatalf("civilian got %q, want %q", p.Word, g.CivilianWord)
		}
	}
	if undercoverCount != 1 {
		t.Fatalf("got %d undercovers, want 1", undercoverCount)
	}
	if g.Phase != phaseDescribing || g.Round != 1 {
		t.Fatalf("unexpected state: phase=%v round=%d", g.Phase, g.Round)
	}
}

func TestDescriptionTurnAndSecretProtection(t *testing.T) {
	g := makeStartedGame(t, 3)
	first := g.Order[0]
	second := g.Order[1]

	if _, _, err := g.describe(second, "还没轮到我"); !errors.Is(err, errNotYourTurn) {
		t.Fatalf("got %v, want errNotYourTurn", err)
	}
	if _, _, err := g.describe(first, "这句话包含"+g.Players[first].Word); err == nil {
		t.Fatal("description containing the secret word was accepted")
	}
	if next, voting, err := g.describe(first, "一种日常可见的东西"); err != nil || voting || next != second {
		t.Fatalf("next=%d voting=%v err=%v", next, voting, err)
	}
}

func TestCivilianWinsWhenUndercoverIsEliminated(t *testing.T) {
	g := makeStartedGame(t, 3)
	finishDescriptions(t, g)
	undercover := g.UndercoverID
	var last voteResult
	for _, voter := range g.Order {
		target := undercover
		if voter == undercover {
			for _, id := range g.Order {
				if id != undercover {
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
	if last.Winner != "平民" || last.Eliminated != undercover || !last.WasUndercover {
		t.Fatalf("unexpected result: %+v", last)
	}
	if _, err := g.vote(g.Order[0], g.Order[1]); !errors.Is(err, errNotVoting) {
		t.Fatalf("finished game accepted another vote: %v", err)
	}
}

func TestUndercoverWinsAtTwoPlayers(t *testing.T) {
	g := makeStartedGame(t, 3)
	finishDescriptions(t, g)
	var civilian int64
	for _, id := range g.Order {
		if id != g.UndercoverID {
			civilian = id
			break
		}
	}
	var last voteResult
	for _, voter := range g.Order {
		target := civilian
		if voter == civilian {
			target = g.UndercoverID
		}
		var err error
		last, err = g.vote(voter, target)
		if err != nil {
			t.Fatal(err)
		}
	}
	if last.Winner != "卧底" || last.Eliminated != civilian || last.WasUndercover {
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
	if _, err := voteTarget(re.FindStringSubmatch("卧底投票 张三")); err == nil {
		t.Fatal("invalid vote target was accepted")
	}
}
