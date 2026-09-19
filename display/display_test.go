package display

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/miroslav-matejovsky/adsb-testbench/simulatorapi"
)

// funcSource is an observation and station source driven entirely by the
// test.
type funcSource struct {
	snapshot func(context.Context, simulatorapi.ReceptionSnapshotRequest) (simulatorapi.ReceptionSnapshot, error)
	history  func(context.Context, simulatorapi.HistoryRequest) (simulatorapi.ReceptionPage, error)
	stations func(context.Context) (simulatorapi.StationsSnapshot, error)
}

func (s funcSource) Stations(ctx context.Context) (simulatorapi.StationsSnapshot, error) {
	return s.stations(ctx)
}

// stationsOf returns source's own station source when it has one.
func stationsOf(source ObservationSource) StationSource {
	if stations, ok := source.(StationSource); ok {
		return stations
	}
	return funcSource{}
}

func (s funcSource) ReceptionSnapshot(ctx context.Context, request simulatorapi.ReceptionSnapshotRequest) (simulatorapi.ReceptionSnapshot, error) {
	return s.snapshot(ctx, request)
}

func (s funcSource) ReceptionHistory(ctx context.Context, request simulatorapi.HistoryRequest) (simulatorapi.ReceptionPage, error) {
	return s.history(ctx, request)
}

// displayFixtureConfig holds explicit fixture settings, not defaults.
func displayFixtureConfig() Config {
	return Config{
		IdentityExpiry: time.Minute, PositionExpiry: 30 * time.Second,
		AltitudeExpiry: 30 * time.Second, VelocityExpiry: 30 * time.Second,
		MaxRequestBytes: 65536, MaxResponseBytes: 16777216,
		RequestTimeout: 5 * time.Second, ReportError: func(error) {},
	}
}

// fixedClock returns a controllable local update clock.
type fixedClock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *fixedClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *fixedClock) Add(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

// trackedSnapshot returns a one-station snapshot carrying one full aircraft.
func trackedSnapshot(t *testing.T, runID string, icao uint32, now time.Time, firstSequence uint64) simulatorapi.ReceptionSnapshot {
	t.Helper()

	station := fixtureStation("alpha", 1, fixtureStart)
	at := now.Add(-time.Second)
	records := []simulatorapi.Reception{
		fixtureReception(firstSequence, firstSequence, station, icao, identificationKind, at,
			identificationFrame(t, icao, "TB00ABCD")),
		fixtureReception(firstSequence+1, firstSequence+1, station, icao, positionKind, at,
			positionFrame(t, icao, 50, 14, feet(35000), false)),
		fixtureReception(firstSequence+2, firstSequence+2, station, icao, positionKind, at,
			positionFrame(t, icao, 50, 14, feet(35000), true)),
	}
	raw := fixtureSnapshot(now, []string{"alpha"}, records)
	raw.RunID = runID
	return raw
}

// newTestDisplay returns a display over a scripted source and a fixed clock.
func newTestDisplay(t *testing.T, source ObservationSource) (*Display, *fixedClock) {
	t.Helper()

	clock := &fixedClock{now: time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)}
	return newDisplay(displayFixtureConfig(), source, stationsOf(source), clock.Now), clock
}

func TestDisplayStartsUnavailable(t *testing.T) {
	t.Parallel()

	display, _ := newTestDisplay(t, funcSource{})
	snapshot := display.Snapshot()
	require.Equal(t, StatusUnavailable, snapshot.Status)
	require.Nil(t, snapshot.Observations)
	require.Nil(t, snapshot.LastUpdatedAt)
	require.Nil(t, snapshot.Error)
}

func TestDisplayPublishesADecodedRefresh(t *testing.T) {
	t.Parallel()

	raw := trackedSnapshot(t, fixtureRunID, fixtureICAO, fixtureStart.Add(10*time.Second), 1)
	display, clock := newTestDisplay(t, funcSource{
		snapshot: func(context.Context, simulatorapi.ReceptionSnapshotRequest) (simulatorapi.ReceptionSnapshot, error) {
			return raw, nil
		},
	})

	snapshot, err := display.Refresh(t.Context(), []string{"alpha"})
	require.NoError(t, err)
	require.Equal(t, StatusFresh, snapshot.Status)
	require.Nil(t, snapshot.Error)
	require.NotNil(t, snapshot.LastUpdatedAt)
	require.Equal(t, simulatorapi.FormatTime(clock.Now()), *snapshot.LastUpdatedAt)

	require.NotNil(t, snapshot.Observations)
	require.Equal(t, fixtureRunID, snapshot.Observations.RunID)
	require.Equal(t, raw.Now, snapshot.Observations.Now)
	require.Len(t, snapshot.Observations.Aircraft, 1)
	require.NotNil(t, snapshot.Observations.Aircraft[0].Position)
	require.Equal(t, snapshot, display.Snapshot())
}

func TestDisplaySnapshotIsDetachedFromPublishedState(t *testing.T) {
	t.Parallel()

	raw := trackedSnapshot(t, fixtureRunID, fixtureICAO, fixtureStart.Add(10*time.Second), 1)
	display, _ := newTestDisplay(t, funcSource{
		snapshot: func(context.Context, simulatorapi.ReceptionSnapshotRequest) (simulatorapi.ReceptionSnapshot, error) {
			return raw, nil
		},
	})
	want, err := display.Refresh(t.Context(), []string{"alpha"})
	require.NoError(t, err)

	mutated := display.Snapshot()
	mutated.Observations.RunID = "changed"
	mutated.Observations.StationIDs[0] = "changed"
	mutated.Observations.Aircraft[0].Identity.Callsign = "changed"
	mutated.Observations.Aircraft[0].Identity.Evidence.Receptions[0].Frame = "changed"
	mutated.Observations.Aircraft[0].Position.Evidence[0].ICAO = "changed"

	require.Equal(t, want, display.Snapshot())
}

func TestDisplayReturnsStaleDataWithItsError(t *testing.T) {
	t.Parallel()

	raw := trackedSnapshot(t, fixtureRunID, fixtureICAO, fixtureStart.Add(10*time.Second), 1)
	fail := false
	cause := errors.New("connection refused")
	display, clock := newTestDisplay(t, funcSource{
		snapshot: func(context.Context, simulatorapi.ReceptionSnapshotRequest) (simulatorapi.ReceptionSnapshot, error) {
			if fail {
				return simulatorapi.ReceptionSnapshot{},
					newSourceError(snapshotOperation, simulatorapi.CategoryUnavailable, cause, "the source is down")
			}
			return raw, nil
		},
	})

	first, err := display.Refresh(t.Context(), []string{"alpha"})
	require.NoError(t, err)
	updated := *first.LastUpdatedAt

	fail = true
	clock.Add(time.Minute)
	second, err := display.Refresh(t.Context(), []string{"alpha"})
	require.Error(t, err)
	require.ErrorIs(t, err, simulatorapi.CategoryUnavailable)
	require.ErrorIs(t, err, cause)
	require.Equal(t, StatusStale, second.Status)
	require.NotNil(t, second.Observations)
	require.Equal(t, first.Observations, second.Observations)
	require.Equal(t, updated, *second.LastUpdatedAt,
		"the local update time describes the last success")
	require.NotNil(t, second.Error)
	require.Equal(t, simulatorapi.CategoryUnavailable, second.Error.Code)
}

func TestDisplayFailedSelectionChangeShowsNoOtherSelection(t *testing.T) {
	t.Parallel()

	raw := trackedSnapshot(t, fixtureRunID, fixtureICAO, fixtureStart.Add(10*time.Second), 1)
	display, _ := newTestDisplay(t, funcSource{
		snapshot: func(_ context.Context, request simulatorapi.ReceptionSnapshotRequest) (simulatorapi.ReceptionSnapshot, error) {
			if len(request.StationIDs) == 1 && request.StationIDs[0] == "alpha" {
				return raw, nil
			}
			return simulatorapi.ReceptionSnapshot{},
				newSourceError(snapshotOperation, simulatorapi.CategoryNotFound, nil, "station bravo")
		},
	})

	_, err := display.Refresh(t.Context(), []string{"alpha"})
	require.NoError(t, err)

	snapshot, err := display.Refresh(t.Context(), []string{"bravo"})
	require.ErrorIs(t, err, simulatorapi.CategoryNotFound)
	require.Equal(t, StatusUnavailable, snapshot.Status)
	require.Nil(t, snapshot.Observations)
}

func TestDisplayNewRunClearsPreviousStateEvenWhenItsPayloadFails(t *testing.T) {
	t.Parallel()

	first := trackedSnapshot(t, "run-1", fixtureICAO, fixtureStart.Add(10*time.Second), 1)
	second := trackedSnapshot(t, "run-2", fixtureICAO, fixtureStart.Add(10*time.Second), 1)
	second.Records[0].Frame = "8D48"

	current := &first
	display, _ := newTestDisplay(t, funcSource{
		snapshot: func(context.Context, simulatorapi.ReceptionSnapshotRequest) (simulatorapi.ReceptionSnapshot, error) {
			return *current, nil
		},
	})

	snapshot, err := display.Refresh(t.Context(), []string{"alpha"})
	require.NoError(t, err)
	require.Equal(t, StatusFresh, snapshot.Status)

	current = &second
	snapshot, err = display.Refresh(t.Context(), []string{"alpha"})
	require.ErrorIs(t, err, simulatorapi.CategorySourceInvalid)
	require.Equal(t, StatusUnavailable, snapshot.Status)
	require.Nil(t, snapshot.Observations, "old-run data must not survive a new run")
	require.Equal(t, "run-2", snapshot.Error.RunID)
}

func TestDisplayConflictWithADifferentRunInvalidatesOldState(t *testing.T) {
	t.Parallel()

	raw := trackedSnapshot(t, "run-1", fixtureICAO, fixtureStart.Add(10*time.Second), 1)
	fail := false
	display, _ := newTestDisplay(t, funcSource{
		snapshot: func(context.Context, simulatorapi.ReceptionSnapshotRequest) (simulatorapi.ReceptionSnapshot, error) {
			if fail {
				return simulatorapi.ReceptionSnapshot{},
					newSourceError(snapshotOperation, simulatorapi.CategoryConflict, nil, "stale run").
						withRunID("run-2")
			}
			return raw, nil
		},
	})

	_, err := display.Refresh(t.Context(), []string{"alpha"})
	require.NoError(t, err)

	fail = true
	snapshot, err := display.Refresh(t.Context(), []string{"alpha"})
	require.ErrorIs(t, err, simulatorapi.CategoryConflict)
	require.Equal(t, StatusUnavailable, snapshot.Status)
	require.Nil(t, snapshot.Observations)
}

func TestDisplayRejectsSameRunRegression(t *testing.T) {
	t.Parallel()

	later := trackedSnapshot(t, fixtureRunID, fixtureICAO, fixtureStart.Add(20*time.Second), 4)
	earlierTime := trackedSnapshot(t, fixtureRunID, fixtureICAO, fixtureStart.Add(10*time.Second), 4)
	earlierSequence := trackedSnapshot(t, fixtureRunID, fixtureICAO, fixtureStart.Add(30*time.Second), 1)

	cases := []struct {
		name  string
		after simulatorapi.ReceptionSnapshot
	}{
		{name: "virtual time regressed", after: earlierTime},
		{name: "station sequence regressed", after: earlierSequence},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			current := later
			display, _ := newTestDisplay(t, funcSource{
				snapshot: func(context.Context, simulatorapi.ReceptionSnapshotRequest) (simulatorapi.ReceptionSnapshot, error) {
					return current, nil
				},
			})
			first, err := display.Refresh(t.Context(), []string{"alpha"})
			require.NoError(t, err)

			current = tc.after
			snapshot, err := display.Refresh(t.Context(), []string{"alpha"})
			require.ErrorIs(t, err, simulatorapi.CategorySourceInvalid)
			require.Equal(t, StatusStale, snapshot.Status)
			require.Equal(t, first.Observations, snapshot.Observations)
		})
	}
}

func TestDisplayRebuildsStateFromCurrentEvidenceOnly(t *testing.T) {
	t.Parallel()

	warm := trackedSnapshot(t, fixtureRunID, fixtureICAO, fixtureStart.Add(10*time.Second), 1)

	// The later snapshot retains only the odd half, as eviction would leave it.
	station := fixtureStation("alpha", 1, fixtureStart)
	evicted := fixtureSnapshot(fixtureStart.Add(20*time.Second), []string{"alpha"},
		[]simulatorapi.Reception{
			fixtureReception(3, 3, station, fixtureICAO, positionKind, fixtureStart.Add(19*time.Second),
				positionFrame(t, fixtureICAO, 50, 14, feet(35000), true)),
		})

	current := warm
	display, _ := newTestDisplay(t, funcSource{
		snapshot: func(context.Context, simulatorapi.ReceptionSnapshotRequest) (simulatorapi.ReceptionSnapshot, error) {
			return current, nil
		},
	})
	first, err := display.Refresh(t.Context(), []string{"alpha"})
	require.NoError(t, err)
	require.NotNil(t, first.Observations.Aircraft[0].Position)

	current = evicted
	second, err := display.Refresh(t.Context(), []string{"alpha"})
	require.NoError(t, err)
	require.Len(t, second.Observations.Aircraft, 1)
	require.Nil(t, second.Observations.Aircraft[0].Position,
		"an evicted CPR half is never recovered from a warm cache")

	cold, _ := newTestDisplay(t, funcSource{
		snapshot: func(context.Context, simulatorapi.ReceptionSnapshotRequest) (simulatorapi.ReceptionSnapshot, error) {
			return evicted, nil
		},
	})
	fromCold, err := cold.Refresh(t.Context(), []string{"alpha"})
	require.NoError(t, err)
	require.Equal(t, second.Observations, fromCold.Observations,
		"a warm and a cold display see the same tracks")
}

func TestDisplayRefreshHonorsCancellationDuringAdmission(t *testing.T) {
	t.Parallel()

	entered := make(chan struct{})
	release := make(chan struct{})
	raw := trackedSnapshot(t, fixtureRunID, fixtureICAO, fixtureStart.Add(10*time.Second), 1)
	display, _ := newTestDisplay(t, funcSource{
		snapshot: func(context.Context, simulatorapi.ReceptionSnapshotRequest) (simulatorapi.ReceptionSnapshot, error) {
			select {
			case entered <- struct{}{}:
			default:
			}
			<-release
			return raw, nil
		},
	})

	first := make(chan error, 1)
	go func() {
		_, err := display.Refresh(context.Background(), []string{"alpha"})
		first <- err
	}()
	<-entered

	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	snapshot, err := display.Refresh(ctx, []string{"alpha"})
	require.ErrorIs(t, err, context.Canceled)
	require.Equal(t, StatusUnavailable, snapshot.Status)

	close(release)
	require.NoError(t, <-first)
}

func TestDisplayHistoryPassesGapsThroughAsSuccess(t *testing.T) {
	t.Parallel()

	request, page := fixturePage(t)
	request.Cursor = &simulatorapi.ReceptionCursor{
		RunID: fixtureRunID, StationID: "alpha", AfterSequence: "0",
	}
	page.OldestSequence = "5"
	page.LatestSequence = "9"
	page.Records = []simulatorapi.Reception{}
	page.NextCursor.AfterSequence = "0"
	page.HasMore = false
	page.Gap = true

	display, _ := newTestDisplay(t, funcSource{
		history: func(context.Context, simulatorapi.HistoryRequest) (simulatorapi.ReceptionPage, error) {
			return page, nil
		},
	})

	got, err := display.ReceptionHistory(t.Context(), request)
	require.NoError(t, err)
	require.True(t, got.Gap)
	require.Empty(t, got.Records)
	require.Equal(t, page, got)
}

func TestDisplayHistoryRejectsInvalidRequestsAndPages(t *testing.T) {
	t.Parallel()

	_, page := fixturePage(t)
	display, _ := newTestDisplay(t, funcSource{
		history: func(context.Context, simulatorapi.HistoryRequest) (simulatorapi.ReceptionPage, error) {
			broken := page
			broken.StationID = "bravo"
			return broken, nil
		},
	})

	_, err := display.ReceptionHistory(t.Context(), simulatorapi.HistoryRequest{StationID: "alpha", Limit: 0})
	require.ErrorIs(t, err, simulatorapi.CategoryInvalid)

	_, err = display.ReceptionHistory(t.Context(), simulatorapi.HistoryRequest{StationID: "alpha", Limit: 2})
	require.ErrorIs(t, err, simulatorapi.CategorySourceInvalid)
}

func TestNewDisplayValidatesItsDependencies(t *testing.T) {
	t.Parallel()

	_, err := New(displayFixtureConfig(), nil, funcSource{})
	require.ErrorIs(t, err, simulatorapi.CategoryInvalid)

	_, err = New(displayFixtureConfig(), funcSource{}, nil)
	require.ErrorIs(t, err, simulatorapi.CategoryInvalid)

	invalid := displayFixtureConfig()
	invalid.ReportError = nil
	_, err = New(invalid, funcSource{}, funcSource{})
	require.ErrorIs(t, err, simulatorapi.CategoryInvalid)

	display, err := New(displayFixtureConfig(), funcSource{}, funcSource{})
	require.NoError(t, err)
	require.Equal(t, StatusUnavailable, display.Snapshot().Status)
}

func TestDisplaySerializesConcurrentRefreshesAndReads(t *testing.T) {
	t.Parallel()

	raw := trackedSnapshot(t, fixtureRunID, fixtureICAO, fixtureStart.Add(10*time.Second), 1)
	var inFlight int
	var mu sync.Mutex
	display, _ := newTestDisplay(t, funcSource{
		snapshot: func(context.Context, simulatorapi.ReceptionSnapshotRequest) (simulatorapi.ReceptionSnapshot, error) {
			mu.Lock()
			inFlight++
			require.Equal(t, 1, inFlight, "refreshes must be serialized")
			mu.Unlock()
			defer func() {
				mu.Lock()
				inFlight--
				mu.Unlock()
			}()
			return raw, nil
		},
	})

	var group sync.WaitGroup
	for range 8 {
		group.Add(2)
		go func() {
			defer group.Done()
			_, err := display.Refresh(context.Background(), []string{"alpha"})
			require.NoError(t, err)
		}()
		go func() {
			defer group.Done()
			display.Snapshot()
		}()
	}
	group.Wait()
	require.Equal(t, StatusFresh, display.Snapshot().Status)
}
