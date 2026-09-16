package simulation

import "math"

// Synthetic motion model constants. This is a deliberately simple spherical
// model for reproducible test traffic, not WGS-84 geodesic navigation.
const (
	earthRadiusMetres     = 6371000.0
	metresPerNauticalMile = 1852.0
	secondsPerHour        = 3600.0
	secondsPerMinute      = 60.0

	// Encodable pressure altitude bounds, matching the codec Q=1 encoder.
	minAltitudeFeet = -1000.0
	maxAltitudeFeet = 50175.0

	// polarEpsilon is the horizontal length below which a position vector is
	// treated as polar and longitude becomes zero by convention.
	polarEpsilon = 1e-12
)

// vector is a Cartesian triple in an Earth-centred frame with the z axis
// through the north pole and the x axis through 0N 0E.
type vector struct{ x, y, z float64 }

func (v vector) scale(f float64) vector { return vector{v.x * f, v.y * f, v.z * f} }

func (v vector) add(o vector) vector { return vector{v.x + o.x, v.y + o.y, v.z + o.z} }

func (v vector) dot(o vector) float64 { return v.x*o.x + v.y*o.y + v.z*o.z }

// birthState is the immutable navigation state an aircraft is created with.
type birthState struct {
	latitudeDegrees           float64
	longitudeDegrees          float64
	altitudeFeet              float64
	groundSpeedKnots          float64
	trackDegrees              float64
	verticalRateFeetPerMinute float64
}

// trajectory evaluates an aircraft position at any absolute age without
// integrating previous steps, so results never depend on how a caller split
// its duration calls.
type trajectory struct {
	birth birthState
	// position and tangent are the orthonormal unit vectors at birth.
	position vector
	tangent  vector
	// angularRate is the great-circle rotation rate in radians per second.
	angularRate float64
}

// navState is one evaluated truth sample. East and north components are the
// signed ground velocity in knots along the local axes at the sampled point.
type navState struct {
	latitudeDegrees           float64
	longitudeDegrees          float64
	altitudeFeet              float64
	groundSpeedKnots          float64
	trackDegrees              float64
	verticalRateFeetPerMinute float64
	eastKnots                 float64
	northKnots                float64
}

// localAxes returns the unit position, north, and east vectors at a
// geographic coordinate given in degrees.
func localAxes(latitudeDegrees, longitudeDegrees float64) (position, north, east vector) {
	lat := latitudeDegrees * math.Pi / 180
	lon := longitudeDegrees * math.Pi / 180
	sinLat, cosLat := math.Sincos(lat)
	sinLon, cosLon := math.Sincos(lon)

	position = vector{cosLat * cosLon, cosLat * sinLon, sinLat}
	north = vector{-sinLat * cosLon, -sinLat * sinLon, cosLat}
	east = vector{-sinLon, cosLon, 0}
	return position, north, east
}

// newTrajectory builds the absolute-time trajectory of a newly created
// aircraft from its immutable birth state.
func newTrajectory(birth birthState) trajectory {
	position, north, east := localAxes(birth.latitudeDegrees, birth.longitudeDegrees)
	track := birth.trackDegrees * math.Pi / 180
	sinTrack, cosTrack := math.Sincos(track)
	tangent := north.scale(cosTrack).add(east.scale(sinTrack))

	metresPerSecond := birth.groundSpeedKnots * metresPerNauticalMile / secondsPerHour
	return trajectory{
		birth:       birth,
		position:    position,
		tangent:     tangent,
		angularRate: metresPerSecond / earthRadiusMetres,
	}
}

// at evaluates truth at an absolute age in seconds since creation.
// Evaluating the same age twice always yields the same result, whatever ages
// were evaluated in between.
func (t trajectory) at(ageSeconds float64) navState {
	theta := t.angularRate * ageSeconds
	sinTheta, cosTheta := math.Sincos(theta)

	position := t.position.scale(cosTheta).add(t.tangent.scale(sinTheta))
	tangent := t.tangent.scale(cosTheta).add(t.position.scale(-sinTheta))

	horizontal := math.Hypot(position.x, position.y)
	latitude := math.Atan2(position.z, horizontal) * 180 / math.Pi
	longitude := 0.0
	if horizontal > polarEpsilon {
		longitude = math.Atan2(position.y, position.x) * 180 / math.Pi
	}
	longitude = normalizeLongitude(longitude)

	_, north, east := localAxes(latitude, longitude)
	state := navState{
		latitudeDegrees:  latitude,
		longitudeDegrees: longitude,
		groundSpeedKnots: t.birth.groundSpeedKnots,
	}

	if t.angularRate == 0 {
		// A stationary target keeps its birth track as a display convention
		// and reports genuinely zero ground components.
		state.trackDegrees = t.birth.trackDegrees
	} else {
		alongNorth := tangent.dot(north)
		alongEast := tangent.dot(east)
		state.trackDegrees = normalizeTrack(math.Atan2(alongEast, alongNorth) * 180 / math.Pi)
		state.northKnots = t.birth.groundSpeedKnots * alongNorth
		state.eastKnots = t.birth.groundSpeedKnots * alongEast
	}

	state.altitudeFeet, state.verticalRateFeetPerMinute = t.altitudeAt(ageSeconds)
	return state
}

// altitudeAt returns clamped pressure altitude and the current vertical rate.
// The rate becomes zero only after the trajectory has reached a bound while
// moving out of it; an inward-pointing rate at a bound still moves inward.
func (t trajectory) altitudeAt(ageSeconds float64) (altitudeFeet, verticalRate float64) {
	rate := t.birth.verticalRateFeetPerMinute
	raw := t.birth.altitudeFeet + rate*(ageSeconds/secondsPerMinute)
	switch {
	case rate > 0 && raw >= maxAltitudeFeet:
		return maxAltitudeFeet, 0
	case rate < 0 && raw <= minAltitudeFeet:
		return minAltitudeFeet, 0
	default:
		return raw, rate
	}
}

// normalizeLongitude maps any finite longitude onto [-180,180).
func normalizeLongitude(degrees float64) float64 {
	wrapped := math.Mod(degrees+180, 360)
	if wrapped < 0 {
		wrapped += 360
	}
	return wrapped - 180
}

// normalizeTrack maps any finite track onto [0,360).
func normalizeTrack(degrees float64) float64 {
	wrapped := math.Mod(degrees, 360)
	if wrapped < 0 {
		wrapped += 360
	}
	return wrapped
}
