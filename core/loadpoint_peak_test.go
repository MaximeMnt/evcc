package core

import (
	"testing"
	"time"

	"github.com/benbjohnson/clock"
	"github.com/evcc-io/evcc/api"
	"github.com/evcc-io/evcc/core/planner"
	"github.com/evcc-io/evcc/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

type fakePeak struct {
	budget   float64
	active   bool
	limit    planner.PowerLimit
	reserved *api.Rates
}

func (p fakePeak) peakBudget() (float64, bool) {
	return p.budget, p.active
}

func (p fakePeak) peakPowerLimit(_ any) planner.PowerLimit {
	return p.limit
}

func (p fakePeak) peakReserve(_ any, plan api.Rates) {
	if p.reserved != nil {
		*p.reserved = plan
	}
}

func TestPeakCapsCurrent(t *testing.T) {
	Voltage = 230

	tc := []struct {
		desc        string
		peak        peakLimiter
		bypass      bool
		chargePower float64
		expCurrent  int64 // 0 = disabled
	}{
		{"no peak", nil, false, 0, int64(maxA)},
		{"inactive", fakePeak{budget: 0, active: false}, false, 0, int64(maxA)},
		{"budget caps current", fakePeak{budget: 10 * 230, active: true}, false, 0, 10},
		{"budget adds to current charge power", fakePeak{budget: 2 * 230, active: true}, false, 8 * 230, 10},
		{"budget below min current disables", fakePeak{budget: 2 * 230, active: true}, false, 0, 0},
		{"plenty of budget", fakePeak{budget: 20e3, active: true}, false, 0, int64(maxA)},
		{"bypassed", fakePeak{budget: 0, active: true}, true, 0, int64(maxA)},
	}

	for _, tc := range tc {
		t.Run(tc.desc, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			charger := api.NewMockCharger(ctrl)

			lp := NewLoadpoint(util.NewLogger("foo"), nil)
			lp.clock = clock.NewMock()
			lp.wakeUpTimer = NewTimer()
			lp.charger = charger
			lp.minCurrent = minA
			lp.maxCurrent = maxA
			lp.phases = 1
			lp.enabled = true
			lp.chargePower = tc.chargePower
			lp.peak = tc.peak
			lp.peakBypass = tc.bypass

			if tc.expCurrent > 0 {
				charger.EXPECT().MaxCurrent(tc.expCurrent).Return(nil)
			} else {
				charger.EXPECT().Enable(false).Return(nil)
			}

			require.NoError(t, lp.setLimit(maxA))
		})
	}
}

func TestPeakPlanLimitMinPower(t *testing.T) {
	Voltage = 230

	lp := NewLoadpoint(util.NewLogger("foo"), nil)
	lp.minCurrent = minA
	lp.maxCurrent = maxA
	lp.phases = 1

	assert.Nil(t, lp.peakPlanLimit(), "no peak")

	lp.peak = fakePeak{active: true}
	assert.Nil(t, lp.peakPlanLimit(), "peak inactive")

	var power float64
	lp.peak = fakePeak{active: true, limit: func(_, _ time.Time) float64 { return power }}
	limit := lp.peakPlanLimit()
	require.NotNil(t, limit)

	power = 3e3
	assert.Equal(t, 3e3, limit(time.Time{}, time.Time{}))

	power = minA*230 - 1
	assert.Equal(t, 0.0, limit(time.Time{}, time.Time{}), "below min power")
}

func TestReservePeak(t *testing.T) {
	Voltage = 230

	start := time.Date(2026, 10, 12, 22, 0, 0, 0, time.UTC)
	plan := api.Rates{
		{Start: start, End: start.Add(15 * time.Minute), Value: 0.1},
		{Start: start.Add(15 * time.Minute), End: start.Add(30 * time.Minute), Value: 0.2},
	}
	available := func(from, _ time.Time) float64 {
		if from.Equal(start) {
			return 2e3
		}
		return 20e3
	}

	var reserved api.Rates

	lp := NewLoadpoint(util.NewLogger("foo"), nil)
	lp.minCurrent = minA
	lp.maxCurrent = maxA
	lp.phases = 1
	lp.peak = fakePeak{active: true, limit: available, reserved: &reserved}

	maxPower := 230 * maxA

	lp.reservePeak(plan, false, true)
	assert.Equal(t, api.Rates{
		{Start: plan[0].Start, End: plan[0].End, Value: 2e3},
		{Start: plan[1].Start, End: plan[1].End, Value: maxPower},
	}, reserved, "derated plan")

	lp.reservePeak(plan, false, false)
	assert.Equal(t, maxPower, reserved[0].Value, "conflicting plan charges unlimited")

	lp.reservePeak(plan, true, true)
	assert.Nil(t, reserved, "optimizer plan")

	lp.reservePeak(plan, false, true)
	lp.reservePeak(nil, false, true)
	assert.Nil(t, reserved, "no plan")
}
