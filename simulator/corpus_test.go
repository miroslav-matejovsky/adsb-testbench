package simulator

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/miroslav-matejovsky/adsb-testbench/display"
	"github.com/miroslav-matejovsky/adsb-testbench/internal/qualitytest"
	"github.com/miroslav-matejovsky/adsb-testbench/simulation"
	"github.com/miroslav-matejovsky/adsb-testbench/simulatorapi"
)

// Shared engine scenario corpus through the real runtime.
//
// Every engine scenario of testdata/quality/scenarios.json runs on a paused
// runtime with a fake clock and is read through the local service and the
// mounted HTTP handler. Virtual advances are applied as real elapsed time at
// the current speed and settled through an ordinary control, so the driver
// performs the same work it does when paced by its ticker.

const qualityScenarios = "../testdata/quality/scenarios.json"

// corpusBench is one runtime executing one scenario run.
type corpusBench struct {
	*acceptanceBench
	count int
	speed uint16
}

// newCorpusBench builds a runtime from scenario inputs, adds its stations,
// and applies the operations of the named run.
func newCorpusBench(t *testing.T, scenario qualitytest.Scenario, name string) *corpusBench {
	t.Helper()

	run, err := scenario.Run(name)
	require.NoError(t, err)
	engine := scenario.Engine
	r := func(value qualitytest.Range) simulation.Range {
		return simulation.Range{Min: value.Min, Max: value.Max}
	}
	config := Config{Simulation: simulation.Config{
		ID: run.RunID, StartTime: engine.StartTime, Seed: engine.Seed,
		InitialAircraftCount: engine.InitialAircraftCount, SpeedHundredths: engine.SpeedHundredths,
		Spawn: simulation.SpawnConfig{
			LatitudeDegrees: r(engine.Spawn.LatitudeDegrees), LongitudeDegrees: r(engine.Spawn.LongitudeDegrees),
			AltitudeFeet: r(engine.Spawn.AltitudeFeet), GroundSpeedKnots: r(engine.Spawn.GroundSpeedKnots),
			TrackDegrees: r(engine.Spawn.TrackDegrees), VerticalRateFeetPerMinute: r(engine.Spawn.VerticalRateFeetPerMinute),
		},
	}}
	bench := &corpusBench{
		acceptanceBench: newAcceptanceBenchFor(t, config),
		count:           engine.InitialAircraftCount, speed: engine.SpeedHundredths,
	}
	for _, station := range scenario.Stations {
		bench.apply(t, qualitytest.Operation{Op: qualitytest.OpAddStation, Station: station})
	}
	for _, op := range run.Operations {
		bench.apply(t, op)
	}
	return bench
}

// corpusSettings converts a scenario station to wire settings.
func corpusSettings(station qualitytest.Station) simulatorapi.StationSettings {
	return simulatorapi.StationSettings{
		ID: station.ID, Enabled: station.Enabled,
		LatitudeDegrees: station.LatitudeDegrees, LongitudeDegrees: station.LongitudeDegrees,
		SiteElevationMetres: station.SiteElevationMetres, AntennaHeightMetres: station.AntennaHeightMetres,
		AntennaGainDBi: station.AntennaGainDBi, SensitivityDBm: station.SensitivityDBm,
		SystemLossDB: station.SystemLossDB, FrameLossProbability: station.FrameLossProbability,
	}
}

// apply performs one scenario operation through the service.
func (b *corpusBench) apply(t *testing.T, op qualitytest.Operation) {
	t.Helper()

	ctx := t.Context()
	runID := b.api.RunID()
	var err error
	switch op.Op {
	case qualitytest.OpAdvance:
		// The runtime only knows real time. A virtual advance is exact real
		// time at 1x; other speeds would need rounding the scenario never states.
		require.Equal(t, uint16(100), b.speed, "a virtual advance runs through the runtime at speed 100")
		b.clock.Add(op.Duration)
		_, err = b.api.SetCount(ctx, simulatorapi.CountCommand{RunID: runID, Count: b.count})
	case qualitytest.OpElapse:
		b.clock.Add(op.Duration)
		_, err = b.api.SetCount(ctx, simulatorapi.CountCommand{RunID: runID, Count: b.count})
	case qualitytest.OpSetCount:
		b.count = op.Count
		_, err = b.api.SetCount(ctx, simulatorapi.CountCommand{RunID: runID, Count: op.Count})
	case qualitytest.OpSetSpeed:
		b.speed = op.SpeedHundredths
		_, err = b.api.SetSpeed(ctx, simulatorapi.SpeedCommand{RunID: runID, SpeedHundredths: int(op.SpeedHundredths)})
	case qualitytest.OpAddStation:
		_, err = b.api.AddStation(ctx, simulatorapi.AddStationCommand{RunID: runID, Station: corpusSettings(op.Station)})
	case qualitytest.OpUpdateStation:
		_, err = b.api.UpdateStation(ctx, op.Station.ID, simulatorapi.UpdateStationCommand{
			RunID: runID, ExpectedRevision: simulatorapi.FormatUint64(op.ExpectedRevision), Station: corpusSettings(op.Station),
		})
	case qualitytest.OpRemoveStation:
		_, err = b.api.RemoveStation(ctx, op.StationID, simulatorapi.RemoveStationCommand{
			RunID: runID, ExpectedRevision: simulatorapi.FormatUint64(op.ExpectedRevision),
		})
	default:
		t.Fatalf("unsupported corpus operation %q", op.Op)
	}
	require.NoError(t, err, "%s", op.Op)
}

// corpusExpiry converts the scenario lifetimes to wire strings.
func corpusExpiry(expiry qualitytest.Expiry) simulatorapi.ObservationExpiry {
	return simulatorapi.ObservationExpiry{
		IdentityNanoseconds: simulatorapi.FormatDuration(expiry.Identity),
		PositionNanoseconds: simulatorapi.FormatDuration(expiry.Position),
		AltitudeNanoseconds: simulatorapi.FormatDuration(expiry.Altitude),
		VelocityNanoseconds: simulatorapi.FormatDuration(expiry.Velocity),
	}
}

// selectable returns the scenario selection restricted to active stations,
// since a run may have removed a selected station.
func (b *corpusBench) selectable(t *testing.T, selection []string) []string {
	t.Helper()

	stations, err := b.api.Stations(t.Context())
	require.NoError(t, err)
	active := map[string]bool{}
	for _, entry := range stations.Stations {
		active[entry.Station.ID] = true
	}
	result := []string{}
	for _, id := range selection {
		if active[id] {
			result = append(result, id)
		}
	}
	return result
}

// TestCorpusScenariosAgreeAcrossTransports runs every engine scenario and
// requires the local and HTTP sources to carry the same raw evidence, and
// displays over each to publish the same tracks as the engine oracle.
func TestCorpusScenariosAgreeAcrossTransports(t *testing.T) {
	t.Parallel()

	scenarios, err := qualitytest.LoadScenarios(qualityScenarios)
	require.NoError(t, err)
	for _, scenario := range scenarios {
		if scenario.Kind != qualitytest.EngineKind {
			continue
		}
		for _, run := range scenario.Runs {
			t.Run(fmt.Sprintf("%s/%s", scenario.ID, run.Name), func(t *testing.T) {
				t.Parallel()

				bench := newCorpusBench(t, scenario, run.Name)
				selection := bench.selectable(t, scenario.Selection)
				request := simulatorapi.ReceptionSnapshotRequest{StationIDs: selection}
				fromLocal, err := bench.local.ReceptionSnapshot(t.Context(), request)
				require.NoError(t, err)
				fromHTTP, err := bench.remote.ReceptionSnapshot(t.Context(), request)
				require.NoError(t, err)
				require.Equal(t, fromLocal, fromHTTP, "both transports carry the same raw evidence")

				oracle, err := bench.api.Observations(t.Context(), simulatorapi.ObservationRequest{
					StationIDs: selection, Expiry: corpusExpiry(scenario.Expiry),
				})
				require.NoError(t, err)
				for name, source := range map[string]display.ObservationSource{"local": bench.local, "http": bench.remote} {
					backend := corpusDisplay(t, source, scenario.Expiry)
					snapshot, err := backend.Refresh(t.Context(), selection)
					require.NoError(t, err, name)
					require.Equal(t, display.StatusFresh, snapshot.Status, name)
					require.Equal(t, oracle.Aircraft, snapshot.Observations.Aircraft, name)
					require.Equal(t, oracle.Retention, snapshot.Observations.Retention, name)
				}
			})
		}
	}
}

// corpusDisplay builds a display with the scenario field lifetimes.
func corpusDisplay(t *testing.T, source display.ObservationSource, expiry qualitytest.Expiry) *display.Display {
	t.Helper()

	backend, err := display.New(display.Config{
		IdentityExpiry: expiry.Identity, PositionExpiry: expiry.Position,
		AltitudeExpiry: expiry.Altitude, VelocityExpiry: expiry.Velocity,
		MaxRequestBytes: 65536, MaxResponseBytes: 16777216,
		RequestTimeout: 10 * time.Second, ReportError: func(err error) { t.Error(err) },
	}, source, noStations{})
	require.NoError(t, err)
	return backend
}

// TestCorpusRestartAcrossTransports covers the restart scenario assertions
// that only exist above the engine: old cursors over HTTP, and a display
// that never falls back to a previous run's evidence.
func TestCorpusRestartAcrossTransports(t *testing.T) {
	t.Parallel()

	scenarios, err := qualitytest.LoadScenarios(qualityScenarios)
	require.NoError(t, err)
	scenario, err := qualitytest.FindScenario(scenarios, "restart")
	require.NoError(t, err)

	if scenario.Asserts("old-cursor-rejected") {
		first := newCorpusBench(t, scenario, "first")
		second := newCorpusBench(t, scenario, "second")
		page, err := first.remote.ReceptionHistory(t.Context(), simulatorapi.HistoryRequest{StationID: "primary", Limit: 1})
		require.NoError(t, err)
		_, err = second.remote.ReceptionHistory(t.Context(), simulatorapi.HistoryRequest{
			StationID: "primary", Cursor: &page.NextCursor, Limit: 1,
		})
		require.ErrorIs(t, err, simulatorapi.CategoryConflict)
	}

	if !scenario.Asserts("no-cross-run-fallback") {
		return
	}
	for _, transport := range []string{"local", "http"} {
		t.Run(transport, func(t *testing.T) {
			t.Parallel()

			first := newCorpusBench(t, scenario, "first")
			second := newCorpusBench(t, scenario, "second")
			pick := func(b *corpusBench) display.ObservationSource {
				if transport == "http" {
					return b.remote
				}
				return b.local
			}
			sources := &switchingSource{current: pick(first)}
			backend := corpusDisplay(t, sources, scenario.Expiry)
			before, err := backend.Refresh(t.Context(), scenario.Selection)
			require.NoError(t, err)
			require.NotEmpty(t, before.Observations.Aircraft)

			// The replacement run fails its read for the same selection. The
			// failure names the new run, so nothing of the old run survives.
			_, err = second.api.RemoveStation(t.Context(), "primary", simulatorapi.RemoveStationCommand{
				RunID: second.api.RunID(), ExpectedRevision: "1",
			})
			require.NoError(t, err)
			sources.set(pick(second))
			after, err := backend.Refresh(t.Context(), scenario.Selection)
			require.ErrorIs(t, err, simulatorapi.CategoryNotFound)
			require.Equal(t, display.StatusUnavailable, after.Status)
			require.Nil(t, after.Observations, "no fallback to the previous run")
			require.Nil(t, backend.Snapshot().Observations)
		})
	}
}
