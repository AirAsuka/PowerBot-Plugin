package mcfish

import (
	"path/filepath"
	"testing"
	"time"

	sql "github.com/FloatTech/sqlite"
)

func TestResetFishTimes(t *testing.T) {
	db := fishdb{db: sql.New(filepath.Join(t.TempDir(), "fish.db"))}
	if err := db.db.Open(time.Hour); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.db.Close() })
	if err := db.resetFishTimes(0); err != nil {
		t.Fatal(err)
	}
	states := []fishState{
		{ID: 1, Duration: time.Now().Unix(), Fish: 100, Equip: 2, Curse: 3, Bless: 4},
		{ID: 2, Duration: time.Now().Unix(), Fish: 25, Equip: 5, Curse: 6, Bless: 7},
	}
	for _, state := range states {
		if err := db.db.Insert("fishState", &state); err != nil {
			t.Fatal(err)
		}
	}
	check := func() {
		t.Helper()
		for _, want := range states {
			var got fishState
			if err := db.db.Find("fishState", &got, "WHERE ID = ?", want.ID); err != nil {
				t.Fatal(err)
			}
			if got != want {
				t.Fatalf("got %+v, want %+v", got, want)
			}
		}
	}
	if err := db.resetFishTimes(1); err != nil {
		t.Fatal(err)
	}
	states[0].Fish = 0
	check()
	if remaining, err := db.getFishResidue(1, FishLimit); err != nil || remaining != FishLimit {
		t.Fatalf("remaining = %d, err = %v", remaining, err)
	}
	if err := db.resetFishTimes(0); err != nil {
		t.Fatal(err)
	}
	states[1].Fish = 0
	check()
}
