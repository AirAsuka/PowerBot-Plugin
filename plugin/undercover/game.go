package undercover

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
	minPlayers   = 3
	maxPlayers   = 12
	maxClueRunes = 80
)

var (
	errRoomExists       = errors.New("本群已经有谁是卧底房间了")
	errRoomNotFound     = errors.New("本群还没有谁是卧底房间，请先发送“创建卧底”")
	errGameStarted      = errors.New("游戏已经开始，无法加入或退出")
	errNotHost          = errors.New("只有房主可以开始游戏")
	errAlreadyJoined    = errors.New("你已经在房间里了")
	errNotJoined        = errors.New("你还没有加入本局游戏")
	errRoomFull         = errors.New("房间已满，最多支持12人")
	errNotEnoughPlayers = errors.New("至少需要3名玩家才能开始")
	errNotDescribing    = errors.New("现在不是描述阶段")
	errNotVoting        = errors.New("现在不是投票阶段")
	errNotNight         = errors.New("现在不是夜晚行动阶段")
	errNoNightAction    = errors.New("你本夜没有行动资格")
	errNightActionUsed  = errors.New("你本夜已经行动过了")
	errPlayerOut        = errors.New("你已经出局，不能继续操作")
	errSelfVote         = errors.New("不能投票给自己")
	errSelfAttack       = errors.New("不能刀自己")
	errInvalidTarget    = errors.New("目标不是本局存活玩家")
	errNotYourTurn      = errors.New("还没轮到你描述")
)

type phase uint8

const (
	phaseLobby phase = iota
	phaseDealing
	phaseDescribing
	phaseVoting
	phaseNight
	phaseFinished
)

func (p phase) String() string {
	switch p {
	case phaseLobby:
		return "等待加入"
	case phaseDealing:
		return "正在发词"
	case phaseDescribing:
		return "描述阶段"
	case phaseVoting:
		return "投票阶段"
	case phaseNight:
		return "夜晚行动"
	case phaseFinished:
		return "已结束"
	default:
		return "未知"
	}
}

type playerRole uint8

const (
	roleCivilian playerRole = iota
	roleWolf
	roleBlank
	roleAngel
)

func (r playerRole) String() string {
	switch r {
	case roleCivilian:
		return "平民"
	case roleWolf:
		return "狼人"
	case roleBlank:
		return "白板"
	case roleAngel:
		return "天使"
	default:
		return "未知"
	}
}

type wordPair struct {
	Civilian   string
	Undercover string
}

type player struct {
	ID    int64
	Name  string
	Words []string
	Role  playerRole
	Alive bool
}

type secret struct {
	UserID int64
	Role   playerRole
	Words  []string
}

type elimination struct {
	ID   int64
	Role playerRole
}

// clueRecord 保存一名玩家在某轮提交的有效描述，用于下一轮开始时生成群聊记录。
type clueRecord struct {
	PlayerID   int64
	PlayerName string
	Text       string
}

type gameReveal struct {
	CivilianWord   string
	UndercoverWord string
	WolfIDs        []int64
	BlankID        int64
	AngelID        int64
}

type voteResult struct {
	Complete    bool
	Changed     bool
	VotesCast   int
	VotesNeeded int
	Voted       []int64
	Pending     []int64
	Tie         []int64
	Eliminated  elimination
	Winner      string
	NightActors []int64
	Reveal      gameReveal
}

type nightResult struct {
	Complete      bool
	Changed       bool
	ActionsCast   int
	ActionsNeeded int
	Killed        []elimination
	Winner        string
	NextDescriber int64
	ClueRound     int
	Clues         []clueRecord
	Reveal        gameReveal
}

type game struct {
	HostID         int64
	Players        map[int64]*player
	JoinOrder      []int64
	Order          []int64
	Phase          phase
	Round          int
	Turn           int
	RoundClues     []clueRecord
	Votes          map[int64]int64
	VoteTargets    map[int64]struct{}
	NightActions   map[int64]int64 // 0 表示主动选择“不刀”
	BlankActed     bool            // 白板本夜已经猜词或主动放弃
	CivilianWord   string
	UndercoverWord string
	WolfIDs        []int64
	BlankID        int64
	AngelID        int64
	UpdatedAt      time.Time
}

func newGame(hostID int64, hostName string) *game {
	return &game{
		HostID: hostID,
		Players: map[int64]*player{
			hostID: {ID: hostID, Name: hostName, Alive: true},
		},
		JoinOrder: []int64{hostID},
		Phase:     phaseLobby,
		UpdatedAt: time.Now(),
	}
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

func (g *game) leave(id int64) (newHost int64, err error) {
	if g.Phase != phaseLobby {
		return 0, errGameStarted
	}
	if _, ok := g.Players[id]; !ok {
		return 0, errNotJoined
	}
	delete(g.Players, id)
	g.JoinOrder = slices.DeleteFunc(g.JoinOrder, func(playerID int64) bool { return playerID == id })
	if len(g.JoinOrder) > 0 && g.HostID == id {
		g.HostID = g.JoinOrder[0]
		newHost = g.HostID
	}
	g.touch()
	return newHost, nil
}

func (g *game) canBegin(requester int64) error {
	if g.Phase != phaseLobby {
		return errGameStarted
	}
	if requester != g.HostID {
		return errNotHost
	}
	if len(g.Players) < minPlayers {
		return errNotEnoughPlayers
	}
	return nil
}

func (g *game) begin(requester int64, pair wordPair) ([]secret, error) {
	if err := g.canBegin(requester); err != nil {
		return nil, err
	}

	g.Order = append([]int64(nil), g.JoinOrder...)
	rand.Shuffle(len(g.Order), func(i, j int) { g.Order[i], g.Order[j] = g.Order[j], g.Order[i] })
	if rand.Intn(2) == 0 {
		pair.Civilian, pair.Undercover = pair.Undercover, pair.Civilian
	}
	g.CivilianWord = pair.Civilian
	g.UndercoverWord = pair.Undercover
	g.assignRoles()
	g.Round = 1
	g.Turn = 0
	g.RoundClues = nil
	g.Phase = phaseDealing
	g.Votes = make(map[int64]int64)
	g.VoteTargets = nil
	g.NightActions = nil
	g.BlankActed = false
	g.touch()

	secrets := make([]secret, 0, len(g.Order))
	for _, id := range g.Order {
		p := g.Players[id]
		p.Alive = true
		switch p.Role {
		case roleCivilian:
			p.Words = []string{g.CivilianWord}
		case roleWolf:
			p.Words = []string{g.UndercoverWord}
		case roleBlank:
			p.Words = nil
		case roleAngel:
			p.Words = []string{g.CivilianWord, g.UndercoverWord}
			if rand.Intn(2) == 0 {
				p.Words[0], p.Words[1] = p.Words[1], p.Words[0]
			}
		}
		secrets = append(secrets, secret{UserID: id, Role: p.Role, Words: append([]string(nil), p.Words...)})
	}
	return secrets, nil
}

func (g *game) assignRoles() {
	ids := append([]int64(nil), g.Order...)
	rand.Shuffle(len(ids), func(i, j int) { ids[i], ids[j] = ids[j], ids[i] })
	wolfCount := 1
	hasBlank := len(ids) >= 5
	hasAngel := len(ids) >= 8
	if hasAngel {
		wolfCount = 2
	}
	for _, p := range g.Players {
		p.Role = roleCivilian
		p.Words = nil
	}
	g.WolfIDs = append([]int64(nil), ids[:wolfCount]...)
	for _, id := range g.WolfIDs {
		g.Players[id].Role = roleWolf
	}
	cursor := wolfCount
	g.BlankID = 0
	if hasBlank {
		g.BlankID = ids[cursor]
		g.Players[g.BlankID].Role = roleBlank
		cursor++
	}
	g.AngelID = 0
	if hasAngel {
		g.AngelID = ids[cursor]
		g.Players[g.AngelID].Role = roleAngel
	}
}

func (g *game) completeDeal() error {
	if g.Phase != phaseDealing {
		return errors.New("发词阶段已经结束")
	}
	g.Phase = phaseDescribing
	g.touch()
	return nil
}

// cancelDeal 将发词失败的游戏恢复为原来的大厅，保留房主和玩家名单以便重试。
func (g *game) cancelDeal() error {
	if g.Phase != phaseDealing {
		return errors.New("发词阶段已经结束")
	}
	g.Order = nil
	g.Phase = phaseLobby
	g.Round = 0
	g.Turn = 0
	g.RoundClues = nil
	g.Votes = nil
	g.VoteTargets = nil
	g.NightActions = nil
	g.BlankActed = false
	g.CivilianWord = ""
	g.UndercoverWord = ""
	g.WolfIDs = nil
	g.BlankID = 0
	g.AngelID = 0
	for _, p := range g.Players {
		p.Words = nil
		p.Role = roleCivilian
		p.Alive = true
	}
	g.touch()
	return nil
}

func (g *game) describe(id int64, clue string) (next int64, voting bool, err error) {
	if g.Phase != phaseDescribing {
		return 0, false, errNotDescribing
	}
	p, ok := g.Players[id]
	if !ok {
		return 0, false, errNotJoined
	}
	if !p.Alive {
		return 0, false, errPlayerOut
	}
	if g.Order[g.Turn] != id {
		return g.Order[g.Turn], false, errNotYourTurn
	}
	clue = strings.TrimSpace(clue)
	if clue == "" {
		return id, false, errors.New("描述不能为空")
	}
	if utf8.RuneCountInString(clue) > maxClueRunes {
		return id, false, fmt.Errorf("描述不能超过%d个字", maxClueRunes)
	}
	for _, word := range p.Words {
		if strings.Contains(strings.ToLower(clue), strings.ToLower(word)) {
			return id, false, errors.New("描述中不能直接包含你看到的词语")
		}
	}

	g.RoundClues = append(g.RoundClues, clueRecord{
		PlayerID:   id,
		PlayerName: p.Name,
		Text:       clue,
	})
	// Turn is the current position in Order, not the number of players that have
	// described this round. Later rounds may start from the middle of Order, so
	// advance it circularly and use RoundClues to decide when everyone has spoken.
	g.Turn = (g.Turn + 1) % len(g.Order)
	g.touch()
	if len(g.RoundClues) == len(g.Order) {
		g.Phase = phaseVoting
		g.Turn = 0
		g.Votes = make(map[int64]int64)
		return 0, true, nil
	}
	return g.Order[g.Turn], false, nil
}

func (g *game) vote(voter, target int64) (voteResult, error) {
	result := voteResult{VotesNeeded: len(g.Order)}
	if g.Phase != phaseVoting {
		return result, errNotVoting
	}
	voterPlayer, ok := g.Players[voter]
	if !ok {
		return result, errNotJoined
	}
	if !voterPlayer.Alive {
		return result, errPlayerOut
	}
	if voter == target {
		return result, errSelfVote
	}
	targetPlayer, ok := g.Players[target]
	if !ok || !targetPlayer.Alive {
		return result, errInvalidTarget
	}
	if len(g.VoteTargets) > 0 {
		if _, ok := g.VoteTargets[target]; !ok {
			return result, errors.New("平票重投时只能投给候选玩家")
		}
	}
	_, result.Changed = g.Votes[voter]
	g.Votes[voter] = target
	result.VotesCast = len(g.Votes)
	result.Voted, result.Pending = g.voteProgress()
	g.touch()
	if len(g.Votes) < len(g.Order) {
		return result, nil
	}

	counts := make(map[int64]int)
	maxVotes := 0
	for _, votedID := range g.Votes {
		counts[votedID]++
		if counts[votedID] > maxVotes {
			maxVotes = counts[votedID]
		}
	}
	for _, id := range g.Order {
		if counts[id] == maxVotes {
			result.Tie = append(result.Tie, id)
		}
	}
	if len(result.Tie) > 1 {
		g.Votes = make(map[int64]int64)
		g.VoteTargets = make(map[int64]struct{}, len(result.Tie))
		for _, id := range result.Tie {
			g.VoteTargets[id] = struct{}{}
		}
		result.Complete = true
		result.VotesCast = 0
		result.Voted, result.Pending = g.voteProgress()
		return result, nil
	}

	eliminated := result.Tie[0]
	result.Complete = true
	result.Tie = nil
	result.Eliminated = elimination{ID: eliminated, Role: g.Players[eliminated].Role}
	g.eliminate(eliminated)
	if winner := g.winner(); winner != "" {
		g.finish(&result.Reveal)
		result.Winner = winner
		return result, nil
	}

	g.Phase = phaseNight
	g.NightActions = make(map[int64]int64)
	g.BlankActed = false
	result.NightActors = g.nightActors()
	return result, nil
}

func (g *game) nightAction(actor, target int64) (nightResult, error) {
	result := nightResult{ActionsNeeded: g.nightActionsNeeded()}
	if g.Phase != phaseNight {
		return result, errNotNight
	}
	p, ok := g.Players[actor]
	if !ok {
		return result, errNotJoined
	}
	if !p.Alive {
		return result, errPlayerOut
	}
	if p.Role == roleAngel || p.Role == roleBlank {
		return result, errNoNightAction
	}
	if target != 0 {
		if actor == target {
			return result, errSelfAttack
		}
		targetPlayer, ok := g.Players[target]
		if !ok || !targetPlayer.Alive {
			return result, errInvalidTarget
		}
	}
	_, result.Changed = g.NightActions[actor]
	g.NightActions[actor] = target
	result.ActionsCast = g.nightActionsCast()
	g.touch()
	if result.ActionsCast < result.ActionsNeeded {
		return result, nil
	}
	return g.resolveNight(false)
}

// blankGuess 提交白板本夜唯一一次猜词机会。两个词的顺序不限；giveUp 表示主动放弃。
func (g *game) blankGuess(actor int64, first, second string, giveUp bool) (nightResult, error) {
	result := nightResult{
		ActionsCast:   g.nightActionsCast(),
		ActionsNeeded: g.nightActionsNeeded(),
	}
	if g.Phase != phaseNight {
		return result, errNotNight
	}
	p, ok := g.Players[actor]
	if !ok {
		return result, errNotJoined
	}
	if !p.Alive {
		return result, errPlayerOut
	}
	if p.Role != roleBlank {
		return result, errNoNightAction
	}
	if g.BlankActed {
		return result, errNightActionUsed
	}

	g.BlankActed = true
	result.ActionsCast = g.nightActionsCast()
	g.touch()
	if !giveUp && g.blankGuessCorrect(first, second) {
		result.Complete = true
		result.Winner = "白板"
		g.finish(&result.Reveal)
		return result, nil
	}
	if result.ActionsCast < result.ActionsNeeded {
		return result, nil
	}
	return g.resolveNight(false)
}

func (g *game) blankGuessCorrect(first, second string) bool {
	first = strings.TrimSpace(first)
	second = strings.TrimSpace(second)
	return strings.EqualFold(first, g.CivilianWord) && strings.EqualFold(second, g.UndercoverWord) ||
		strings.EqualFold(first, g.UndercoverWord) && strings.EqualFold(second, g.CivilianWord)
}

// resolveNight 同时结算所有夜间行动。force 为 true 时，普通玩家未提交按“不刀”、白板未提交按放弃处理。
func (g *game) resolveNight(force bool) (nightResult, error) {
	result := nightResult{
		ActionsCast:   g.nightActionsCast(),
		ActionsNeeded: g.nightActionsNeeded(),
	}
	if g.Phase != phaseNight {
		return result, errNotNight
	}
	if !force && result.ActionsCast < result.ActionsNeeded {
		return result, nil
	}

	killSet := make(map[int64]struct{})
	for actor, target := range g.NightActions {
		if target == 0 {
			continue
		}
		if g.Players[actor].Role == roleWolf {
			killSet[target] = struct{}{}
		} else {
			// 普通拿词玩家不知道自己的身份；平民贸然开刀会自杀。
			killSet[actor] = struct{}{}
		}
	}
	oldOrder := append([]int64(nil), g.Order...)
	firstKilledIndex := -1
	for i, id := range oldOrder {
		if _, ok := killSet[id]; ok {
			if firstKilledIndex < 0 {
				firstKilledIndex = i
			}
			result.Killed = append(result.Killed, elimination{ID: id, Role: g.Players[id].Role})
			g.Players[id].Alive = false
		}
	}
	g.Order = slices.DeleteFunc(g.Order, func(id int64) bool {
		_, killed := killSet[id]
		return killed
	})
	result.Complete = true
	if winner := g.winner(); winner != "" {
		g.finish(&result.Reveal)
		result.Winner = winner
		return result, nil
	}

	g.Round++
	result.ClueRound = g.Round - 1
	result.Clues = append([]clueRecord(nil), g.RoundClues...)
	g.RoundClues = nil
	g.Phase = phaseDescribing
	g.NightActions = nil
	g.BlankActed = false
	g.Votes = make(map[int64]int64)
	g.VoteTargets = nil
	g.Turn = 0
	if firstKilledIndex >= 0 {
		for offset := 0; offset < len(oldOrder); offset++ {
			candidate := oldOrder[(firstKilledIndex+offset)%len(oldOrder)]
			if g.Players[candidate].Alive {
				g.Turn = slices.Index(g.Order, candidate)
				break
			}
		}
	}
	result.NextDescriber = g.Order[g.Turn]
	g.touch()
	return result, nil
}

func (g *game) eliminate(id int64) {
	g.Players[id].Alive = false
	g.Order = slices.DeleteFunc(g.Order, func(playerID int64) bool { return playerID == id })
}

func (g *game) nightActors() []int64 {
	actors := make([]int64, 0, len(g.Order))
	for _, id := range g.Order {
		role := g.Players[id].Role
		if role == roleCivilian || role == roleWolf {
			actors = append(actors, id)
		}
	}
	return actors
}

func (g *game) blankCanGuess() bool {
	return g.BlankID != 0 && g.Players[g.BlankID] != nil && g.Players[g.BlankID].Alive
}

func (g *game) nightActionsNeeded() int {
	needed := len(g.nightActors())
	if g.blankCanGuess() {
		needed++
	}
	return needed
}

func (g *game) nightActionsCast() int {
	cast := len(g.NightActions)
	if g.BlankActed && g.blankCanGuess() {
		cast++
	}
	return cast
}

func (g *game) winner() string {
	wolves, others := 0, 0
	for _, id := range g.Order {
		if g.Players[id].Role == roleWolf {
			wolves++
		} else {
			others++
		}
	}
	if wolves == 0 {
		return "平民"
	}
	if wolves >= others {
		return "狼人"
	}
	return ""
}

func (g *game) finish(reveal *gameReveal) {
	g.Phase = phaseFinished
	*reveal = g.reveal()
	g.touch()
}

func (g *game) reveal() gameReveal {
	return gameReveal{
		CivilianWord:   g.CivilianWord,
		UndercoverWord: g.UndercoverWord,
		WolfIDs:        append([]int64(nil), g.WolfIDs...),
		BlankID:        g.BlankID,
		AngelID:        g.AngelID,
	}
}

func (g *game) currentDescriber() int64 {
	if g.Phase != phaseDescribing || len(g.Order) == 0 {
		return 0
	}
	return g.Order[g.Turn]
}

func (g *game) alivePlayers() []*player {
	players := make([]*player, 0, len(g.Order))
	for _, id := range g.Order {
		players = append(players, g.Players[id])
	}
	return players
}

func (g *game) voteProgress() (voted, pending []int64) {
	voted = make([]int64, 0, len(g.Votes))
	pending = make([]int64, 0, len(g.Order)-len(g.Votes))
	for _, id := range g.Order {
		if _, ok := g.Votes[id]; ok {
			voted = append(voted, id)
		} else {
			pending = append(pending, id)
		}
	}
	return voted, pending
}

func (g *game) expired(now time.Time) bool {
	limit := 15 * time.Minute
	if g.Phase != phaseLobby {
		limit = 45 * time.Minute
	}
	return now.Sub(g.UpdatedAt) > limit
}

func (g *game) touch() {
	g.UpdatedAt = time.Now()
}
