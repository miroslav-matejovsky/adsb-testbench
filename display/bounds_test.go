package display

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/miroslav-matejovsky/adsb-testbench/simulatorapi"
)

func TestDisplayRetainsOnlyOneLastGoodSnapshot(t *testing.T) {
	t.Parallel()

	alpha := trackedSnapshot(t, fixtureRunID, fixtureICAO, fixtureStart.Add(10*time.Second), 1)
	bravoStation := fixtureStation("bravo", 1, fixtureStart)
	bravo := fixtureSnapshot(fixtureStart.Add(10*time.Second), []string{"bravo"},
		[]simulatorapi.Reception{
			fixtureReception(1, 1, bravoStation, 0x000002, identificationKind,
				fixtureStart.Add(9*time.Second), identificationFrame(t, 0x000002, "TB000002")),
		})
	bravo.RunID = fixtureRunID

	fail := false
	display, _ := newTestDisplay(t, funcSource{
		snapshot: func(_ context.Context, request simulatorapi.ReceptionSnapshotRequest) (simulatorapi.ReceptionSnapshot, error) {
			if fail {
				return simulatorapi.ReceptionSnapshot{},
					newSourceError(snapshotOperation, simulatorapi.CategoryUnavailable, nil, "down")
			}
			if len(request.StationIDs) == 1 && request.StationIDs[0] == "bravo" {
				return bravo, nil
			}
			return alpha, nil
		},
	})

	_, err := display.Refresh(t.Context(), []string{"alpha"})
	require.NoError(t, err)
	_, err = display.Refresh(t.Context(), []string{"bravo"})
	require.NoError(t, err)

	// Only the newest selection can fall back; the previous one is gone.
	fail = true
	stale, err := display.Refresh(t.Context(), []string{"bravo"})
	require.Error(t, err)
	require.Equal(t, StatusStale, stale.Status)
	require.Equal(t, "000002", stale.Observations.Aircraft[0].ICAO)

	dropped, err := display.Refresh(t.Context(), []string{"alpha"})
	require.Error(t, err)
	require.Equal(t, StatusUnavailable, dropped.Status)
	require.Nil(t, dropped.Observations)
}

func TestDisplayHandlesManyDistinctReceivedAddresses(t *testing.T) {
	t.Parallel()

	station := fixtureStation("alpha", 1, fixtureStart)
	now := fixtureStart.Add(10 * time.Second)
	at := fixtureStart.Add(9 * time.Second)

	const targets = 500
	records := make([]simulatorapi.Reception, 0, targets)
	for i := range targets {
		icao := uint32(0x000001 + i)
		records = append(records, fixtureReception(uint64(i+1), uint64(i+1), station, icao,
			identificationKind, at, identificationFrame(t, icao, "")))
	}
	raw := fixtureSnapshot(now, []string{"alpha"}, records)

	display, _ := newTestDisplay(t, funcSource{
		snapshot: func(context.Context, simulatorapi.ReceptionSnapshotRequest) (simulatorapi.ReceptionSnapshot, error) {
			return raw, nil
		},
	})

	snapshot, err := display.Refresh(t.Context(), []string{"alpha"})
	require.NoError(t, err)
	require.Len(t, snapshot.Observations.Aircraft, targets,
		"track count follows distinct received addresses, not a truth count")
	require.Equal(t, "000001", snapshot.Observations.Aircraft[0].ICAO)
	require.Equal(t, "", snapshot.Observations.Aircraft[0].Identity.Callsign,
		"an empty decoded callsign is a received empty value, not a missing identity")
}

func TestDisplayRejectsSnapshotsAboveTheRecordBound(t *testing.T) {
	t.Parallel()

	raw := simulatorapi.ReceptionSnapshot{
		RunID: fixtureRunID, Now: simulatorapi.FormatTime(fixtureStart),
		StationIDs: []string{"alpha"},
		Retention: []simulatorapi.StationRetention{{
			StationID: "alpha", OldestSequence: "0", LatestSequence: "0", Limit: maxStationRecords,
		}},
		Records: make([]simulatorapi.Reception, maxSnapshotRecords+1),
	}

	_, _, err := validateSnapshot(simulatorapi.ReceptionSnapshotRequest{StationIDs: []string{"alpha"}}, raw)
	require.ErrorContains(t, err, "above the")
}

func TestHTTPSourceClampsLargeRemoteErrorText(t *testing.T) {
	t.Parallel()

	long := strings.Repeat("e", simulatorapi.MaxMessageBytes*4)
	body, err := json.Marshal(simulatorapi.ErrorResponse{
		Error: simulatorapi.APIError{Code: simulatorapi.CategoryInvalid, Message: long},
	})
	require.NoError(t, err)

	source, _ := newHTTPSourceFor(t, func(*http.Request) (*http.Response, error) {
		return jsonResponse(http.StatusBadRequest, string(body)), nil
	})
	_, err = source.ReceptionSnapshot(t.Context(),
		simulatorapi.ReceptionSnapshotRequest{StationIDs: []string{"alpha"}})
	require.ErrorIs(t, err, simulatorapi.CategoryInvalid)

	var failure *SourceError
	require.ErrorAs(t, err, &failure)
	require.Len(t, failure.Message, simulatorapi.MaxMessageBytes)
}

func TestDisplayErrorEnvelopeFitsTheMinimumBudget(t *testing.T) {
	t.Parallel()

	long := strings.Repeat("m", simulatorapi.MaxMessageBytes*4)
	handler, _, _ := newTestDisplayHandler(t, funcSource{
		snapshot: func(context.Context, simulatorapi.ReceptionSnapshotRequest) (simulatorapi.ReceptionSnapshot, error) {
			return simulatorapi.ReceptionSnapshot{},
				newSourceError(snapshotOperation, simulatorapi.CategoryUnavailable, nil, long)
		},
	})

	recorder := displayCall(t, handler, http.MethodPost, "/observations", `{"stationIds":["alpha"]}`, nil)
	require.Equal(t, http.StatusServiceUnavailable, recorder.Code)
	require.LessOrEqual(t, recorder.Body.Len(), simulatorapi.MinResponseBytes)
}
