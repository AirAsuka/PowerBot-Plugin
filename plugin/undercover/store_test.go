package undercover

import (
	"errors"
	"sync"
	"testing"
)

func TestBeginOnlyDrawsForValidRoom(t *testing.T) {
	s := newRoomStore()
	draws := 0
	draw := func() (wordPair, error) {
		draws++
		return wordPair{"牛奶", "豆浆"}, nil
	}
	if _, _, err := s.begin(10, 1, draw); !errors.Is(err, errRoomNotFound) {
		t.Fatalf("missing room: %v", err)
	}
	g, err := s.create(10, 1, "房主")
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.begin(10, 2, draw); !errors.Is(err, errNotHost) {
		t.Fatalf("non-host: %v", err)
	}
	if _, _, err := s.begin(10, 1, draw); !errors.Is(err, errNotEnoughPlayers) {
		t.Fatalf("insufficient players: %v", err)
	}
	if draws != 0 {
		t.Fatalf("invalid requests consumed %d pairs", draws)
	}
	for _, id := range []int64{2, 3} {
		if err := g.join(id, "玩家"); err != nil {
			t.Fatal(err)
		}
	}
	drawErr := errors.New("词库耗尽")
	if _, _, err := s.begin(10, 1, func() (wordPair, error) { return wordPair{}, drawErr }); !errors.Is(err, drawErr) {
		t.Fatalf("failed draw: %v", err)
	}
	if err := g.canBegin(1); err != nil {
		t.Fatalf("failed draw changed room state: %v", err)
	}
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _, _ = s.begin(10, 1, draw)
		}()
	}
	wg.Wait()
	if draws != 1 || g.Phase != phaseDealing {
		t.Fatalf("concurrent starts: draws=%d phase=%v", draws, g.Phase)
	}
}
