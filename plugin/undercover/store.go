package undercover

import (
	"errors"
	"sync"
	"time"
)

type roomStore struct {
	mu    sync.Mutex
	rooms map[int64]*game
}

func newRoomStore() *roomStore {
	return &roomStore{rooms: make(map[int64]*game)}
}

func (s *roomStore) create(groupID, hostID int64, hostName string) (*game, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if existing := s.room(groupID); existing != nil {
		return nil, errRoomExists
	}
	g := newGame(hostID, hostName)
	s.rooms[groupID] = g
	return g, nil
}

func (s *roomStore) withRoom(groupID int64, fn func(*game) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	g := s.room(groupID)
	if g == nil {
		return errRoomNotFound
	}
	return fn(g)
}

func (s *roomStore) begin(groupID, requester int64, pair wordPair) (*game, []secret, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	g := s.room(groupID)
	if g == nil {
		return nil, nil, errRoomNotFound
	}
	secrets, err := g.begin(requester, pair)
	return g, secrets, err
}

func (s *roomStore) canBegin(groupID, requester int64) error {
	return s.withRoom(groupID, func(g *game) error {
		return g.canBegin(requester)
	})
}

func (s *roomStore) finishDeal(groupID int64, expected *game, success bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	g := s.rooms[groupID]
	if g == nil || g != expected {
		return errRoomNotFound
	}
	if !success {
		delete(s.rooms, groupID)
		return nil
	}
	return g.completeDeal()
}

func (s *roomStore) end(groupID, requester int64, force bool) (*game, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	g := s.room(groupID)
	if g == nil {
		return nil, errRoomNotFound
	}
	if requester != g.HostID && !force {
		return nil, errors.New("只有房主或群管理员可以结束游戏")
	}
	delete(s.rooms, groupID)
	return g, nil
}

func (s *roomStore) removeIfSame(groupID int64, expected *game) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.rooms[groupID] == expected {
		delete(s.rooms, groupID)
	}
}

func (s *roomStore) pendingNightGroups(userID int64) []int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	groups := make([]int64, 0, 1)
	for groupID := range s.rooms {
		g := s.room(groupID)
		if g == nil || g.Phase != phaseNight {
			continue
		}
		p := g.Players[userID]
		if p != nil && p.Alive && (p.Role == roleCivilian || p.Role == roleWolf) {
			groups = append(groups, groupID)
		}
	}
	return groups
}

// room 必须在持有 s.mu 时调用。
func (s *roomStore) room(groupID int64) *game {
	g := s.rooms[groupID]
	if g != nil && g.expired(time.Now()) {
		delete(s.rooms, groupID)
		return nil
	}
	return g
}
