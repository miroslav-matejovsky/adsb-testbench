package simulation

import (
	"testing"
	"time"

	"github.com/miroslav-matejovsky/adsb-testbench/internal/adsb"
	"github.com/stretchr/testify/require"
)

// receptionsFor returns the receptions of one station in batch order.
func receptionsFor(batch Batch, id string) []Reception {
	out := make([]Reception, 0, len(batch.Receptions))
	for _, got := range batch.Receptions {
		if got.StationID == id {
			out = append(out, got)
		}
	}
	return out
}

// An engine without stations behaves exactly as before and still allocates an
// empty reception slice.
func TestReceptionBatchWithoutStations(t *testing.T) {
	t.Parallel()

	withStation := newEngine(t, validConfig())
	addStation(t, withStation, validStationConfig())
	withStationBatch, err := withStation.Advance(t.Context(), 5*time.Second)
	require.NoError(t, err)

	bare := newEngine(t, validConfig())
	bareBatch, err := bare.Advance(t.Context(), 5*time.Second)
	require.NoError(t, err)

	require.NotNil(t, bareBatch.Receptions)
	require.Empty(t, bareBatch.Receptions)
	require.NotEmpty(t, withStationBatch.Receptions)
	require.Equal(t, bareBatch.Transmissions, withStationBatch.Transmissions)
}

// Every reception carries the transmission it came from, in batch order.
func TestReceptionBatchFanOutAndOrder(t *testing.T) {
	t.Parallel()

	engine := newEngine(t, validConfig())
	addStation(t, engine, namedStation("alpha"))
	addStation(t, engine, namedStation("bravo"))

	batch, err := engine.Advance(t.Context(), 4*time.Second)
	require.NoError(t, err)
	require.NotEmpty(t, batch.Transmissions)

	bySequence := make(map[uint64]Transmission, len(batch.Transmissions))
	for _, t := range batch.Transmissions {
		bySequence[t.Sequence] = t
	}

	order := []string{"alpha", "bravo"}
	seen := make(map[uint64]int)
	var previousSequence uint64
	position := 0
	for _, got := range batch.Receptions {
		source, ok := bySequence[got.TransmissionSequence]
		require.True(t, ok, "reception references a transmission of this batch")
		require.Equal(t, source.ICAO, got.ICAO)
		require.Equal(t, source.Kind, got.Kind)
		require.Equal(t, source.Frame, got.Frame)
		require.True(t, got.Timestamp.Equal(source.Timestamp), "no propagation delay is modelled")
		require.Equal(t, uint64(1), got.StationRevision)
		require.Greater(t, got.SlantRangeNauticalMiles, 0.0)
		require.Less(t, got.ReceivedPowerDBm, 0.0)

		require.GreaterOrEqual(t, got.TransmissionSequence, previousSequence)
		if got.TransmissionSequence != previousSequence {
			previousSequence, position = got.TransmissionSequence, 0
		}
		require.Equal(t, order[position], got.StationID, "stations are scanned in creation order")
		position++

		seen[got.TransmissionSequence]++
		require.LessOrEqual(t, seen[got.TransmissionSequence], MaxStations)
	}

	// Both stations use identical settings, so both hear every transmission.
	require.Len(t, batch.Receptions, 2*len(batch.Transmissions))

	// A received frame still decodes to the emitting aircraft.
	message, err := adsb.Decode(batch.Receptions[0].Frame[:])
	require.NoError(t, err)
	require.Equal(t, batch.Receptions[0].ICAO, message.Header.ICAO)
}

// A disabled station evaluates nothing and consumes no random draws, so its
// stream resumes exactly where it stopped.
func TestReceptionBatchDisabledStation(t *testing.T) {
	t.Parallel()

	lossy := validStationConfig()
	lossy.FrameLossProbability = 0.5

	engine := newEngine(t, validConfig())
	created := addStation(t, engine, lossy)

	first, err := engine.Advance(t.Context(), 2*time.Second)
	require.NoError(t, err)
	require.NotEmpty(t, first.Receptions)
	require.Less(t, len(first.Receptions), len(first.Transmissions), "some frames are dropped")
	stopped := engine.state.stations.active[0].rng

	disabled := lossy
	disabled.Enabled = false
	updated, err := engine.UpdateStation(t.Context(), created.Revision, disabled)
	require.NoError(t, err)
	require.Equal(t, stopped, engine.state.stations.active[0].rng, "an update leaves the stream alone")

	quiet, err := engine.Advance(t.Context(), 2*time.Second)
	require.NoError(t, err)
	require.NotEmpty(t, quiet.Transmissions)
	require.Empty(t, quiet.Receptions, "a disabled station hears nothing")
	require.Equal(t, stopped, engine.state.stations.active[0].rng, "a disabled station draws nothing")

	_, err = engine.UpdateStation(t.Context(), updated.Revision, lossy)
	require.NoError(t, err)
	require.Equal(t, stopped, engine.state.stations.active[0].rng)

	resumed, err := engine.Advance(t.Context(), 2*time.Second)
	require.NoError(t, err)
	require.NotEmpty(t, resumed.Receptions)
	require.NotEqual(t, stopped, engine.state.stations.active[0].rng, "the stream advanced again")
}

// frameSet reduces receptions to the decision each transmission got.
func frameSet(receptions []Reception) []uint64 {
	out := make([]uint64, 0, len(receptions))
	for _, got := range receptions {
		out = append(out, got.TransmissionSequence)
	}
	return out
}

func TestReceptionBatchFrameLossExtremes(t *testing.T) {
	t.Parallel()

	never := validStationConfig()
	never.FrameLossProbability = 0
	always := validStationConfig()
	always.FrameLossProbability = 1

	for _, test := range []struct {
		name    string
		cfg     StationConfig
		wantAll bool
	}{
		{"probability zero", never, true},
		{"probability one", always, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			engine := newEngine(t, validConfig())
			addStation(t, engine, test.cfg)
			batch, err := engine.Advance(t.Context(), 5*time.Second)
			require.NoError(t, err)
			require.NotEmpty(t, batch.Transmissions)

			if test.wantAll {
				require.Len(t, batch.Receptions, len(batch.Transmissions))
			} else {
				require.Empty(t, batch.Receptions)
			}

			// Transmissions are identical either way.
			control := newEngine(t, validConfig())
			controlBatch, err := control.Advance(t.Context(), 5*time.Second)
			require.NoError(t, err)
			require.Equal(t, controlBatch.Transmissions, batch.Transmissions)
		})
	}
}

// Editing only the probability must not shift the stream, so setting it to
// another value and back reproduces the original receptions.
func TestReceptionBatchProbabilityDoesNotShiftStream(t *testing.T) {
	t.Parallel()

	lossy := validStationConfig()
	lossy.FrameLossProbability = 0.5

	control := newEngine(t, validConfig())
	addStation(t, control, lossy)
	wanted, err := control.Advance(t.Context(), 5*time.Second)
	require.NoError(t, err)

	subject := newEngine(t, validConfig())
	created := addStation(t, subject, lossy)

	other := lossy
	other.FrameLossProbability = 0.9
	updated, err := subject.UpdateStation(t.Context(), created.Revision, other)
	require.NoError(t, err)
	restored, err := subject.UpdateStation(t.Context(), updated.Revision, lossy)
	require.NoError(t, err)
	require.Equal(t, uint64(3), restored.Revision)

	got, err := subject.Advance(t.Context(), 5*time.Second)
	require.NoError(t, err)
	require.Equal(t, frameSet(wanted.Receptions), frameSet(got.Receptions))
}

// A station created between two calls never hears an earlier transmission.
func TestReceptionBatchStationHearsOnlyLaterTransmissions(t *testing.T) {
	t.Parallel()

	engine := newEngine(t, validConfig())
	early, err := engine.Advance(t.Context(), 3*time.Second)
	require.NoError(t, err)
	require.NotEmpty(t, early.Transmissions)
	require.Empty(t, early.Receptions)

	created := addStation(t, engine, validStationConfig())
	later, err := engine.Advance(t.Context(), 3*time.Second)
	require.NoError(t, err)
	require.NotEmpty(t, later.Receptions)

	earliest := later.Receptions[0].Timestamp
	require.False(t, earliest.Before(created.CreatedAt))
	for _, got := range later.Receptions {
		require.False(t, got.Timestamp.Before(created.CreatedAt))
	}

	// New retains creation reports but no station existed to hear them.
	require.Empty(t, receptionsFor(early, created.Config.ID))
}

// A station created before a count change hears the new creation reports.
func TestReceptionBatchCoversCreationReports(t *testing.T) {
	t.Parallel()

	cfg := validConfig()
	cfg.InitialAircraftCount = 1
	engine := newEngine(t, cfg)
	addStation(t, engine, validStationConfig())

	batch, err := engine.SetCount(t.Context(), 3)
	require.NoError(t, err)
	require.Len(t, batch.Transmissions, 6)
	require.Len(t, batch.Receptions, 6)
	for i, got := range batch.Receptions {
		require.Equal(t, batch.Transmissions[i].Sequence, got.TransmissionSequence)
	}
}

// Editing a returned batch cannot reach engine state or later output.
func TestReceptionOwnership(t *testing.T) {
	t.Parallel()

	subject := newEngine(t, validConfig())
	addStation(t, subject, validStationConfig())
	batch, err := subject.Advance(t.Context(), 4*time.Second)
	require.NoError(t, err)
	require.NotEmpty(t, batch.Receptions)

	before := subject.Snapshot()
	batch.Receptions[0].Frame[2] = 0xff
	batch.Receptions[0].StationID = "tampered"
	batch.Receptions[0].StationRevision = 99
	batch.Transmissions[0].Frame[1] = 0xff
	require.Equal(t, before, subject.Snapshot())

	control := newEngine(t, validConfig())
	addStation(t, control, validStationConfig())
	_, err = control.Advance(t.Context(), 4*time.Second)
	require.NoError(t, err)

	tampered, err := subject.Advance(t.Context(), 4*time.Second)
	require.NoError(t, err)
	clean, err := control.Advance(t.Context(), 4*time.Second)
	require.NoError(t, err)
	require.Equal(t, clean, tampered)
}

// The reception bound is a guard that the frame bound always reaches first.
func TestReceptionBatchBound(t *testing.T) {
	t.Parallel()

	require.Equal(t, MaxBatchFrames*MaxStations, MaxBatchReceptions)

	engine := newEngine(t, validConfig())
	addStation(t, engine, validStationConfig())
	batch := newBatch()
	for range MaxBatchReceptions {
		batch.Receptions = append(batch.Receptions, Reception{})
	}

	craft := &engine.state.fleet[0]
	err := engine.state.deliver(Transmission{Sequence: 1, ICAO: craft.icao}, craft.navAt(0), &batch)
	require.ErrorIs(t, err, ErrLimit)
	require.ErrorContains(t, err, "primary")
}
