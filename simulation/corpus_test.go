package simulation

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/miroslav-matejovsky/adsb-testbench/internal/qualitytest"
)

// Shared engine scenario corpus.
//
// The scenarios in testdata/quality/scenarios.json are synthetic. Comparing
// two independently initialized executions is reproducibility evidence for
// this engine, not independent protocol evidence. Assertions owned by other
// layers (display decoding of literal frames, cross-run display fallback)
// are executed there.

const qualityScenarios = "../testdata/quality/scenarios.json"

// engineAssertions lists the corpus assertions this layer executes.
var engineAssertions = map[string]func(*testing.T, qualitytest.Scenario){
	"allocated-empty-arrays":    assertAllocatedEmptyArrays,
	"no-fabricated-tracks":      assertNoFabricatedTracks,
	"identical-batches":         assertIdenticalBatches,
	"identical-future":          assertIdenticalFuture,
	"paused-time-discarded":     assertPausedTimeDiscarded,
	"physical-speed-unchanged":  assertPhysicalSpeedUnchanged,
	"historical-revisions":      assertHistoricalRevisions,
	"receiver-provenance":       assertReceiverProvenance,
	"unchanged-transmissions":   assertUnchangedTransmissions,
	"receive-all":               assertReceiveAll,
	"drop-all":                  assertDropAll,
	"seeded-partial-loss":       assertSeededPartialLoss,
	"reproducible":              assertReproducible,
	"inclusive-lifetime":        assertFieldLifetimes,
	"expired-after-lifetime":    assertFieldLifetimes,
	"bounded-records":           assertBoundedRecords,
	"eviction-gap":              assertEvictionGap,
	"removed-aircraft-observed": assertRemovedAircraftObserved,
	"new-identities":            assertNewIdentities,
	"same-generated-payloads":   assertSameGeneratedPayloads,
	"old-cursor-rejected":       assertOldCursorRejected,
}

func loadEngineScenarios(t *testing.T) []qualitytest.Scenario {
	t.Helper()

	all, err := qualitytest.LoadScenarios(qualityScenarios)
	require.NoError(t, err)
	var scenarios []qualitytest.Scenario
	for _, scenario := range all {
		if scenario.Kind == qualitytest.EngineKind {
			scenarios = append(scenarios, scenario)
		}
	}
	return scenarios
}

func TestCorpusEngineScenarios(t *testing.T) {
	t.Parallel()

	for _, scenario := range loadEngineScenarios(t) {
		t.Run(scenario.ID, func(t *testing.T) {
			t.Parallel()

			executed := 0
			for _, name := range scenario.Assertions {
				assertion, ok := engineAssertions[name]
				if !ok {
					continue
				}
				executed++
				t.Run(name, func(t *testing.T) { assertion(t, scenario) })
			}
			require.Positive(t, executed, "every engine scenario has an engine-layer assertion")
		})
	}
}

// corpusConfig converts scenario engine inputs.
func corpusConfig(scenario qualitytest.Scenario, runID string) Config {
	engine := scenario.Engine
	spawn := engine.Spawn
	r := func(value qualitytest.Range) Range { return Range{Min: value.Min, Max: value.Max} }
	return Config{
		ID: runID, StartTime: engine.StartTime, Seed: engine.Seed,
		InitialAircraftCount: engine.InitialAircraftCount, SpeedHundredths: engine.SpeedHundredths,
		Spawn: SpawnConfig{
			LatitudeDegrees: r(spawn.LatitudeDegrees), LongitudeDegrees: r(spawn.LongitudeDegrees),
			AltitudeFeet: r(spawn.AltitudeFeet), GroundSpeedKnots: r(spawn.GroundSpeedKnots),
			TrackDegrees: r(spawn.TrackDegrees), VerticalRateFeetPerMinute: r(spawn.VerticalRateFeetPerMinute),
		},
	}
}

// corpusStation converts one scenario station.
func corpusStation(station qualitytest.Station) StationConfig {
	return StationConfig{
		ID: station.ID, Enabled: station.Enabled,
		LatitudeDegrees: station.LatitudeDegrees, LongitudeDegrees: station.LongitudeDegrees,
		SiteElevationMetres: station.SiteElevationMetres, AntennaHeightMetres: station.AntennaHeightMetres,
		AntennaGainDBi: station.AntennaGainDBi, SensitivityDBm: station.SensitivityDBm,
		SystemLossDB: station.SystemLossDB, FrameLossProbability: station.FrameLossProbability,
	}
}

// corpusExpiry converts the scenario field lifetimes.
func corpusExpiry(expiry qualitytest.Expiry) ObservationExpiry {
	return ObservationExpiry{
		Identity: expiry.Identity, Position: expiry.Position,
		Altitude: expiry.Altitude, Velocity: expiry.Velocity,
	}
}

// corpusOperation converts one scenario operation into a scripted command.
func corpusOperation(t testing.TB, op qualitytest.Operation) operation {
	t.Helper()

	switch op.Op {
	case qualitytest.OpAdvance:
		return advanceOp(op.Duration)
	case qualitytest.OpElapse:
		return elapseOp(op.Duration)
	case qualitytest.OpSetCount:
		return countOp(op.Count)
	case qualitytest.OpSetSpeed:
		return speedOp(op.SpeedHundredths)
	case qualitytest.OpAddStation:
		return addStationOp(corpusStation(op.Station))
	case qualitytest.OpUpdateStation:
		return updateStationOp(op.ExpectedRevision, corpusStation(op.Station))
	case qualitytest.OpRemoveStation:
		return removeStationOp(op.StationID, op.ExpectedRevision)
	}
	t.Fatalf("unsupported corpus operation %q", op.Op)
	return operation{}
}

// corpusRun is one executed scenario run.
type corpusRun struct {
	engine *Engine
	// initial is the snapshot right after construction and station setup.
	initial Snapshot
	// batch concatenates every operation's output in order.
	batch Batch
}

// executeRun builds a fresh engine for one named run, adds the scenario
// stations, and applies the run operations.
func executeRun(t *testing.T, scenario qualitytest.Scenario, name string) corpusRun {
	t.Helper()

	run, err := scenario.Run(name)
	require.NoError(t, err)
	engine := newEngine(t, corpusConfig(scenario, run.RunID))
	for _, station := range scenario.Stations {
		addStation(t, engine, corpusStation(station))
	}
	script := make([]operation, 0, len(run.Operations))
	for _, op := range run.Operations {
		script = append(script, corpusOperation(t, op))
	}
	initial := engine.Snapshot()
	return corpusRun{engine: engine, initial: initial, batch: runScript(t, engine, script)}
}

// executeRuns executes every run of the scenario in declaration order.
func executeRuns(t *testing.T, scenario qualitytest.Scenario) []corpusRun {
	t.Helper()

	runs := make([]corpusRun, 0, len(scenario.Runs))
	for _, run := range scenario.Runs {
		runs = append(runs, executeRun(t, scenario, run.Name))
	}
	return runs
}

func assertAllocatedEmptyArrays(t *testing.T, scenario qualitytest.Scenario) {
	for _, run := range executeRuns(t, scenario) {
		require.NotNil(t, run.batch.Transmissions)
		require.NotNil(t, run.batch.Receptions)
		require.Empty(t, run.batch.Transmissions)

		raw, err := run.engine.ReceptionSnapshot(t.Context(), ReceptionSnapshotRequest{StationIDs: scenario.Selection})
		require.NoError(t, err)
		require.NotNil(t, raw.StationIDs)
		require.NotNil(t, raw.Retention)
		require.NotNil(t, raw.Records)
		require.Empty(t, raw.Records)

		observed, err := run.engine.Observations(t.Context(), ObservationRequest{
			StationIDs: scenario.Selection, Expiry: corpusExpiry(scenario.Expiry),
		})
		require.NoError(t, err)
		require.NotNil(t, observed.StationIDs)
		require.NotNil(t, observed.Retention)
		require.NotNil(t, observed.Aircraft)
	}
}

func assertNoFabricatedTracks(t *testing.T, scenario qualitytest.Scenario) {
	for _, run := range executeRuns(t, scenario) {
		selections := [][]string{scenario.Selection}
		for _, station := range run.engine.Snapshot().Stations {
			selections = append(selections, []string{station.Config.ID})
		}
		for _, selection := range selections {
			observed, err := run.engine.Observations(t.Context(), ObservationRequest{
				StationIDs: selection, Expiry: corpusExpiry(scenario.Expiry),
			})
			require.NoError(t, err)
			require.Empty(t, observed.Aircraft, "selection %v", selection)
		}
	}
}

func assertIdenticalBatches(t *testing.T, scenario qualitytest.Scenario) {
	runs := executeRuns(t, scenario)
	require.GreaterOrEqual(t, len(runs), 2)
	require.NotEmpty(t, runs[0].batch.Transmissions)
	for _, run := range runs[1:] {
		require.Equal(t, runs[0].batch, run.batch, "reproducibility: identical ordered frames and receptions")
	}
}

func assertIdenticalFuture(t *testing.T, scenario qualitytest.Scenario) {
	runs := executeRuns(t, scenario)
	require.Len(t, runs, 2)
	requireIdenticalFuture(t, runs[1].engine, runs[0].engine)
}

func assertPausedTimeDiscarded(t *testing.T, scenario qualitytest.Scenario) {
	scaled := executeRun(t, scenario, "scaled")
	virtual := executeRun(t, scenario, "virtual")
	require.Equal(t, virtual.engine.Snapshot().Elapsed, scaled.engine.Snapshot().Elapsed)
	require.Equal(t, 12*time.Second, scaled.engine.Snapshot().Elapsed,
		"2 s at 1x plus 100 ms at 100x; the paused 5 s adds nothing")
}

func assertPhysicalSpeedUnchanged(t *testing.T, scenario qualitytest.Scenario) {
	scaled := executeRun(t, scenario, "scaled")
	virtual := executeRun(t, scenario, "virtual")
	speeds := func(snapshot Snapshot) map[uint32]float64 {
		result := map[uint32]float64{}
		for _, aircraft := range snapshot.Aircraft {
			result[aircraft.ICAO] = aircraft.GroundSpeedKnots
		}
		return result
	}
	initial := speeds(scaled.initial)
	require.NotEmpty(t, initial)
	require.Equal(t, initial, speeds(scaled.engine.Snapshot()), "time scaling never changes aircraft speed")
	require.Equal(t, speeds(virtual.engine.Snapshot()), speeds(scaled.engine.Snapshot()))
	require.Equal(t, virtual.engine.Snapshot().Aircraft, scaled.engine.Snapshot().Aircraft)
}

// stationRecords returns the retained records of one station.
func stationRecords(t *testing.T, engine *Engine, id string) []Reception {
	t.Helper()

	page, err := engine.ReceptionHistory(t.Context(), HistoryRequest{StationID: id, Limit: MaxHistoryPageSize})
	require.NoError(t, err)
	require.False(t, page.HasMore)
	return page.Records
}

func assertHistoricalRevisions(t *testing.T, scenario qualitytest.Scenario) {
	run := executeRun(t, scenario, "receivers")
	gains := map[uint64]float64{}
	for _, record := range stationRecords(t, run.engine, "primary") {
		require.Equal(t, record.StationRevision, record.Receiver.Revision)
		gains[record.StationRevision] = record.Receiver.Config.AntennaGainDBi
		require.True(t, record.Receiver.Config.Enabled, "a disabled revision receives nothing")
	}
	require.Equal(t, map[uint64]float64{1: 3, 2: 10, 4: 10}, gains,
		"records keep the settings of the revision that heard them; revision 3 was disabled")
	stations := run.engine.Snapshot().Stations
	require.Len(t, stations, 1)
	require.Equal(t, uint64(4), stations[0].Revision)
}

func assertReceiverProvenance(t *testing.T, scenario qualitytest.Scenario) {
	run := executeRun(t, scenario, "receivers")
	removedAt := scenario.Engine.StartTime.Add(20 * time.Second)
	secondary := 0
	for _, record := range run.batch.Receptions {
		require.Equal(t, record.StationID, record.Receiver.Config.ID)
		if record.StationID == "secondary" {
			secondary++
			require.Equal(t, uint64(1), record.StationRevision)
			require.False(t, record.Timestamp.After(removedAt), "a removed station hears nothing")
		}
	}
	require.Positive(t, secondary)
	_, err := run.engine.ReceptionSnapshot(t.Context(), ReceptionSnapshotRequest{StationIDs: []string{"secondary"}})
	require.ErrorIs(t, err, ErrNotFound)
}

func assertUnchangedTransmissions(t *testing.T, scenario qualitytest.Scenario) {
	receivers := executeRun(t, scenario, "receivers")
	none := executeRun(t, scenario, "no-receivers")
	require.NotEmpty(t, receivers.batch.Receptions)
	require.Empty(t, none.batch.Receptions)
	require.Equal(t, none.batch.Transmissions, receivers.batch.Transmissions,
		"station commands never alter generated frames")
}

// receptionsByStation counts batch receptions per station.
func receptionsByStation(batch Batch) map[string]int {
	counts := map[string]int{}
	for _, record := range batch.Receptions {
		counts[record.StationID]++
	}
	return counts
}

func assertReceiveAll(t *testing.T, scenario qualitytest.Scenario) {
	run := executeRun(t, scenario, "first")
	require.NotEmpty(t, run.batch.Transmissions)
	require.Equal(t, len(run.batch.Transmissions), receptionsByStation(run.batch)["receive-all"])
}

func assertDropAll(t *testing.T, scenario qualitytest.Scenario) {
	run := executeRun(t, scenario, "first")
	require.NotEmpty(t, run.batch.Transmissions)
	require.Zero(t, receptionsByStation(run.batch)["drop-all"])
}

func assertSeededPartialLoss(t *testing.T, scenario qualitytest.Scenario) {
	run := executeRun(t, scenario, "first")
	heard := receptionsByStation(run.batch)["quarter"]
	require.Positive(t, heard)
	require.Less(t, heard, len(run.batch.Transmissions))
}

func assertReproducible(t *testing.T, scenario qualitytest.Scenario) {
	first := executeRun(t, scenario, "first")
	replay := executeRun(t, scenario, "replay")
	require.Equal(t, first.batch, replay.batch, "reproducibility: same seed and script")
	requireIdenticalFuture(t, replay.engine, first.engine)
}

// assertFieldLifetimes checks every field at exactly its lifetime, where it
// is still present, and one nanosecond later, where it has expired.
func assertFieldLifetimes(t *testing.T, scenario qualitytest.Scenario) {
	run := executeRun(t, scenario, "stop-transmitting")
	require.Empty(t, run.engine.Snapshot().Aircraft, "transmission stopped")
	request := ObservationRequest{StationIDs: scenario.Selection, Expiry: corpusExpiry(scenario.Expiry)}

	observed, err := run.engine.Observations(t.Context(), request)
	require.NoError(t, err)
	require.Len(t, observed.Aircraft, 1)
	aircraft := observed.Aircraft[0]
	require.NotNil(t, aircraft.Identity)
	require.NotNil(t, aircraft.Position)
	require.NotNil(t, aircraft.BarometricAltitude)
	require.NotNil(t, aircraft.Velocity)

	type boundary struct {
		name    string
		expires time.Time
		present func(ObservedAircraft) bool
	}
	boundaries := []boundary{
		{"identity", aircraft.Identity.ObservedAt.Add(request.Expiry.Identity), func(a ObservedAircraft) bool { return a.Identity != nil }},
		{"position", aircraft.Position.ObservedAt.Add(request.Expiry.Position), func(a ObservedAircraft) bool { return a.Position != nil }},
		{"altitude", aircraft.BarometricAltitude.ObservedAt.Add(request.Expiry.Altitude), func(a ObservedAircraft) bool { return a.BarometricAltitude != nil }},
		{"velocity", aircraft.Velocity.ObservedAt.Add(request.Expiry.Velocity), func(a ObservedAircraft) bool { return a.Velocity != nil }},
	}
	slices.SortFunc(boundaries, func(a, b boundary) int { return a.expires.Compare(b.expires) })

	fieldAt := func(at time.Time, present func(ObservedAircraft) bool) bool {
		now := run.engine.Snapshot().Now
		require.False(t, at.Before(now), "boundaries are distinct and increasing")
		if at.After(now) {
			_, err := run.engine.Advance(context.Background(), at.Sub(now))
			require.NoError(t, err)
		}
		snapshot, err := run.engine.Observations(t.Context(), request)
		require.NoError(t, err)
		return len(snapshot.Aircraft) == 1 && present(snapshot.Aircraft[0])
	}
	for _, b := range boundaries {
		if scenario.Asserts("inclusive-lifetime") {
			require.True(t, fieldAt(b.expires, b.present), "%s is fresh at exactly its lifetime", b.name)
		}
		if scenario.Asserts("expired-after-lifetime") {
			require.False(t, fieldAt(b.expires.Add(time.Nanosecond), b.present), "%s expires one nanosecond later", b.name)
		}
	}
}

func assertBoundedRecords(t *testing.T, scenario qualitytest.Scenario) {
	run := executeRun(t, scenario, "churn")
	require.Greater(t, len(run.batch.Receptions), ReceptionHistoryLimit, "the run wraps the ring")
	require.Len(t, stationRecords(t, run.engine, "primary"), ReceptionHistoryLimit)
	require.LessOrEqual(t, len(run.engine.Snapshot().History.Messages), HistoryLimit)
	raw, err := run.engine.ReceptionSnapshot(t.Context(), ReceptionSnapshotRequest{StationIDs: scenario.Selection})
	require.NoError(t, err)
	require.Len(t, raw.Records, ReceptionHistoryLimit*len(scenario.Selection))
}

func assertEvictionGap(t *testing.T, scenario qualitytest.Scenario) {
	run := executeRun(t, scenario, "churn")
	raw, err := run.engine.ReceptionSnapshot(t.Context(), ReceptionSnapshotRequest{StationIDs: []string{"primary"}})
	require.NoError(t, err)
	retention := raw.Retention[0]
	require.True(t, retention.Truncated)
	require.Equal(t, uint64(len(run.batch.Receptions)), retention.LatestSequence)
	require.Equal(t, retention.LatestSequence-ReceptionHistoryLimit+1, retention.OldestSequence)

	page, err := run.engine.ReceptionHistory(t.Context(), HistoryRequest{
		StationID: "primary", Limit: 1,
		Cursor: &ReceptionCursor{RunID: raw.RunID, StationID: "primary", AfterSequence: 1},
	})
	require.NoError(t, err)
	require.True(t, page.Gap, "a cursor before the oldest retained record reports the eviction gap")
	require.Equal(t, retention.OldestSequence, page.Records[0].Sequence)
}

// addressSet returns the ICAO addresses of a slice.
func addressSet[T any](items []T, icao func(T) uint32) map[uint32]bool {
	set := map[uint32]bool{}
	for _, item := range items {
		set[icao(item)] = true
	}
	return set
}

func assertRemovedAircraftObserved(t *testing.T, scenario qualitytest.Scenario) {
	run := executeRun(t, scenario, "churn")
	truth := addressSet(run.engine.Snapshot().Aircraft, func(a Aircraft) uint32 { return a.ICAO })
	observed, err := run.engine.Observations(t.Context(), ObservationRequest{
		StationIDs: scenario.Selection, Expiry: corpusExpiry(scenario.Expiry),
	})
	require.NoError(t, err)
	received := addressSet(observed.Aircraft, func(a ObservedAircraft) uint32 { return a.ICAO })
	require.Len(t, truth, 5)
	require.Greater(t, len(received), len(truth), "retained evidence covers removed aircraft")
	for icao := range truth {
		require.True(t, received[icao], "current aircraft %06X is received", icao)
	}
}

func assertNewIdentities(t *testing.T, scenario qualitytest.Scenario) {
	run := executeRun(t, scenario, "churn")
	first := map[uint32]bool{}
	for _, transmission := range run.batch.Transmissions {
		if transmission.Timestamp.Before(scenario.Engine.StartTime.Add(60 * time.Second)) {
			first[transmission.ICAO] = true
		}
	}
	current := addressSet(run.engine.Snapshot().Aircraft, func(a Aircraft) uint32 { return a.ICAO })
	require.Len(t, first, 5)
	require.Len(t, current, 5)
	for icao := range current {
		require.False(t, first[icao], "replacement aircraft %06X has a new identity", icao)
	}
}

func assertSameGeneratedPayloads(t *testing.T, scenario qualitytest.Scenario) {
	runs := executeRuns(t, scenario)
	require.Len(t, runs, 2)
	require.NotEqual(t, runs[0].engine.Snapshot().Config.ID, runs[1].engine.Snapshot().Config.ID)
	require.NotEmpty(t, runs[0].batch.Transmissions)
	require.Equal(t, runs[0].batch, runs[1].batch, "the run ID never changes generated payloads")
}

func assertOldCursorRejected(t *testing.T, scenario qualitytest.Scenario) {
	runs := executeRuns(t, scenario)
	page, err := runs[0].engine.ReceptionHistory(t.Context(), HistoryRequest{StationID: "primary", Limit: 1})
	require.NoError(t, err)
	_, err = runs[1].engine.ReceptionHistory(t.Context(), HistoryRequest{
		StationID: "primary", Cursor: &page.NextCursor, Limit: 1,
	})
	require.ErrorIs(t, err, ErrConflict)
}
