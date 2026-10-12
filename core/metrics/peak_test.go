package metrics

import (
	"testing"
	"time"

	"github.com/benbjohnson/clock"
	"github.com/evcc-io/evcc/db"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCollectorSlotEnergy(t *testing.T) {
	clock := clock.NewMock()

	require.NoError(t, db.NewInstance("sqlite", ":memory:"))
	require.NoError(t, SetupSchema())

	col, err := NewCollector("grid", "grid", "", WithClock(clock))
	require.NoError(t, err)

	// nothing collected yet
	_, since := col.SlotEnergy()
	assert.True(t, since.IsZero())

	// mid-slot start: energy since start
	clock.Add(5 * time.Minute)
	require.NoError(t, col.AddEnergy(nil, nil, 6e3))
	clock.Add(5 * time.Minute)
	require.NoError(t, col.AddEnergy(nil, nil, 6e3))

	energy, since := col.SlotEnergy()
	assert.InDelta(t, 0.5, energy, 1e-9)
	assert.Equal(t, clock.Now().Add(-5*time.Minute), since)

	// next slot: energy since slot start
	clock.Add(10 * time.Minute)
	require.NoError(t, col.AddEnergy(nil, nil, 6e3))

	energy, since = col.SlotEnergy()
	assert.Equal(t, 0.0, energy)
	assert.Equal(t, clock.Now().Truncate(15*time.Minute), since)

	// no sample in the current slot yet
	clock.Add(15 * time.Minute)
	_, since = col.SlotEnergy()
	assert.True(t, since.IsZero())
}

func TestCollectorMaxSlotEnergy(t *testing.T) {
	clock := clock.NewMock()

	require.NoError(t, db.NewInstance("sqlite", ":memory:"))
	require.NoError(t, SetupSchema())

	col, err := NewCollector("grid", "grid", "", WithClock(clock))
	require.NoError(t, err)

	energy, ts, err := col.MaxSlotEnergy(clock.Now())
	require.NoError(t, err)
	assert.Equal(t, 0.0, energy)
	assert.True(t, ts.IsZero())

	start := clock.Now()
	require.NoError(t, persist(col.entity, start, 1, 0, nil, false))
	require.NoError(t, persist(col.entity, start.Add(15*time.Minute), 2, 0, nil, false))
	require.NoError(t, persist(col.entity, start.Add(30*time.Minute), 5, 0, nil, true)) // recovered downtime
	require.NoError(t, persist(col.entity, start.Add(45*time.Minute), 1.5, 0, nil, false))

	energy, ts, err = col.MaxSlotEnergy(start)
	require.NoError(t, err)
	assert.Equal(t, 2.0, energy)
	assert.Equal(t, start.Add(15*time.Minute).Unix(), ts.Unix())

	// slots before from are ignored
	energy, _, err = col.MaxSlotEnergy(start.Add(30 * time.Minute))
	require.NoError(t, err)
	assert.Equal(t, 1.5, energy)
}
