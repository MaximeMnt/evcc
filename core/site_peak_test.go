package core

import (
	"math"
	"testing"
	"time"

	"github.com/evcc-io/evcc/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPeakSettings(t *testing.T) {
	site := &Site{log: util.NewLogger("foo")}

	for _, v := range []float64{math.NaN(), math.Inf(1), -1} {
		assert.Error(t, site.SetPeakLimit(v), v)
	}

	assert.ErrorIs(t, site.SetPeakFollowing(true), ErrPeakLimitNotSet, "limit required")
	assert.False(t, site.GetPeakFollowing())

	require.NoError(t, site.SetPeakLimit(4e3))
	require.NoError(t, site.SetPeakFollowing(true))
	assert.True(t, site.GetPeakFollowing())
	assert.Equal(t, 4e3, site.GetPeakLimit())

	assert.ErrorIs(t, site.SetPeakLimit(0), ErrPeakLimitNotSet, "limit required while following")
	assert.Equal(t, 4e3, site.GetPeakLimit())

	require.NoError(t, site.SetPeakFollowing(false))
	require.NoError(t, site.SetPeakLimit(0))
}

type fakeSlotCollector struct {
	energy    float64 // kWh
	since     time.Time
	monthPeak float64 // kWh
	from      time.Time
}

func (c *fakeSlotCollector) SlotEnergy() (float64, time.Time) {
	return c.energy, c.since
}

func (c *fakeSlotCollector) MaxSlotEnergy(from time.Time) (float64, time.Time, error) {
	c.from = from
	return c.monthPeak, from.Add(time.Hour), nil
}

func TestUpdatePeak(t *testing.T) {
	ts := time.Date(2026, 10, 12, 18, 10, 0, 0, time.Local)
	slot := ts.Truncate(15 * time.Minute)

	tc := []struct {
		desc            string
		monthPeak       float64 // kWh per quarter hour
		energy          float64 // kWh in current quarter hour
		since           time.Time
		gridPower       float64
		budget          float64
		monthPeakResult float64
	}{
		{"idle", 0, 0, slot, 0, 4e3, 0},
		{"household used most of the budget", 0, 0.75, slot, 3e3, 0, 0},
		{"budget used up", 0, 1, slot, 2e3, -2e3, 0},
		{"follows higher month peak", 1.5, 0, slot, 2e3, 4e3, 6e3},
		{"unknown energy assumes target", 0, 0, time.Time{}, 0, 4e3, 0},
		{"mid quarter hour start is extrapolated", 0, 0.375, slot.Add(5 * time.Minute), 0, 3e3, 0},
	}

	for _, tc := range tc {
		t.Run(tc.desc, func(t *testing.T) {
			site := &Site{log: util.NewLogger("foo")}
			require.NoError(t, site.SetPeakLimit(4e3))
			require.NoError(t, site.SetPeakFollowing(true))

			col := &fakeSlotCollector{energy: tc.energy, since: tc.since, monthPeak: tc.monthPeak}
			site.updatePeakFrom(col, ts, tc.gridPower, 500)

			assert.Equal(t, time.Date(2026, 10, 1, 0, 0, 0, 0, time.Local), col.from, "month start")
			assert.Equal(t, tc.monthPeakResult, site.peak.monthPeak)

			budget, active := site.peakBudget()
			assert.True(t, active)
			assert.InDelta(t, tc.budget, budget, 1e-6)
		})
	}
}

func TestUpdatePeakInactive(t *testing.T) {
	site := &Site{log: util.NewLogger("foo")}
	require.NoError(t, site.SetPeakLimit(4e3))

	site.updatePeakFrom(new(fakeSlotCollector), time.Now(), 0, 0)
	_, active := site.peakBudget()
	assert.False(t, active, "following disabled")
	assert.Nil(t, site.peakPowerLimit())

	require.NoError(t, site.SetPeakFollowing(true))
	site.updatePeakFrom(nil, time.Now(), 0, 0)
	_, active = site.peakBudget()
	assert.False(t, active, "no grid meter")
}

func TestPeakPowerLimit(t *testing.T) {
	slot := time.Date(2026, 10, 31, 23, 0, 0, 0, time.Local)

	site := &Site{log: util.NewLogger("foo")}
	site.peak = peakState{
		following: true,
		limit:     4e3,
		active:    true,
		slot:      slot,
		monthPeak: 5e3,
		home:      []float64{1e3, 3e3, 500, 6e3},
	}

	limit := site.peakPowerLimit()
	require.NotNil(t, limit)

	q := func(i int) time.Time { return slot.Add(time.Duration(i) * 15 * time.Minute) }

	assert.Equal(t, 4e3, limit(q(0), q(1)), "month peak above limit is followed")
	assert.Equal(t, 2e3, limit(q(1), q(2)))
	assert.Equal(t, 2e3, limit(q(0), q(3)), "window takes the tightest quarter hour")
	assert.Equal(t, 0.0, limit(q(3), q(4)), "household above target")
	assert.Equal(t, 3.5e3, limit(q(6), q(7)), "next month only knows the limit, profile repeats")
}
