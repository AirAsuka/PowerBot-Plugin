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
	errPlayerOut        = errors.New("你已经出局，不能继续操作")
	errSelfVote         = errors.New("不能投票给自己")
	errInvalidTarget    = errors.New("投票目标不是本局存活玩家")
	errNotYourTurn      = errors.New("还没轮到你描述")
)

type phase uint8

const (
	phaseLobby phase = iota
	phaseDealing
	phaseDescribing
	phaseVoting
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
	case phaseFinished:
		return "已结束"
	default:
		return "未知"
	}
}

type wordPair struct {
	Civilian   string
	Undercover string
}

type player struct {
	ID           int64
	Name         string
	Word         string
	IsUndercover bool
	Alive        bool
}

type secret struct {
	UserID int64
	Word   string
}

type voteResult struct {
	Complete       bool
	Changed        bool
	VotesCast      int
	VotesNeeded    int
	Tie            []int64
	Eliminated     int64
	WasUndercover  bool
	Winner         string
	NextDescriber  int64
	CivilianWord   string
	UndercoverWord string
	UndercoverID   int64
}

type game struct {
	HostID         int64
	Players        map[int64]*player
	JoinOrder      []int64
	Order          []int64
	Phase          phase
	Round          int
	Turn           int
	Votes          map[int64]int64
	VoteTargets    map[int64]struct{}
	CivilianWord   string
	UndercoverWord string
	UndercoverID   int64
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
	g.UndercoverID = g.Order[rand.Intn(len(g.Order))]
	g.Round = 1
	g.Turn = 0
	g.Phase = phaseDealing
	g.Votes = make(map[int64]int64)
	g.VoteTargets = nil
	g.touch()

	secrets := make([]secret, 0, len(g.Order))
	for _, id := range g.Order {
		p := g.Players[id]
		p.Alive = true
		p.IsUndercover = id == g.UndercoverID
		p.Word = g.CivilianWord
		if p.IsUndercover {
			p.Word = g.UndercoverWord
		}
		secrets = append(secrets, secret{UserID: id, Word: p.Word})
	}
	return secrets, nil
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

func (g *game) completeDeal() error {
	if g.Phase != phaseDealing {
		return errors.New("发词阶段已经结束")
	}
	g.Phase = phaseDescribing
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
	if strings.Contains(strings.ToLower(clue), strings.ToLower(p.Word)) {
		return id, false, errors.New("描述中不能直接包含你的词语")
	}

	g.Turn++
	g.touch()
	if g.Turn == len(g.Order) {
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
		return result, nil
	}

	eliminated := result.Tie[0]
	result.Complete = true
	result.Tie = nil
	result.Eliminated = eliminated
	result.WasUndercover = g.Players[eliminated].IsUndercover
	g.Players[eliminated].Alive = false
	eliminatedIndex := slices.Index(g.Order, eliminated)
	g.Order = slices.Delete(g.Order, eliminatedIndex, eliminatedIndex+1)

	if result.WasUndercover {
		result.Winner = "平民"
	} else if len(g.Order) <= 2 {
		result.Winner = "卧底"
	}
	if result.Winner != "" {
		g.Phase = phaseFinished
		result.CivilianWord = g.CivilianWord
		result.UndercoverWord = g.UndercoverWord
		result.UndercoverID = g.UndercoverID
		return result, nil
	}

	g.Round++
	g.Phase = phaseDescribing
	g.Turn = eliminatedIndex % len(g.Order)
	g.Votes = make(map[int64]int64)
	g.VoteTargets = nil
	result.NextDescriber = g.Order[g.Turn]
	return result, nil
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
