package mcfish

import (
	"testing"
	"time"
)

func TestCurrentDateKey(t *testing.T) {
	t.Parallel()

	got := currentDateKey(time.Date(2026, time.September, 7, 23, 59, 59, 0, time.Local))
	if got != 20260907 {
		t.Fatalf("currentDateKey() = %d, want 20260907", got)
	}
}

func TestAddDailyDiamondPoleStock(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   int
		want int
	}{
		{name: "empty stock", in: 0, want: 1},
		{name: "unsold stock accumulates", in: 6, want: 7},
		{name: "stock is capped", in: 10, want: 10},
		{name: "stock above cap is repaired", in: 12, want: 10},
		{name: "negative stock is repaired", in: -1, want: 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := addDailyDiamondPoleStock(tt.in); got != tt.want {
				t.Fatalf("addDailyDiamondPoleStock(%d) = %d, want %d", tt.in, got, tt.want)
			}
		})
	}
}

func TestDailyDiamondPoleIdentity(t *testing.T) {
	t.Parallel()

	dailyPole := store{
		Duration: dailyDiamondPoleDuration,
		Name:     "钻石竿",
		Price:    dailyDiamondPolePrice,
		Other:    dailyDiamondPoleOther,
		Type:     "pole",
	}
	if !isDailyDiamondPole(dailyPole) {
		t.Fatal("daily diamond pole was not recognized")
	}

	playerPole := dailyPole
	playerPole.Duration = time.Now().Unix()
	if isDailyDiamondPole(playerPole) {
		t.Fatal("player-listed diamond pole was recognized as the daily fixed-price pole")
	}
}
