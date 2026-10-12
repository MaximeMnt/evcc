package metrics

import (
	"time"

	"github.com/evcc-io/evcc/db"
	"github.com/evcc-io/evcc/tariff"
)

// MaxSlotEnergy returns the highest persisted slot energy in kWh since from and the slot's start.
// Recovered slots are skipped since they hold the energy of a downtime.
func (c *Collector) MaxSlotEnergy(from time.Time) (float64, time.Time, error) {
	var m meter
	if err := db.Instance.Where("meter = ? AND ts >= ? AND COALESCE(recovered, 0) = 0", c.entity.Id, from.Unix()).
		Order("energy DESC").Limit(1).Find(&m).Error; err != nil {
		return 0, time.Time{}, err
	}

	if m.Timestamp == 0 {
		return 0, time.Time{}, nil
	}

	return m.Energy, time.Unix(m.Timestamp, 0), nil
}

// SlotEnergy returns the energy in kWh accumulated in the current slot and the time it is accumulated since.
// The time is zero if the energy is unknown, e.g. it contains the energy of a downtime.
func (c *Collector) SlotEnergy() (float64, time.Time) {
	slotStart := c.accu.clock.Now().Truncate(tariff.SlotDuration)

	if c.started.Before(slotStart) {
		return 0, time.Time{}
	}

	if c.restored && !c.started.Equal(c.lastSlot.Add(tariff.SlotDuration)) {
		return 0, time.Time{}
	}

	return c.accu.Energy, c.started
}
