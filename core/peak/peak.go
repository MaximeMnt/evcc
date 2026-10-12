// Package peak implements demand peak following for capacity tariffs like the Belgian one,
// which bill the highest quarter-hour average grid import of each month.
package peak

import (
	"time"

	"github.com/evcc-io/evcc/tariff"
)

// Interval is the metering interval. Intervals are aligned to the clock quarter hours.
const Interval = tariff.SlotDuration

// Target returns the average import power in W the current interval may reach without raising the bill:
// the configured limit, or the month's peak once it exceeds the limit.
func Target(limit, monthPeak float64) float64 {
	return max(limit, monthPeak)
}

// IntervalEnergy estimates the import energy in Wh of the interval up to now from the energy observed since.
// The unobserved part of the interval is extrapolated from the observed average. Unknown energy is
// assumed to have been drawn at target power, which leaves no headroom beyond the target.
func IntervalEnergy(energy float64, since, now time.Time, target float64) float64 {
	start := now.Truncate(Interval)
	elapsed := now.Sub(start)

	observed := now.Sub(since)
	if since.IsZero() || since.Before(start) || observed <= 0 {
		return target * elapsed.Hours()
	}

	return energy * float64(elapsed) / float64(observed)
}

// Headroom returns the average import power in W allowed for the rest of the interval
// to keep the interval average at or below target. It never exceeds the target, since
// power above it would spill into the next interval due to the control loop's latency.
func Headroom(target, energy float64, now time.Time) float64 {
	remaining := now.Truncate(Interval).Add(Interval).Sub(now)
	budget := target*Interval.Hours() - energy

	return max(0, min(target, budget/remaining.Hours()))
}

// Projected returns the interval's average import power in W if the given power is drawn for the rest of the interval
func Projected(energy, power float64, now time.Time) float64 {
	remaining := now.Truncate(Interval).Add(Interval).Sub(now)
	return (energy + max(0, power)*remaining.Hours()) / Interval.Hours()
}
