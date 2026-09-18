package simulator

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/miroslav-matejovsky/adsb-testbench/internal/simdriver"
	"github.com/miroslav-matejovsky/adsb-testbench/simulation"
	"github.com/miroslav-matejovsky/adsb-testbench/simulatorapi"
)

// apiTestConfig is an explicit fixture service configuration, not a default.
func apiTestConfig() APIConfig {
	return APIConfig{
		MaxRequestBytes: 65536, MaxResponseBytes: 16777216,
		RequestTimeout: 5 * time.Second, CoverageReferenceAltitudeFeet: 35000,
		ReportError: func(error) {},
	}
}

// newTestAPI returns a service over a runtime whose clock the test controls.
func newTestAPI(t *testing.T) (*API, *runtimeClock) {
	t.Helper()

	clock := &runtimeClock{
		now:     time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC),
		started: make(chan struct{}), ticks: make(chan time.Time), stopped: make(chan struct{}),
	}
	runtime, err := newSimulator(runtimeTestConfig(), clock)
	require.NoError(t, err)
	api, err := NewAPI(runtime, apiTestConfig())
	require.NoError(t, err)
	return api, clock
}

// runWithTraffic creates a station, runs virtual time forward, and returns
// the service. Real time is settled through an ordinary control.
func runWithTraffic(t *testing.T, api *API, clock *runtimeClock, real time.Duration) {
	t.Helper()

	_, err := api.AddStation(t.Context(), simulatorapi.AddStationCommand{
		RunID: api.RunID(), Station: stationSettingsDTO(runtimeStation()),
	})
	require.NoError(t, err)
	clock.Add(real)
	_, err = api.SetCount(t.Context(), simulatorapi.CountCommand{RunID: api.RunID(), Count: 1})
	require.NoError(t, err)
	clock.Add(real)
	_, err = api.SetCount(t.Context(), simulatorapi.CountCommand{RunID: api.RunID(), Count: 1})
	require.NoError(t, err)
}

func TestNewAPIRejectsMissingRuntimeAndSettings(t *testing.T) {
	t.Parallel()

	_, err := NewAPI(nil, apiTestConfig())
	require.ErrorIs(t, err, simulatorapi.CategoryInvalid)

	runtime, err := New(runtimeTestConfig())
	require.NoError(t, err)

	invalid := apiTestConfig()
	invalid.ReportError = nil
	_, err = NewAPI(runtime, invalid)
	require.ErrorIs(t, err, simulatorapi.CategoryInvalid)

	api, err := NewAPI(runtime, apiTestConfig())
	require.NoError(t, err)
	require.Equal(t, "runtime-test", api.RunID())
}

func TestMetadataPublishesEffectiveSettingsAndCurrentSpeed(t *testing.T) {
	t.Parallel()

	api, clock := newTestAPI(t)
	clock.Add(time.Second)
	_, err := api.SetSpeed(t.Context(), simulatorapi.SpeedCommand{RunID: api.RunID(), SpeedHundredths: 0})
	require.NoError(t, err)
	_, err = api.SetCount(t.Context(), simulatorapi.CountCommand{RunID: api.RunID(), Count: 2})
	require.NoError(t, err)

	metadata, err := api.Metadata(t.Context())
	require.NoError(t, err)
	require.Equal(t, "runtime-test", metadata.RunID)
	require.Equal(t, 0, metadata.Simulation.SpeedHundredths)
	require.Equal(t, 0, metadata.Simulation.InitialAircraftCount)
	require.Equal(t, 2, metadata.AircraftCount)
	require.Equal(t, "1000000000", metadata.ElapsedNanoseconds)
	require.Equal(t, "2030-01-02T03:04:06Z", metadata.Now)
	require.Equal(t, simulation.MaxStations, metadata.Limits.MaxStations)
	require.Equal(t, simulatorapi.FormatDuration(simdriver.Heartbeat), metadata.Driver.HeartbeatNanoseconds)
	require.Equal(t, 65536, metadata.Service.MaxRequestBytes)
	require.Equal(t, 35000.0, metadata.Service.CoverageReferenceAltitudeFeet)
	require.Equal(t, "1", metadata.Simulation.Seed)
}

func TestTruthReportsInitialAndLiveCounts(t *testing.T) {
	t.Parallel()

	api, clock := newTestAPI(t)
	clock.Add(time.Second)
	_, err := api.SetCount(t.Context(), simulatorapi.CountCommand{RunID: api.RunID(), Count: 2})
	require.NoError(t, err)

	truth, err := api.Truth(t.Context())
	require.NoError(t, err)
	require.Equal(t, 0, truth.InitialAircraftCount)
	require.Equal(t, 2, truth.AircraftCount)
	require.Len(t, truth.Aircraft, 2)
	require.Equal(t, 100, truth.SpeedHundredths)
	require.NotEmpty(t, truth.History.Messages)
	require.Equal(t, simulation.HistoryLimit, truth.History.Limit)
	for _, aircraft := range truth.Aircraft {
		require.Len(t, aircraft.ICAO, 6)
		require.Equal(t, "TB"+aircraft.ICAO, aircraft.Callsign)
	}
}

func TestStationsReportCoverageAtTheConfiguredAltitude(t *testing.T) {
	t.Parallel()

	api, _ := newTestAPI(t)
	_, err := api.AddStation(t.Context(), simulatorapi.AddStationCommand{
		RunID: api.RunID(), Station: stationSettingsDTO(runtimeStation()),
	})
	require.NoError(t, err)

	stations, err := api.Stations(t.Context())
	require.NoError(t, err)
	require.Len(t, stations.Stations, 1)

	state := stations.Stations[0]
	require.Equal(t, "primary", state.Station.ID)
	require.Equal(t, "1", state.Station.Revision)
	require.Equal(t, 35000.0, state.Coverage.ReferenceAltitudeFeet)
	require.Positive(t, state.Coverage.EffectiveRadiusNauticalMiles)
	require.Equal(t,
		min(state.Coverage.HorizonRadiusNauticalMiles, state.Coverage.LinkBudgetRadiusNauticalMiles),
		state.Coverage.EffectiveRadiusNauticalMiles)
}

func TestReceptionSnapshotAndObservationsShareTheSameEvidence(t *testing.T) {
	t.Parallel()

	api, clock := newTestAPI(t)
	runWithTraffic(t, api, clock, 2*time.Second)

	raw, err := api.ReceptionSnapshot(t.Context(),
		simulatorapi.ReceptionSnapshotRequest{StationIDs: []string{"primary", "primary"}})
	require.NoError(t, err)
	require.Equal(t, []string{"primary"}, raw.StationIDs)
	require.NotEmpty(t, raw.Records)
	require.Len(t, raw.Retention, 1)
	require.Equal(t, raw.Retention[0].OldestSequence, raw.Records[0].Sequence)

	decoded, err := api.Observations(t.Context(), simulatorapi.ObservationRequest{
		StationIDs: []string{"primary"},
		Expiry: simulatorapi.ObservationExpiry{
			IdentityNanoseconds: "60000000000", PositionNanoseconds: "60000000000",
			AltitudeNanoseconds: "60000000000", VelocityNanoseconds: "60000000000",
		},
	})
	require.NoError(t, err)
	require.Equal(t, raw.RunID, decoded.RunID)
	require.Equal(t, raw.Now, decoded.Now)
	require.Equal(t, raw.Retention, decoded.Retention)
	require.Len(t, decoded.Aircraft, 1)
	require.NotNil(t, decoded.Aircraft[0].Identity)

	for _, record := range raw.Records {
		require.Len(t, record.Frame, 28)
		require.Len(t, record.ICAO, 6)
		require.Equal(t, record.StationRevision, record.Receiver.Revision)
	}
}

func TestReadRequestsRejectInvalidLocalValues(t *testing.T) {
	t.Parallel()

	api, clock := newTestAPI(t)
	runWithTraffic(t, api, clock, time.Second)

	_, err := api.Observations(t.Context(), simulatorapi.ObservationRequest{
		StationIDs: []string{"primary"},
		Expiry: simulatorapi.ObservationExpiry{
			IdentityNanoseconds: "0", PositionNanoseconds: "1",
			AltitudeNanoseconds: "1", VelocityNanoseconds: "1",
		},
	})
	require.ErrorIs(t, err, simulatorapi.CategoryInvalid)

	_, err = api.Observations(t.Context(), simulatorapi.ObservationRequest{
		StationIDs: []string{"primary"},
		Expiry: simulatorapi.ObservationExpiry{
			IdentityNanoseconds: "1s", PositionNanoseconds: "1",
			AltitudeNanoseconds: "1", VelocityNanoseconds: "1",
		},
	})
	require.ErrorIs(t, err, simulatorapi.CategoryInvalid)

	_, err = api.ReceptionSnapshot(t.Context(),
		simulatorapi.ReceptionSnapshotRequest{StationIDs: []string{"unknown"}})
	require.ErrorIs(t, err, simulatorapi.CategoryNotFound)

	_, err = api.ReceptionHistory(t.Context(), simulatorapi.HistoryRequest{StationID: "primary", Limit: 0})
	require.ErrorIs(t, err, simulatorapi.CategoryInvalid)
}

func TestReceptionHistoryPagesAndRejectsForeignCursors(t *testing.T) {
	t.Parallel()

	api, clock := newTestAPI(t)
	runWithTraffic(t, api, clock, 2*time.Second)

	first, err := api.ReceptionHistory(t.Context(), simulatorapi.HistoryRequest{StationID: "primary", Limit: 1})
	require.NoError(t, err)
	require.Len(t, first.Records, 1)
	require.True(t, first.HasMore)
	require.False(t, first.Gap)
	require.Equal(t, "primary", first.NextCursor.StationID)
	require.Equal(t, api.RunID(), first.NextCursor.RunID)
	require.Equal(t, first.Records[0].Sequence, first.NextCursor.AfterSequence)

	second, err := api.ReceptionHistory(t.Context(), simulatorapi.HistoryRequest{
		StationID: "primary", Cursor: &first.NextCursor, Limit: 1,
	})
	require.NoError(t, err)
	require.Len(t, second.Records, 1)
	require.NotEqual(t, first.Records[0].Sequence, second.Records[0].Sequence)

	stale := first.NextCursor
	stale.RunID = "another-run"
	_, err = api.ReceptionHistory(t.Context(), simulatorapi.HistoryRequest{
		StationID: "primary", Cursor: &stale, Limit: 1,
	})
	require.ErrorIs(t, err, simulatorapi.CategoryConflict)

	future := first.NextCursor
	future.AfterSequence = "18446744073709551615"
	_, err = api.ReceptionHistory(t.Context(), simulatorapi.HistoryRequest{
		StationID: "primary", Cursor: &future, Limit: 1,
	})
	require.ErrorIs(t, err, simulatorapi.CategoryInvalid)
}

func TestControlsRequireTheServedRun(t *testing.T) {
	t.Parallel()

	api, clock := newTestAPI(t)
	clock.Add(time.Second)
	before := api.runtime.Snapshot()

	_, err := api.SetCount(t.Context(), simulatorapi.CountCommand{RunID: "other", Count: 1})
	require.ErrorIs(t, err, simulatorapi.CategoryConflict)
	_, err = api.SetSpeed(t.Context(), simulatorapi.SpeedCommand{RunID: "other", SpeedHundredths: 0})
	require.ErrorIs(t, err, simulatorapi.CategoryConflict)
	_, err = api.AddStation(t.Context(), simulatorapi.AddStationCommand{
		RunID: "other", Station: stationSettingsDTO(runtimeStation()),
	})
	require.ErrorIs(t, err, simulatorapi.CategoryConflict)
	_, err = api.UpdateStation(t.Context(), "primary", simulatorapi.UpdateStationCommand{
		RunID: "other", ExpectedRevision: "1", Station: stationSettingsDTO(runtimeStation()),
	})
	require.ErrorIs(t, err, simulatorapi.CategoryConflict)
	_, err = api.RemoveStation(t.Context(), "primary", simulatorapi.RemoveStationCommand{
		RunID: "other", ExpectedRevision: "1",
	})
	require.ErrorIs(t, err, simulatorapi.CategoryConflict)

	var typed *simulatorapi.APIError
	require.ErrorAs(t, err, &typed)
	require.Equal(t, api.RunID(), typed.RunID)
	require.Equal(t, before, api.runtime.Snapshot(), "a stale run must not settle real time")
}

func TestZeroCountAndZeroSpeedAreAcceptedAssignments(t *testing.T) {
	t.Parallel()

	api, clock := newTestAPI(t)
	clock.Add(time.Second)
	_, err := api.SetCount(t.Context(), simulatorapi.CountCommand{RunID: api.RunID(), Count: 3})
	require.NoError(t, err)

	ack, err := api.SetCount(t.Context(), simulatorapi.CountCommand{RunID: api.RunID(), Count: 0})
	require.NoError(t, err)
	require.Equal(t, simulatorapi.CommandAck{RunID: api.RunID(), Operation: simulatorapi.OperationSetCount}, ack)

	ack, err = api.SetSpeed(t.Context(), simulatorapi.SpeedCommand{RunID: api.RunID(), SpeedHundredths: 0})
	require.NoError(t, err)
	require.Equal(t, simulatorapi.OperationSetSpeed, ack.Operation)

	truth, err := api.Truth(t.Context())
	require.NoError(t, err)
	require.Equal(t, 0, truth.AircraftCount)
	require.Equal(t, 0, truth.SpeedHundredths)

	_, err = api.SetSpeed(t.Context(), simulatorapi.SpeedCommand{RunID: api.RunID(), SpeedHundredths: -1})
	require.ErrorIs(t, err, simulatorapi.CategoryInvalid)
	_, err = api.SetSpeed(t.Context(), simulatorapi.SpeedCommand{
		RunID: api.RunID(), SpeedHundredths: simulation.MaxSpeedHundredths + 1,
	})
	require.ErrorIs(t, err, simulatorapi.CategoryInvalid)
	_, err = api.SetCount(t.Context(), simulatorapi.CountCommand{
		RunID: api.RunID(), Count: simulation.MaxAircraft + 1,
	})
	require.ErrorIs(t, err, simulatorapi.CategoryInvalid)
}

func TestStationLifecycleGuardsRevisionsAndIdentifiers(t *testing.T) {
	t.Parallel()

	api, _ := newTestAPI(t)
	settings := stationSettingsDTO(runtimeStation())

	created, err := api.AddStation(t.Context(), simulatorapi.AddStationCommand{
		RunID: api.RunID(), Station: settings,
	})
	require.NoError(t, err)
	require.Equal(t, "1", created.Station.Revision)
	require.Equal(t, simulatorapi.OperationAddStation, created.Operation)

	_, err = api.AddStation(t.Context(), simulatorapi.AddStationCommand{
		RunID: api.RunID(), Station: settings,
	})
	require.ErrorIs(t, err, simulatorapi.CategoryInvalid)

	edited := settings
	edited.Enabled = false
	updated, err := api.UpdateStation(t.Context(), "primary", simulatorapi.UpdateStationCommand{
		RunID: api.RunID(), ExpectedRevision: "1", Station: edited,
	})
	require.NoError(t, err)
	require.Equal(t, "2", updated.Station.Revision)
	require.False(t, updated.Station.Enabled)

	_, err = api.UpdateStation(t.Context(), "primary", simulatorapi.UpdateStationCommand{
		RunID: api.RunID(), ExpectedRevision: "1", Station: edited,
	})
	require.ErrorIs(t, err, simulatorapi.CategoryConflict)

	_, err = api.UpdateStation(t.Context(), "other", simulatorapi.UpdateStationCommand{
		RunID: api.RunID(), ExpectedRevision: "2", Station: edited,
	})
	require.ErrorIs(t, err, simulatorapi.CategoryInvalid)

	for _, revision := range []string{"0", "", "-1", "01"} {
		_, err = api.UpdateStation(t.Context(), "primary", simulatorapi.UpdateStationCommand{
			RunID: api.RunID(), ExpectedRevision: revision, Station: edited,
		})
		require.ErrorIs(t, err, simulatorapi.CategoryInvalid, revision)
	}

	invalid := settings
	invalid.SensitivityDBm = 5
	_, err = api.UpdateStation(t.Context(), "primary", simulatorapi.UpdateStationCommand{
		RunID: api.RunID(), ExpectedRevision: "2", Station: invalid,
	})
	require.ErrorIs(t, err, simulatorapi.CategoryInvalid)

	_, err = api.RemoveStation(t.Context(), "primary", simulatorapi.RemoveStationCommand{
		RunID: api.RunID(), ExpectedRevision: "1",
	})
	require.ErrorIs(t, err, simulatorapi.CategoryConflict)

	removed, err := api.RemoveStation(t.Context(), "primary", simulatorapi.RemoveStationCommand{
		RunID: api.RunID(), ExpectedRevision: "2",
	})
	require.NoError(t, err)
	require.Equal(t, simulatorapi.OperationRemoveStation, removed.Operation)

	_, err = api.AddStation(t.Context(), simulatorapi.AddStationCommand{
		RunID: api.RunID(), Station: settings,
	})
	require.ErrorIs(t, err, simulatorapi.CategoryInvalid, "a removed identifier stays reserved")

	_, err = api.RemoveStation(t.Context(), "never-existed", simulatorapi.RemoveStationCommand{
		RunID: api.RunID(), ExpectedRevision: "1",
	})
	require.ErrorIs(t, err, simulatorapi.CategoryNotFound)
}

func TestStoppedRuntimeFailsControlsButNotReads(t *testing.T) {
	t.Parallel()

	api, clock := newTestAPI(t)
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() { done <- api.runtime.Run(ctx) }()
	<-clock.started
	cancel()
	require.NoError(t, <-done)
	<-clock.stopped

	_, err := api.SetCount(t.Context(), simulatorapi.CountCommand{RunID: api.RunID(), Count: 1})
	require.ErrorIs(t, err, simulatorapi.CategoryUnavailable)
	require.ErrorIs(t, err, simdriver.ErrStopped)

	_, err = api.Truth(t.Context())
	require.NoError(t, err)
	_, err = api.Metadata(t.Context())
	require.NoError(t, err)
}

func TestCancellationIsPropagatedUnchanged(t *testing.T) {
	t.Parallel()

	api, clock := newTestAPI(t)
	runWithTraffic(t, api, clock, time.Second)

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	_, err := api.ReceptionSnapshot(ctx, simulatorapi.ReceptionSnapshotRequest{StationIDs: []string{"primary"}})
	require.ErrorIs(t, err, context.Canceled)

	var typed *simulatorapi.APIError
	require.False(t, asAPIError(err, &typed), "cancellation must not be recategorized")

	_, err = api.Truth(ctx)
	require.ErrorIs(t, err, context.Canceled)
}

func TestResponsesAreDetachedFromRuntimeState(t *testing.T) {
	t.Parallel()

	api, clock := newTestAPI(t)
	runWithTraffic(t, api, clock, 2*time.Second)

	request := simulatorapi.ReceptionSnapshotRequest{StationIDs: []string{"primary"}}
	want, err := api.ReceptionSnapshot(t.Context(), request)
	require.NoError(t, err)

	mutated, err := api.ReceptionSnapshot(t.Context(), request)
	require.NoError(t, err)
	mutated.StationIDs[0] = "changed"
	mutated.Records[0].Frame = "00000000000000000000000000000000"
	mutated.Records[0].Receiver.ID = "changed"
	mutated.Retention[0].Limit = -1

	again, err := api.ReceptionSnapshot(t.Context(), request)
	require.NoError(t, err)
	require.Equal(t, want, again)

	requestIDs := []string{"primary"}
	_, err = api.ReceptionSnapshot(t.Context(), simulatorapi.ReceptionSnapshotRequest{StationIDs: requestIDs})
	require.NoError(t, err)
	require.Equal(t, []string{"primary"}, requestIDs)
}

// asAPIError reports whether err carries a shared service error.
func asAPIError(err error, target **simulatorapi.APIError) bool {
	for current := err; current != nil; {
		if typed, ok := current.(*simulatorapi.APIError); ok {
			*target = typed
			return true
		}
		unwrapped, ok := current.(interface{ Unwrap() error })
		if !ok {
			return false
		}
		current = unwrapped.Unwrap()
	}
	return false
}
