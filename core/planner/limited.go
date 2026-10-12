package planner

import (
	"slices"
	"time"

	"github.com/evcc-io/evcc/api"
	"github.com/evcc-io/evcc/tariff"
)

// PowerLimit returns the power in W available for charging during [start, end).
// Zero means charging in that window would exceed the limit.
type PowerLimit func(start, end time.Time) float64

// limitedSlot is a rate slot with the share of max power available for charging
type limitedSlot struct {
	api.Rate
	factor float64 // 0..1
}

func (s limitedSlot) capacity() time.Duration {
	return time.Duration(float64(s.End.Sub(s.Start)) * s.factor)
}

// PlanLimited creates a plan like Plan, but derates every slot to the power the limit leaves for charging.
// Durations are expressed at maxPower. If the limit cannot be respected until the target time, the
// unlimited plan is returned together with false, so the charge goal is not silently missed.
func (t *Planner) PlanLimited(requiredDuration, precondition time.Duration, targetTime time.Time, continuous bool, maxPower float64, limit PowerLimit) (api.Rates, bool) {
	if limit == nil || maxPower <= 0 {
		return t.Plan(requiredDuration, precondition, targetTime, continuous), true
	}
	if t == nil || requiredDuration <= 0 {
		return nil, true
	}

	unlimited := func() (api.Rates, bool) {
		t.log.DEBUG.Printf("planner: power limit cannot be respected until %v- falling back to unlimited plan", targetTime.Round(time.Second).Local())
		return t.Plan(requiredDuration, precondition, targetTime, continuous), false
	}

	now := t.clock.Now().Truncate(time.Second)
	if !targetTime.After(now) {
		return unlimited()
	}

	var rates api.Rates
	if t.tariff != nil {
		if rr, err := t.tariff.Rates(); err == nil {
			rates = rr
		}
	}

	slots := limitSlots(fillRates(rates, now, targetTime), maxPower, limit)

	// unknown prices after the last rate: defer charging there, as Plan does
	if len(rates) > 0 {
		if last := rates[len(rates)-1].End; targetTime.After(last) {
			var after time.Duration
			for _, s := range slots {
				if !s.Start.Before(last) {
					after += s.capacity()
				}
			}

			if after >= requiredDuration {
				return nil, true
			}

			requiredDuration -= after
			precondition = max(precondition-targetTime.Sub(last), 0)
			targetTime = last
			slots = slices.DeleteFunc(slots, func(s limitedSlot) bool { return !s.Start.Before(last) })
		}
	}

	var available time.Duration
	for _, s := range slots {
		available += s.capacity()
	}
	if available < requiredDuration {
		return unlimited()
	}

	precondition = min(precondition, requiredDuration)

	var precond []limitedSlot
	if precondition > 0 {
		slots, precond = splitLimitedPreconditionSlots(slots, targetTime.Add(-precondition))

		for _, s := range precond {
			requiredDuration -= s.capacity()
		}
		if requiredDuration <= 0 {
			return toRates(precond), true
		}
	}

	var plan api.Rates
	if continuous {
		plan = continuousLimitedWindow(slots, requiredDuration)
		if plan == nil {
			return unlimited()
		}
	} else {
		plan = optimalLimitedPlan(slots, requiredDuration)
		plan.Sort()
	}

	return append(plan, toRates(precond)...), true
}

// fillRates returns quarter-hour slots covering [start, end), with gaps in the rates filled at zero price
func fillRates(rates api.Rates, start, end time.Time) api.Rates {
	var res api.Rates

	for ts := start; ts.Before(end); {
		next := ts.Truncate(tariff.SlotDuration).Add(tariff.SlotDuration)
		if next.After(end) {
			next = end
		}

		r, err := rates.At(ts)
		if err == nil && r.End.Before(next) {
			next = r.End
		}

		res = append(res, api.Rate{Start: ts, End: next, Value: r.Value})
		ts = next
	}

	return res
}

func limitSlots(rates api.Rates, maxPower float64, limit PowerLimit) []limitedSlot {
	res := make([]limitedSlot, 0, len(rates))

	for _, r := range rates {
		power := max(0, min(limit(r.Start, r.End), maxPower))
		res = append(res, limitedSlot{Rate: r, factor: power / maxPower})
	}

	return res
}

func toRates(slots []limitedSlot) api.Rates {
	res := make(api.Rates, 0, len(slots))
	for _, s := range slots {
		res = append(res, s.Rate)
	}
	return res
}

func splitLimitedPreconditionSlots(slots []limitedSlot, preCondStart time.Time) ([]limitedSlot, []limitedSlot) {
	var res, precond []limitedSlot

	for _, s := range slots {
		if !s.End.After(preCondStart) {
			res = append(res, s)
			continue
		}

		if s.Start.Before(preCondStart) {
			head := s
			head.End = preCondStart
			res = append(res, head)
			s.Start = preCondStart
		}

		if s.factor > 0 {
			precond = append(precond, s)
		}
	}

	return res, precond
}

// optimalLimitedPlan picks the cheapest slots until their capacity covers the required duration.
// Prices are per energy, so the greedy choice by price stays optimal for derated slots.
func optimalLimitedPlan(slots []limitedSlot, requiredDuration time.Duration) api.Rates {
	slots = slices.DeleteFunc(slices.Clone(slots), func(s limitedSlot) bool { return s.factor == 0 })
	slices.SortStableFunc(slots, func(i, j limitedSlot) int { return sortByCost(i.Rate, j.Rate) })

	var plan api.Rates
	for _, s := range slots {
		if requiredDuration <= 0 {
			break
		}

		if c := s.capacity(); c > requiredDuration {
			// charge as late as possible within the first slot, as early as possible otherwise
			needed := time.Duration(float64(requiredDuration) / s.factor)
			if IsFirst(s.Rate, plan) && len(plan) > 0 {
				s.Start = s.End.Add(-needed)
			} else {
				s.End = s.Start.Add(needed)
			}
			requiredDuration = 0
		} else {
			requiredDuration -= c
		}

		plan = append(plan, s.Rate)
	}

	return plan
}

// continuousLimitedWindow finds the cheapest gapless window of chargeable slots covering the required duration
func continuousLimitedWindow(slots []limitedSlot, requiredDuration time.Duration) api.Rates {
	var (
		best     api.Rates
		bestCost float64
	)

	for i := range slots {
		var (
			window    api.Rates
			cost      float64
			remaining = requiredDuration
		)

		for j := i; j < len(slots) && remaining > 0; j++ {
			s := slots[j]
			if s.factor == 0 || j > i && !s.Start.Equal(slots[j-1].End) {
				break
			}

			c := s.capacity()
			if c > remaining {
				s.End = s.Start.Add(time.Duration(float64(remaining) / s.factor))
				c = remaining
			}

			remaining -= c
			cost += s.Value * float64(c)
			window = append(window, s.Rate)
		}

		// prefer later start if equal cost
		if remaining <= 0 && (best == nil || cost <= bestCost) {
			best, bestCost = window, cost
		}
	}

	return best
}
