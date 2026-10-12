package core

import (
	"time"

	"github.com/evcc-io/evcc/api"
	"github.com/evcc-io/evcc/core/planner"
)

// peakPowerCap returns the charge power in W the demand peak allows, false if the peak does not apply
func (lp *Loadpoint) peakPowerCap() (float64, bool) {
	if lp.peak == nil || lp.peakBypass {
		return 0, false
	}

	budget, ok := lp.peak.peakBudget()
	if !ok {
		return 0, false
	}

	return max(0, lp.chargePower+budget), true
}

// peakPlanLimit returns the power available for planned charging below the demand peak, nil if not following the peak
func (lp *Loadpoint) peakPlanLimit() planner.PowerLimit {
	if lp.peak == nil {
		return nil
	}

	limit := lp.peak.peakPowerLimit(lp)
	if limit == nil {
		return nil
	}

	minPower := lp.EffectiveMinPower()

	return func(start, end time.Time) float64 {
		if power := limit(start, end); power >= minPower {
			return power
		}
		return 0
	}
}

// reservePeak shares the power the plan is expected to draw with the other loadpoints' peak planning.
// Optimizer plans are not shared, the optimizer accounts for all loadpoints itself.
func (lp *Loadpoint) reservePeak(plan api.Rates, optimized, peakOk bool) {
	if lp.peak == nil {
		return
	}

	limit := lp.peakPlanLimit()
	if optimized || limit == nil || len(plan) == 0 {
		lp.peak.peakReserve(lp, nil)
		return
	}

	maxPower := lp.EffectiveMaxPower()

	res := make(api.Rates, 0, len(plan))
	for _, slot := range plan {
		// a conflicting plan charges unlimited
		power := maxPower
		if peakOk {
			power = min(maxPower, limit(slot.Start, slot.End))
		}

		res = append(res, api.Rate{Start: slot.Start, End: slot.End, Value: power})
	}

	lp.peak.peakReserve(lp, res)
}
