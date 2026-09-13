package werewolf

import (
	"errors"
	"sort"
	"sync"
	"time"
)

type roomStore struct {
	mu    sync.Mutex
	rooms map[int64]*game
}

func newRoomStore() *roomStore { return &roomStore{rooms: make(map[int64]*game)} }
func (s *roomStore) room(groupID int64) *game {
	g := s.rooms[groupID]
	if g != nil && g.expired(time.Now()) {
		delete(s.rooms, groupID)
		return nil
	}
	return g
}
func (s *roomStore) create(groupID, hostID int64, name string) (*game, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.room(groupID) != nil {
		return nil, errRoomExists
	}
	g := newGame(hostID, name)
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
func (s *roomStore) begin(groupID, userID int64) (*game, []secret, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	g := s.room(groupID)
	if g == nil {
		return nil, nil, errRoomNotFound
	}
	v, e := g.begin(userID)
	return g, v, e
}
func (s *roomStore) finishDeal(groupID int64, expected *game, ok bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	g := s.rooms[groupID]
	if g == nil || g != expected {
		return errRoomNotFound
	}
	if ok {
		g.completeDeal()
	} else {
		g.cancelDeal()
	}
	return nil
}
func (s *roomStore) end(groupID, userID int64, force bool) (*game, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	g := s.room(groupID)
	if g == nil {
		return nil, errRoomNotFound
	}
	if userID != g.HostID && !force {
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
func (s *roomStore) pending(userID int64, want phase, roles ...role) []int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []int64
	for gid := range s.rooms {
		g := s.room(gid)
		if g == nil || g.Phase != want {
			continue
		}
		p := g.Players[userID]
		if p == nil || !p.Alive {
			continue
		}
		for _, r := range roles {
			if p.Role == r {
				out = append(out, gid)
				break
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

func (s *roomStore) pendingWolfBroadcasts(userID int64) []int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []int64
	for gid := range s.rooms {
		g := s.room(gid)
		if g == nil || !g.Phase.isNight() {
			continue
		}
		p := g.Players[userID]
		if p != nil && p.Alive && g.canBroadcast(userID) {
			out = append(out, gid)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

func (s *roomStore) pendingLastWords(userID int64) []int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []int64
	for gid := range s.rooms {
		g := s.room(gid)
		if g == nil || g.Phase != phaseNightLastWords {
			continue
		}
		for _, d := range g.NightDeaths {
			if d.ID == userID {
				if _, submitted := g.LastWords[userID]; !submitted {
					out = append(out, gid)
				}
				break
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}
