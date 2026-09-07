package werewolf

import (
	"errors"
	"fmt"
	"math/rand"
	"slices"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	minPlayers     = 6
	maxPlayers     = 12
	maxSpeechRunes = 200
)

var (
	errRoomExists       = errors.New("本群已经有狼人杀房间了")
	errRoomNotFound     = errors.New("本群还没有狼人杀房间，请先发送“创建狼人杀”")
	errGameStarted      = errors.New("游戏已经开始，无法加入或退出")
	errNotHost          = errors.New("只有房主可以执行此操作")
	errAlreadyJoined    = errors.New("你已经在房间里了")
	errNotJoined        = errors.New("你还没有加入本局游戏")
	errRoomFull         = errors.New("房间已满，最多支持12人")
	errNotEnoughPlayers = errors.New("至少需要6名玩家才能开始")
	errPlayerDead       = errors.New("你已经出局，不能执行此操作")
	errInvalidTarget    = errors.New("目标不是本局存活玩家")
	errNotYourTurn      = errors.New("还没轮到你发言")
)

type phase uint8

const (
	phaseLobby phase = iota
	phaseDealing
	phaseNightWolf
	phaseNightSpecial
	phaseDay
	phaseVoting
	phaseHunter
	phaseFinished
)

func (p phase) String() string {
	switch p {
	case phaseLobby:
		return "等待加入"
	case phaseDealing:
		return "正在发身份"
	case phaseNightWolf:
		return "夜晚·狼人行动"
	case phaseNightSpecial:
		return "夜晚·神职行动"
	case phaseDay:
		return "白天发言"
	case phaseVoting:
		return "放逐投票"
	case phaseHunter:
		return "猎人开枪"
	case phaseFinished:
		return "已结束"
	default:
		return "未知"
	}
}

type role uint8

const (
	roleVillager role = iota
	roleWolf
	roleSeer
	roleWitch
	roleHunter
)

func (r role) String() string {
	switch r {
	case roleVillager:
		return "平民"
	case roleWolf:
		return "狼人"
	case roleSeer:
		return "预言家"
	case roleWitch:
		return "女巫"
	case roleHunter:
		return "猎人"
	default:
		return "未知"
	}
}

type player struct {
	ID    int64
	Name  string
	Role  role
	Alive bool
}
type secret struct {
	UserID    int64
	Role      role
	Teammates []int64
}
type death struct {
	ID    int64
	Role  role
	Cause string
}
type reveal struct{ Roles map[int64]role }
type speech struct {
	PlayerID         int64
	PlayerName, Text string
}

type nightResult struct {
	Complete     bool
	Deaths       []death
	Winner       string
	NeedHunter   bool
	FirstSpeaker int64
	Reveal       reveal
}

type wolfVoteResult struct {
	Ready        bool
	Changed      bool
	Cast, Needed int
	Victim       int64
	Outcome      nightResult
}

type inspectResult struct {
	IsWolf  bool
	Outcome nightResult
}

type voteResult struct {
	Complete, Changed   bool
	Cast, Needed        int
	Voted, Pending, Tie []int64
	Eliminated          int64
	Winner              string
	NeedHunter          bool
	Reveal              reveal
}

type hunterResult struct {
	Shot         int64
	Winner       string
	StartNight   bool
	FirstSpeaker int64
	Reveal       reveal
}

type game struct {
	HostID    int64
	Players   map[int64]*player
	JoinOrder []int64
	Phase     phase
	Round     int

	WolfVotes                map[int64]int64
	WolfVictim               int64
	SeerActed, WitchActed    bool
	WitchHeal, WitchPoison   int64
	AntidoteUsed, PoisonUsed bool

	DayOrder    []int64
	DayTurn     int
	Speeches    []speech
	Votes       map[int64]int64
	VoteTargets map[int64]struct{}

	PendingHunter   int64
	HunterFromNight bool
	HunterDeaths    []death
	UpdatedAt       time.Time
}

func newGame(hostID int64, hostName string) *game {
	return &game{HostID: hostID, Players: map[int64]*player{hostID: {ID: hostID, Name: hostName, Alive: true}}, JoinOrder: []int64{hostID}, Phase: phaseLobby, UpdatedAt: time.Now()}
}

func (g *game) join(id int64, name string) error {
	if g.Phase != phaseLobby {
		return errGameStarted
	}
	if _, ok := g.Players[id]; ok {
		return errAlreadyJoined
	}
	if len(g.Players) >= maxPlayers {
		return errRoomFull
	}
	g.Players[id] = &player{ID: id, Name: name, Alive: true}
	g.JoinOrder = append(g.JoinOrder, id)
	g.touch()
	return nil
}

func (g *game) leave(id int64) (int64, error) {
	if g.Phase != phaseLobby {
		return 0, errGameStarted
	}
	if _, ok := g.Players[id]; !ok {
		return 0, errNotJoined
	}
	delete(g.Players, id)
	g.JoinOrder = slices.DeleteFunc(g.JoinOrder, func(v int64) bool { return v == id })
	var next int64
	if id == g.HostID && len(g.JoinOrder) > 0 {
		g.HostID = g.JoinOrder[0]
		next = g.HostID
	}
	g.touch()
	return next, nil
}

func roleCounts(n int) map[role]int {
	wolves := 2
	if n >= 9 {
		wolves = 3
	}
	if n == 12 {
		wolves = 4
	}
	counts := map[role]int{roleWolf: wolves, roleSeer: 1, roleWitch: 1}
	if n >= 8 {
		counts[roleHunter] = 1
	}
	counts[roleVillager] = n - wolves - counts[roleSeer] - counts[roleWitch] - counts[roleHunter]
	return counts
}

func (g *game) canBegin(id int64) error {
	if g.Phase != phaseLobby {
		return errGameStarted
	}
	if id != g.HostID {
		return errNotHost
	}
	if len(g.Players) < minPlayers {
		return errNotEnoughPlayers
	}
	return nil
}

func (g *game) begin(id int64) ([]secret, error) {
	if err := g.canBegin(id); err != nil {
		return nil, err
	}
	roles := make([]role, 0, len(g.Players))
	for r, count := range roleCounts(len(g.Players)) {
		for range count {
			roles = append(roles, r)
		}
	}
	rand.Shuffle(len(roles), func(i, j int) { roles[i], roles[j] = roles[j], roles[i] })
	order := append([]int64(nil), g.JoinOrder...)
	rand.Shuffle(len(order), func(i, j int) { order[i], order[j] = order[j], order[i] })
	wolves := make([]int64, 0)
	for i, id := range order {
		p := g.Players[id]
		p.Role, p.Alive = roles[i], true
		if p.Role == roleWolf {
			wolves = append(wolves, id)
		}
	}
	result := make([]secret, 0, len(order))
	for _, id := range order {
		s := secret{UserID: id, Role: g.Players[id].Role}
		if s.Role == roleWolf {
			for _, wolf := range wolves {
				if wolf != id {
					s.Teammates = append(s.Teammates, wolf)
				}
			}
		}
		result = append(result, s)
	}
	g.Phase = phaseDealing
	g.Round = 1
	g.AntidoteUsed = false
	g.PoisonUsed = false
	g.touch()
	return result, nil
}

func (g *game) completeDeal() { g.startNight() }

func (g *game) cancelDeal() {
	g.Phase = phaseLobby
	g.Round = 0
	g.resetRound()
	for _, p := range g.Players {
		p.Role, p.Alive = roleVillager, true
	}
	g.touch()
}

func (g *game) startNight() {
	g.Phase = phaseNightWolf
	g.WolfVotes = make(map[int64]int64)
	g.WolfVictim = 0
	g.SeerActed, g.WitchActed = false, false
	g.WitchHeal, g.WitchPoison = 0, 0
	g.DayOrder = nil
	g.DayTurn = 0
	g.Speeches = nil
	g.Votes = nil
	g.VoteTargets = nil
	g.touch()
}

func (g *game) wolfVote(actor, target int64) (wolfVoteResult, error) {
	r := wolfVoteResult{Needed: g.aliveRoleCount(roleWolf)}
	if g.Phase != phaseNightWolf {
		return r, errors.New("现在不是狼人行动阶段")
	}
	p := g.Players[actor]
	if p == nil {
		return r, errNotJoined
	}
	if !p.Alive {
		return r, errPlayerDead
	}
	if p.Role != roleWolf {
		return r, errors.New("你不是狼人")
	}
	t := g.Players[target]
	if t == nil || !t.Alive {
		return r, errInvalidTarget
	}
	if t.Role == roleWolf {
		return r, errors.New("不能选择狼人队友")
	}
	_, r.Changed = g.WolfVotes[actor]
	g.WolfVotes[actor] = target
	r.Cast = len(g.WolfVotes)
	g.touch()
	if r.Cast < r.Needed {
		return r, nil
	}
	return g.finishWolfVotes(r), nil
}

func (g *game) forceWolfVotes() wolfVoteResult {
	r := wolfVoteResult{Needed: g.aliveRoleCount(roleWolf)}
	for _, id := range g.aliveIDs() {
		if g.Players[id].Role == roleWolf {
			if _, ok := g.WolfVotes[id]; !ok {
				g.WolfVotes[id] = 0
			}
		}
	}
	r.Cast = len(g.WolfVotes)
	return g.finishWolfVotes(r)
}

func (g *game) finishWolfVotes(r wolfVoteResult) wolfVoteResult {
	counts := map[int64]int{}
	max := 0
	for _, target := range g.WolfVotes {
		if target != 0 {
			counts[target]++
			if counts[target] > max {
				max = counts[target]
			}
		}
	}
	var winners []int64
	for id, count := range counts {
		if count == max {
			winners = append(winners, id)
		}
	}
	if len(winners) == 1 {
		g.WolfVictim = winners[0]
	}
	r.Ready, r.Victim = true, g.WolfVictim
	g.Phase = phaseNightSpecial
	if g.aliveRoleCount(roleSeer) == 0 {
		g.SeerActed = true
	}
	if g.aliveRoleCount(roleWitch) == 0 || g.AntidoteUsed && g.PoisonUsed {
		g.WitchActed = true
	}
	if g.SeerActed && g.WitchActed {
		r.Outcome = g.resolveNight()
	}
	g.touch()
	return r
}

func (g *game) inspect(actor, target int64) (inspectResult, error) {
	var r inspectResult
	if g.Phase != phaseNightSpecial {
		return r, errors.New("现在不是神职行动阶段")
	}
	p := g.Players[actor]
	if p == nil {
		return r, errNotJoined
	}
	if !p.Alive {
		return r, errPlayerDead
	}
	if p.Role != roleSeer {
		return r, errors.New("你不是预言家")
	}
	if g.SeerActed {
		return r, errors.New("你本夜已经查验过了")
	}
	t := g.Players[target]
	if t == nil || !t.Alive {
		return r, errInvalidTarget
	}
	if actor == target {
		return r, errors.New("不能查验自己")
	}
	g.SeerActed = true
	r.IsWolf = t.Role == roleWolf
	if g.WitchActed {
		r.Outcome = g.resolveNight()
	}
	g.touch()
	return r, nil
}

func (g *game) witchAct(actor int64, action string, target int64) (nightResult, error) {
	if g.Phase != phaseNightSpecial {
		return nightResult{}, errors.New("现在不是神职行动阶段")
	}
	p := g.Players[actor]
	if p == nil {
		return nightResult{}, errNotJoined
	}
	if !p.Alive {
		return nightResult{}, errPlayerDead
	}
	if p.Role != roleWitch {
		return nightResult{}, errors.New("你不是女巫")
	}
	if g.WitchActed {
		return nightResult{}, errors.New("你本夜已经行动过了")
	}
	switch action {
	case "救":
		if g.AntidoteUsed {
			return nightResult{}, errors.New("解药已经用过了")
		}
		if g.WolfVictim == 0 {
			return nightResult{}, errors.New("本夜没有狼人击杀目标")
		}
		g.WitchHeal = g.WolfVictim
		g.AntidoteUsed = true
	case "毒":
		if g.PoisonUsed {
			return nightResult{}, errors.New("毒药已经用过了")
		}
		t := g.Players[target]
		if t == nil || !t.Alive {
			return nightResult{}, errInvalidTarget
		}
		if target == actor {
			return nightResult{}, errors.New("不能毒自己")
		}
		g.WitchPoison = target
		g.PoisonUsed = true
	case "跳过":
	default:
		return nightResult{}, errors.New("女巫行动只能是“救”“毒 目标QQ”或“跳过”")
	}
	g.WitchActed = true
	g.touch()
	if g.SeerActed {
		return g.resolveNight(), nil
	}
	return nightResult{}, nil
}

func (g *game) forceSpecial() nightResult {
	g.SeerActed, g.WitchActed = true, true
	return g.resolveNight()
}

func (g *game) resolveNight() nightResult {
	r := nightResult{Complete: true}
	deaths := map[int64]string{}
	if g.WolfVictim != 0 && g.WitchHeal != g.WolfVictim {
		deaths[g.WolfVictim] = "狼人袭击"
	}
	if g.WitchPoison != 0 {
		deaths[g.WitchPoison] = "女巫毒杀"
	}
	for _, id := range g.JoinOrder {
		if cause, ok := deaths[id]; ok && g.Players[id].Alive {
			g.Players[id].Alive = false
			r.Deaths = append(r.Deaths, death{ID: id, Role: g.Players[id].Role, Cause: cause})
		}
	}
	for _, d := range r.Deaths {
		if d.Role == roleHunter && d.Cause != "女巫毒杀" {
			g.Phase, g.PendingHunter, g.HunterFromNight = phaseHunter, d.ID, true
			g.HunterDeaths = append([]death(nil), r.Deaths...)
			r.NeedHunter = true
			return r
		}
	}
	if winner := g.winner(); winner != "" {
		r.Winner = winner
		g.finish(&r.Reveal)
		return r
	}
	g.startDay(r.Deaths)
	r.FirstSpeaker = g.currentSpeaker()
	return r
}

func (g *game) startDay(deaths []death) {
	alive := g.aliveIDs()
	start := 0
	if len(deaths) > 0 && len(alive) > 0 {
		deadIndex := slices.Index(g.JoinOrder, deaths[0].ID)
		for i, id := range alive {
			if slices.Index(g.JoinOrder, id) > deadIndex {
				start = i
				break
			}
		}
	}
	g.DayOrder = append(append([]int64(nil), alive[start:]...), alive[:start]...)
	g.DayTurn = 0
	g.Speeches = nil
	g.Votes = nil
	g.VoteTargets = nil
	g.Phase = phaseDay
	g.touch()
}

func (g *game) speak(actor int64, text string) (int64, bool, error) {
	if g.Phase != phaseDay {
		return 0, false, errors.New("现在不是白天发言阶段")
	}
	p := g.Players[actor]
	if p == nil {
		return 0, false, errNotJoined
	}
	if !p.Alive {
		return 0, false, errPlayerDead
	}
	if g.currentSpeaker() != actor {
		return g.currentSpeaker(), false, errNotYourTurn
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return actor, false, errors.New("发言不能为空")
	}
	if utf8.RuneCountInString(text) > maxSpeechRunes {
		return actor, false, fmt.Errorf("发言不能超过%d个字", maxSpeechRunes)
	}
	g.Speeches = append(g.Speeches, speech{PlayerID: actor, PlayerName: p.Name, Text: text})
	g.DayTurn++
	g.touch()
	if g.DayTurn == len(g.DayOrder) {
		g.beginVoting()
		return 0, true, nil
	}
	return g.currentSpeaker(), false, nil
}

func (g *game) skipToVote(actor int64) error {
	if g.Phase != phaseDay {
		return errors.New("现在不是白天发言阶段")
	}
	if actor != g.HostID {
		return errNotHost
	}
	g.beginVoting()
	return nil
}

func (g *game) beginVoting() {
	g.Phase = phaseVoting
	g.Votes = make(map[int64]int64)
	g.VoteTargets = nil
	g.touch()
}

func (g *game) vote(actor, target int64) (voteResult, error) {
	alive := g.aliveIDs()
	r := voteResult{Needed: len(alive)}
	if g.Phase != phaseVoting {
		return r, errors.New("现在不是放逐投票阶段")
	}
	p := g.Players[actor]
	if p == nil {
		return r, errNotJoined
	}
	if !p.Alive {
		return r, errPlayerDead
	}
	if actor == target {
		return r, errors.New("不能投票给自己")
	}
	t := g.Players[target]
	if t == nil || !t.Alive {
		return r, errInvalidTarget
	}
	if len(g.VoteTargets) > 0 {
		if _, ok := g.VoteTargets[target]; !ok {
			return r, errors.New("平票重投只能选择候选玩家")
		}
	}
	_, r.Changed = g.Votes[actor]
	g.Votes[actor] = target
	r.Cast = len(g.Votes)
	r.Voted, r.Pending = g.voteProgress()
	g.touch()
	if r.Cast < r.Needed {
		return r, nil
	}
	counts := map[int64]int{}
	max := 0
	for _, id := range g.Votes {
		counts[id]++
		if counts[id] > max {
			max = counts[id]
		}
	}
	for _, id := range alive {
		if counts[id] == max {
			r.Tie = append(r.Tie, id)
		}
	}
	if len(r.Tie) > 1 {
		g.Votes = make(map[int64]int64)
		g.VoteTargets = make(map[int64]struct{}, len(r.Tie))
		for _, id := range r.Tie {
			g.VoteTargets[id] = struct{}{}
		}
		r.Complete = true
		r.Cast = 0
		r.Voted, r.Pending = g.voteProgress()
		return r, nil
	}
	r.Complete, r.Eliminated = true, r.Tie[0]
	r.Tie = nil
	g.Players[r.Eliminated].Alive = false
	if g.Players[r.Eliminated].Role == roleHunter {
		g.Phase, g.PendingHunter, g.HunterFromNight = phaseHunter, r.Eliminated, false
		g.HunterDeaths = []death{{ID: r.Eliminated, Role: roleHunter, Cause: "放逐"}}
		r.NeedHunter = true
		return r, nil
	}
	if winner := g.winner(); winner != "" {
		r.Winner = winner
		g.finish(&r.Reveal)
		return r, nil
	}
	g.Round++
	g.startNight()
	return r, nil
}

func (g *game) hunterShoot(actor, target int64) (hunterResult, error) {
	var r hunterResult
	if g.Phase != phaseHunter || actor != g.PendingHunter {
		return r, errors.New("现在不需要你发动猎人技能")
	}
	if target != 0 {
		t := g.Players[target]
		if t == nil || !t.Alive {
			return r, errInvalidTarget
		}
		t.Alive = false
		r.Shot = target
		g.HunterDeaths = append(g.HunterDeaths, death{ID: target, Role: t.Role, Cause: "猎人开枪"})
	}
	if winner := g.winner(); winner != "" {
		r.Winner = winner
		g.finish(&r.Reveal)
		return r, nil
	}
	if g.HunterFromNight {
		g.startDay(g.HunterDeaths)
		r.FirstSpeaker = g.currentSpeaker()
	} else {
		g.Round++
		g.startNight()
		r.StartNight = true
	}
	g.PendingHunter = 0
	g.HunterDeaths = nil
	return r, nil
}

func (g *game) winner() string {
	wolves, good := 0, 0
	for _, p := range g.Players {
		if !p.Alive {
			continue
		}
		if p.Role == roleWolf {
			wolves++
		} else {
			good++
		}
	}
	if wolves == 0 {
		return "好人"
	}
	if wolves >= good {
		return "狼人"
	}
	return ""
}

func (g *game) finish(r *reveal) {
	g.Phase = phaseFinished
	r.Roles = make(map[int64]role, len(g.Players))
	for id, p := range g.Players {
		r.Roles[id] = p.Role
	}
	g.touch()
}
func (g *game) aliveIDs() []int64 {
	ids := make([]int64, 0)
	for _, id := range g.JoinOrder {
		if g.Players[id].Alive {
			ids = append(ids, id)
		}
	}
	return ids
}
func (g *game) aliveRoleCount(want role) int {
	n := 0
	for _, p := range g.Players {
		if p.Alive && p.Role == want {
			n++
		}
	}
	return n
}
func (g *game) currentSpeaker() int64 {
	if g.Phase != phaseDay || g.DayTurn >= len(g.DayOrder) {
		return 0
	}
	return g.DayOrder[g.DayTurn]
}
func (g *game) voteProgress() (voted, pending []int64) {
	for _, id := range g.aliveIDs() {
		if _, ok := g.Votes[id]; ok {
			voted = append(voted, id)
		} else {
			pending = append(pending, id)
		}
	}
	return
}
func (g *game) resetRound() {
	g.WolfVotes = nil
	g.WolfVictim = 0
	g.SeerActed = false
	g.WitchActed = false
	g.WitchHeal = 0
	g.WitchPoison = 0
	g.DayOrder = nil
	g.Speeches = nil
	g.Votes = nil
	g.VoteTargets = nil
	g.PendingHunter = 0
	g.HunterDeaths = nil
}
func (g *game) touch() { g.UpdatedAt = time.Now() }
func (g *game) expired(now time.Time) bool {
	limit := 20 * time.Minute
	if g.Phase != phaseLobby {
		limit = 90 * time.Minute
	}
	return now.Sub(g.UpdatedAt) > limit
}
