package simulation

import (
	"math"
	"testing"

	"github.com/stretchr/testify/require"
)

// The helpers below recompute the published model from its textbook forms, so
// the tests do not simply call the production helpers they are checking.

// oracleFreeSpaceConstantDB is the distance-independent free space term from
// the usual 32.45 + 20log10(MHz) + 20log10(km) form, restated for metres.
func oracleFreeSpaceConstantDB(frequencyMHz float64) float64 {
	return 32.45 + 20*math.Log10(frequencyMHz) - 60
}

// oracleHorizonMetres is the published 4.12 km per root metre radio horizon.
func oracleHorizonMetres(antennaMetres, aircraftMetres float64) float64 {
	return 4120 * (math.Sqrt(max(antennaMetres, 0)) + math.Sqrt(max(aircraftMetres, 0)))
}

// oracleSurfaceMetres is the great-circle distance between two coordinates,
// computed from the spherical law of cosines rather than from vectors.
func oracleSurfaceMetres(lat1, lon1, lat2, lon2 float64) float64 {
	const degree = math.Pi / 180
	cosine := math.Sin(lat1*degree)*math.Sin(lat2*degree) +
		math.Cos(lat1*degree)*math.Cos(lat2*degree)*math.Cos((lon2-lon1)*degree)
	return earthRadiusMetres * math.Acos(min(max(cosine, -1), 1))
}

// oracleChordMetres is the straight-line distance between two points at given
// heights, from the law of cosines on the central angle.
func oracleChordMetres(surfaceMetres, height1, height2 float64) float64 {
	r1 := earthRadiusMetres + height1
	r2 := earthRadiusMetres + height2
	angle := surfaceMetres / earthRadiusMetres
	return math.Sqrt(r1*r1 + r2*r2 - 2*r1*r2*math.Cos(angle))
}

// latitudeForSurfaceMetres returns the latitude reached by travelling north
// along a meridian from a starting latitude.
func latitudeForSurfaceMetres(startLatitude, surfaceMetres float64) float64 {
	return startLatitude + (surfaceMetres/earthRadiusMetres)*180/math.Pi
}

func TestReceptionModelConstants(t *testing.T) {
	t.Parallel()

	model := Model()
	require.Equal(t, 51.0, model.TransmitPowerDBm)
	require.Equal(t, 1090.0, model.FrequencyMHz)
	require.Equal(t, 6371000.0, model.EarthRadiusMetres)
	require.InDelta(t, 4.0/3.0, model.RefractionFactor, 1e-12)

	// The derived value is 33.1963 dB. The textbook 32.45 and 147.55 constants
	// are rounded to two decimals, so the oracle agrees only to about 0.01 dB.
	require.InDelta(t, 33.1963, model.FreeSpacePathLossConstantDB, 0.001)
	require.InDelta(t, oracleFreeSpaceConstantDB(model.FrequencyMHz),
		model.FreeSpacePathLossConstantDB, 0.01)

	require.InDelta(t, 4122, model.HorizonMetresPerSqrtMetre, 1)
}

// Free space path loss must match the published form at known distances.
func TestReceptionPathLoss(t *testing.T) {
	t.Parallel()

	// 1090 MHz at 1 km is about 93.2 dB.
	require.InDelta(t, 93.2, pathLossDB(1000), 0.01)
	// Doubling the distance adds about 6 dB.
	require.InDelta(t, pathLossDB(1000)+6.0206, pathLossDB(2000), 0.01)

	// Zero range is clamped, not infinite.
	require.False(t, math.IsInf(pathLossDB(0), 0))
	require.Equal(t, pathLossDB(minimumSlantMetres), pathLossDB(0))
	require.False(t, math.IsInf(receivedPowerDBm(validStationConfig(), 0), 0))
}

func TestReceptionGeometry(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                         string
		stationLat, stationLon       float64
		aircraftLat, aircraftLon     float64
		altitudeFeet                 float64
		siteElevation, antennaHeight float64
	}{
		{"meridian at sea level", 0, 0, 1, 0, 0, 0, 0},
		{"equator at sea level", 0, 0, 0, 1, 0, 0, 0},
		{"meridian at altitude", 50, 14, 52, 14, 35000, 100, 30},
		{"equator at altitude", 0, 179, 0, -179, 20000, 0, 10},
		{"same point", 50, 14, 50, 14, 35000, 100, 30},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			cfg := validStationConfig()
			cfg.LatitudeDegrees = test.stationLat
			cfg.LongitudeDegrees = test.stationLon
			cfg.SiteElevationMetres = test.siteElevation
			cfg.AntennaHeightMetres = test.antennaHeight

			got := measure(cfg, test.aircraftLat, test.aircraftLon, test.altitudeFeet)

			wantSurface := oracleSurfaceMetres(test.stationLat, test.stationLon, test.aircraftLat, test.aircraftLon)
			require.InDelta(t, wantSurface, got.surfaceMetres, 1)

			wantChord := oracleChordMetres(wantSurface,
				test.siteElevation+test.antennaHeight, test.altitudeFeet*metresPerFoot)
			require.InDelta(t, wantChord, got.slantMetres, 1)
		})
	}
}

func TestReceptionHorizonLimit(t *testing.T) {
	t.Parallel()

	cases := [][2]float64{{130, 10668}, {0, 0}, {500, 15293}, {-200, -305}, {30, 305}}
	for _, heights := range cases {
		got := horizonLimitMetres(heights[0], heights[1])
		require.InDelta(t, oracleHorizonMetres(heights[0], heights[1]), got, got*0.001+1)
		require.False(t, math.IsNaN(got))
	}

	// Heights below the sphere contribute nothing.
	require.Equal(t, horizonLimitMetres(0, 0), horizonLimitMetres(-500, -100))
}

func TestCoverageEstimateRegimes(t *testing.T) {
	t.Parallel()

	horizonLimited := validStationConfig()
	budgetLimited := validStationConfig()
	budgetLimited.SensitivityDBm = -85

	const reference = 35000.0

	for _, test := range []struct {
		name          string
		cfg           StationConfig
		wantEffective string
	}{
		{"horizon limited", horizonLimited, "horizon"},
		{"link budget limited", budgetLimited, "budget"},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			got, err := EstimateCoverage(test.cfg, reference)
			require.NoError(t, err)
			require.Equal(t, reference, got.ReferenceAltitudeFeet)

			antenna := test.cfg.SiteElevationMetres + test.cfg.AntennaHeightMetres
			aircraft := reference * metresPerFoot

			// The published 4.12 km per root metre factor is rounded to three
			// significant digits, so the oracle agrees only to about 0.1 percent.
			wantHorizon := oracleHorizonMetres(antenna, aircraft) / metresPerNauticalMile
			require.InDelta(t, wantHorizon, got.HorizonRadiusNauticalMiles, wantHorizon*0.001+0.1)

			budgetDB := 51 + test.cfg.AntennaGainDBi - test.cfg.SystemLossDB - test.cfg.SensitivityDBm
			chord := math.Pow(10, (budgetDB-oracleFreeSpaceConstantDB(1090))/20)
			wantBudget := chordToSurfaceOracle(chord, antenna, aircraft) / metresPerNauticalMile
			require.InDelta(t, wantBudget, got.LinkBudgetRadiusNauticalMiles, 0.1)

			require.Equal(t, math.Min(got.HorizonRadiusNauticalMiles, got.LinkBudgetRadiusNauticalMiles),
				got.EffectiveRadiusNauticalMiles)

			if test.wantEffective == "horizon" {
				require.Less(t, got.HorizonRadiusNauticalMiles, got.LinkBudgetRadiusNauticalMiles)
			} else {
				require.Less(t, got.LinkBudgetRadiusNauticalMiles, got.HorizonRadiusNauticalMiles)
			}
		})
	}
}

// chordToSurfaceOracle inverts a chord to a surface radius independently of
// the production helper.
func chordToSurfaceOracle(chord, height1, height2 float64) float64 {
	r1 := earthRadiusMetres + height1
	r2 := earthRadiusMetres + height2
	cosine := (r1*r1 + r2*r2 - chord*chord) / (2 * r1 * r2)
	switch {
	case cosine >= 1:
		return 0
	case cosine <= -1:
		return math.Pi * earthRadiusMetres
	default:
		return earthRadiusMetres * math.Acos(cosine)
	}
}

// A budget shorter than the height difference cannot reach the aircraft even
// directly overhead.
func TestCoverageEstimateUnreachable(t *testing.T) {
	t.Parallel()

	cfg := validStationConfig()
	cfg.SiteElevationMetres = 0
	cfg.AntennaHeightMetres = 0
	cfg.AntennaGainDBi = -10
	cfg.SystemLossDB = 30
	cfg.SensitivityDBm = 0

	got, err := EstimateCoverage(cfg, maxAltitudeFeet)
	require.NoError(t, err)
	require.Equal(t, 0.0, got.LinkBudgetRadiusNauticalMiles)
	require.Equal(t, 0.0, got.EffectiveRadiusNauticalMiles)

	overhead := decide(cfg, cfg.LatitudeDegrees, cfg.LongitudeDegrees, maxAltitudeFeet)
	require.False(t, overhead.received)
}

// A very generous budget covers the whole sphere and then the horizon binds.
func TestCoverageEstimateWholeSphere(t *testing.T) {
	t.Parallel()

	cfg := validStationConfig()
	cfg.AntennaGainDBi = 40
	cfg.SystemLossDB = 0
	cfg.SensitivityDBm = -140

	got, err := EstimateCoverage(cfg, 35000)
	require.NoError(t, err)
	require.InDelta(t, math.Pi*earthRadiusMetres/metresPerNauticalMile,
		got.LinkBudgetRadiusNauticalMiles, 0.1)
	require.Equal(t, got.HorizonRadiusNauticalMiles, got.EffectiveRadiusNauticalMiles)
}

func TestCoverageEstimateRejects(t *testing.T) {
	t.Parallel()

	invalid := validStationConfig()
	invalid.AntennaGainDBi = 100
	_, err := EstimateCoverage(invalid, 35000)
	require.ErrorIs(t, err, ErrInvalid)

	for _, altitude := range []float64{minAltitudeFeet - 1, maxAltitudeFeet + 1, math.NaN(), math.Inf(1)} {
		got, err := EstimateCoverage(validStationConfig(), altitude)
		require.ErrorIs(t, err, ErrInvalid)
		require.Equal(t, Coverage{}, got)
	}

	// Both altitude endpoints are accepted.
	for _, altitude := range []float64{minAltitudeFeet, maxAltitudeFeet} {
		_, err := EstimateCoverage(validStationConfig(), altitude)
		require.NoError(t, err)
	}
}

// Negative heights stay finite everywhere.
func TestCoverageEstimateNegativeHeights(t *testing.T) {
	t.Parallel()

	cfg := validStationConfig()
	cfg.SiteElevationMetres = -500
	cfg.AntennaHeightMetres = 0

	got, err := EstimateCoverage(cfg, minAltitudeFeet)
	require.NoError(t, err)
	require.False(t, math.IsNaN(got.HorizonRadiusNauticalMiles))
	require.False(t, math.IsNaN(got.LinkBudgetRadiusNauticalMiles))
	require.Equal(t, 0.0, got.HorizonRadiusNauticalMiles, "both heights are below the sphere")
	require.Equal(t, 0.0, got.EffectiveRadiusNauticalMiles)
}

// The published effective radius is the exact reception boundary.
func TestCoverageMatchesReception(t *testing.T) {
	t.Parallel()

	horizonLimited := validStationConfig()
	budgetLimited := validStationConfig()
	budgetLimited.SensitivityDBm = -85

	const reference = 35000.0

	for _, test := range []struct {
		name string
		cfg  StationConfig
	}{
		{"horizon limited", horizonLimited},
		{"link budget limited", budgetLimited},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			coverage, err := EstimateCoverage(test.cfg, reference)
			require.NoError(t, err)
			radius := coverage.EffectiveRadiusNauticalMiles * metresPerNauticalMile
			require.Greater(t, radius, 1000.0)

			inside := latitudeForSurfaceMetres(test.cfg.LatitudeDegrees, radius*0.999)
			outside := latitudeForSurfaceMetres(test.cfg.LatitudeDegrees, radius*1.001)

			require.True(t, decide(test.cfg, inside, test.cfg.LongitudeDegrees, reference).received)
			require.False(t, decide(test.cfg, outside, test.cfg.LongitudeDegrees, reference).received)
		})
	}
}

// A disabled station is still a valid configuration; the decision helper is
// only ever called for enabled stations, so it ignores the flag.
func TestReceptionDecisionReportsRangeAndPower(t *testing.T) {
	t.Parallel()

	cfg := validStationConfig()
	got := decide(cfg, cfg.LatitudeDegrees+1, cfg.LongitudeDegrees, 35000)

	require.True(t, got.received)
	require.Greater(t, got.slantMetres, 0.0)
	require.InDelta(t, receivedPowerDBm(cfg, got.slantMetres), got.receivedPowerDBm, 1e-12)
	require.GreaterOrEqual(t, got.receivedPowerDBm, cfg.SensitivityDBm)
}
