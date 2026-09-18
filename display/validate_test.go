package display

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/miroslav-matejovsky/adsb-testbench/simulatorapi"
)

// validFixtureSnapshot returns a complete two-station snapshot.
func validFixtureSnapshot(t *testing.T) simulatorapi.ReceptionSnapshot {
	t.Helper()

	alpha := fixtureStation("alpha", 1, fixtureStart)
	bravo := fixtureStation("bravo", 2, fixtureStart)
	frame := identificationFrame(t, fixtureICAO, "TB00ABCD")
	at := fixtureStart.Add(time.Second)

	return fixtureSnapshot(fixtureStart.Add(2*time.Second), []string{"alpha", "bravo"},
		[]simulatorapi.Reception{
			fixtureReception(1, 1, alpha, fixtureICAO, identificationKind, at, frame),
			fixtureReception(1, 1, bravo, fixtureICAO, identificationKind, at, frame),
			fixtureReception(2, 2, alpha, fixtureICAO, identificationKind, at, frame),
		})
}

func TestValidateSnapshotAcceptsACompleteFixture(t *testing.T) {
	t.Parallel()

	raw := validFixtureSnapshot(t)
	validated, runID, err := validateSnapshot(
		simulatorapi.ReceptionSnapshotRequest{StationIDs: []string{"bravo", "alpha"}}, raw)
	require.NoError(t, err)
	require.Equal(t, fixtureRunID, runID)
	require.Equal(t, fixtureStart.Add(2*time.Second), validated.now)
	require.Len(t, validated.records, 3)
	require.Equal(t, []string{"alpha", "bravo"}, validated.stationIDs)
}

func TestValidateSnapshotRejectsInvalidEnvelopes(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		edit func(*simulatorapi.ReceptionSnapshot)
	}{
		{name: "empty run", edit: func(s *simulatorapi.ReceptionSnapshot) { s.RunID = "" }},
		{name: "malformed now", edit: func(s *simulatorapi.ReceptionSnapshot) { s.Now = "not a time" }},
		{name: "offset now", edit: func(s *simulatorapi.ReceptionSnapshot) { s.Now = "2024-03-05T12:00:00+01:00" }},
		{name: "null selection", edit: func(s *simulatorapi.ReceptionSnapshot) { s.StationIDs = nil }},
		{name: "null retention", edit: func(s *simulatorapi.ReceptionSnapshot) { s.Retention = nil }},
		{name: "null records", edit: func(s *simulatorapi.ReceptionSnapshot) { s.Records = nil }},
		{name: "unsorted selection", edit: func(s *simulatorapi.ReceptionSnapshot) {
			s.StationIDs = []string{"bravo", "alpha"}
		}},
		{name: "extra station", edit: func(s *simulatorapi.ReceptionSnapshot) {
			s.StationIDs = append(s.StationIDs, "charlie")
		}},
		{name: "retention count mismatch", edit: func(s *simulatorapi.ReceptionSnapshot) {
			s.Retention = s.Retention[:1]
		}},
		{name: "retention order mismatch", edit: func(s *simulatorapi.ReceptionSnapshot) {
			s.Retention[0].StationID, s.Retention[1].StationID = s.Retention[1].StationID, s.Retention[0].StationID
		}},
		{name: "retention limit zero", edit: func(s *simulatorapi.ReceptionSnapshot) { s.Retention[0].Limit = 0 }},
		{name: "invalid station id", edit: func(s *simulatorapi.ReceptionSnapshot) {
			s.StationIDs[0] = "bad id"
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			raw := validFixtureSnapshot(t)
			request := simulatorapi.ReceptionSnapshotRequest{StationIDs: append([]string(nil), raw.StationIDs...)}
			tc.edit(&raw)

			_, runID, err := validateSnapshot(request, raw)
			require.Error(t, err)
			require.Empty(t, runID, "an invalid envelope yields no validated run identity")
		})
	}
}

func TestValidateSnapshotRejectsInvalidRecordsButKeepsTheRunIdentity(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		edit func(*simulatorapi.ReceptionSnapshot)
	}{
		{name: "zero sequence", edit: func(s *simulatorapi.ReceptionSnapshot) { s.Records[0].Sequence = "0" }},
		{name: "non canonical sequence", edit: func(s *simulatorapi.ReceptionSnapshot) { s.Records[0].Sequence = "01" }},
		{name: "zero transmission", edit: func(s *simulatorapi.ReceptionSnapshot) {
			s.Records[0].TransmissionSequence = "0"
		}},
		{name: "lowercase frame", edit: func(s *simulatorapi.ReceptionSnapshot) {
			s.Records[0].Frame = "8d4840d6202cc371c32ce0576098"
		}},
		{name: "short frame", edit: func(s *simulatorapi.ReceptionSnapshot) { s.Records[0].Frame = "8D48" }},
		{name: "invalid icao", edit: func(s *simulatorapi.ReceptionSnapshot) { s.Records[0].ICAO = "FFFFFF" }},
		{name: "unknown kind", edit: func(s *simulatorapi.ReceptionSnapshot) { s.Records[0].Kind = "surface" }},
		{name: "future record", edit: func(s *simulatorapi.ReceptionSnapshot) {
			s.Records[0].Timestamp = simulatorapi.FormatTime(fixtureStart.Add(time.Hour))
		}},
		{name: "unselected station", edit: func(s *simulatorapi.ReceptionSnapshot) {
			s.Records[0].StationID = "charlie"
			s.Records[0].Receiver.ID = "charlie"
		}},
		{name: "receiver id mismatch", edit: func(s *simulatorapi.ReceptionSnapshot) {
			s.Records[0].Receiver.ID = "bravo"
		}},
		{name: "receiver revision mismatch", edit: func(s *simulatorapi.ReceptionSnapshot) {
			s.Records[0].Receiver.Revision = "9"
		}},
		{name: "receiver created after reception", edit: func(s *simulatorapi.ReceptionSnapshot) {
			s.Records[0].Receiver.CreatedAt = simulatorapi.FormatTime(fixtureStart.Add(time.Minute))
		}},
		{name: "receiver setting out of domain", edit: func(s *simulatorapi.ReceptionSnapshot) {
			s.Records[0].Receiver.SensitivityDBm = 5
		}},
		{name: "negative slant range", edit: func(s *simulatorapi.ReceptionSnapshot) {
			s.Records[0].SlantRangeNauticalMiles = -1
		}},
		{name: "sequence gap within a station", edit: func(s *simulatorapi.ReceptionSnapshot) {
			s.Records[2].Sequence = "5"
			s.Retention[0].LatestSequence = "5"
		}},
		{name: "duplicate station copy", edit: func(s *simulatorapi.ReceptionSnapshot) {
			s.Records[2].TransmissionSequence = "1"
		}},
		{name: "copies disagree on frame", edit: func(s *simulatorapi.ReceptionSnapshot) {
			s.Records[1].Frame = simulatorapi.FormatFrame(identificationFrame(t, fixtureICAO, "TBOTHER1"))
		}},
		{name: "contradictory receiver snapshot", edit: func(s *simulatorapi.ReceptionSnapshot) {
			s.Records[2].Receiver.AntennaGainDBi = 30
		}},
		{name: "retention bounds disagree", edit: func(s *simulatorapi.ReceptionSnapshot) {
			s.Retention[0].LatestSequence = "9"
		}},
		{name: "truncation contradicts bounds", edit: func(s *simulatorapi.ReceptionSnapshot) {
			s.Retention[0].Truncated = true
		}},
		{name: "empty station claims bounds", edit: func(s *simulatorapi.ReceptionSnapshot) {
			s.Retention[1].OldestSequence = "1"
			s.Retention[1].LatestSequence = "1"
			s.Records = s.Records[:1]
			s.Retention[0].LatestSequence = "1"
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			raw := validFixtureSnapshot(t)
			request := simulatorapi.ReceptionSnapshotRequest{StationIDs: append([]string(nil), raw.StationIDs...)}
			tc.edit(&raw)

			validated, runID, err := validateSnapshot(request, raw)
			require.Error(t, err)
			require.Equal(t, fixtureRunID, runID,
				"a valid envelope still reports its run identity")
			require.Equal(t, snapshot{}, validated)
		})
	}
}

func TestValidateSnapshotRequestChecksSelection(t *testing.T) {
	t.Parallel()

	require.NoError(t, validateSnapshotRequest(simulatorapi.ReceptionSnapshotRequest{StationIDs: []string{}}))
	require.NoError(t, validateSnapshotRequest(
		simulatorapi.ReceptionSnapshotRequest{StationIDs: []string{"alpha", "bravo"}}))

	require.Error(t, validateSnapshotRequest(simulatorapi.ReceptionSnapshotRequest{}))
	require.Error(t, validateSnapshotRequest(
		simulatorapi.ReceptionSnapshotRequest{StationIDs: []string{"alpha", "alpha"}}))
	require.Error(t, validateSnapshotRequest(
		simulatorapi.ReceptionSnapshotRequest{StationIDs: []string{"bad id"}}))

	tooMany := make([]string, 0, maxSelectedStations+1)
	for i := range maxSelectedStations + 1 {
		tooMany = append(tooMany, string(rune('a'+i)))
	}
	require.Error(t, validateSnapshotRequest(simulatorapi.ReceptionSnapshotRequest{StationIDs: tooMany}))
}

func TestValidateHistoryRequestChecksLimitsAndCursors(t *testing.T) {
	t.Parallel()

	require.NoError(t, validateHistoryRequest(simulatorapi.HistoryRequest{StationID: "alpha", Limit: 1}))
	require.NoError(t, validateHistoryRequest(simulatorapi.HistoryRequest{
		StationID: "alpha", Limit: maxStationRecords,
		Cursor: &simulatorapi.ReceptionCursor{RunID: fixtureRunID, StationID: "alpha", AfterSequence: "0"},
	}))

	cases := []struct {
		name    string
		request simulatorapi.HistoryRequest
	}{
		{name: "zero limit", request: simulatorapi.HistoryRequest{StationID: "alpha", Limit: 0}},
		{name: "limit above retention", request: simulatorapi.HistoryRequest{
			StationID: "alpha", Limit: maxStationRecords + 1,
		}},
		{name: "invalid station", request: simulatorapi.HistoryRequest{StationID: "bad id", Limit: 1}},
		{name: "cursor station mismatch", request: simulatorapi.HistoryRequest{
			StationID: "alpha", Limit: 1,
			Cursor: &simulatorapi.ReceptionCursor{RunID: fixtureRunID, StationID: "bravo", AfterSequence: "0"},
		}},
		{name: "cursor run empty", request: simulatorapi.HistoryRequest{
			StationID: "alpha", Limit: 1,
			Cursor: &simulatorapi.ReceptionCursor{StationID: "alpha", AfterSequence: "0"},
		}},
		{name: "cursor sequence malformed", request: simulatorapi.HistoryRequest{
			StationID: "alpha", Limit: 1,
			Cursor: &simulatorapi.ReceptionCursor{RunID: fixtureRunID, StationID: "alpha", AfterSequence: "-1"},
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			require.Error(t, validateHistoryRequest(tc.request))
		})
	}
}

// fixturePage returns a complete first page of one station's history.
func fixturePage(t *testing.T) (simulatorapi.HistoryRequest, simulatorapi.ReceptionPage) {
	t.Helper()

	alpha := fixtureStation("alpha", 1, fixtureStart)
	frame := identificationFrame(t, fixtureICAO, "TB00ABCD")
	at := fixtureStart.Add(time.Second)

	request := simulatorapi.HistoryRequest{StationID: "alpha", Limit: 2}
	page := simulatorapi.ReceptionPage{
		RunID: fixtureRunID, StationID: "alpha",
		Now: simulatorapi.FormatTime(fixtureStart.Add(2 * time.Second)),
		Records: []simulatorapi.Reception{
			fixtureReception(1, 1, alpha, fixtureICAO, identificationKind, at, frame),
			fixtureReception(2, 2, alpha, fixtureICAO, identificationKind, at, frame),
		},
		OldestSequence: "1", LatestSequence: "3",
		NextCursor: simulatorapi.ReceptionCursor{
			RunID: fixtureRunID, StationID: "alpha", AfterSequence: "2",
		},
		Gap: false, HasMore: true, RetentionLimit: maxStationRecords,
	}
	return request, page
}

func TestValidatePageAcceptsACompleteFixture(t *testing.T) {
	t.Parallel()

	request, page := fixturePage(t)
	runID, err := validatePage(request, page)
	require.NoError(t, err)
	require.Equal(t, fixtureRunID, runID)
}

func TestValidatePageReportsAGapOnlyWithACursor(t *testing.T) {
	t.Parallel()

	request, page := fixturePage(t)
	request.Cursor = &simulatorapi.ReceptionCursor{
		RunID: fixtureRunID, StationID: "alpha", AfterSequence: "0",
	}
	page.OldestSequence = "5"
	page.Records = []simulatorapi.Reception{}
	page.LatestSequence = "9"
	page.NextCursor.AfterSequence = "0"
	page.HasMore = false
	page.Gap = true

	runID, err := validatePage(request, page)
	require.NoError(t, err)
	require.Equal(t, fixtureRunID, runID)

	page.Gap = false
	_, err = validatePage(request, page)
	require.ErrorContains(t, err, "gap")
}

func TestValidatePageRejectsInconsistentPages(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		edit func(*simulatorapi.HistoryRequest, *simulatorapi.ReceptionPage)
	}{
		{name: "empty run", edit: func(_ *simulatorapi.HistoryRequest, p *simulatorapi.ReceptionPage) {
			p.RunID = ""
		}},
		{name: "station mismatch", edit: func(_ *simulatorapi.HistoryRequest, p *simulatorapi.ReceptionPage) {
			p.StationID = "bravo"
		}},
		{name: "null records", edit: func(_ *simulatorapi.HistoryRequest, p *simulatorapi.ReceptionPage) {
			p.Records = nil
		}},
		{name: "above requested limit", edit: func(r *simulatorapi.HistoryRequest, _ *simulatorapi.ReceptionPage) {
			r.Limit = 1
		}},
		{name: "record outside bounds", edit: func(_ *simulatorapi.HistoryRequest, p *simulatorapi.ReceptionPage) {
			p.OldestSequence = "2"
		}},
		{name: "sequence gap", edit: func(_ *simulatorapi.HistoryRequest, p *simulatorapi.ReceptionPage) {
			p.Records[1].Sequence = "3"
			p.NextCursor.AfterSequence = "3"
		}},
		{name: "foreign record station", edit: func(_ *simulatorapi.HistoryRequest, p *simulatorapi.ReceptionPage) {
			p.Records[0].StationID = "bravo"
			p.Records[0].Receiver.ID = "bravo"
		}},
		{name: "cursor run mismatch", edit: func(_ *simulatorapi.HistoryRequest, p *simulatorapi.ReceptionPage) {
			p.NextCursor.RunID = "other"
		}},
		{name: "cursor does not name the last record", edit: func(_ *simulatorapi.HistoryRequest, p *simulatorapi.ReceptionPage) {
			p.NextCursor.AfterSequence = "1"
		}},
		{name: "bounds inverted", edit: func(_ *simulatorapi.HistoryRequest, p *simulatorapi.ReceptionPage) {
			p.OldestSequence = "9"
		}},
		{name: "retention limit zero", edit: func(_ *simulatorapi.HistoryRequest, p *simulatorapi.ReceptionPage) {
			p.RetentionLimit = 0
		}},
		{name: "empty page advances its cursor", edit: func(_ *simulatorapi.HistoryRequest, p *simulatorapi.ReceptionPage) {
			p.Records = []simulatorapi.Reception{}
			p.NextCursor.AfterSequence = "2"
			p.HasMore = false
		}},
		{name: "empty page claims more", edit: func(_ *simulatorapi.HistoryRequest, p *simulatorapi.ReceptionPage) {
			p.Records = []simulatorapi.Reception{}
			p.NextCursor.AfterSequence = "0"
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			request, page := fixturePage(t)
			tc.edit(&request, &page)
			_, err := validatePage(request, page)
			require.Error(t, err)
		})
	}
}
