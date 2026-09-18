package display

import (
	"errors"
	"fmt"
	"math"
	"sort"
	"time"

	"github.com/miroslav-matejovsky/adsb-testbench/internal/adsb"
	"github.com/miroslav-matejovsky/adsb-testbench/simulatorapi"
)

// lifetimes are the independent virtual-time field lifetimes one decode
// applies. They come from explicit display configuration.
type lifetimes struct {
	identity time.Duration
	position time.Duration
	altitude time.Duration
	velocity time.Duration
}

// track accumulates one aircraft's received state while records are replayed
// in transmission order. Even and odd hold the latest CPR halves, which are
// usable only within this snapshot: nothing survives into the next refresh.
type track struct {
	icao           uint32
	lastReceivedAt time.Time
	identity       *simulatorapi.IdentityObservation
	position       *simulatorapi.PositionObservation
	altitude       *simulatorapi.AltitudeObservation
	velocity       *simulatorapi.VelocityObservation
	identityAt     time.Time
	positionAt     time.Time
	altitudeAt     time.Time
	velocityAt     time.Time
	even           *positionSample
	odd            *positionSample
}

// positionSample is one received CPR half with the evidence that carried it.
type positionSample struct {
	sample   adsb.PositionSample
	evidence simulatorapi.Evidence
}

// decodeSnapshot rebuilds received aircraft state from validated raw
// evidence alone.
//
// Every refresh starts from nothing: identification, altitude, velocity, and
// CPR state are accumulated from the retained records of this snapshot only.
// A late joiner and a continuously connected display therefore see the same
// tracks, and no evicted CPR half or expired field can survive in a cache.
//
// A frame that does not decode, or whose decoded address or family disagrees
// with its metadata, fails the whole snapshot. An unusable CPR pair is not a
// failure: it leaves the previously accepted fix in place until that fix
// expires.
func decodeSnapshot(source snapshot, expiry lifetimes) (simulatorapi.ObservationSnapshot, error) {
	evidence := groupEvidence(source.records)
	tracks := make(map[uint32]*track, len(evidence))

	for _, item := range evidence {
		state := tracks[item.icao]
		if state == nil {
			state = &track{icao: item.icao}
			tracks[item.icao] = state
		}
		if err := state.accumulate(item); err != nil {
			return simulatorapi.ObservationSnapshot{}, err
		}
	}

	aircraft := make([]simulatorapi.Aircraft, 0, len(tracks))
	for _, state := range tracks {
		if observed, ok := state.observe(source.now, expiry); ok {
			aircraft = append(aircraft, observed)
		}
	}
	sort.Slice(aircraft, func(i, j int) bool { return aircraft[i].ICAO < aircraft[j].ICAO })

	return simulatorapi.ObservationSnapshot{
		RunID:      source.runID,
		Now:        simulatorapi.FormatTime(source.now),
		StationIDs: append([]string{}, source.stationIDs...),
		Retention:  append([]simulatorapi.StationRetention{}, source.retention...),
		Expiry: simulatorapi.ObservationExpiry{
			IdentityNanoseconds: simulatorapi.FormatDuration(expiry.identity),
			PositionNanoseconds: simulatorapi.FormatDuration(expiry.position),
			AltitudeNanoseconds: simulatorapi.FormatDuration(expiry.altitude),
			VelocityNanoseconds: simulatorapi.FormatDuration(expiry.velocity),
		},
		Aircraft: aircraft,
	}, nil
}

// transmission is one distinct received frame and every selected receiver
// copy of it, in station order.
type transmission struct {
	sequence uint64
	icao     uint32
	kind     string
	at       time.Time
	frame    [simulatorapi.FrameBytes]byte
	evidence simulatorapi.Evidence
}

// groupEvidence collects receiver copies of each transmission, ordered by
// transmission sequence, with receptions ordered by station identifier.
func groupEvidence(records []reception) []transmission {
	byTransmission := make(map[uint64]*transmission, len(records))
	order := make([]uint64, 0, len(records))
	for _, record := range records {
		item := byTransmission[record.transmissionSequence]
		if item == nil {
			item = &transmission{
				sequence: record.transmissionSequence, icao: record.icao, kind: record.kind,
				at: record.timestamp, frame: record.frame,
				evidence: simulatorapi.Evidence{
					TransmissionSequence: record.record.TransmissionSequence,
					ICAO:                 record.record.ICAO,
					Kind:                 record.record.Kind,
					Timestamp:            record.record.Timestamp,
					Frame:                record.record.Frame,
					Receptions:           []simulatorapi.Reception{},
				},
			}
			byTransmission[record.transmissionSequence] = item
			order = append(order, record.transmissionSequence)
		}
		item.evidence.Receptions = append(item.evidence.Receptions, record.record)
	}

	sort.Slice(order, func(i, j int) bool { return order[i] < order[j] })
	grouped := make([]transmission, 0, len(order))
	for _, sequence := range order {
		item := byTransmission[sequence]
		sort.SliceStable(item.evidence.Receptions, func(i, j int) bool {
			return item.evidence.Receptions[i].StationID < item.evidence.Receptions[j].StationID
		})
		grouped = append(grouped, *item)
	}
	return grouped
}

// accumulate decodes one transmission into this track.
func (t *track) accumulate(item transmission) error {
	message, err := adsb.Decode(item.frame[:])
	if err != nil {
		return fmt.Errorf("decode received transmission %d: %w", item.sequence, err)
	}
	if message.Header.ICAO != item.icao {
		return fmt.Errorf("transmission %d metadata address %06X differs from the decoded %06X",
			item.sequence, item.icao, message.Header.ICAO)
	}
	if !matchesKind(message, item.kind) {
		return fmt.Errorf("transmission %d metadata kind %q differs from the decoded payload",
			item.sequence, item.kind)
	}
	if item.at.After(t.lastReceivedAt) {
		t.lastReceivedAt = item.at
	}

	switch item.kind {
	case identificationKind:
		t.identity = &simulatorapi.IdentityObservation{
			Callsign: message.Identification.Callsign, ObservedAt: item.evidence.Timestamp,
			Evidence: item.evidence,
		}
		t.identityAt = item.at
	case positionKind:
		if err := t.accumulatePosition(item, message); err != nil {
			return err
		}
	case velocityKind:
		t.velocity = decodeVelocity(*message.Velocity, item.evidence)
		t.velocityAt = item.at
	}
	return nil
}

// accumulatePosition records altitude and the even or odd CPR half, and
// pairs them globally when both are available.
func (t *track) accumulatePosition(item transmission, message adsb.Message) error {
	if message.Position.AltitudeFeet == nil {
		// A received position without altitude replaces a known altitude.
		t.altitude = nil
		t.altitudeAt = time.Time{}
	} else {
		t.altitude = &simulatorapi.AltitudeObservation{
			Feet: *message.Position.AltitudeFeet, ObservedAt: item.evidence.Timestamp,
			Evidence: item.evidence,
		}
		t.altitudeAt = item.at
	}

	half := &positionSample{
		sample:   adsb.PositionSample{Frame: adsb.Frame(item.frame), At: item.at},
		evidence: item.evidence,
	}
	if message.Position.CPR.Odd {
		t.odd = half
	} else {
		t.even = half
	}
	if t.even == nil || t.odd == nil {
		return nil
	}

	fix, err := adsb.DecodeGlobal(t.even.sample, t.odd.sample, item.at)
	if errors.Is(err, adsb.ErrCPR) {
		// An unusable pair leaves the last accepted fix until it expires.
		return nil
	}
	if err != nil {
		return fmt.Errorf("decode CPR at transmission %d: %w", item.sequence, err)
	}
	t.position = &simulatorapi.PositionObservation{
		LatitudeDegrees: fix.Coordinates.Latitude, LongitudeDegrees: fix.Coordinates.Longitude,
		ObservedAt: simulatorapi.FormatTime(fix.At),
		Evidence:   []simulatorapi.Evidence{t.even.evidence, t.odd.evidence},
	}
	t.positionAt = fix.At
	return nil
}

// observe applies independent field expiry and returns the aircraft, or
// false when no field survives.
func (t *track) observe(now time.Time, expiry lifetimes) (simulatorapi.Aircraft, bool) {
	if !fresh(t.identityAt, now, expiry.identity) {
		t.identity = nil
	}
	if !fresh(t.positionAt, now, expiry.position) {
		t.position = nil
	}
	if !fresh(t.altitudeAt, now, expiry.altitude) {
		t.altitude = nil
	}
	if !fresh(t.velocityAt, now, expiry.velocity) {
		t.velocity = nil
	}
	if t.identity == nil && t.position == nil && t.altitude == nil && t.velocity == nil {
		return simulatorapi.Aircraft{}, false
	}
	return simulatorapi.Aircraft{
		ICAO:               simulatorapi.FormatICAO(t.icao),
		LastReceivedAt:     simulatorapi.FormatTime(t.lastReceivedAt),
		Identity:           t.identity,
		Position:           t.position,
		BarometricAltitude: t.altitude,
		Velocity:           t.velocity,
	}, true
}

// fresh reports whether a field observed at observedAt is still displayed.
// A field is fresh at exactly its lifetime and absent once age exceeds it.
func fresh(observedAt, now time.Time, lifetime time.Duration) bool {
	if observedAt.IsZero() {
		return false
	}
	return !observedAt.After(now) && now.Sub(observedAt) <= lifetime
}

// matchesKind reports whether a decoded payload is the claimed family.
func matchesKind(message adsb.Message, kind string) bool {
	switch kind {
	case identificationKind:
		return message.Identification != nil
	case positionKind:
		return message.Position != nil
	case velocityKind:
		return message.Velocity != nil
	default:
		return false
	}
}

// decodeVelocity converts one decoded TC19 payload, deriving ground motion
// only from available, in-range east and north components.
func decodeVelocity(value adsb.Velocity, evidence simulatorapi.Evidence) *simulatorapi.VelocityObservation {
	observation := &simulatorapi.VelocityObservation{
		Subtype: value.Subtype, IntentChange: value.IntentChange, IFRCapability: value.IFRCapability,
		NACv: value.NACv, EastKnots: measurement(value.EastKnots), NorthKnots: measurement(value.NorthKnots),
		HeadingDegrees: copyFloat(value.HeadingDegrees), AirspeedKnots: measurement(value.AirspeedKnots),
		TrueAirspeed: value.TrueAirspeed, BarometricVerticalRate: value.BarometricVerticalRate,
		VerticalRateFeetPerMinute: measurement(value.VerticalRateFeetPerMinute),
		GNSSMinusBaroFeet:         measurement(value.GNSSMinusBaroFeet),
		ObservedAt:                evidence.Timestamp, Evidence: evidence,
	}
	east, north := value.EastKnots, value.NorthKnots
	if east == nil || north == nil || east.OverRange || north.OverRange {
		return observation
	}
	speed := math.Hypot(east.Value, north.Value)
	observation.GroundSpeedKnots = &speed
	if speed == 0 {
		// A stationary target has no ground track.
		return observation
	}
	track := math.Atan2(east.Value, north.Value) * 180 / math.Pi
	if track < 0 {
		track += 360
	}
	observation.TrackDegrees = &track
	return observation
}

func measurement(value *adsb.Measurement) *simulatorapi.Measurement {
	if value == nil {
		return nil
	}
	return &simulatorapi.Measurement{Value: value.Value, OverRange: value.OverRange}
}

func copyFloat(value *float64) *float64 {
	if value == nil {
		return nil
	}
	copied := *value
	return &copied
}
