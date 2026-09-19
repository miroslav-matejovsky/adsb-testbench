package simulation

import (
	"fmt"
	"math"
)

// Fixed reception model constants.
//
// These are deliberate scenario constants of this testbench, not calibrated RF
// figures. Nothing here models terrain, obstructions, antenna patterns,
// multipath, interference, message-rate limits, propagation delay, or Doppler.
const (
	// transmitPowerDBm is the effective radiated power of every engine
	// aircraft, including its own antenna. 51 dBm is about 125 W, typical of
	// a Mode S transponder class.
	transmitPowerDBm = 51.0
	// frequencyHz is the 1090 MHz extended squitter downlink frequency.
	frequencyHz = 1090e6
	// speedOfLightMetresPerSecond is the vacuum speed of light.
	speedOfLightMetresPerSecond = 299792458.0
	// refractionFactor is the classic 4/3 Earth radius approximation used for
	// the radio horizon.
	refractionFactor = 4.0 / 3.0
	// metresPerFoot converts aircraft altitude to the model length unit.
	metresPerFoot = 0.3048
	// minimumSlantMetres keeps the path-loss logarithm finite at zero range.
	minimumSlantMetres = 1.0
)

// Derived model parameters. Both are computed from the constants above rather
// than written as literals, so the published values can never drift from the
// values the decision actually uses.
var (
	// freeSpacePathLossConstantDB is the distance-independent term of the free
	// space path loss at frequencyHz, about 33.198 dB.
	freeSpacePathLossConstantDB = 20 * math.Log10(4*math.Pi*frequencyHz/speedOfLightMetresPerSecond)
	// horizonMetresPerSqrtMetre is the radio horizon factor, about 4122 metres
	// per square root metre of height, that is about 4.12 km per root metre.
	horizonMetresPerSqrtMetre = math.Sqrt(2 * refractionFactor * earthRadiusMetres)
)

// ReceptionModel publishes the effective parameters of the synthetic reception
// model. Both the reception decision and [EstimateCoverage] use exactly these
// values. They are fixed engine policy and cannot be configured.
//
// The model is synthetic. Its outputs describe this testbench, not any real
// receiver, antenna, or propagation environment.
type ReceptionModel struct {
	// TransmitPowerDBm is the effective radiated power assumed for every
	// aircraft, in dBm.
	TransmitPowerDBm float64
	// FrequencyMHz is the modelled downlink frequency.
	FrequencyMHz float64
	// FreeSpacePathLossConstantDB is the distance-independent term of the free
	// space path loss. Total loss is 20*log10(metres) plus this value.
	FreeSpacePathLossConstantDB float64
	// RefractionFactor is the Earth radius multiplier used for the radio
	// horizon, 4/3.
	RefractionFactor float64
	// HorizonMetresPerSqrtMetre is the derived horizon factor: the horizon
	// distance in metres is this value times the sum of the square roots of
	// the two heights in metres.
	HorizonMetresPerSqrtMetre float64
	// EarthRadiusMetres is the radius of the model sphere, shared with the
	// aircraft motion model.
	EarthRadiusMetres float64
}

// Model returns the fixed reception model parameters.
func Model() ReceptionModel {
	return ReceptionModel{
		TransmitPowerDBm:            transmitPowerDBm,
		FrequencyMHz:                frequencyHz / 1e6,
		FreeSpacePathLossConstantDB: freeSpacePathLossConstantDB,
		RefractionFactor:            refractionFactor,
		HorizonMetresPerSqrtMetre:   horizonMetresPerSqrtMetre,
		EarthRadiusMetres:           earthRadiusMetres,
	}
}

// Coverage is the estimated reach of one station at one reference altitude.
//
// It is synthetic model output, not a calibrated coverage prediction and not a
// statement about any real installation. All three radii are great-circle
// surface radii on the model sphere.
type Coverage struct {
	// ReferenceAltitudeFeet is the pressure altitude the estimate was made
	// for, treated directly as geometric height above the model sphere.
	ReferenceAltitudeFeet float64
	// HorizonRadiusNauticalMiles is the radio horizon limit.
	HorizonRadiusNauticalMiles float64
	// LinkBudgetRadiusNauticalMiles is the surface radius at which the slant
	// path loss exactly reaches the configured sensitivity.
	LinkBudgetRadiusNauticalMiles float64
	// EffectiveRadiusNauticalMiles is the smaller of the other two radii, and
	// is the exact boundary the reception decision applies.
	EffectiveRadiusNauticalMiles float64
}

// geometry holds the two distances the model needs between one station and one
// aircraft position, both in metres.
type geometry struct {
	// surfaceMetres is the great-circle distance along the model sphere.
	surfaceMetres float64
	// slantMetres is the straight-line chord between the two points.
	slantMetres float64
}

// antennaAltitudeMetres is the height of the station antenna above the model
// sphere.
func antennaAltitudeMetres(cfg StationConfig) float64 {
	return cfg.SiteElevationMetres + cfg.AntennaHeightMetres
}

// measure returns the surface and slant distances between a station and an
// aircraft position. Pressure altitude is used directly as geometric height
// above the model sphere; no pressure-to-geometric conversion exists.
func measure(cfg StationConfig, latitudeDegrees, longitudeDegrees, altitudeFeet float64) geometry {
	stationUnit, _, _ := localAxes(cfg.LatitudeDegrees, cfg.LongitudeDegrees)
	aircraftUnit, _, _ := localAxes(latitudeDegrees, longitudeDegrees)

	stationRadius := earthRadiusMetres + antennaAltitudeMetres(cfg)
	aircraftRadius := earthRadiusMetres + altitudeFeet*metresPerFoot

	station := stationUnit.scale(stationRadius)
	aircraft := aircraftUnit.scale(aircraftRadius)
	difference := aircraft.add(station.scale(-1))

	cosine := min(max(stationUnit.dot(aircraftUnit), -1), 1)
	return geometry{
		surfaceMetres: earthRadiusMetres * math.Acos(cosine),
		slantMetres:   math.Sqrt(difference.dot(difference)),
	}
}

// horizonLimitMetres is the radio horizon between the station antenna and an
// aircraft height, both in metres above the model sphere. Heights below the
// sphere contribute nothing.
func horizonLimitMetres(antennaMetres, aircraftMetres float64) float64 {
	return horizonMetresPerSqrtMetre * (math.Sqrt(max(antennaMetres, 0)) + math.Sqrt(max(aircraftMetres, 0)))
}

// pathLossDB is the free space path loss over a slant distance in metres.
func pathLossDB(slantMetres float64) float64 {
	return 20*math.Log10(max(slantMetres, minimumSlantMetres)) + freeSpacePathLossConstantDB
}

// receivedPowerDBm is the power arriving at the receiver over a slant path.
func receivedPowerDBm(cfg StationConfig, slantMetres float64) float64 {
	return transmitPowerDBm + cfg.AntennaGainDBi - cfg.SystemLossDB - pathLossDB(slantMetres)
}

// linkBudgetChordMetres is the longest slant path whose received power still
// reaches the configured sensitivity.
func linkBudgetChordMetres(cfg StationConfig) float64 {
	budgetDB := transmitPowerDBm + cfg.AntennaGainDBi - cfg.SystemLossDB - cfg.SensitivityDBm
	return math.Pow(10, (budgetDB-freeSpacePathLossConstantDB)/20)
}

// decision is the deterministic part of one reception evaluation, excluding
// the configured random impairment.
type decision struct {
	received         bool
	slantMetres      float64
	receivedPowerDBm float64
}

// decide applies the horizon rule and the link budget to one aircraft position.
// The caller has already checked that the station is enabled.
func decide(cfg StationConfig, latitudeDegrees, longitudeDegrees, altitudeFeet float64) decision {
	g := measure(cfg, latitudeDegrees, longitudeDegrees, altitudeFeet)
	power := receivedPowerDBm(cfg, g.slantMetres)

	horizon := horizonLimitMetres(antennaAltitudeMetres(cfg), altitudeFeet*metresPerFoot)
	received := g.surfaceMetres <= horizon && power >= cfg.SensitivityDBm

	return decision{received: received, slantMetres: g.slantMetres, receivedPowerDBm: power}
}

// EstimateCoverage returns the synthetic coverage of any valid station settings
// at an explicit reference altitude in feet, within [-1000,50175].
//
// It needs no engine and no live station, so a caller can preview settings
// before applying them. The returned radii come from the same two limits the
// reception decision applies, inverted to great-circle surface radii: chord
// length grows strictly with angular separation for fixed heights, so a slant
// limit maps to exactly one surface radius and EffectiveRadiusNauticalMiles is
// the exact reception boundary rather than an approximation.
//
// Invalid settings or an out-of-domain reference altitude return a zero
// Coverage and an error wrapping ErrInvalid.
func EstimateCoverage(cfg StationConfig, referenceAltitudeFeet float64) (Coverage, error) {
	if err := validateStation(cfg); err != nil {
		return Coverage{}, err
	}
	if math.IsNaN(referenceAltitudeFeet) || math.IsInf(referenceAltitudeFeet, 0) {
		return Coverage{}, fmt.Errorf("%w: reference altitude must be a finite number", ErrInvalid)
	}
	if referenceAltitudeFeet < minAltitudeFeet || referenceAltitudeFeet > maxAltitudeFeet {
		return Coverage{}, fmt.Errorf("%w: reference altitude %g is outside [%g,%g] feet",
			ErrInvalid, referenceAltitudeFeet, minAltitudeFeet, maxAltitudeFeet)
	}

	antenna := antennaAltitudeMetres(cfg)
	aircraft := referenceAltitudeFeet * metresPerFoot

	horizon := horizonLimitMetres(antenna, aircraft)
	budget := chordToSurfaceMetres(linkBudgetChordMetres(cfg), antenna, aircraft)

	return Coverage{
		ReferenceAltitudeFeet:         referenceAltitudeFeet,
		HorizonRadiusNauticalMiles:    horizon / metresPerNauticalMile,
		LinkBudgetRadiusNauticalMiles: budget / metresPerNauticalMile,
		EffectiveRadiusNauticalMiles:  min(horizon, budget) / metresPerNauticalMile,
	}, nil
}

// chordToSurfaceMetres converts a maximum slant chord into the great-circle
// surface radius that produces it, for two fixed heights above the sphere.
//
// A chord shorter than the height difference cannot be reached even directly
// overhead, so the radius is zero. A chord longer than the diameter through
// both points covers the whole sphere, so the radius is half its circumference.
func chordToSurfaceMetres(chordMetres, antennaMetres, aircraftMetres float64) float64 {
	stationRadius := earthRadiusMetres + antennaMetres
	aircraftRadius := earthRadiusMetres + aircraftMetres

	cosine := (stationRadius*stationRadius + aircraftRadius*aircraftRadius - chordMetres*chordMetres) /
		(2 * stationRadius * aircraftRadius)
	switch {
	case cosine >= 1:
		return 0
	case cosine <= -1:
		return math.Pi * earthRadiusMetres
	default:
		return earthRadiusMetres * math.Acos(cosine)
	}
}
