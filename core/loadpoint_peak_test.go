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
	budget float64
	active bool
	limit  planner.PowerLimit
}

func (p fakePeak) peakBudget() (float64, bool) {
	return p.budget, p.active
}

func (p fakePeak) peakPowerLimit() planner.PowerLimit {
	return p.limit
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
