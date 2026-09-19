package display

import (
	"context"
	"errors"
	"fmt"

	"github.com/miroslav-matejovsky/adsb-testbench/simulatorapi"
)

// Coverage reference altitude domain in pressure-altitude feet. It mirrors
// the simulator's published altitude domain, which a display cannot import.
const (
	minReferenceAltitudeFeet = -1000.0
	maxReferenceAltitudeFeet = 50175.0
)

// Stations reads the station catalog through the configured station source,
// validates it, and returns a detached copy.
//
// It uses the same serialized admission as Refresh and ReceptionHistory. A
// validated run identifier that differs from the published one clears the
// previous run's tracks and fallback, so a browser that discovers a
// replacement run first never sees old-run data on its next refresh.
func (d *Display) Stations(ctx context.Context) (simulatorapi.StationsSnapshot, error) {
	release, err := d.admit(ctx)
	if err != nil {
		return simulatorapi.StationsSnapshot{},
			newSourceError(stationsOperation, simulatorapi.CategoryUnavailable, err, err.Error())
	}
	defer release()

	stations, err := d.stations.Stations(ctx)
	if err != nil {
		failure := asSourceError(stationsOperation, err)
		d.invalidateOnRunChange(failure.RunID)
		return simulatorapi.StationsSnapshot{}, failure
	}
	runID, err := validateStations(stations)
	if err != nil {
		failure := invalidPayloadError(stationsOperation, runID, err)
		d.invalidateOnRunChange(failure.RunID)
		return simulatorapi.StationsSnapshot{}, failure
	}
	d.invalidateOnRunChange(runID)
	return cloneStations(stations), nil
}

// validateStations checks one station catalog semantically. It returns the
// run identifier once the envelope (run and time) is valid, so a caller can
// attribute a payload failure to a known run.
//
// Every station must have a valid unique ID, a positive revision, a creation
// instant no later than the catalog instant, finite settings within their
// published domains, and consistent coverage: finite nonnegative radii, one
// shared valid reference altitude, and an effective radius equal to the
// smaller of the horizon and link-budget radii.
func validateStations(raw simulatorapi.StationsSnapshot) (string, error) {
	if raw.RunID == "" {
		return "", errors.New("runId must not be empty")
	}
	now, err := simulatorapi.ParseTime(raw.Now)
	if err != nil {
		return "", fmt.Errorf("now: %w", err)
	}
	if raw.Stations == nil {
		return raw.RunID, errors.New("stations must be an array, including when empty")
	}
	if len(raw.Stations) > maxSelectedStations {
		return raw.RunID, fmt.Errorf("at most %d stations may exist, got %d", maxSelectedStations, len(raw.Stations))
	}
	seen := make(map[string]bool, len(raw.Stations))
	var reference *float64
	for index, state := range raw.Stations {
		station := state.Station
		if err := validateStationID(station.ID); err != nil {
			return raw.RunID, fmt.Errorf("stations[%d].id: %w", index, err)
		}
		if seen[station.ID] {
			return raw.RunID, fmt.Errorf("station %q is listed twice", station.ID)
		}
		seen[station.ID] = true
		if _, err := positiveSequence(station.Revision, "revision"); err != nil {
			return raw.RunID, fmt.Errorf("station %q: %w", station.ID, err)
		}
		createdAt, err := simulatorapi.ParseTime(station.CreatedAt)
		if err != nil {
			return raw.RunID, fmt.Errorf("station %q createdAt: %w", station.ID, err)
		}
		if createdAt.After(now) {
			return raw.RunID, fmt.Errorf("station %q was created at %s, after the catalog instant %s",
				station.ID, station.CreatedAt, raw.Now)
		}
		for _, domain := range receiverDomains {
			value := domain.get(station.StationSettings)
			if !simulatorapi.Finite(value) || value < domain.lo || value > domain.hi {
				return raw.RunID, fmt.Errorf("station %q %s is %g, outside the accepted range [%g,%g]",
					station.ID, domain.name, value, domain.lo, domain.hi)
			}
		}
		if err := validateCoverage(state.Coverage); err != nil {
			return raw.RunID, fmt.Errorf("station %q coverage: %w", station.ID, err)
		}
		if reference != nil && *reference != state.Coverage.ReferenceAltitudeFeet {
			return raw.RunID, fmt.Errorf("station %q coverage reference altitude %g differs from %g",
				station.ID, state.Coverage.ReferenceAltitudeFeet, *reference)
		}
		reference = &state.Coverage.ReferenceAltitudeFeet
	}
	return raw.RunID, nil
}

// validateCoverage checks one synthetic coverage estimate.
func validateCoverage(coverage simulatorapi.Coverage) error {
	altitude := coverage.ReferenceAltitudeFeet
	if !simulatorapi.Finite(altitude) || altitude < minReferenceAltitudeFeet || altitude > maxReferenceAltitudeFeet {
		return fmt.Errorf("referenceAltitudeFeet %g is outside [%g,%g]",
			altitude, minReferenceAltitudeFeet, maxReferenceAltitudeFeet)
	}
	for _, radius := range []struct {
		name  string
		value float64
	}{
		{"horizonRadiusNauticalMiles", coverage.HorizonRadiusNauticalMiles},
		{"linkBudgetRadiusNauticalMiles", coverage.LinkBudgetRadiusNauticalMiles},
		{"effectiveRadiusNauticalMiles", coverage.EffectiveRadiusNauticalMiles},
	} {
		if !simulatorapi.Finite(radius.value) || radius.value < 0 {
			return fmt.Errorf("%s %g must be finite and nonnegative", radius.name, radius.value)
		}
	}
	want := min(coverage.HorizonRadiusNauticalMiles, coverage.LinkBudgetRadiusNauticalMiles)
	if coverage.EffectiveRadiusNauticalMiles != want {
		return fmt.Errorf("effectiveRadiusNauticalMiles %g is not the smaller of the horizon and link-budget radii (%g)",
			coverage.EffectiveRadiusNauticalMiles, want)
	}
	return nil
}

// cloneStations deep-copies a station catalog. Station states hold no
// pointers or slices, so copying the slice detaches every value.
func cloneStations(value simulatorapi.StationsSnapshot) simulatorapi.StationsSnapshot {
	copied := value
	copied.Stations = append([]simulatorapi.StationState{}, value.Stations...)
	return copied
}

// parseStationsSnapshot strictly binds a station catalog response.
func parseStationsSnapshot(value *simulatorapi.Value) (simulatorapi.StationsSnapshot, error) {
	object, err := value.Object()
	if err != nil {
		return simulatorapi.StationsSnapshot{}, err
	}
	var snapshot simulatorapi.StationsSnapshot
	if snapshot.RunID, err = readText(object, "runId"); err != nil {
		return simulatorapi.StationsSnapshot{}, err
	}
	if snapshot.Now, err = readText(object, "now"); err != nil {
		return simulatorapi.StationsSnapshot{}, err
	}
	field, err := object.Field("stations")
	if err != nil {
		return simulatorapi.StationsSnapshot{}, err
	}
	items, err := field.Array()
	if err != nil {
		return simulatorapi.StationsSnapshot{}, err
	}
	snapshot.Stations = make([]simulatorapi.StationState, 0, len(items))
	for _, item := range items {
		state, err := parseStationState(item)
		if err != nil {
			return simulatorapi.StationsSnapshot{}, err
		}
		snapshot.Stations = append(snapshot.Stations, state)
	}
	if err := object.Done(); err != nil {
		return simulatorapi.StationsSnapshot{}, err
	}
	return snapshot, nil
}

func parseStationState(value *simulatorapi.Value) (simulatorapi.StationState, error) {
	object, err := value.Object()
	if err != nil {
		return simulatorapi.StationState{}, err
	}
	var state simulatorapi.StationState
	stationValue, err := object.Field("station")
	if err != nil {
		return simulatorapi.StationState{}, err
	}
	if state.Station, err = parseStation(stationValue); err != nil {
		return simulatorapi.StationState{}, err
	}
	coverageValue, err := object.Field("coverage")
	if err != nil {
		return simulatorapi.StationState{}, err
	}
	coverage, err := coverageValue.Object()
	if err != nil {
		return simulatorapi.StationState{}, err
	}
	for _, number := range []struct {
		key    string
		target *float64
	}{
		{"referenceAltitudeFeet", &state.Coverage.ReferenceAltitudeFeet},
		{"horizonRadiusNauticalMiles", &state.Coverage.HorizonRadiusNauticalMiles},
		{"linkBudgetRadiusNauticalMiles", &state.Coverage.LinkBudgetRadiusNauticalMiles},
		{"effectiveRadiusNauticalMiles", &state.Coverage.EffectiveRadiusNauticalMiles},
	} {
		if *number.target, err = readFloat(coverage, number.key); err != nil {
			return simulatorapi.StationState{}, err
		}
	}
	if err := coverage.Done(); err != nil {
		return simulatorapi.StationState{}, err
	}
	if err := object.Done(); err != nil {
		return simulatorapi.StationState{}, err
	}
	return state, nil
}
