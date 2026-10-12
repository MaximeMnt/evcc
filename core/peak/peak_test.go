package peak

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestTarget(t *testing.T) {
	assert.Equal(t, 4e3, Target(4e3, 0))
	assert.Equal(t, 4e3, Target(4e3, 3e3))
	assert.Equal(t, 5e3, Target(4e3, 5e3), "follows a higher month peak")
}

func TestIntervalEnergy(t *testing.T) {
	start := time.Date(2026, 10, 12, 10, 0, 0, 0, time.UTC)
	now := start.Add(10 * time.Minute)

	tc := []struct {
		desc   string
		energy float64
		since  time.Time
		exp    float64
	}{
		{"observed since interval start", 500, start, 500},
		{"observed since mid interval", 250, start.Add(5 * time.Minute), 500},
		{"unknown", 0, time.Time{}, 4e3 / 6},
		{"stale previous interval", 800, start.Add(-time.Minute), 4e3 / 6},
		{"observed just now", 0, now, 4e3 / 6},
	}

	for _, tc := range tc {
		t.Run(tc.desc, func(t *testing.T) {
			assert.InDelta(t, tc.exp, IntervalEnergy(tc.energy, tc.since, now, 4e3), 1e-9)
		})
	}
}

func TestHeadroom(t *testing.T) {
	start := time.Date(2026, 10, 12, 10, 0, 0, 0, time.UTC)

	tc := []struct {
		desc    string
		energy  float64 // Wh
		elapsed time.Duration
		exp     float64
	}{
		{"interval start", 0, 0, 4e3},
		{"on target so far", 500, 7*time.Minute + 30*time.Second, 4e3},
		{"high consumption so far leaves less", 750, 7*time.Minute + 30*time.Second, 2e3},
		{"low consumption so far is capped at target", 0, 7*time.Minute + 30*time.Second, 4e3},
		{"target already used up", 1000, 10 * time.Minute, 0},
		{"exceeded", 1200, 10 * time.Minute, 0},
		{"interval end", 990, 15*time.Minute - time.Second, 4e3},
	}

	for _, tc := range tc {
		t.Run(tc.desc, func(t *testing.T) {
			assert.InDelta(t, tc.exp, Headroom(4e3, tc.energy, start.Add(tc.elapsed)), 1e-6)
		})
	}
}

func TestProjected(t *testing.T) {
	start := time.Date(2026, 10, 12, 10, 0, 0, 0, time.UTC)

	assert.InDelta(t, 4e3, Projected(0, 4e3, start), 1e-9)
	assert.InDelta(t, 3e3, Projected(500, 2e3, start.Add(7*time.Minute+30*time.Second)), 1e-9)
	assert.InDelta(t, 2e3, Projected(500, -2e3, start.Add(7*time.Minute+30*time.Second)), 1e-9, "export does not count")
}
