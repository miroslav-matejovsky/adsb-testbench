package simulator

import (
	"fmt"
	"time"

	"github.com/miroslav-matejovsky/adsb-testbench/simulation"
	"github.com/miroslav-matejovsky/adsb-testbench/simulatorapi"
)

// Converters between engine values and transport-safe data.
//
// Every converter allocates: no returned slice, map, or pointer aliases
// engine state, so editing a response can never change the runtime or a later
// response. Empty collections become empty arrays, never null.

func stationSettingsDTO(cfg simulation.StationConfig) simulatorapi.StationSettings {
	return simulatorapi.StationSettings{
		ID:                   cfg.ID,
		Enabled:              cfg.Enabled,
		LatitudeDegrees:      cfg.LatitudeDegrees,
		LongitudeDegrees:     cfg.LongitudeDegrees,
		SiteElevationMetres:  cfg.SiteElevationMetres,
		AntennaHeightMetres:  cfg.AntennaHeightMetres,
		AntennaGainDBi:       cfg.AntennaGainDBi,
		SensitivityDBm:       cfg.SensitivityDBm,
		SystemLossDB:         cfg.SystemLossDB,
		FrameLossProbability: cfg.FrameLossProbability,
	}
}

// stationSettings converts wire settings back to engine settings. Every
// floating-point value is checked, because a local Go caller can supply a
// value no JSON document could have carried.
func stationSettings(settings simulatorapi.StationSettings) (simulation.StationConfig, error) {
	cfg := simulation.StationConfig{
		ID:                   settings.ID,
		Enabled:              settings.Enabled,
		LatitudeDegrees:      settings.LatitudeDegrees,
		LongitudeDegrees:     settings.LongitudeDegrees,
		SiteElevationMetres:  settings.SiteElevationMetres,
		AntennaHeightMetres:  settings.AntennaHeightMetres,
		AntennaGainDBi:       settings.AntennaGainDBi,
		SensitivityDBm:       settings.SensitivityDBm,
		SystemLossDB:         settings.SystemLossDB,
		FrameLossProbability: settings.FrameLossProbability,
	}
	if err := simulation.ValidateStationConfig(cfg); err != nil {
		return simulation.StationConfig{}, err
	}
	return cfg, nil
}

func stationDTO(station simulation.Station) simulatorapi.Station {
	return simulatorapi.Station{
		StationSettings: stationSettingsDTO(station.Config),
		Revision:        simulatorapi.FormatUint64(station.Revision),
		CreatedAt:       simulatorapi.FormatTime(station.CreatedAt),
	}
}

func coverageDTO(coverage simulation.Coverage) simulatorapi.Coverage {
	return simulatorapi.Coverage{
		ReferenceAltitudeFeet:         coverage.ReferenceAltitudeFeet,
		HorizonRadiusNauticalMiles:    coverage.HorizonRadiusNauticalMiles,
		LinkBudgetRadiusNauticalMiles: coverage.LinkBudgetRadiusNauticalMiles,
		EffectiveRadiusNauticalMiles:  coverage.EffectiveRadiusNauticalMiles,
	}
}

func receptionDTO(reception simulation.Reception) simulatorapi.Reception {
	return simulatorapi.Reception{
		Sequence:                simulatorapi.FormatUint64(reception.Sequence),
		TransmissionSequence:    simulatorapi.FormatUint64(reception.TransmissionSequence),
		StationID:               reception.StationID,
		StationRevision:         simulatorapi.FormatUint64(reception.StationRevision),
		ICAO:                    simulatorapi.FormatICAO(reception.ICAO),
		Kind:                    reception.Kind.String(),
		Timestamp:               simulatorapi.FormatTime(reception.Timestamp),
		Frame:                   simulatorapi.FormatFrame(reception.Frame),
		SlantRangeNauticalMiles: reception.SlantRangeNauticalMiles,
		ReceivedPowerDBm:        reception.ReceivedPowerDBm,
		Receiver:                stationDTO(reception.Receiver),
	}
}

func receptionsDTO(receptions []simulation.Reception) []simulatorapi.Reception {
	converted := make([]simulatorapi.Reception, 0, len(receptions))
	for _, reception := range receptions {
		converted = append(converted, receptionDTO(reception))
	}
	return converted
}

func retentionDTO(retention []simulation.StationRetention) []simulatorapi.StationRetention {
	converted := make([]simulatorapi.StationRetention, 0, len(retention))
	for _, item := range retention {
		converted = append(converted, simulatorapi.StationRetention{
			StationID:      item.StationID,
			OldestSequence: simulatorapi.FormatUint64(item.OldestSequence),
			LatestSequence: simulatorapi.FormatUint64(item.LatestSequence),
			Truncated:      item.Truncated,
			Limit:          item.Limit,
		})
	}
	return converted
}

func cursorDTO(cursor simulation.ReceptionCursor) simulatorapi.ReceptionCursor {
	return simulatorapi.ReceptionCursor{
		RunID:         cursor.RunID,
		StationID:     cursor.StationID,
		AfterSequence: simulatorapi.FormatUint64(cursor.AfterSequence),
	}
}

func receptionPageDTO(page simulation.ReceptionPage) simulatorapi.ReceptionPage {
	return simulatorapi.ReceptionPage{
		RunID:          page.RunID,
		StationID:      page.StationID,
		Now:            simulatorapi.FormatTime(page.Now),
		Records:        receptionsDTO(page.Records),
		OldestSequence: simulatorapi.FormatUint64(page.OldestSequence),
		LatestSequence: simulatorapi.FormatUint64(page.LatestSequence),
		NextCursor:     cursorDTO(page.NextCursor),
		Gap:            page.Gap,
		HasMore:        page.HasMore,
		RetentionLimit: page.RetentionLimit,
	}
}

func receptionSnapshotDTO(snapshot simulation.ReceptionSnapshot) simulatorapi.ReceptionSnapshot {
	return simulatorapi.ReceptionSnapshot{
		RunID:      snapshot.RunID,
		Now:        simulatorapi.FormatTime(snapshot.Now),
		StationIDs: copyStrings(snapshot.StationIDs),
		Retention:  retentionDTO(snapshot.Retention),
		Records:    receptionsDTO(snapshot.Records),
	}
}

func evidenceDTO(evidence simulation.ObservationEvidence) simulatorapi.Evidence {
	return simulatorapi.Evidence{
		TransmissionSequence: simulatorapi.FormatUint64(evidence.TransmissionSequence),
		ICAO:                 simulatorapi.FormatICAO(evidence.ICAO),
		Kind:                 evidence.Kind.String(),
		Timestamp:            simulatorapi.FormatTime(evidence.Timestamp),
		Frame:                simulatorapi.FormatFrame(evidence.Frame),
		Receptions:           receptionsDTO(evidence.Receptions),
	}
}

func measurementDTO(measurement *simulation.ObservedMeasurement) *simulatorapi.Measurement {
	if measurement == nil {
		return nil
	}
	return &simulatorapi.Measurement{Value: measurement.Value, OverRange: measurement.OverRange}
}

func floatDTO(value *float64) *float64 {
	if value == nil {
		return nil
	}
	copied := *value
	return &copied
}

func observedAircraftDTO(aircraft simulation.ObservedAircraft) simulatorapi.Aircraft {
	converted := simulatorapi.Aircraft{
		ICAO:           simulatorapi.FormatICAO(aircraft.ICAO),
		LastReceivedAt: simulatorapi.FormatTime(aircraft.LastReceivedAt),
	}
	if identity := aircraft.Identity; identity != nil {
		converted.Identity = &simulatorapi.IdentityObservation{
			Callsign:   identity.Callsign,
			ObservedAt: simulatorapi.FormatTime(identity.ObservedAt),
			Evidence:   evidenceDTO(identity.Evidence),
		}
	}
	if position := aircraft.Position; position != nil {
		evidence := make([]simulatorapi.Evidence, 0, len(position.Evidence))
		for _, item := range position.Evidence {
			evidence = append(evidence, evidenceDTO(item))
		}
		converted.Position = &simulatorapi.PositionObservation{
			LatitudeDegrees:  position.LatitudeDegrees,
			LongitudeDegrees: position.LongitudeDegrees,
			ObservedAt:       simulatorapi.FormatTime(position.ObservedAt),
			Evidence:         evidence,
		}
	}
	if altitude := aircraft.BarometricAltitude; altitude != nil {
		converted.BarometricAltitude = &simulatorapi.AltitudeObservation{
			Feet:       altitude.Feet,
			ObservedAt: simulatorapi.FormatTime(altitude.ObservedAt),
			Evidence:   evidenceDTO(altitude.Evidence),
		}
	}
	if velocity := aircraft.Velocity; velocity != nil {
		converted.Velocity = &simulatorapi.VelocityObservation{
			Subtype:                   velocity.Subtype,
			IntentChange:              velocity.IntentChange,
			IFRCapability:             velocity.IFRCapability,
			NACv:                      velocity.NACv,
			EastKnots:                 measurementDTO(velocity.EastKnots),
			NorthKnots:                measurementDTO(velocity.NorthKnots),
			GroundSpeedKnots:          floatDTO(velocity.GroundSpeedKnots),
			TrackDegrees:              floatDTO(velocity.TrackDegrees),
			HeadingDegrees:            floatDTO(velocity.HeadingDegrees),
			AirspeedKnots:             measurementDTO(velocity.AirspeedKnots),
			TrueAirspeed:              velocity.TrueAirspeed,
			BarometricVerticalRate:    velocity.BarometricVerticalRate,
			VerticalRateFeetPerMinute: measurementDTO(velocity.VerticalRateFeetPerMinute),
			GNSSMinusBaroFeet:         measurementDTO(velocity.GNSSMinusBaroFeet),
			ObservedAt:                simulatorapi.FormatTime(velocity.ObservedAt),
			Evidence:                  evidenceDTO(velocity.Evidence),
		}
	}
	return converted
}

func expiryDTO(expiry simulation.ObservationExpiry) simulatorapi.ObservationExpiry {
	return simulatorapi.ObservationExpiry{
		IdentityNanoseconds: simulatorapi.FormatDuration(expiry.Identity),
		PositionNanoseconds: simulatorapi.FormatDuration(expiry.Position),
		AltitudeNanoseconds: simulatorapi.FormatDuration(expiry.Altitude),
		VelocityNanoseconds: simulatorapi.FormatDuration(expiry.Velocity),
	}
}

func observationSnapshotDTO(snapshot simulation.ObservationSnapshot) simulatorapi.ObservationSnapshot {
	aircraft := make([]simulatorapi.Aircraft, 0, len(snapshot.Aircraft))
	for _, item := range snapshot.Aircraft {
		aircraft = append(aircraft, observedAircraftDTO(item))
	}
	return simulatorapi.ObservationSnapshot{
		RunID:      snapshot.RunID,
		Now:        simulatorapi.FormatTime(snapshot.Now),
		StationIDs: copyStrings(snapshot.StationIDs),
		Retention:  retentionDTO(snapshot.Retention),
		Expiry:     expiryDTO(snapshot.Expiry),
		Aircraft:   aircraft,
	}
}

func truthAircraftDTO(aircraft simulation.Aircraft) simulatorapi.TruthAircraft {
	return simulatorapi.TruthAircraft{
		ICAO:                      simulatorapi.FormatICAO(aircraft.ICAO),
		Callsign:                  aircraft.Callsign,
		CreatedAt:                 simulatorapi.FormatTime(aircraft.CreatedAt),
		LatitudeDegrees:           aircraft.LatitudeDegrees,
		LongitudeDegrees:          aircraft.LongitudeDegrees,
		BarometricAltitudeFeet:    aircraft.BarometricAltitudeFeet,
		GroundSpeedKnots:          aircraft.GroundSpeedKnots,
		TrackDegrees:              aircraft.TrackDegrees,
		VerticalRateFeetPerMinute: aircraft.VerticalRateFeetPerMinute,
	}
}

func transmissionHistoryDTO(history simulation.HistorySnapshot) simulatorapi.TransmissionHistory {
	messages := make([]simulatorapi.Transmission, 0, len(history.Messages))
	for _, message := range history.Messages {
		messages = append(messages, simulatorapi.Transmission{
			Sequence:  simulatorapi.FormatUint64(message.Sequence),
			ICAO:      simulatorapi.FormatICAO(message.ICAO),
			Timestamp: simulatorapi.FormatTime(message.Timestamp),
			Kind:      message.Kind.String(),
			Frame:     simulatorapi.FormatFrame(message.Frame),
		})
	}
	return simulatorapi.TransmissionHistory{
		Messages:       messages,
		OldestSequence: simulatorapi.FormatUint64(history.OldestSequence),
		LatestSequence: simulatorapi.FormatUint64(history.LatestSequence),
		Limit:          history.Limit,
	}
}

func spawnRangeDTO(value simulation.Range) simulatorapi.SpawnRange {
	return simulatorapi.SpawnRange{Min: value.Min, Max: value.Max}
}

func simulationSettingsDTO(cfg simulation.Config) simulatorapi.SimulationSettings {
	return simulatorapi.SimulationSettings{
		ID:                   cfg.ID,
		StartTime:            simulatorapi.FormatTime(cfg.StartTime),
		Seed:                 simulatorapi.FormatUint64(cfg.Seed),
		InitialAircraftCount: cfg.InitialAircraftCount,
		SpeedHundredths:      int(cfg.SpeedHundredths),
		Spawn: simulatorapi.SpawnSettings{
			LatitudeDegrees:           spawnRangeDTO(cfg.Spawn.LatitudeDegrees),
			LongitudeDegrees:          spawnRangeDTO(cfg.Spawn.LongitudeDegrees),
			AltitudeFeet:              spawnRangeDTO(cfg.Spawn.AltitudeFeet),
			GroundSpeedKnots:          spawnRangeDTO(cfg.Spawn.GroundSpeedKnots),
			TrackDegrees:              spawnRangeDTO(cfg.Spawn.TrackDegrees),
			VerticalRateFeetPerMinute: spawnRangeDTO(cfg.Spawn.VerticalRateFeetPerMinute),
		},
	}
}

func receptionModelDTO(model simulation.ReceptionModel) simulatorapi.ReceptionModel {
	return simulatorapi.ReceptionModel{
		TransmitPowerDBm:            model.TransmitPowerDBm,
		FrequencyMHz:                model.FrequencyMHz,
		FreeSpacePathLossConstantDB: model.FreeSpacePathLossConstantDB,
		RefractionFactor:            model.RefractionFactor,
		HorizonMetresPerSqrtMetre:   model.HorizonMetresPerSqrtMetre,
		EarthRadiusMetres:           model.EarthRadiusMetres,
	}
}

// observationRequest converts a wire observation request, validating the
// station selection and all four positive expiry durations.
func observationRequest(request simulatorapi.ObservationRequest) (simulation.ObservationRequest, error) {
	var expiry simulation.ObservationExpiry
	lifetimes := []struct {
		field  string
		text   string
		target *time.Duration
	}{
		{"$.expiry.identityNanoseconds", request.Expiry.IdentityNanoseconds, &expiry.Identity},
		{"$.expiry.positionNanoseconds", request.Expiry.PositionNanoseconds, &expiry.Position},
		{"$.expiry.altitudeNanoseconds", request.Expiry.AltitudeNanoseconds, &expiry.Altitude},
		{"$.expiry.velocityNanoseconds", request.Expiry.VelocityNanoseconds, &expiry.Velocity},
	}
	for _, lifetime := range lifetimes {
		value, err := simulatorapi.ParseDuration(lifetime.text)
		if err != nil {
			return simulation.ObservationRequest{}, fmt.Errorf("%s: %w", lifetime.field, err)
		}
		if value <= 0 {
			return simulation.ObservationRequest{}, fmt.Errorf("%s must be positive", lifetime.field)
		}
		*lifetime.target = value
	}
	return simulation.ObservationRequest{
		StationIDs: copyStrings(request.StationIDs), Expiry: expiry,
	}, nil
}

// historyRequest converts a wire history request, including its optional
// resume cursor.
func historyRequest(request simulatorapi.HistoryRequest) (simulation.HistoryRequest, error) {
	converted := simulation.HistoryRequest{StationID: request.StationID, Limit: request.Limit}
	if request.Cursor == nil {
		return converted, nil
	}
	after, err := simulatorapi.ParseUint64(request.Cursor.AfterSequence)
	if err != nil {
		return simulation.HistoryRequest{}, fmt.Errorf("$.cursor.afterSequence: %w", err)
	}
	converted.Cursor = &simulation.ReceptionCursor{
		RunID:         request.Cursor.RunID,
		StationID:     request.Cursor.StationID,
		AfterSequence: after,
	}
	return converted, nil
}

// copyStrings returns a detached copy that is an empty slice, never nil, so
// it serializes as an array.
func copyStrings(values []string) []string {
	copied := make([]string, len(values))
	copy(copied, values)
	return copied
}
