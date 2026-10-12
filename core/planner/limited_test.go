package planner

import (
	"testing"
	"time"

	"github.com/benbjohnson/clock"
	"github.com/evcc-io/evcc/api"
	"github.com/evcc-io/evcc/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

const maxPower = 10e3

// hourlyLimit returns the available power per hour since start
func hourlyLimit(start time.Time, power ...float64) PowerLimit {
	return func(from, _ time.Time) float64 {
		idx := int(from.Sub(start) / time.Hour)
		if idx < 0 || idx >= len(power) {
			return maxPower
		}
		return power[idx]
	}
}

func limitedPlanner(t *testing.T, clk *clock.Mock, prices ...float64) *Planner {
	ctrl := gomock.NewController(t)
	trf := api.NewMockTariff(ctrl)
	trf.EXPECT().Rates().AnyTimes().Return(rates(prices, clk.Now(), time.Hour), nil)

	return &Planner{
		log:    util.NewLogger("foo"),
		clock:  clk,
		tariff: trf,
	}
}

func TestPlanLimitedWithoutLimit(t *testing.T) {
	clk := clock.NewMock()
	p := limitedPlanner(t, clk, 20, 60, 10, 80)

	plan, ok := p.PlanLimited(time.Hour, 0, clk.Now().Add(4*time.Hour), false, maxPower, nil)
	assert.True(t, ok)
	assert.Equal(t, p.Plan(time.Hour, 0, clk.Now().Add(4*time.Hour), false), plan)
}

func TestPlanLimitedFullPower(t *testing.T) {
	clk := clock.NewMock()
	p := limitedPlanner(t, clk, 20, 60, 10, 80)

	plan, ok := p.PlanLimited(time.Hour, 0, clk.Now().Add(4*time.Hour), false, maxPower, hourlyLimit(clk.Now()))
	assert.True(t, ok)
	assert.Equal(t, clk.Now().Add(2*time.Hour), Start(plan))
	assert.Equal(t, time.Hour, Duration(plan))
	assert.Equal(t, 10.0, AverageCost(plan))
}

func TestPlanLimitedBaseLoad(t *testing.T) {
	clk := clock.NewMock()
	p := limitedPlanner(t, clk, 20, 60, 10, 80)

	// household load leaves half the power in the cheapest hour
	plan, ok := p.PlanLimited(time.Hour, 0, clk.Now().Add(4*time.Hour), false, maxPower, hourlyLimit(clk.Now(), maxPower, maxPower, maxPower/2, maxPower))
	require.True(t, ok)

	// full cheapest hour at half power plus the late half of the next cheapest hour
	assert.Equal(t, 90*time.Minute, Duration(plan))
	assert.Equal(t, clk.Now().Add(30*time.Minute), Start(plan))
	assert.Equal(t, clk.Now().Add(3*time.Hour), End(plan))
}

func TestPlanLimitedSkipsBlockedSlots(t *testing.T) {
	clk := clock.NewMock()
	p := limitedPlanner(t, clk, 20, 60, 10, 80)

	// cheapest hour has no headroom left
	plan, ok := p.PlanLimited(time.Hour, 0, clk.Now().Add(4*time.Hour), false, maxPower, hourlyLimit(clk.Now(), maxPower, maxPower, 0, maxPower))
	require.True(t, ok)
	assert.Equal(t, clk.Now(), Start(plan))
	assert.Equal(t, time.Hour, Duration(plan))
	assert.Equal(t, 20.0, AverageCost(plan))
}

func TestPlanLimitedInfeasible(t *testing.T) {
	clk := clock.NewMock()
	p := limitedPlanner(t, clk, 20, 60, 10, 80)
	target := clk.Now().Add(4 * time.Hour)

	for _, tc := range []struct {
		desc  string
		limit PowerLimit
	}{
		{"no headroom at all", hourlyLimit(clk.Now(), 0, 0, 0, 0)},
		{"headroom too small for deadline", hourlyLimit(clk.Now(), maxPower/4, maxPower/4, maxPower/4, maxPower/4)},
	} {
		t.Run(tc.desc, func(t *testing.T) {
			plan, ok := p.PlanLimited(3*time.Hour, 0, target, false, maxPower, tc.limit)
			assert.False(t, ok)
			assert.Equal(t, p.Plan(3*time.Hour, 0, target, false), plan, "deadline wins over the limit")
		})
	}
}

func TestPlanLimitedContinuous(t *testing.T) {
	clk := clock.NewMock()
	p := limitedPlanner(t, clk, 20, 60, 10, 80)

	// cheapest hour is blocked, so no window can include it
	plan, ok := p.PlanLimited(time.Hour, 0, clk.Now().Add(4*time.Hour), true, maxPower, hourlyLimit(clk.Now(), maxPower/2, maxPower, 0, maxPower))
	require.True(t, ok)

	assert.Equal(t, clk.Now(), Start(plan))
	assert.Equal(t, clk.Now().Add(90*time.Minute), End(plan))
}

func TestPlanLimitedPrecondition(t *testing.T) {
	clk := clock.NewMock()
	p := limitedPlanner(t, clk, 20, 60, 10, 80)
	target := clk.Now().Add(4 * time.Hour)

	// precondition hour runs at half power and covers half the requirement
	plan, ok := p.PlanLimited(time.Hour, time.Hour, target, false, maxPower, hourlyLimit(clk.Now(), maxPower, maxPower, maxPower, maxPower/2))
	require.True(t, ok)

	assert.Equal(t, target, End(plan))
	assert.Equal(t, 90*time.Minute, Duration(plan))
	assert.Equal(t, clk.Now().Add(150*time.Minute), Start(plan))
}

func TestPlanLimitedWithoutTariff(t *testing.T) {
	clk := clock.NewMock()
	p := &Planner{log: util.NewLogger("foo"), clock: clk}

	// flat prices: charge as late as possible, at half power for double the time
	plan, ok := p.PlanLimited(time.Hour, 0, clk.Now().Add(4*time.Hour), false, maxPower, hourlyLimit(clk.Now(), maxPower/2, maxPower/2, maxPower/2, maxPower/2))
	require.True(t, ok)
	assert.Equal(t, clk.Now().Add(2*time.Hour), Start(plan))
	assert.Equal(t, clk.Now().Add(4*time.Hour), End(plan))
}

func TestPlanLimitedBeyondRates(t *testing.T) {
	clk := clock.NewMock()
	p := limitedPlanner(t, clk, 20, 60)

	// enough capacity after the known rates: wait
	plan, ok := p.PlanLimited(time.Hour, 0, clk.Now().Add(6*time.Hour), false, maxPower, hourlyLimit(clk.Now()))
	assert.True(t, ok)
	assert.Nil(t, plan)

	// limited capacity after the known rates: use the cheapest known slot for the remainder
	plan, ok = p.PlanLimited(time.Hour, 0, clk.Now().Add(3*time.Hour), false, maxPower, hourlyLimit(clk.Now(), maxPower, maxPower, maxPower/2))
	assert.True(t, ok)
	assert.Equal(t, 30*time.Minute, Duration(plan))
	assert.Equal(t, 20.0, AverageCost(plan))
}
