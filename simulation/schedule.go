package simulation

import (
	"math/rand/v2"
	"time"
)

// Randomized emission intervals in whole milliseconds, drawn uniformly from
// the inclusive integer sets. The ranges follow ICAO Annex 10 Volume IV as
// summarized in the codec documentation; the one-millisecond grid is this
// engine's explicit simplification.
const (
	identificationIntervalLowMS  = 4800
	identificationIntervalHighMS = 5200
	positionIntervalLowMS        = 400
	positionIntervalHighMS       = 600
	velocityIntervalLowMS        = 400
	velocityIntervalHighMS       = 600
)

// families lists the message families in creation and tie-break order.
var families = [...]MessageKind{IdentificationMessage, PositionMessage, VelocityMessage}

// intervalBounds returns the inclusive millisecond bounds of a family.
func intervalBounds(kind MessageKind) (low, high uint64) {
	switch kind {
	case IdentificationMessage:
		return identificationIntervalLowMS, identificationIntervalHighMS
	case PositionMessage:
		return positionIntervalLowMS, positionIntervalHighMS
	default:
		return velocityIntervalLowMS, velocityIntervalHighMS
	}
}

// source returns the family's own generator. Each family consumes only its
// own stream, so the number of events in one family never shifts another.
func (a *aircraft) source(kind MessageKind) *rand.PCG {
	switch kind {
	case IdentificationMessage:
		return &a.identificationRNG
	case PositionMessage:
		return &a.positionRNG
	default:
		return &a.velocityRNG
	}
}

// deadline returns the family's next scheduled emission time, as elapsed time
// since the run start.
func (a *aircraft) deadline(kind MessageKind) time.Duration {
	switch kind {
	case IdentificationMessage:
		return a.identificationDeadline
	case PositionMessage:
		return a.positionDeadline
	default:
		return a.velocityDeadline
	}
}

// setDeadline replaces the family's next scheduled emission time.
func (a *aircraft) setDeadline(kind MessageKind, at time.Duration) {
	switch kind {
	case IdentificationMessage:
		a.identificationDeadline = at
	case PositionMessage:
		a.positionDeadline = at
	default:
		a.velocityDeadline = at
	}
}

// drawInterval consumes exactly one value from the family's stream.
func (a *aircraft) drawInterval(kind MessageKind) time.Duration {
	low, high := intervalBounds(kind)
	return time.Duration(sampleInterval(rand.New(a.source(kind)), low, high)) * time.Millisecond
}

// scheduleFirst sets each family's first deadline one randomized interval
// after creation. Creation itself has already emitted one report per family,
// so the first scheduled position report is the odd half of the CPR pair.
func (a *aircraft) scheduleFirst() error {
	for _, kind := range families {
		at, err := addDuration(a.createdAt, a.drawInterval(kind))
		if err != nil {
			return err
		}
		a.setDeadline(kind, at)
	}
	a.nextPositionOdd = true
	return nil
}

// rescheduleAfter moves one family to its next deadline. The new deadline is
// measured from the previous deadline, never from the end of the caller's
// batch, so splitting a duration cannot shift the schedule.
func (a *aircraft) rescheduleAfter(kind MessageKind) error {
	at, err := addDuration(a.deadline(kind), a.drawInterval(kind))
	if err != nil {
		return err
	}
	a.setDeadline(kind, at)
	return nil
}

// event identifies one due emission.
type event struct {
	index int // Position in the fleet slice, which is creation order.
	kind  MessageKind
	at    time.Duration
}

// nextEvent returns the earliest emission due at or before limit.
// Ties break by deadline, then creation ordinal, then family order, which is
// exactly the iteration order of the fleet slice and the families array.
func nextEvent(fleet []aircraft, limit time.Duration) (event, bool) {
	var best event
	found := false
	for i := range fleet {
		for _, kind := range families {
			at := fleet[i].deadline(kind)
			if at > limit {
				continue
			}
			if !found || at < best.at {
				best = event{index: i, kind: kind, at: at}
				found = true
			}
		}
	}
	return best, found
}
