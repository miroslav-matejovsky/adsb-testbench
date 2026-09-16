package simulation

import (
	"math"
	"testing"

	"github.com/stretchr/testify/require"
)

// secondsForDegrees returns the time needed to cover an angular distance at a
// ground speed, derived directly from the documented unit conversions rather
// than from the trajectory implementation.
func secondsForDegrees(groundSpeedKnots, degrees float64) float64 {
	metresPerSecond := groundSpeedKnots * 1852 / 3600
	radiansPerSecond := metresPerSecond / 6371000
	return (degrees * math.Pi / 180) / radiansPerSecond
}

func level(latitude, longitude, track, groundSpeed float64) birthState {
	return birthState{
		latitudeDegrees:  latitude,
		longitudeDegrees: longitude,
		altitudeFeet:     30000,
		groundSpeedKnots: groundSpeed,
		trackDegrees:     track,
	}
}

func TestMotionZeroAge(t *testing.T) {
	t.Parallel()

	birth := level(48.5, -3.25, 137.5, 420)
	got := newTrajectory(birth).at(0)

	require.InDelta(t, birth.latitudeDegrees, got.latitudeDegrees, 1e-9)
	require.InDelta(t, birth.longitudeDegrees, got.longitudeDegrees, 1e-9)
	require.InDelta(t, birth.trackDegrees, got.trackDegrees, 1e-9)
	require.Equal(t, birth.altitudeFeet, got.altitudeFeet)
	require.Equal(t, birth.groundSpeedKnots, got.groundSpeedKnots)
}

// An eastbound equatorial leg moves purely in longitude at the analytic rate.
func TestMotionEquatorEastbound(t *testing.T) {
	t.Parallel()

	const speed = 600
	traj := newTrajectory(level(0, 0, 90, speed))
	got := traj.at(secondsForDegrees(speed, 12))

	require.InDelta(t, 0, got.latitudeDegrees, 1e-9)
	require.InDelta(t, 12, got.longitudeDegrees, 1e-9)
	require.InDelta(t, 90, got.trackDegrees, 1e-9)
	require.InDelta(t, speed, got.eastKnots, 1e-9)
	require.InDelta(t, 0, got.northKnots, 1e-9)
}

// A northbound meridian leg moves purely in latitude at the analytic rate.
func TestMotionMeridianNorthbound(t *testing.T) {
	t.Parallel()

	const speed = 480
	traj := newTrajectory(level(10, 25, 0, speed))
	got := traj.at(secondsForDegrees(speed, 7.5))

	require.InDelta(t, 17.5, got.latitudeDegrees, 1e-9)
	require.InDelta(t, 25, got.longitudeDegrees, 1e-9)
	require.InDelta(t, 0, got.trackDegrees, 1e-9)
	require.InDelta(t, speed, got.northKnots, 1e-9)
	require.InDelta(t, 0, got.eastKnots, 1e-9)
}

func TestMotionCrossesDateLine(t *testing.T) {
	t.Parallel()

	const speed = 500
	traj := newTrajectory(level(0, 179.5, 90, speed))
	got := traj.at(secondsForDegrees(speed, 1))

	require.InDelta(t, -179.5, got.longitudeDegrees, 1e-9)
	require.GreaterOrEqual(t, got.longitudeDegrees, -180.0)
	require.Less(t, got.longitudeDegrees, 180.0)
}

// Passing over the north pole reverses the track and shifts longitude by 180.
func TestMotionCrossesPole(t *testing.T) {
	t.Parallel()

	const speed = 450
	traj := newTrajectory(level(85, 10, 0, speed))

	atPole := traj.at(secondsForDegrees(speed, 5))
	require.InDelta(t, 90, atPole.latitudeDegrees, 1e-9)
	require.GreaterOrEqual(t, atPole.longitudeDegrees, -180.0)
	require.Less(t, atPole.longitudeDegrees, 180.0)
	require.False(t, math.IsNaN(atPole.trackDegrees))

	// An exactly polar position vector uses the explicit longitude convention.
	require.Equal(t, 0.0, newTrajectory(level(90, 137, 0, 0)).at(0).longitudeDegrees)

	beyond := traj.at(secondsForDegrees(speed, 15))
	require.InDelta(t, 80, beyond.latitudeDegrees, 1e-9)
	require.InDelta(t, -170, beyond.longitudeDegrees, 1e-9)
	require.InDelta(t, 180, beyond.trackDegrees, 1e-9)
	require.True(t, math.Abs(beyond.eastKnots) < 1e-6)
	require.InDelta(t, -speed, beyond.northKnots, 1e-9)
}

// A stationary aircraft keeps its birth track and reports zero components.
func TestMotionZeroGroundSpeed(t *testing.T) {
	t.Parallel()

	traj := newTrajectory(level(45, 9, 275, 0))
	got := traj.at(3600)

	require.Equal(t, 45.0, got.latitudeDegrees)
	require.Equal(t, 9.0, got.longitudeDegrees)
	require.Equal(t, 275.0, got.trackDegrees)
	require.Equal(t, 0.0, got.eastKnots)
	require.Equal(t, 0.0, got.northKnots)
}

// The same absolute age always yields the same state, whatever ages were
// evaluated in between. This is the property that makes split calls safe.
func TestMotionEvaluationOrderIndependent(t *testing.T) {
	t.Parallel()

	traj := newTrajectory(birthState{
		latitudeDegrees:           -33.9,
		longitudeDegrees:          151.2,
		altitudeFeet:              12000,
		groundSpeedKnots:          317.25,
		trackDegrees:              221.75,
		verticalRateFeetPerMinute: 1750,
	})

	direct := traj.at(600)
	for _, age := range []float64{0.001, 1, 7.5, 123.25, 599.999, 1200, 3} {
		traj.at(age)
	}
	require.Equal(t, direct, traj.at(600))
}

func TestMotionAltitudeLimits(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		birth        birthState
		ageSeconds   float64
		wantAltitude float64
		wantRate     float64
	}{
		{"climbing", birthState{altitudeFeet: 30000, verticalRateFeetPerMinute: 1200}, 120, 32400, 1200},
		{"descending", birthState{altitudeFeet: 30000, verticalRateFeetPerMinute: -1200}, 120, 27600, -1200},
		{"level", birthState{altitudeFeet: 30000}, 12345, 30000, 0},
		{"levels off at ceiling", birthState{altitudeFeet: 50000, verticalRateFeetPerMinute: 6000}, 600, maxAltitudeFeet, 0},
		{"exactly at ceiling", birthState{altitudeFeet: 50175, verticalRateFeetPerMinute: 500}, 0, maxAltitudeFeet, 0},
		{"descends away from ceiling", birthState{altitudeFeet: 50175, verticalRateFeetPerMinute: -600}, 60, 49575, -600},
		{"levels off at floor", birthState{altitudeFeet: -500, verticalRateFeetPerMinute: -6000}, 600, minAltitudeFeet, 0},
		{"climbs away from floor", birthState{altitudeFeet: -1000, verticalRateFeetPerMinute: 600}, 60, -400, 600},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			got := newTrajectory(test.birth).at(test.ageSeconds)
			require.InDelta(t, test.wantAltitude, got.altitudeFeet, 1e-9)
			require.Equal(t, test.wantRate, got.verticalRateFeetPerMinute)
		})
	}
}

func TestMotionNormalizers(t *testing.T) {
	t.Parallel()

	require.Equal(t, -180.0, normalizeLongitude(180))
	require.Equal(t, -180.0, normalizeLongitude(-180))
	require.Equal(t, -179.0, normalizeLongitude(181))
	require.Equal(t, 179.0, normalizeLongitude(-181))
	require.Equal(t, 0.0, normalizeTrack(360))
	require.Equal(t, 350.0, normalizeTrack(-10))
	require.Equal(t, 10.0, normalizeTrack(370))
}
