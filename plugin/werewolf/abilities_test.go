package werewolf

import (
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"
)

func lobby(t *testing.T, n int) *game {
	t.Helper()
	g := newGame(1, "玩家1")
	for i := 2; i <= n; i++ {
		if err := g.join(int64(i), fmt.Sprintf("玩家%d", i)); err != nil {
			t.Fatal(err)
		}
	}
	return g
}

func TestSupportedSeatsAndBoardSelection(t *testing.T) {
	for n := 1; n <= 12; n++ {
		g := lobby(t, n)
		if err := g.canBegin(1); (err == nil) != supportedPlayerCount(n) {
			t.Fatalf("n=%d err=%v", n, err)
		}
	}
	g := lobby(t, 12)
	if err := g.join(13, "多余"); err != errRoomFull {
		t.Fatal(err)
	}
	g.Players[13] = &player{ID: 13, Alive: true}
	if err := g.canBegin(1); err != errRoomFull {
		t.Fatal("begin must also enforce maximum")
	}
	g = lobby(t, 10)
	if err := g.selectBoard(2, "10人速推场"); err != errNotHost {
		t.Fatal(err)
	}
	if err := g.selectBoard(1, "11人标准场"); err == nil {
		t.Fatal("accepted 11-player board")
	}
	if err := g.selectBoard(1, "12人预女猎白"); err != nil {
		t.Fatal(err)
	}
	if _, err := g.begin(1); err == nil {
		t.Fatal("accepted mismatched seats")
	}
	if err := g.selectBoard(1, "10人假面之夜"); err != nil {
		t.Fatal(err)
	}
	if _, err := g.begin(1); err != nil {
		t.Fatal(err)
	}
	if err := g.selectBoard(1, "自动"); err != errGameStarted {
		t.Fatal(err)
	}
}

func TestEveryBoardDealsAndAdvancesTwoRounds(t *testing.T) {
	seen := map[string]bool{}
	for _, b := range boards {
		t.Run(b.Name, func(t *testing.T) {
			if seen[b.Name] {
				t.Fatal("duplicate board")
			}
			seen[b.Name] = true
			for trial := 0; trial < 8; trial++ {
				g := lobby(t, b.Players)
				if err := g.selectBoard(1, b.Name); err != nil {
					t.Fatal(err)
				}
				secrets, err := g.begin(1)
				if err != nil {
					t.Fatal(err)
				}
				if len(secrets) != b.Players {
					t.Fatal("wrong deal count")
				}
				gods, wolves := 0, 0
				for _, p := range g.Players {
					if p.Role.isWolf() {
						wolves++
					} else if p.Role != roleVillager {
						gods++
					}
				}
				if wolves == 0 || gods == 0 {
					t.Fatal("empty camp")
				}
				if b.RandomGods && gods != 3 {
					t.Fatal("mask must draw exactly three gods")
				}
				for _, s := range secrets {
					for _, id := range s.Teammates {
						if !g.Players[id].Role.isWolf() || id == s.UserID {
							t.Fatal("invalid teammate disclosure")
						}
					}
					if s.Role == roleGargoyle && len(s.Teammates) > 0 {
						t.Fatal("gargoyle received teammates")
					}
				}
				g.completeDeal()
				for step := 0; g.Round < 3 && g.Phase != phaseFinished; step++ {
					if step > 150 {
						t.Fatalf("stuck in %s", g.Phase)
					}
					switch g.Phase {
					case phaseNightPrepare:
						g.forcePreparation()
					case phaseNightWolf:
						if _, err := g.wolfVote(g.currentWolf(), 0); err != nil {
							t.Fatal(err)
						}
					case phaseNightSpecial:
						g.forceSpecial()
					case phaseNightLastWords:
						g.forceLastWords()
					case phaseDayLastWords:
						g.forceDayLastWords()
					case phaseHunter:
						if _, err := g.hunterShoot(g.PendingHunter, 0); err != nil {
							t.Fatal(err)
						}
					case phaseDay:
						if _, _, err := g.speak(g.currentSpeaker(), "过"); err != nil {
							t.Fatal(err)
						}
					case phaseVoting:
						voters := g.voterIDs()
						for _, id := range voters {
							if _, err := g.vote(id, 0); err != nil {
								t.Fatal(err)
							}
						}
					default:
						t.Fatalf("unexpected phase %s", g.Phase)
					}
				}
			}
		})
	}
}

func TestFirstNightPeaceAndGuard(t *testing.T) {
	g := gameWithRoles(t, roleWolf, roleWolf, roleWolf, roleVillager, roleVillager, roleSeer, roleGuard, roleKnight)
	g.BoardName = "8人末日捍卫"
	if _, err := g.wolfVote(1, 4); err == nil {
		t.Fatal("knife before guard")
	}
	if _, err := g.useSkill(7, "目标", 4); err != nil {
		t.Fatal(err)
	}
	if g.Phase != phaseNightSpecial || g.WolfVictim != 0 {
		t.Fatal("first night should skip knife")
	}
	r, err := g.inspect(6, 1)
	if err != nil || !r.Outcome.Complete || len(r.Outcome.Deaths) != 0 || g.Phase != phaseDay {
		t.Fatalf("%+v %v", r, err)
	}
	g.Round = 2
	g.startNight()
	if _, err := g.useSkill(7, "目标", 4); err == nil {
		t.Fatal("consecutive guard accepted")
	}
	if _, err := g.useSkill(7, "目标", 7); err != nil {
		t.Fatal(err)
	}
	if g.Phase != phaseNightWolf {
		t.Fatal("second night must have a knife")
	}
}

func TestGuardWitchAndPoisonInteractions(t *testing.T) {
	for _, tt := range []struct {
		name                string
		guard, heal, poison bool
		dead                bool
	}{
		{"knife", false, false, false, true}, {"guard", true, false, false, false}, {"heal", false, true, false, false},
		{"both", true, true, false, true}, {"poison", true, false, true, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			g := gameWithRoles(t, roleWolf, roleSeer, roleWitch, roleGuard, roleVillager, roleVillager)
			g.WolfVictim = 5
			if tt.guard {
				g.GuardTargets[4] = []int64{5}
			}
			if tt.heal {
				g.WitchHeal = 5
			}
			if tt.poison {
				g.WitchPoison = 5
			}
			r := g.resolveNight()
			if g.Players[5].Alive == tt.dead {
				t.Fatalf("result=%+v", r)
			}
		})
	}
}

func TestBoardWitchSelfSaveAndKnifePrivacy(t *testing.T) {
	for _, tt := range []struct {
		board string
		allow bool
	}{{"12人标准场", false}, {"10人速推场", true}, {"12人纯白夜影", true}} {
		g := gameWithRoles(t, roleWolf, roleSeer, roleWitch, roleHunter, roleVillager, roleVillager)
		g.BoardName = tt.board
		g.Phase = phaseNightSpecial
		g.WolfVictim = 3
		_, err := g.witchAct(3, "救", 0)
		if (err == nil) != tt.allow {
			t.Fatalf("%s: %v", tt.board, err)
		}
	}
	g := gameWithRoles(t, roleWolf, roleSeer, roleWitch, roleHunter, roleVillager, roleVillager)
	g.WolfVictim = 5
	g.AntidoteUsed = true
	text := witchPrompt(g, 100)
	if strings.Contains(text, "狼人目标是 玩家5") || !strings.Contains(text, "无法得知") {
		t.Fatal(text)
	}
}

func TestIdiotLosesVoteButRemainsAlive(t *testing.T) {
	g := gameWithRoles(t, roleWolf, roleWolf, roleSeer, roleWitch, roleIdiot, roleVillager, roleVillager, roleVillager)
	g.startDay(nil)
	g.beginVoting()
	var r voteResult
	for _, id := range g.aliveIDs() {
		target := int64(5)
		if id == 5 {
			target = 1
		}
		r, _ = g.vote(id, target)
	}
	if r.Revealed != 5 || !g.Players[5].Alive || !g.Players[5].Revealed {
		t.Fatalf("%+v", r)
	}
	g.startDay(nil)
	g.beginVoting()
	if _, err := g.vote(5, 1); err == nil {
		t.Fatal("revealed idiot voted")
	}
	if _, err := g.vote(1, 5); err == nil {
		t.Fatal("voted for revealed idiot")
	}
	if len(g.voterIDs()) != 7 {
		t.Fatal("wrong required vote count")
	}
	for _, id := range g.voterIDs() {
		r, _ = g.vote(id, 0)
	}
	if !r.Complete || g.Phase != phaseNightWolf {
		t.Fatal("ballot waited for idiot")
	}
}

func TestWinnerUsesCampAndBoardNotParity(t *testing.T) {
	g := gameWithRoles(t, roleWolfKing, roleWhiteWolfKing, roleEvilKnight, roleSeer, roleWitch, roleHunter, roleGuard, roleIdiot)
	g.BoardName = "8人诸神黄昏"
	if g.winner() != "" {
		t.Fatal("no villagers does not end slaughter board")
	}
	g.Players[6].Alive = false
	g.Players[7].Alive = false
	if g.winner() != "" {
		t.Fatal("parity must not end game")
	}
	for _, id := range []int64{4, 5, 8} {
		g.Players[id].Alive = false
	}
	if g.winner() != "狼人" {
		t.Fatal("all good players eliminated")
	}
	g = gameWithRoles(t, roleWolfKing, roleSeer, roleVillager)
	if g.winner() != "" {
		t.Fatal("special wolf was not counted")
	}
	g.Players[1].Alive = false
	if g.winner() != "好人" {
		t.Fatal("last special wolf died")
	}
}

func TestGunChainAndPoisonSuppression(t *testing.T) {
	g := gameWithRoles(t, roleWolf, roleWolfKing, roleHunter, roleHunter, roleSeer, roleVillager, roleVillager, roleVillager)
	g.Players[2].Alive = false
	g.HunterDeaths = []death{{2, roleWolfKing, "放逐"}}
	if !g.nextShooter() {
		t.Fatal("wolf king needs gun")
	}
	r, err := g.hunterShoot(2, 3)
	if err != nil || !r.NeedHunter || g.PendingHunter != 3 {
		t.Fatalf("%+v %v", r, err)
	}
	r, err = g.hunterShoot(3, 4)
	if err != nil || !r.NeedHunter || g.PendingHunter != 4 {
		t.Fatalf("%+v %v", r, err)
	}
	r, err = g.hunterShoot(4, 0)
	if err != nil || !r.AwaitingLastWords || g.PendingHunter != 0 {
		t.Fatalf("%+v %v", r, err)
	}
	if g.canShoot(death{2, roleWolfKing, "放逐"}) {
		t.Fatal("gun can only fire once")
	}
	for _, cause := range []string{"女巫毒杀", "梦游", "殉情", "狼人自爆"} {
		g.Players[2].ShotUsed = false
		if g.canShoot(death{2, roleWolfKing, cause}) {
			t.Fatalf("gun fired after %s", cause)
		}
	}
}

func TestKnightDuelAndResumeSpeech(t *testing.T) {
	g := gameWithRoles(t, roleWolf, roleWolfBeauty, roleKnight, roleSeer, roleWitch, roleVillager, roleVillager, roleVillager)
	g.startDay(nil)
	g.CharmTarget = 4
	r, err := g.daySkill(3, 2, roleKnight)
	if err != nil || g.Players[2].Alive || !g.Players[4].Alive || !r.Outcome.AwaitingLastWords {
		t.Fatalf("%+v %v", r, err)
	}
	g.forceDayLastWords()
	if g.Round != 2 || g.Phase != phaseNightWolf {
		t.Fatal("duel hit should end day")
	}
	g = gameWithRoles(t, roleWolf, roleWolf, roleKnight, roleSeer, roleWitch, roleVillager, roleVillager, roleVillager)
	g.startDay(nil)
	_, _, _ = g.speak(1, "已发言")
	_, err = g.daySkill(3, 4, roleKnight)
	if err != nil {
		t.Fatal(err)
	}
	next := g.forceDayLastWords()
	if next.FirstSpeaker != 2 || len(g.Speeches) != 1 || g.Players[3].Alive || !g.Players[4].Alive {
		t.Fatalf("%+v", next)
	}
	_, _, _ = g.speak(2, "继续")
	if g.currentSpeaker() != 4 {
		t.Fatal("dead knight remains in speech queue")
	}
}

func TestWhiteWolfExplosionNoLastWordsAndGunChain(t *testing.T) {
	g := gameWithRoles(t, roleWhiteWolfKing, roleWolf, roleHunter, roleSeer, roleWitch, roleVillager, roleVillager, roleVillager)
	g.startDay(nil)
	r, err := g.daySkill(1, 3, roleWhiteWolfKing)
	if err != nil || !r.Outcome.NeedHunter || g.PendingHunter != 3 {
		t.Fatalf("%+v %v", r, err)
	}
	shot, err := g.hunterShoot(3, 6)
	if err != nil || !shot.StartNight || shot.AwaitingLastWords || g.Round != 2 {
		t.Fatalf("%+v %v", shot, err)
	}
}

func TestWolfBeautyCharmAndDuelImmunity(t *testing.T) {
	g := gameWithRoles(t, roleWolfBeauty, roleWolf, roleHunter, roleSeer, roleWitch, roleVillager, roleVillager, roleVillager)
	g.CharmTarget = 3
	deaths := g.killDay(1, "放逐")
	if len(deaths) != 2 || g.Players[3].Alive || g.canShoot(deaths[1]) {
		t.Fatalf("%v", deaths)
	}
	g = gameWithRoles(t, roleWolfBeauty, roleWolf, roleHunter, roleSeer, roleWitch, roleVillager, roleVillager, roleVillager)
	if _, err := g.useSkill(1, "目标", 3); err != nil {
		t.Fatal(err)
	}
	g.Round = 2
	g.startNight()
	if _, err := g.useSkill(1, "目标", 3); err == nil {
		t.Fatal("consecutive charm")
	}
}

func TestDreamProtectionDoubleDreamAndDreamerDeath(t *testing.T) {
	for _, kind := range []string{"protect", "double", "dreamer", "charm"} {
		g := gameWithRoles(t, roleWolf, roleWolfBeauty, roleDreamer, roleSeer, roleWitch, roleHunter, roleVillager, roleVillager)
		g.DreamTarget = 6
		g.WolfVictim = 6
		g.WitchPoison = 6
		switch kind {
		case "double":
			g.PreviousDream = 6
		case "dreamer":
			g.WolfVictim = 3
		case "charm":
			g.CharmTarget = 3
			g.WitchPoison = 2
		}
		r := g.resolveNight()
		if g.Players[6].Alive != (kind == "protect") {
			t.Fatalf("%s: %+v", kind, r)
		}
		for _, d := range r.Deaths {
			if d.ID == 6 && g.canShoot(d) {
				t.Fatal("dream/charm victim could shoot")
			}
		}
	}
}

func TestNightmareFearBlocksKnifeAndAbilities(t *testing.T) {
	g := gameWithRoles(t, roleNightmare, roleWolf, roleSeer, roleWitch, roleDreamer, roleVillager, roleVillager, roleVillager)
	if g.canBroadcast(1) || g.canBroadcast(2) {
		t.Fatal("wolf identities disclosed before fear")
	}
	if _, err := g.useSkill(1, "目标", 2); err != nil {
		t.Fatal(err)
	}
	if _, err := g.useSkill(5, "目标", 6); err != nil {
		t.Fatal(err)
	}
	if g.Phase != phaseNightSpecial || g.WolfVictim != 0 {
		t.Fatal("fear wolf should skip knife")
	}
	g = gameWithRoles(t, roleNightmare, roleWolf, roleSeer, roleWitch, roleDreamer, roleVillager, roleVillager, roleVillager)
	if _, err := g.useSkill(1, "目标", 5); err != nil {
		t.Fatal(err)
	}
	if g.Phase != phaseNightWolf || g.DreamTarget != 0 {
		t.Fatal("feared dreamer should be skipped")
	}
	if _, err := g.useSkill(5, "目标", 6); err == nil {
		t.Fatal("feared skill accepted")
	}
}

func TestEvilKnightReflectionOnceWithSeerPriority(t *testing.T) {
	g := gameWithRoles(t, roleEvilKnight, roleWolf, roleSeer, roleWitch, roleHunter, roleVillager, roleVillager, roleVillager)
	g.SeerTarget = 1
	g.WitchPoison = 1
	g.WolfVictim = 5
	deaths := g.nightDeaths()
	if deaths[3] != "反伤" || deaths[4] != "" || deaths[1] != "" || !g.Players[1].Reflected {
		t.Fatal(deaths)
	}
	if again := g.nightDeaths(); again[3] != "" || again[4] != "" {
		t.Fatal("reflection repeated")
	}
	g.Players[1].Reflected = false
	g.SeerTarget = 0
	if deaths = g.nightDeaths(); deaths[4] != "反伤" {
		t.Fatal(deaths)
	}
	g.startDay(nil)
	if _, err := g.explode(1); err == nil {
		t.Fatal("evil knight exploded")
	}
}

func TestGargoyleExactInspectionAndKnifeInheritance(t *testing.T) {
	g := gameWithRoles(t, roleGargoyle, roleWolf, roleSeer, roleWitch, roleHunter, roleGravekeeper, roleVillager, roleVillager)
	if slices.Contains(g.wolfTeam(), 1) || g.canBroadcast(1) {
		t.Fatal("gargoyle sees wolves")
	}
	r, err := g.useSkill(1, "目标", 3)
	if err != nil || !strings.Contains(r.Text, "预言家") {
		t.Fatalf("%+v %v", r, err)
	}
	g.Players[2].Alive = false
	g.Round = 2
	g.startNight()
	if _, err := g.useSkill(1, "目标", 3); err == nil {
		t.Fatal("repeat gargoyle inspection")
	}
	if _, err := g.useSkill(1, "跳过", 0); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(g.WolfOrder, []int64{1}) {
		t.Fatal(g.WolfOrder)
	}
	if _, err := g.wolfVote(1, 7); err != nil {
		t.Fatal(err)
	}
}

func TestPureSeerWolfWizardSimultaneousInspection(t *testing.T) {
	for _, round := range []int{1, 2} {
		g := gameWithRoles(t, roleWolfWizard, roleWolf, rolePureSeer, roleWitch, roleGuard, roleHunter, roleVillager, roleVillager)
		g.Round = round
		r, err := g.useSkill(3, "目标", 1)
		if err != nil || !strings.Contains(r.Text, "狼巫") {
			t.Fatalf("%+v %v", r, err)
		}
		_, _ = g.useSkill(5, "目标", 3)
		_, _ = g.wolfVote(1, 0)
		_, _ = g.wolfVote(2, 0)
		if _, err := g.useSkill(1, "目标", 2); err == nil {
			t.Fatal("wolf wizard inspected teammate")
		}
		_, err = g.useSkill(1, "目标", 3)
		if err != nil {
			t.Fatal(err)
		}
		outcome, err := g.witchAct(4, "跳过", 0)
		if err != nil || !outcome.Complete {
			t.Fatalf("%+v %v", outcome, err)
		}
		if g.Players[1].Alive != (round == 1) || g.Players[3].Alive != (round == 1) {
			t.Fatal("incorrect simultaneous inspection deaths")
		}
	}
}

func TestMerchantGiftCanBeRetainedAndUsedLater(t *testing.T) {
	g := gameWithRoles(t, roleWolfKing, roleWolf, roleMerchant, roleSeer, roleWitch, roleVillager, roleVillager, roleVillager)
	if _, err := g.useSkill(3, "毒药", 6); err != nil {
		t.Fatal(err)
	}
	_, _ = g.wolfVote(1, 0)
	_, _ = g.wolfVote(2, 0)
	_, _ = g.inspect(4, 2)
	_, _ = g.witchAct(5, "跳过", 0)
	if g.Phase != phaseNightSpecial {
		t.Fatal("must wait for lucky player")
	}
	if _, err := g.useSkill(6, "保留", 0); err != nil {
		t.Fatal(err)
	}
	if g.Players[6].GiftUsed || g.Phase != phaseDay {
		t.Fatal("retain consumed gift")
	}
	g.Round = 2
	g.startNight()
	if g.Phase != phaseNightWolf {
		t.Fatal("used merchant should not act again")
	}
	_, _ = g.wolfVote(1, 0)
	_, _ = g.wolfVote(2, 0)
	if _, err := g.useSkill(6, "幸运", 1); err != nil {
		t.Fatal(err)
	}
	_, _ = g.inspect(4, 7)
	r, err := g.witchAct(5, "跳过", 0)
	if err != nil || !g.Players[6].GiftUsed || g.Players[1].Alive {
		t.Fatalf("%+v %v", r, err)
	}
	g.forceLastWords()
	if g.Phase == phaseHunter {
		t.Fatal("poisoned wolf king got gun")
	}
}

func TestMerchantFailureAndInvalidActionDoNotLeak(t *testing.T) {
	g := gameWithRoles(t, roleWolf, roleWolfKing, roleMerchant, roleSeer, roleWitch, roleVillager, roleVillager, roleVillager)
	if _, err := g.useSkill(3, "不存在", 6); err == nil || g.Players[3].SkillUsed {
		t.Fatal("invalid gift mutated state")
	}
	r, err := g.useSkill(3, "查验", 2)
	if err != nil || g.Players[2].HasGift || strings.Contains(r.Text, "狼人") {
		t.Fatalf("%+v %v", r, err)
	}
	_, _ = g.wolfVote(1, 0)
	_, _ = g.wolfVote(2, 0)
	g.forceSpecial()
	if g.Players[3].Alive {
		t.Fatal("merchant survived bad trade")
	}
}

func TestSkillRoomParsing(t *testing.T) {
	for _, tt := range []struct {
		gids   []int64
		input  string
		gid    int64
		action string
		target int64
	}{
		{[]int64{100}, "100 毒药 9", 100, "毒药", 9}, {[]int64{100}, "幸运 9", 100, "幸运", 9},
		{[]int64{100, 200}, "200 跳过", 200, "跳过", 0}, {[]int64{100}, "保留", 100, "保留", 0},
	} {
		gid, action, target, err := parseSkillChoice(tt.gids, strings.Fields(tt.input))
		if err != nil || gid != tt.gid || action != tt.action || target != tt.target {
			t.Fatalf("%+v -> %d %s %d %v", tt, gid, action, target, err)
		}
	}
	for _, input := range []string{"", "300 跳过", "幸运 9", "100 乱写 9", "100 0", "100 毒药 9 垃圾"} {
		if _, _, _, err := parseSkillChoice([]int64{100, 200}, strings.Fields(input)); err == nil {
			t.Fatalf("accepted %q", input)
		}
	}
	gid, act, target, err := parseWitchChoice([]int64{100}, []string{"100", "毒", "9"})
	if err != nil || gid != 100 || act != "毒" || target != 9 {
		t.Fatal("explicit single-room witch command rejected")
	}
	if _, _, _, err := parseWitchChoice([]int64{100}, []string{"200", "救"}); err == nil {
		t.Fatal("wrong room accepted")
	}
}

func TestIdiotVotingTimeoutNeedsOnlyEligibleVoters(t *testing.T) {
	g := gameWithRoles(t, roleWolf, roleSeer, roleIdiot, roleWitch, roleVillager, roleVillager)
	g.Players[3].Revealed = true
	g.startDay(nil)
	g.beginVoting()
	r, err := g.expireVoting(g.VoteDeadline, g.VoteDeadline.Add(time.Second))
	if err != nil || r.Needed != 5 || !r.NoElimination {
		t.Fatalf("%+v %v", r, err)
	}
}

func TestWitchGiftPoisonCannotDuplicateOwnPoison(t *testing.T) {
	for _, giftFirst := range []bool{true, false} {
		g := gameWithRoles(t, roleWolfKing, roleWolf, roleMerchant, roleSeer, roleWitch, roleVillager, roleVillager, roleVillager)
		if _, err := g.useSkill(3, "毒药", 5); err != nil {
			t.Fatal(err)
		}
		_, _ = g.wolfVote(1, 0)
		_, _ = g.wolfVote(2, 0)
		if giftFirst {
			if _, err := g.useSkill(5, "幸运", 1); err != nil {
				t.Fatal(err)
			}
			if _, err := g.witchAct(5, "毒", 1); err == nil || g.PoisonUsed {
				t.Fatal("duplicate own poison accepted or consumed")
			}
		} else {
			if _, err := g.witchAct(5, "毒", 1); err != nil {
				t.Fatal(err)
			}
			if _, err := g.useSkill(5, "幸运", 1); err == nil || g.Players[5].GiftUsed {
				t.Fatal("duplicate gifted poison accepted or consumed")
			}
		}
	}
}

func TestNightCharmVictimDoesNotRepeatDayLastWordsAfterGun(t *testing.T) {
	g := gameWithRoles(t, roleWolf, roleWolfBeauty, roleHunter, roleSeer, roleWitch, roleVillager, roleVillager, roleVillager)
	g.Phase = phaseHunter
	g.PendingHunter = 3
	g.HunterFromNight = true
	g.NightDeaths = []death{{3, roleHunter, "狼人袭击"}, {2, roleWolfBeauty, "毒药"}, {6, roleVillager, "殉情"}}
	g.HunterDeaths = append([]death(nil), g.NightDeaths...)
	for _, d := range g.NightDeaths {
		g.Players[d.ID].Alive = false
	}
	r, err := g.hunterShoot(3, 7)
	if err != nil || !r.AwaitingLastWords {
		t.Fatalf("%+v %v", r, err)
	}
	if len(g.DayDeaths) != 1 || g.DayDeaths[0].ID != 7 {
		t.Fatalf("night victim repeated in daytime words: %+v", g.DayDeaths)
	}
}
