package adsb

import (
	"fmt"
	"math"
	"time"
)

// CPR contains the two 17-bit fractions of an airborne position message.
// Zero fractions are valid coordinates, not unavailable sentinels.
type CPR struct {
	Odd       bool
	Latitude  uint32
	Longitude uint32
}

// Coordinates are latitude [-90,90] and longitude [-180,180] in degrees.
// Decoders normalize longitude to [-180,180).
type Coordinates struct {
	Latitude  float64
	Longitude float64
}

// PositionSample pairs an exact received frame with its caller-supplied time.
// The frame is validated again when used for position reconstruction.
type PositionSample struct {
	Frame Frame
	At    time.Time
}

// Fix is a reconstructed position at the selected frame's time.
// A reference Fix must be trustworthy and belong to the same aircraft.
type Fix struct {
	ICAO        uint32
	Coordinates Coordinates
	At          time.Time
}

// MaxCPRAge is the inclusive maximum age of frames and local references.
// It is a conservative testbench policy, not a guarantee of plausible motion.
const MaxCPRAge = 10 * time.Second

const cprScale = 131072.0

// EncodeCPR encodes a geographic position as even or odd airborne CPR.
// Quantization rounds to the nearest bin, ties upward, with modulo wrapping.
func EncodeCPR(p Coordinates, odd bool) (CPR, error) {
	if err := p.validate(); err != nil {
		return CPR{}, err
	}
	i := int(bit(odd))
	dlat := 360 / float64(60-i)
	dlon := 360 / float64(max(longitudeZones(p.Latitude)-i, 1))
	return CPR{
		Odd:       odd,
		Latitude:  uint32(math.Floor(cprScale*mod(p.Latitude, dlat)/dlat+0.5)) % 131072,
		Longitude: uint32(math.Floor(cprScale*mod(p.Longitude, dlon)/dlon+0.5)) % 131072,
	}, nil
}

// DecodeGlobal combines different-parity frames from the same aircraft.
// Both must be supported airborne positions within MaxCPRAge of now and not
// future-dated. The newest frame supplies the result; even wins timestamp ties.
// Different latitude zones or impossible coordinates return ErrCPR. Callers
// must still check receiver range and track continuity before accepting a fix.
func DecodeGlobal(a, b PositionSample, now time.Time) (Fix, error) {
	am, err := decodeSample(a, now)
	if err != nil {
		return Fix{}, fmt.Errorf("first CPR frame: %w", err)
	}
	bm, err := decodeSample(b, now)
	if err != nil {
		return Fix{}, fmt.Errorf("second CPR frame: %w", err)
	}
	if am.Header.ICAO != bm.Header.ICAO {
		return Fix{}, fmt.Errorf("%w: frames have different aircraft addresses", ErrCPR)
	}
	even, odd := am.Position.CPR, bm.Position.CPR
	evenAt, oddAt := a.At, b.At
	if even.Odd {
		even, odd = odd, even
		evenAt, oddAt = oddAt, evenAt
	}
	if even.Odd || !odd.Odd {
		return Fix{}, fmt.Errorf("%w: need one even and one odd frame", ErrCPR)
	}
	y0, y1 := float64(even.Latitude)/cprScale, float64(odd.Latitude)/cprScale
	j := math.Floor(59*y0 - 60*y1 + 0.5)
	lat0 := latitude(6 * (mod(j, 60) + y0))
	lat1 := latitude((360.0 / 59) * (mod(j, 59) + y1))
	if math.Abs(lat0) > 90 || math.Abs(lat1) > 90 || longitudeZones(lat0) != longitudeZones(lat1) {
		return Fix{}, fmt.Errorf("%w: inconsistent latitude zones", ErrCPR)
	}
	selected, at, lat, i := even, evenAt, lat0, 0
	if oddAt.After(evenAt) {
		selected, at, lat, i = odd, oddAt, lat1, 1
	}
	nl := longitudeZones(lat)
	m := math.Floor((float64(even.Longitude)*float64(nl-1)-float64(odd.Longitude)*float64(nl))/cprScale + 0.5)
	n := float64(max(nl-i, 1))
	lon := longitude((360 / n) * (mod(m, n) + float64(selected.Longitude)/cprScale))
	return Fix{ICAO: am.Header.ICAO, Coordinates: Coordinates{Latitude: lat, Longitude: lon}, At: at}, nil
}

func decodeSample(s PositionSample, now time.Time) (Message, error) {
	if err := fresh(s.At, now); err != nil {
		return Message{}, err
	}
	m, err := Decode(s.Frame[:])
	if err != nil {
		return Message{}, err
	}
	if m.Position == nil {
		return Message{}, fmt.Errorf("%w: frame is not airborne position", ErrCPR)
	}
	return m, nil
}

func fresh(at, now time.Time) error {
	if now.IsZero() || at.IsZero() || at.After(now) || now.Sub(at) > MaxCPRAge {
		return fmt.Errorf("%w: time is missing, future, or older than %s", ErrCPR, MaxCPRAge)
	}
	return nil
}

func (c CPR) validate() error {
	if c.Latitude >= 131072 || c.Longitude >= 131072 {
		return fmt.Errorf("%w: CPR fractions exceed 17 bits", ErrInvalid)
	}
	return nil
}

func (p Coordinates) validate() error {
	if !finite(p.Latitude) || !finite(p.Longitude) || math.Abs(p.Latitude) > 90 || math.Abs(p.Longitude) > 180 {
		return fmt.Errorf("%w: coordinates must be finite latitude [-90,90], longitude [-180,180]", ErrInvalid)
	}
	return nil
}

// longitudeZones uses the CPR NL equation, with explicit equator/pole cases.
// In particular, NL(+/-87) is 2, while latitudes beyond +/-87 have NL=1.
func longitudeZones(lat float64) int {
	lat = math.Abs(lat)
	switch {
	case lat == 0:
		return 59
	case lat == 87:
		return 2
	case lat > 87:
		return 1
	default:
		c := math.Cos(lat * math.Pi / 180)
		return int(math.Floor(2 * math.Pi / math.Acos(1-(1-math.Cos(math.Pi/30))/(c*c))))
	}
}

func mod(a, b float64) float64      { return a - b*math.Floor(a/b) }
func longitude(lon float64) float64 { return mod(lon+180, 360) - 180 }
func latitude(lat float64) float64 {
	if lat >= 270 {
		return lat - 360
	}
	return lat
}
