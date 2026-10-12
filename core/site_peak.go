package core

import (
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/evcc-io/evcc/core/keys"
	"github.com/evcc-io/evcc/core/metrics"
	"github.com/evcc-io/evcc/core/peak"
	"github.com/evcc-io/evcc/core/planner"
	"github.com/evcc-io/evcc/db/settings"
	"github.com/jinzhu/now"
)

// peakLimiter limits the loadpoints' grid import to follow the month's demand peak
type peakLimiter interface {
	// peakBudget returns the import power in W the loadpoints may add right now
	peakBudget() (float64, bool)
	// peakPowerLimit returns the import power in W left for charging per planning window
	peakPowerLimit() planner.PowerLimit
}

var _ peakLimiter = (*Site)(nil)

// peakStatus is the published demand peak state. Powers are in W.
type peakStatus struct {
	Limit         float64    `json:"limit"`                   // configured limit
	Target        float64    `json:"target"`                  // effective target, the limit or the higher month peak
	MonthPeak     float64    `json:"monthPeak"`               // highest completed quarter-hour average this month
	MonthPeakTime *time.Time `json:"monthPeakTime,omitempty"` // start of the month peak's quarter hour
	Projected     float64    `json:"projected"`               // current quarter-hour average if grid power stays as is
	Exceeded      bool       `json:"exceeded"`                // month peak above the limit
}

// peakState is the site's demand peak state, guarded by the site mutex
type peakState struct {
	following bool    // enabled by the user
	limit     float64 // W

	slot          time.Time // quarter hour the cached values below were computed for
	monthPeak     float64   // W
	monthPeakTime time.Time
	home          []float64 // forecast household import per quarter hour from slot in W
	budget        float64   // W the loadpoints may add right now
	active        bool
}

var ErrPeakLimitNotSet = errors.New("peak limit not set")

// GetPeakFollowing returns if peak following is enabled
func (site *Site) GetPeakFollowing() bool {
	site.RLock()
	defer site.RUnlock()
	return site.peak.following
}

// SetPeakFollowing enables or disables peak following
func (site *Site) SetPeakFollowing(enable bool) error {
	site.Lock()
	if enable && site.peak.limit <= 0 {
		site.Unlock()
		return ErrPeakLimitNotSet
	}

	changed := site.peak.following != enable
	site.peak.following = enable
	if !enable {
		site.peak.active = false
	}
	site.Unlock()

	if changed {
		site.log.DEBUG.Println("set peak following:", enable)
		settings.SetBool(keys.PeakFollowing, enable)
		site.publish(keys.PeakFollowing, enable)
		if !enable {
			site.publish(keys.Peak, nil)
		}
	}

	return nil
}

// GetPeakLimit returns the peak limit in W
func (site *Site) GetPeakLimit() float64 {
	site.RLock()
	defer site.RUnlock()
	return site.peak.limit
}

// SetPeakLimit sets the peak limit in W
func (site *Site) SetPeakLimit(power float64) error {
	if math.IsNaN(power) || math.IsInf(power, 0) || power < 0 {
		return fmt.Errorf("invalid peak limit: %g", power)
	}

	site.Lock()
	if power == 0 && site.peak.following {
		site.Unlock()
		return ErrPeakLimitNotSet
	}

	changed := site.peak.limit != power
	site.peak.limit = power
	site.Unlock()

	if changed {
		site.log.DEBUG.Println("set peak limit:", power)
		settings.SetFloat(keys.PeakLimit, power)
		site.publish(keys.PeakLimit, power)
	}

	return nil
}

// slotEnergyCollector provides the grid meter's quarter-hour import energy
type slotEnergyCollector interface {
	SlotEnergy() (float64, time.Time)
	MaxSlotEnergy(time.Time) (float64, time.Time, error)
}

// updatePeak updates the demand peak state from the grid meter's quarter-hour energy
func (site *Site) updatePeak(gridPower, homePower float64) {
	var col slotEnergyCollector
	if site.gridMeter != nil {
		if c, ok := site.collectors[site.gridMeter.Config().Name]; ok {
			col = c
		}
	}

	site.updatePeakFrom(col, time.Now(), gridPower, homePower)
}

func (site *Site) updatePeakFrom(col slotEnergyCollector, ts time.Time, gridPower, homePower float64) {
	site.RLock()
	following, limit := site.peak.following, site.peak.limit
	site.RUnlock()

	if !following || limit <= 0 || col == nil {
		site.Lock()
		site.peak.active = false
		site.Unlock()
		return
	}

	slot := ts.Truncate(peak.Interval)

	site.RLock()
	cached := slot.Equal(site.peak.slot)
	monthPeak, monthPeakTime, home := site.peak.monthPeak, site.peak.monthPeakTime, site.peak.home
	site.RUnlock()

	if !cached {
		energy, at, err := col.MaxSlotEnergy(now.With(ts).BeginningOfMonth())
		if err != nil {
			site.log.ERROR.Printf("peak: month peak: %v", err)
		}
		monthPeak, monthPeakTime = energy*1e3/peak.Interval.Hours(), at

		home = site.peakHomeForecast(homePower)
	}

	target := peak.Target(limit, monthPeak)

	kwh, since := col.SlotEnergy()
	energy := peak.IntervalEnergy(kwh*1e3, since, ts, target)
	headroom := peak.Headroom(target, energy, ts)
	projected := peak.Projected(energy, gridPower, ts)

	site.Lock()
	site.peak.slot = slot
	site.peak.monthPeak, site.peak.monthPeakTime = monthPeak, monthPeakTime
	site.peak.home = home
	site.peak.budget = headroom - gridPower
	site.peak.active = true
	site.Unlock()

	site.log.DEBUG.Printf("peak: target %.0fW (month peak %.0fW), quarter hour projected %.0fW, headroom %.0fW", target, monthPeak, projected, headroom)

	status := peakStatus{
		Limit:     limit,
		Target:    target,
		MonthPeak: monthPeak,
		Projected: projected,
		Exceeded:  monthPeak > limit,
	}
	if !monthPeakTime.IsZero() {
		status.MonthPeakTime = &monthPeakTime
	}

	site.publish(keys.Peak, status)
}

// peakHomeForecast returns the forecast household import per quarter hour of the next day in W,
// falling back to the current household power without a history profile
func (site *Site) peakHomeForecast(homePower float64) []float64 {
	const slots = 24 * time.Hour / peak.Interval

	var profile []float64
	err := errors.New("no household metrics")
	if site.collectors[metrics.Home] != nil {
		profile, err = site.homeProfile(int(slots))
	}
	if err != nil {
		site.log.DEBUG.Printf("peak: no household profile, assuming %.0fW: %v", homePower, err)
		profile = make([]float64, slots)
		for i := range profile {
			profile[i] = homePower * peak.Interval.Hours()
		}
	}

	// Wh to W
	res := make([]float64, len(profile))
	for i, v := range profile {
		res[i] = v / peak.Interval.Hours()
	}

	return res
}

// peakBudget implements peakLimiter
func (site *Site) peakBudget() (float64, bool) {
	site.RLock()
	defer site.RUnlock()
	return site.peak.budget, site.peak.active
}

// peakPowerLimit implements peakLimiter. Household import is taken from the forecast profile,
// planned charging of other loadpoints is not accounted for.
func (site *Site) peakPowerLimit() planner.PowerLimit {
	site.RLock()
	defer site.RUnlock()

	if !site.peak.active || len(site.peak.home) == 0 {
		return nil
	}

	limit, monthPeak, slot := site.peak.limit, site.peak.monthPeak, site.peak.slot
	home := site.peak.home
	monthEnd := now.With(slot).EndOfMonth()

	return func(start, end time.Time) float64 {
		res := math.MaxFloat64

		for ts := start.Truncate(peak.Interval); ts.Before(end); ts = ts.Add(peak.Interval) {
			// the month peak resets with the next month
			target := limit
			if !ts.After(monthEnd) {
				target = peak.Target(limit, monthPeak)
			}

			idx := int(ts.Sub(slot) / peak.Interval)
			res = min(res, target-home[(idx%len(home)+len(home))%len(home)])
		}

		return max(0, res)
	}
}
