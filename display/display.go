package display

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/miroslav-matejovsky/adsb-testbench/simulatorapi"
)

// Display turns raw received evidence into published aircraft tracks.
//
// It owns no goroutine and polls nothing: a host decides when to refresh.
// One refresh fetches a complete bounded raw snapshot, validates it, decodes
// it, and publishes the result atomically. Nothing is published partially and
// no decoded field is ever merged across refreshes: every refresh rebuilds
// identification, altitude, velocity, and CPR state from the evidence the
// source still retains. A late joiner and a continuously connected display
// therefore see the same tracks.
//
// At most one successful snapshot is retained, together with the exact
// selection and lifetimes that produced it. A failed refresh returns an error
// and, when the selection is unchanged and no run change was detected, the
// last good data explicitly marked stale. A detected run change clears the
// previous run's tracks, cursors, and fallback before the new run's payload is
// decoded, so corrupt new-run data can never fall back to an obsolete run.
type Display struct {
	config Config
	source ObservationSource
	now    func() time.Time

	// gate admits one refresh or history call at a time. It is a channel
	// rather than a mutex so admission itself can honor cancellation.
	gate chan struct{}

	mu    sync.Mutex
	state state
}

// state is everything a display publishes. It holds at most one snapshot.
type state struct {
	status       Status
	runID        string
	selection    []string
	observations *simulatorapi.ObservationSnapshot
	sourceNow    time.Time
	latest       map[string]uint64
	updatedAt    time.Time
	lastError    *SourceError
}

// New validates the configuration and source and returns a display. It starts
// no background work.
func New(config Config, source ObservationSource) (*Display, error) {
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("create display: %w", err)
	}
	if source == nil {
		return nil, fmt.Errorf("%w: display observation source is nil", simulatorapi.CategoryInvalid)
	}
	return newDisplay(config, source, time.Now), nil
}

// newDisplay builds a display with an injected clock. The clock is used only
// for the diagnostic local update timestamp, never to age a received field.
func newDisplay(config Config, source ObservationSource, now func() time.Time) *Display {
	display := &Display{
		config: config, source: source, now: now,
		gate:  make(chan struct{}, 1),
		state: state{status: StatusUnavailable},
	}
	display.gate <- struct{}{}
	return display
}

// Refresh fetches, validates, and decodes one complete raw snapshot for an
// explicit station selection, and publishes it on success.
//
// It returns the display snapshot it produced together with any failure. A
// failure may still carry explicitly stale observations, so a caller must not
// drop the error. A successful refresh is never reported for a failed fetch.
func (d *Display) Refresh(ctx context.Context, stationIDs []string) (Snapshot, error) {
	release, err := d.admit(ctx)
	if err != nil {
		return d.Snapshot(), newSourceError(snapshotOperation, simulatorapi.CategoryUnavailable, err, err.Error())
	}
	defer release()

	selection := normalizeSelection(stationIDs)
	request := simulatorapi.ReceptionSnapshotRequest{StationIDs: selection}
	if err := validateSnapshotRequest(request); err != nil {
		return d.Snapshot(), invalidRequestError(snapshotOperation, err)
	}

	raw, err := d.source.ReceptionSnapshot(ctx, request)
	if err != nil {
		failure := asSourceError(snapshotOperation, err)
		d.invalidateOnRunChange(failure.RunID)
		return d.fail(selection, failure)
	}

	// Hosts may supply their own source, so a refresh validates before it
	// publishes rather than trusting an adapter to have done so.
	validated, runID, err := validateSnapshot(request, raw)
	if err != nil {
		failure := invalidPayloadError(snapshotOperation, runID, err)
		d.invalidateOnRunChange(failure.RunID)
		return d.fail(selection, failure)
	}
	d.invalidateOnRunChange(runID)

	if err := d.checkProgress(runID, validated); err != nil {
		failure := invalidPayloadError(snapshotOperation, runID, err)
		return d.fail(selection, failure)
	}

	observations, err := decodeSnapshot(validated, d.lifetimes())
	if err != nil {
		return d.fail(selection, invalidPayloadError(snapshotOperation, runID, err))
	}
	return d.publish(runID, selection, validated, observations), nil
}

// ReceptionHistory passes one inspection request through the same source and
// validators. A gap is a successful, incomplete history response, not a
// failed read. The caller owns its cursor; the display caches no page.
func (d *Display) ReceptionHistory(ctx context.Context, request simulatorapi.HistoryRequest) (simulatorapi.ReceptionPage, error) {
	release, err := d.admit(ctx)
	if err != nil {
		return simulatorapi.ReceptionPage{},
			newSourceError(historyOperation, simulatorapi.CategoryUnavailable, err, err.Error())
	}
	defer release()

	if err := validateHistoryRequest(request); err != nil {
		return simulatorapi.ReceptionPage{}, invalidRequestError(historyOperation, err)
	}
	page, err := d.source.ReceptionHistory(ctx, copyHistoryRequest(request))
	if err != nil {
		failure := asSourceError(historyOperation, err)
		d.invalidateOnRunChange(failure.RunID)
		return simulatorapi.ReceptionPage{}, failure
	}
	if runID, err := validatePage(request, page); err != nil {
		failure := invalidPayloadError(historyOperation, runID, err)
		d.invalidateOnRunChange(failure.RunID)
		return simulatorapi.ReceptionPage{}, failure
	}
	d.invalidateOnRunChange(page.RunID)
	return page, nil
}

// Snapshot returns the currently published state, detached from the display.
func (d *Display) Snapshot() Snapshot {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.snapshotLocked()
}

func (d *Display) snapshotLocked() Snapshot {
	snapshot := Snapshot{Status: d.state.status, Error: failureOf(d.state.lastError)}
	if !d.state.updatedAt.IsZero() {
		updated := simulatorapi.FormatTime(d.state.updatedAt)
		snapshot.LastUpdatedAt = &updated
	}
	if d.state.status != StatusUnavailable {
		snapshot.Observations = cloneObservations(d.state.observations)
	}
	return snapshot
}

// admit takes the single admission token, honoring cancellation while it
// waits. The returned function returns the token.
func (d *Display) admit(ctx context.Context) (func(), error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case token := <-d.gate:
		return func() { d.gate <- token }, nil
	}
}

// lifetimes returns the configured field lifetimes.
func (d *Display) lifetimes() lifetimes {
	return lifetimes{
		identity: d.config.IdentityExpiry, position: d.config.PositionExpiry,
		altitude: d.config.AltitudeExpiry, velocity: d.config.VelocityExpiry,
	}
}

// invalidateOnRunChange clears every trace of a previous run once a different
// run identifier has been validated. An unknown identity changes nothing.
func (d *Display) invalidateOnRunChange(runID string) {
	if runID == "" {
		return
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.state.runID == "" || d.state.runID == runID {
		return
	}
	d.state = state{status: StatusUnavailable, updatedAt: d.state.updatedAt}
}

// checkProgress rejects a same-run snapshot that moved backwards. Virtual
// time may not regress, and no station's latest retained sequence may regress
// for a station both the previous and the current selection contain.
func (d *Display) checkProgress(runID string, validated snapshot) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.state.runID != runID || d.state.observations == nil {
		return nil
	}
	if validated.now.Before(d.state.sourceNow) {
		return fmt.Errorf("source virtual time %s regressed from %s within run %q",
			simulatorapi.FormatTime(validated.now), simulatorapi.FormatTime(d.state.sourceNow), runID)
	}
	for _, entry := range validated.retention {
		previous, ok := d.state.latest[entry.StationID]
		if !ok {
			continue
		}
		current, err := simulatorapi.ParseUint64(entry.LatestSequence)
		if err != nil {
			return fmt.Errorf("station %q latestSequence: %w", entry.StationID, err)
		}
		if current < previous {
			return fmt.Errorf("station %q latest sequence %d regressed from %d within run %q",
				entry.StationID, current, previous, runID)
		}
	}
	return nil
}

// publish replaces the published state atomically after validation and
// decoding both succeeded.
func (d *Display) publish(runID string, selection []string, validated snapshot,
	observations simulatorapi.ObservationSnapshot,
) Snapshot {
	latest := make(map[string]uint64, len(validated.retention))
	for _, entry := range validated.retention {
		value, err := simulatorapi.ParseUint64(entry.LatestSequence)
		if err != nil {
			continue
		}
		latest[entry.StationID] = value
	}

	d.mu.Lock()
	defer d.mu.Unlock()
	d.state = state{
		status: StatusFresh, runID: runID, selection: selection,
		observations: &observations, sourceNow: validated.now,
		latest: latest, updatedAt: d.now().UTC(),
	}
	return d.snapshotLocked()
}

// fail records a failure and returns the snapshot a caller should show.
//
// Last good data is retained only for the same selection and only while the
// run is unchanged. A failed selection change shows no other selection's
// aircraft.
func (d *Display) fail(selection []string, failure *SourceError) (Snapshot, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.state.lastError = failure
	switch {
	case d.state.observations == nil:
		d.state.status = StatusUnavailable
	case sameSelection(d.state.selection, selection):
		d.state.status = StatusStale
	default:
		d.state.status = StatusUnavailable
	}
	return d.snapshotLocked(), failure
}

// asSourceError recovers a source failure, categorizing anything a custom
// source returned without one.
func asSourceError(operation string, err error) *SourceError {
	var failure *SourceError
	if errors.As(err, &failure) {
		return failure
	}
	return sourceErrorFrom(operation, err)
}
