---
title: "02 - Reception model, geometry, and published coverage"
dependencies: ["01-station-contract-and-configuration.md"]
effort: "M"
complexity: "high"
---

# 02 - Reception model, geometry, and published coverage

## Objective

Implement the deterministic synthetic reception model as pure functions, and
publish both its effective parameters and its coverage radii at an explicit
reference altitude, using the same two limits that decide reception.

## Target Artifacts

- Create `simulation/reception.go` and `simulation/reception_test.go`.
- Reuse `earthRadiusMetres`, `metresPerNauticalMile`, and `localAxes` from
  `simulation/motion.go` without duplicating them.
- Extend `simulation/doc.go` with the model description and its limits.

## Implementation Tasks

1. Define the model constants: transmit power 51 dBm effective radiated power,
   frequency 1090 MHz, speed of light 299792458 m/s, refraction factor 4/3, and
   0.3048 metres per foot. Derive the free-space constant as
   `20*log10(4*pi*frequencyHz/speedOfLight)` and the horizon factor as
   `sqrt(2*refractionFactor*earthRadiusMetres)` rather than hard-coding either.

2. Define exported `ReceptionModel` with `TransmitPowerDBm`, `FrequencyMHz`,
   `FreeSpacePathLossConstantDB`, `RefractionFactor`,
   `HorizonMetresPerSqrtMetre`, and `EarthRadiusMetres`, and `Model()`
   returning it. Document each field as a fixed scenario parameter.

3. Implement private geometry: given a station configuration and an aircraft
   latitude, longitude, and altitude in feet, return the great-circle surface
   distance and the slant distance in metres. Build both position vectors from
   `localAxes` scaled by the Earth radius plus that height.

4. Implement the two private limit rules described below, plus a private
   decision helper that returns whether a station receives a transmission,
   the slant range, and the received power, excluding the random impairment.

5. Define exported `Coverage` with `ReferenceAltitudeFeet`,
   `HorizonRadiusNauticalMiles`, `LinkBudgetRadiusNauticalMiles`, and
   `EffectiveRadiusNauticalMiles`, and `EstimateCoverage(StationConfig,
   float64) (Coverage, error)`. Validate the settings with the step 01
   validator and the reference altitude against [-1000, 50175] feet before any
   arithmetic.

6. Invert both limits to great-circle surface radii on the model sphere, so the
   published effective radius is the exact reception boundary rather than an
   approximation.

7. Add tests for the derived constants, analytic distances, both limit
   regimes, the documented clamps, and the exact agreement between the inverted
   radii and the forward decision.

## Technical Details

Reception decision for one transmission and one enabled station:

1. Convert the aircraft truth altitude in feet to metres. Treat it as geometric
   height above the model sphere; no pressure-to-geometric conversion exists.
2. Effective antenna height is `SiteElevationMetres + AntennaHeightMetres`.
3. Horizon limit in metres is
   `HorizonMetresPerSqrtMetre * (sqrt(max(antennaHeight,0)) + sqrt(max(aircraftHeight,0)))`.
   Reject when the surface distance exceeds it.
4. Free-space path loss is `20*log10(max(slantMetres,1)) + FreeSpacePathLossConstantDB`.
   The one-metre clamp keeps the logarithm finite at zero range.
5. Received power is `TransmitPowerDBm + AntennaGainDBi - SystemLossDB` minus
   that path loss. Reject when it is below `SensitivityDBm`.
6. Otherwise the station receives, subject only to the random impairment that
   step 04 applies.

Coverage inverts the same two rules at the reference altitude:

- Horizon surface radius uses the same expression as rule 3 with the reference
  altitude in place of the aircraft height.
- The link budget gives a maximum slant chord
  `10^((TransmitPowerDBm + AntennaGainDBi - SystemLossDB - SensitivityDBm - FreeSpacePathLossConstantDB)/20)`
  metres. With `r1` the station radius and `r2` the aircraft radius from the
  sphere centre, the corresponding angle satisfies
  `cos(angle) = (r1*r1 + r2*r2 - chord*chord) / (2*r1*r2)`. A cosine above 1
  means the aircraft is unreachable even directly overhead, so the radius is 0.
  A cosine below -1 means the whole sphere is inside the budget, so the radius
  is `pi * earthRadiusMetres`. Otherwise the radius is
  `earthRadiusMetres * acos(cosine)`.
- The effective radius is the smaller of the two, and all three are converted
  with 1852 metres per nautical mile.

Chord length grows strictly with angular separation for fixed radii, so the
slant test and the inverted surface radius accept exactly the same positions.
State that property next to the inversion.

`EstimateCoverage` takes settings rather than a live station so a caller can
preview a configuration before applying it. It needs no engine and no lock.

Document `Coverage`, `ReceptionModel`, `Reception.SlantRangeNauticalMiles`, and
`Reception.ReceivedPowerDBm` as synthetic model output, explicitly not a
calibrated RF prediction and not a claim about any real receiver.

Do not add terrain, obstruction masks, antenna patterns, multipath,
interference, message-rate limits, propagation delay, or Doppler. Do not add a
configurable model; the parameters in `ReceptionModel` are fixed policy.

## Verification

These commands apply to subsequent implementation, not this planning task.

- `go test ./simulation -run 'TestReceptionModel|TestReceptionGeometry|TestCoverage'`
- `go doc -all ./simulation` to inspect the published parameters and their
  synthetic-output labels.

## Acceptance Criteria

- `Model()` returns the derived free-space constant within 0.001 dB of
  33.198 dB and the derived horizon factor within 1 m of 4122 m per root metre.
- Surface distance matches analytic great-circle expectations along a meridian
  and along the equator within 1 metre, and slant distance matches an
  independently computed chord within 1 metre.
- A station with typical settings is horizon limited at 35000 feet and link
  budget limited with a degraded sensitivity, both matching independently
  computed radii within 0.1 NM.
- Positions at 0.999 of the effective radius are received and positions at
  1.001 are not, for both limiting regimes, at the reference altitude.
- Negative site elevation and negative aircraft altitude produce finite radii
  and no NaN, and zero range produces a finite received power.
- Invalid settings and out-of-domain reference altitudes return `ErrInvalid`
  and a zero `Coverage`.

## Non-Goals

Station storage, revisions, engine commands, random impairments, reception
records, batch changes, and per-station history.
