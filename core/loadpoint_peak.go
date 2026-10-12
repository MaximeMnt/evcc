package core

import (
	"time"

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

	limit := lp.peak.peakPowerLimit()
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
