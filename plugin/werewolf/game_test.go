package werewolf

import (
	"fmt"
	"regexp"
	"slices"
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
	wantWolves := map[int]int{6: 2, 7: 2, 8: 3, 9: 3, 10: 3, 11: 3, 12: 4}
	for n := minPlayers; n <= maxPlayers; n++ {
		counts := roleCounts(n)
		total := 0
		for _, count := range counts {
			total += count
		}
		if total != n || counts[roleWolf] != wantWolves[n] || counts[roleSeer] != 1 {
			t.Fatalf("%d players: %v", n, counts)
		}
		if counts[roleHunter] != 1 || (n >= 7) != (counts[roleWitch] == 1) {
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
	if err != nil || !outcome.Complete || !outcome.AwaitingLastWords || len(outcome.Deaths) != 1 || outcome.Deaths[0].ID != 5 {
		t.Fatalf("outcome=%+v err=%v", outcome, err)
	}
	if g.Phase != phaseNightLastWords || g.Players[5].Alive {
		t.Fatalf("phase=%v victim alive=%v", g.Phase, g.Players[5].Alive)
	}
	outcome, err = g.submitLastWords(5, "预言家请带队")
	if err != nil || g.Phase != phaseDay || outcome.LastWords[5] != "预言家请带队" {
		t.Fatalf("last words outcome=%+v phase=%v err=%v", outcome, g.Phase, err)
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
	g.VoteSummarySent = true
	votes := [][2]int64{{1, 3}, {2, 4}, {3, 4}, {4, 3}, {5, 3}, {6, 4}, {7, 3}, {8, 4}}
	var result voteResult
	for _, v := range votes {
		var err error
		result, err = g.vote(v[0], v[1])
		if err != nil {
			t.Fatal(err)
		}
	}
	if len(result.Tie) != 2 || len(g.VoteTargets) != 2 || len(g.Votes) != 0 || g.VoteSummarySent {
		t.Fatalf("tie result=%+v", result)
	}
	if _, err := g.vote(1, 5); err == nil {
		t.Fatal("vote outside tied candidates was accepted")
	}
	if _, err := g.vote(1, 0); err != nil {
		t.Fatalf("abstention during revote was rejected: %v", err)
	}
}

func TestAllPlayersMayAbstainFromExileVote(t *testing.T) {
	g := gameWithRoles(t, roleWolf, roleWolf, roleSeer, roleWitch, roleVillager, roleVillager)
	g.startDay(nil)
	g.beginVoting()

	var result voteResult
	var err error
	for _, voter := range g.aliveIDs() {
		result, err = g.vote(voter, 0)
		if err != nil {
			t.Fatal(err)
		}
	}
	if !result.Complete || !result.NoElimination || result.Eliminated != 0 {
		t.Fatalf("result=%+v", result)
	}
	if g.Phase != phaseNightWolf || g.Round != 2 {
		t.Fatalf("phase=%v round=%d, want next night", g.Phase, g.Round)
	}
}

func TestWerewolfVotePatternAcceptsAbstention(t *testing.T) {
	re := regexp.MustCompile(votePattern)
	for _, input := range []string{"狼人杀投票 弃票", "狼人杀投票弃票"} {
		matches := re.FindStringSubmatch(input)
		if len(matches) != 3 || matches[1] != "" || matches[2] != "" {
			t.Errorf("vote pattern did not recognize abstention %q: %v", input, matches)
		}
	}
}

func TestVoteArchivesEverySpeechForLaterDays(t *testing.T) {
	g := gameWithRoles(t, roleWolf, roleWolf, roleSeer, roleWitch, roleHunter, roleVillager, roleVillager, roleVillager)
	g.startDay(nil)
	for g.Phase == phaseDay {
		speaker := g.currentSpeaker()
		if _, _, err := g.speak(speaker, fmt.Sprintf("玩家%d的发言", speaker), 1000+speaker); err != nil {
			t.Fatal(err)
		}
	}
	want := append([]speech(nil), g.Speeches...)
	for _, item := range want {
		if item.SourceMessageID == 0 {
			t.Fatalf("speech did not retain its source message ID: %+v", item)
		}
	}

	for _, voter := range g.aliveIDs() {
		target := int64(8)
		if voter == target {
			target = 7
		}
		if _, err := g.vote(voter, target); err != nil {
			t.Fatal(err)
		}
	}
	if g.Phase != phaseDayLastWords {
		t.Fatalf("phase=%v, want daytime last words", g.Phase)
	}
	lastWords, err := g.submitDayLastWords(8, "请复盘票型")
	if err != nil || !lastWords.StartNight || lastWords.LastWords[8] != "请复盘票型" {
		t.Fatalf("last words result=%+v err=%v", lastWords, err)
	}
	if g.Phase != phaseNightWolf || g.Round != 2 {
		t.Fatalf("phase=%v round=%d, want next night", g.Phase, g.Round)
	}
	archives := g.speechArchives(false)
	if len(archives) != 1 || archives[0].Round != 1 || !slices.Equal(archives[0].Speeches, want) {
		t.Fatalf("archives=%+v, want day-1 speeches=%+v", archives, want)
	}
	archives[0].Speeches[0].Text = "被外部修改"
	if g.SpeechHistory[0].Speeches[0].Text == "被外部修改" {
		t.Fatal("speechArchives returned game-owned storage")
	}

	g.Phase = phaseNightSpecial
	result := g.finalizeNight()
	if result.FirstSpeaker == 0 || len(result.Archives) != 1 || result.Archives[0].Round != 1 {
		t.Fatalf("next-day result=%+v, want archived day-1 speeches", result)
	}
}

func TestSkippedDayArchivesOnlySubmittedSpeeches(t *testing.T) {
	g := gameWithRoles(t, roleWolf, roleWolf, roleSeer, roleWitch, roleHunter, roleVillager, roleVillager, roleVillager)
	g.startDay(nil)
	for range 2 {
		if _, _, err := g.speak(g.currentSpeaker(), "提前投票前的发言"); err != nil {
			t.Fatal(err)
		}
	}
	want := append([]speech(nil), g.Speeches...)
	if err := g.skipToVote(g.HostID); err != nil {
		t.Fatal(err)
	}
	current := g.speechArchives(true)
	if len(current) != 1 || !slices.Equal(current[0].Speeches, want) {
		t.Fatalf("vote preview=%+v, want submitted speeches=%+v", current, want)
	}

	for _, voter := range g.aliveIDs() {
		target := int64(8)
		if voter == target {
			target = 7
		}
		if _, err := g.vote(voter, target); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := g.submitDayLastWords(8, "放弃"); err != nil {
		t.Fatal(err)
	}
	archives := g.speechArchives(false)
	if len(archives) != 1 || !slices.Equal(archives[0].Speeches, want) {
		t.Fatalf("archives=%+v, want only submitted speeches=%+v", archives, want)
	}
}

func TestNightHunterResolutionCarriesSpeechArchivesIntoDay(t *testing.T) {
	g := gameWithRoles(t, roleWolf, roleWolf, roleSeer, roleWitch, roleHunter, roleVillager, roleVillager, roleVillager)
	want := []speech{{PlayerID: 1, PlayerName: "玩家1", Text: "第一天发言"}}
	g.SpeechHistory = []speechArchive{{Round: 1, Speeches: append([]speech(nil), want...)}}
	g.Round = 2
	g.Players[5].Alive = false
	g.Phase = phaseHunter
	g.PendingHunter = 5
	g.HunterFromNight = true
	g.HunterDeaths = []death{{ID: 5, Role: roleHunter, Cause: "狼人袭击"}}

	result, err := g.hunterShoot(5, 0)
	if err != nil {
		t.Fatal(err)
	}
	if result.FirstSpeaker == 0 || len(result.Archives) != 1 || !slices.Equal(result.Archives[0].Speeches, want) {
		t.Fatalf("hunter result=%+v, want archived day-1 speeches", result)
	}
}

func TestNightHunterShotTargetGetsGroupLastWordsBeforeDay(t *testing.T) {
	g := gameWithRoles(t, roleWolf, roleWolf, roleSeer, roleWitch, roleHunter, roleVillager, roleVillager, roleVillager)
	g.Players[5].Alive = false
	g.Phase = phaseHunter
	g.PendingHunter = 5
	g.HunterFromNight = true
	g.HunterDeaths = []death{{ID: 5, Role: roleHunter, Cause: "狼人袭击"}}

	hunter, err := g.hunterShoot(5, 8)
	if err != nil || !hunter.AwaitingLastWords || hunter.Shot != 8 || g.Phase != phaseDayLastWords {
		t.Fatalf("hunter result=%+v phase=%v err=%v", hunter, g.Phase, err)
	}
	if len(g.DayDeaths) != 1 || g.DayDeaths[0].ID != 8 {
		t.Fatalf("eligible daytime deaths=%+v, want only shot target", g.DayDeaths)
	}
	if _, err := g.submitDayLastWords(5, "重复遗言"); err == nil {
		t.Fatal("night-dead hunter was allowed a second, group last words message")
	}
	result, err := g.submitDayLastWords(8, "我是被猎人带走的")
	if err != nil || result.FirstSpeaker == 0 || g.Phase != phaseDay || result.LastWords[8] != "我是被猎人带走的" {
		t.Fatalf("last words result=%+v phase=%v err=%v", result, g.Phase, err)
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
	if err != nil || !hunter.AwaitingLastWords || hunter.Winner != "" {
		t.Fatalf("hunter result=%+v err=%v", hunter, err)
	}
	if result, err := g.submitDayLastWords(2, "带走狼人"); err != nil || !result.AwaitingLastWords {
		t.Fatalf("hunter last words result=%+v err=%v", result, err)
	}
	if result, err := g.submitDayLastWords(1, "放弃"); err != nil || result.Winner != "好人" || result.LastWords[2] != "带走狼人" {
		t.Fatalf("shot player's last words result=%+v err=%v", result, err)
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
	if !outcome.AwaitingLastWords {
		t.Fatalf("night did not wait for last words: %+v", outcome)
	}
	_, _ = g.submitLastWords(5, "放弃")
	outcome, err = g.submitLastWords(6, "放弃")
	if err != nil || outcome.NeedHunter || g.Phase == phaseHunter {
		t.Fatalf("poisoned hunter was allowed to shoot: %+v", outcome)
	}
}

func TestWolvesActSequentially(t *testing.T) {
	g := gameWithRoles(t, roleWolf, roleWolf, roleSeer, roleWitch, roleVillager, roleVillager)
	if _, err := g.wolfVote(2, 5); err == nil {
		t.Fatal("second wolf acted before the first wolf")
	}
	first, err := g.wolfVote(1, 5)
	if err != nil || first.NextWolf != 2 || first.Ready {
		t.Fatalf("first=%+v err=%v", first, err)
	}
	if _, err := g.wolfVote(1, 6); err == nil {
		t.Fatal("first wolf voted twice")
	}
	second, err := g.wolfVote(2, 5)
	if err != nil || !second.Ready || second.Victim != 5 {
		t.Fatalf("second=%+v err=%v", second, err)
	}
}

func TestWolfCanSelfKillOrChooseNoKill(t *testing.T) {
	g := gameWithRoles(t, roleWolf, roleWolf, roleSeer, roleWitch, roleVillager, roleVillager)
	if _, err := g.wolfVote(1, 1); err != nil {
		t.Fatalf("self kill rejected: %v", err)
	}
	result, err := g.wolfVote(2, 0)
	if err != nil || !result.Deciding || result.Ready || result.NextWolf != 1 {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	result, err = g.wolfVote(1, 1)
	if err != nil || !result.Ready || result.Victim != 1 {
		t.Fatalf("decision=%+v err=%v", result, err)
	}
}

func TestWolfCanTargetTeammate(t *testing.T) {
	g := gameWithRoles(t, roleWolf, roleWolf, roleSeer, roleWitch, roleVillager, roleVillager)
	if _, err := g.wolfVote(1, 2); err != nil {
		t.Fatalf("targeting wolf teammate was rejected: %v", err)
	}
	result, err := g.wolfVote(2, 2)
	if err != nil || !result.Ready || result.Victim != 2 {
		t.Fatalf("result=%+v err=%v", result, err)
	}
}

func TestTwoDifferentWolfVotesReturnToFirstWolf(t *testing.T) {
	g := gameWithRoles(t, roleWolf, roleWolf, roleSeer, roleWitch, roleVillager, roleVillager)
	if _, err := g.wolfVote(1, 5); err != nil {
		t.Fatal(err)
	}
	result, err := g.wolfVote(2, 6)
	if err != nil || !result.Deciding || result.Ready || result.NextWolf != 1 || g.currentWolf() != 1 || g.Phase != phaseNightWolf {
		t.Fatalf("result=%+v current=%d phase=%v err=%v", result, g.currentWolf(), g.Phase, err)
	}
	if _, err := g.wolfVote(1, 3); err == nil {
		t.Fatal("first wolf was allowed to choose outside the two original results")
	}
	result, err = g.wolfVote(1, 6)
	if err != nil || !result.Ready || result.Victim != 6 || g.Phase != phaseNightSpecial {
		t.Fatalf("decision=%+v phase=%v err=%v", result, g.Phase, err)
	}
}

func TestSlaughteredVillagersOrGodsLose(t *testing.T) {
	g := gameWithRoles(t, roleWolf, roleWolf, roleSeer, roleWitch, roleVillager, roleVillager)
	g.Players[5].Alive, g.Players[6].Alive = false, false
	if got := g.winner(); got != "狼人" {
		t.Fatalf("villager slaughter winner=%q", got)
	}
	g = gameWithRoles(t, roleWolf, roleWolf, roleSeer, roleWitch, roleVillager, roleVillager)
	g.Players[3].Alive, g.Players[4].Alive = false, false
	if got := g.winner(); got != "狼人" {
		t.Fatalf("god slaughter winner=%q", got)
	}
}

func TestWitchCanOnlySelfSaveOnFirstNight(t *testing.T) {
	g := gameWithRoles(t, roleWolf, roleWolf, roleSeer, roleWitch, roleVillager, roleVillager, roleVillager)
	_, _ = g.wolfVote(1, 4)
	_, _ = g.wolfVote(2, 4)
	_, _ = g.inspect(3, 1)
	if _, err := g.witchAct(4, "救", 0); err != nil {
		t.Fatalf("first-night self save rejected: %v", err)
	}
	g = gameWithRoles(t, roleWolf, roleWolf, roleSeer, roleWitch, roleVillager, roleVillager, roleVillager)
	g.Round = 2
	_, _ = g.wolfVote(1, 4)
	_, _ = g.wolfVote(2, 4)
	_, _ = g.inspect(3, 1)
	if _, err := g.witchAct(4, "救", 0); err == nil {
		t.Fatal("self save after first night was accepted")
	}
}

func TestWolfExplosionEndsDayAndStartsNextNight(t *testing.T) {
	g := gameWithRoles(t, roleWolf, roleWolf, roleSeer, roleWitch, roleVillager, roleVillager)
	g.startDay(nil)
	g.Speeches = append(g.Speeches, speech{PlayerID: g.currentSpeaker(), Text: "尚未完成的发言"})
	result, err := g.explode(1)
	if err != nil {
		t.Fatal(err)
	}
	if !result.AwaitingLastWords || result.Winner != "" || g.Players[1].Alive || g.Phase != phaseDayLastWords || g.Round != 1 {
		t.Fatalf("result=%+v phase=%v round=%d alive=%v", result, g.Phase, g.Round, g.Players[1].Alive)
	}
	if _, err := g.submitDayLastWords(1, "我自爆了"); err != nil {
		t.Fatal(err)
	}
	if g.Phase != phaseNightWolf || g.Round != 2 || len(g.Speeches) != 0 || len(g.DayOrder) != 0 {
		t.Fatalf("day did not advance cleanly: phase=%v round=%d speeches=%v order=%v", g.Phase, g.Round, g.Speeches, g.DayOrder)
	}
}

func TestWolfExplosionDiscardsVoting(t *testing.T) {
	g := gameWithRoles(t, roleWolf, roleWolf, roleSeer, roleWitch, roleVillager, roleVillager)
	g.startDay(nil)
	g.beginVoting()
	if _, err := g.vote(3, 1); err != nil {
		t.Fatal(err)
	}
	if _, err := g.explode(2); err != nil {
		t.Fatal(err)
	}
	if g.Phase != phaseDayLastWords {
		t.Fatalf("phase=%v, want daytime last words", g.Phase)
	}
	if _, err := g.submitDayLastWords(2, "放弃"); err != nil {
		t.Fatal(err)
	}
	if g.Phase != phaseNightWolf || len(g.Votes) != 0 || len(g.VoteTargets) != 0 {
		t.Fatalf("phase=%v votes=%v targets=%v", g.Phase, g.Votes, g.VoteTargets)
	}
}

func TestLastWolfExplosionEndsGame(t *testing.T) {
	g := gameWithRoles(t, roleWolf, roleSeer, roleWitch, roleVillager)
	g.startDay(nil)
	result, err := g.explode(1)
	if err != nil {
		t.Fatal(err)
	}
	if !result.AwaitingLastWords || result.Winner != "" || g.Phase != phaseDayLastWords {
		t.Fatalf("result=%+v phase=%v", result, g.Phase)
	}
	lastWords, err := g.submitDayLastWords(1, "我是最后一匹狼")
	if err != nil || lastWords.Winner != "好人" || g.Phase != phaseFinished || len(lastWords.Reveal.Roles) != len(g.Players) {
		t.Fatalf("last words result=%+v phase=%v err=%v", lastWords, g.Phase, err)
	}
}

func TestNonWolfCannotExplode(t *testing.T) {
	g := gameWithRoles(t, roleWolf, roleWolf, roleSeer, roleWitch, roleVillager, roleVillager)
	g.startDay(nil)
	if _, err := g.explode(3); err == nil {
		t.Fatal("seer was allowed to explode")
	}
	if !g.Players[3].Alive || g.Phase != phaseDay {
		t.Fatal("failed explosion changed game state")
	}
}
