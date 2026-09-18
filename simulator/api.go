package simulator

import (
	"context"
	"errors"
	"fmt"

	"github.com/miroslav-matejovsky/adsb-testbench/internal/simdriver"
	"github.com/miroslav-matejovsky/adsb-testbench/simulation"
	"github.com/miroslav-matejovsky/adsb-testbench/simulatorapi"
)

// API is one transport-neutral service over a running simulator.
//
// It is the single implementation behind both direct Go calls and the HTTP
// handler, so a local caller and a remote client observe the same operations,
// the same data, and the same failure categories. It owns no goroutine, no
// listener, and no second driver: every mutation goes through the runtime's
// existing serialized driver.
//
// An API is permanently attached to one runtime, whose run identifier cannot
// change. Every mutation must name that run, so a command written for a
// replacement run is a conflict instead of a silent misapplication.
//
// Every returned value is detached. Editing a response cannot change runtime
// state or a later response. Reads never advance virtual time.
type API struct {
	runtime *Simulator
	config  APIConfig
	runID   string
}

// NewAPI validates the runtime and configuration and returns a service. It
// starts no goroutine and no server.
func NewAPI(runtime *Simulator, config APIConfig) (*API, error) {
	if runtime == nil {
		return nil, fmt.Errorf("%w: simulator runtime is nil", simulatorapi.CategoryInvalid)
	}
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("create simulator API: %w", err)
	}
	return &API{runtime: runtime, config: config, runID: runtime.Snapshot().Config.ID}, nil
}

// RunID is the immutable identifier of the served engine lifetime.
func (a *API) RunID() string { return a.runID }

// Metadata returns effective settings, fixed engine and driver policy, the
// service's own explicit settings, and the current virtual instant, all from
// one read of the committed engine state.
func (a *API) Metadata(ctx context.Context) (simulatorapi.Metadata, error) {
	if err := ctx.Err(); err != nil {
		return simulatorapi.Metadata{}, a.serviceError("read metadata", err)
	}
	snapshot := a.runtime.Snapshot()
	return simulatorapi.Metadata{
		RunID:              snapshot.Config.ID,
		Now:                simulatorapi.FormatTime(snapshot.Now),
		ElapsedNanoseconds: simulatorapi.FormatDuration(snapshot.Elapsed),
		AircraftCount:      len(snapshot.Aircraft),
		StationCount:       len(snapshot.Stations),
		Simulation:         simulationSettingsDTO(snapshot.Config),
		Model:              receptionModelDTO(simulation.Model()),
		Limits: simulatorapi.EngineLimits{
			MaxAircraft:           simulation.MaxAircraft,
			MaxStations:           simulation.MaxStations,
			MaxSpeedHundredths:    simulation.MaxSpeedHundredths,
			HistoryLimit:          simulation.HistoryLimit,
			ReceptionHistoryLimit: simulation.ReceptionHistoryLimit,
			MaxHistoryPageSize:    simulation.MaxHistoryPageSize,
			MaxBatchFrames:        simulation.MaxBatchFrames,
			MaxBatchReceptions:    simulation.MaxBatchReceptions,
			MaxAdvanceNanoseconds: simulatorapi.FormatDuration(simulation.MaxAdvance),
		},
		Driver: simulatorapi.DriverLimits{
			HeartbeatNanoseconds:  simulatorapi.FormatDuration(simdriver.Heartbeat),
			MaxCatchUpNanoseconds: simulatorapi.FormatDuration(simdriver.MaxCatchUp),
		},
		Service: simulatorapi.ServiceSettings{
			MaxRequestBytes:               a.config.MaxRequestBytes,
			MaxResponseBytes:              a.config.MaxResponseBytes,
			RequestTimeoutNanoseconds:     simulatorapi.FormatDuration(a.config.RequestTimeout),
			CoverageReferenceAltitudeFeet: a.config.CoverageReferenceAltitudeFeet,
		},
	}, nil
}

// Truth returns one coherent manager-facing view of simulated state: current
// aircraft, current speed, virtual time, and bounded generated history.
//
// It is diagnostic data. A display must not build tracks from it.
func (a *API) Truth(ctx context.Context) (simulatorapi.TruthSnapshot, error) {
	if err := ctx.Err(); err != nil {
		return simulatorapi.TruthSnapshot{}, a.serviceError("read truth", err)
	}
	snapshot := a.runtime.Snapshot()
	aircraft := make([]simulatorapi.TruthAircraft, 0, len(snapshot.Aircraft))
	for _, item := range snapshot.Aircraft {
		aircraft = append(aircraft, truthAircraftDTO(item))
	}
	return simulatorapi.TruthSnapshot{
		RunID:                snapshot.Config.ID,
		Now:                  simulatorapi.FormatTime(snapshot.Now),
		ElapsedNanoseconds:   simulatorapi.FormatDuration(snapshot.Elapsed),
		InitialAircraftCount: snapshot.Config.InitialAircraftCount,
		AircraftCount:        len(snapshot.Aircraft),
		SpeedHundredths:      int(snapshot.Config.SpeedHundredths),
		Aircraft:             aircraft,
		History:              transmissionHistoryDTO(snapshot.History),
	}, nil
}

// Stations returns active stations in creation order with their estimated
// coverage at the configured reference altitude, all from one read instant.
func (a *API) Stations(ctx context.Context) (simulatorapi.StationsSnapshot, error) {
	if err := ctx.Err(); err != nil {
		return simulatorapi.StationsSnapshot{}, a.serviceError("read stations", err)
	}
	snapshot := a.runtime.Snapshot()
	stations := make([]simulatorapi.StationState, 0, len(snapshot.Stations))
	for _, station := range snapshot.Stations {
		coverage, err := simulation.EstimateCoverage(station.Config, a.config.CoverageReferenceAltitudeFeet)
		if err != nil {
			return simulatorapi.StationsSnapshot{}, a.serviceError("estimate station coverage", err)
		}
		stations = append(stations, simulatorapi.StationState{
			Station: stationDTO(station), Coverage: coverageDTO(coverage),
		})
	}
	return simulatorapi.StationsSnapshot{
		RunID: snapshot.Config.ID, Now: simulatorapi.FormatTime(snapshot.Now), Stations: stations,
	}, nil
}

// Observations returns decoded received state for an explicit station
// selection and explicit field lifetimes. Run identity and virtual time come
// from the single underlying read.
func (a *API) Observations(ctx context.Context, request simulatorapi.ObservationRequest) (simulatorapi.ObservationSnapshot, error) {
	native, err := observationRequest(request)
	if err != nil {
		return simulatorapi.ObservationSnapshot{}, a.invalidRequest("$.expiry", err)
	}
	snapshot, err := a.runtime.Observations(ctx, native)
	if err != nil {
		return simulatorapi.ObservationSnapshot{}, a.serviceError("read observations", err)
	}
	return observationSnapshotDTO(snapshot), nil
}

// ReceptionSnapshot returns all retained raw receptions of an explicit
// station selection at one virtual instant. It is the evidence a display
// decodes; it contains no decoded field and no aircraft truth.
func (a *API) ReceptionSnapshot(ctx context.Context, request simulatorapi.ReceptionSnapshotRequest) (simulatorapi.ReceptionSnapshot, error) {
	snapshot, err := a.runtime.ReceptionSnapshot(ctx,
		simulation.ReceptionSnapshotRequest{StationIDs: copyStrings(request.StationIDs)})
	if err != nil {
		return simulatorapi.ReceptionSnapshot{}, a.serviceError("read reception snapshot", err)
	}
	return receptionSnapshotDTO(snapshot), nil
}

// ReceptionHistory returns one explicit page of one station's retained
// receptions, resuming after an optional cursor bound to this run.
func (a *API) ReceptionHistory(ctx context.Context, request simulatorapi.HistoryRequest) (simulatorapi.ReceptionPage, error) {
	native, err := historyRequest(request)
	if err != nil {
		return simulatorapi.ReceptionPage{}, a.invalidRequest("$.cursor", err)
	}
	page, err := a.runtime.ReceptionHistory(ctx, native)
	if err != nil {
		return simulatorapi.ReceptionPage{}, a.serviceError("read reception history", err)
	}
	return receptionPageDTO(page), nil
}

// SetCount assigns the absolute number of active aircraft. The command must
// name the served run. Settlement of measured real time at the current speed
// stays inside the runtime's serialized driver.
func (a *API) SetCount(ctx context.Context, command simulatorapi.CountCommand) (simulatorapi.CommandAck, error) {
	if err := a.guardRun(command.RunID); err != nil {
		return simulatorapi.CommandAck{}, err
	}
	if err := a.runtime.SetCount(ctx, command.Count); err != nil {
		return simulatorapi.CommandAck{}, a.serviceError("set aircraft count", err)
	}
	return a.ack(simulatorapi.OperationSetCount), nil
}

// SetSpeed assigns virtual time per real time in hundredths. Zero pauses.
// Pending real time is settled at the previous speed by the runtime.
func (a *API) SetSpeed(ctx context.Context, command simulatorapi.SpeedCommand) (simulatorapi.CommandAck, error) {
	if err := a.guardRun(command.RunID); err != nil {
		return simulatorapi.CommandAck{}, err
	}
	if command.SpeedHundredths < 0 || command.SpeedHundredths > simulation.MaxSpeedHundredths {
		return simulatorapi.CommandAck{}, a.invalidRequest("$.speedHundredths",
			fmt.Errorf("speedHundredths %d is outside 0-%d", command.SpeedHundredths, simulation.MaxSpeedHundredths))
	}
	if err := a.runtime.SetSpeed(ctx, uint16(command.SpeedHundredths)); err != nil {
		return simulatorapi.CommandAck{}, a.serviceError("set speed", err)
	}
	return a.ack(simulatorapi.OperationSetSpeed), nil
}

// AddStation creates one station from complete settings and returns the
// station exactly as the command produced it, including its first revision.
func (a *API) AddStation(ctx context.Context, command simulatorapi.AddStationCommand) (simulatorapi.StationAck, error) {
	if err := a.guardRun(command.RunID); err != nil {
		return simulatorapi.StationAck{}, err
	}
	settings, err := stationSettings(command.Station)
	if err != nil {
		return simulatorapi.StationAck{}, a.invalidRequest("$.station", err)
	}
	station, err := a.runtime.AddStation(ctx, settings)
	if err != nil {
		return simulatorapi.StationAck{}, a.serviceError("add station", err)
	}
	return a.stationAck(simulatorapi.OperationAddStation, station), nil
}

// UpdateStation replaces every setting of the station named by id. The body
// station identifier must match id, and the expected revision must be the
// positive revision the caller last observed.
func (a *API) UpdateStation(ctx context.Context, id string, command simulatorapi.UpdateStationCommand) (simulatorapi.StationAck, error) {
	if err := a.guardRun(command.RunID); err != nil {
		return simulatorapi.StationAck{}, err
	}
	if command.Station.ID != id {
		return simulatorapi.StationAck{}, a.invalidRequest("$.station.id",
			fmt.Errorf("station id %q does not match the addressed station %q", command.Station.ID, id))
	}
	revision, err := a.expectedRevision(command.ExpectedRevision)
	if err != nil {
		return simulatorapi.StationAck{}, err
	}
	settings, err := stationSettings(command.Station)
	if err != nil {
		return simulatorapi.StationAck{}, a.invalidRequest("$.station", err)
	}
	station, err := a.runtime.UpdateStation(ctx, revision, settings)
	if err != nil {
		return simulatorapi.StationAck{}, a.serviceError("update station", err)
	}
	return a.stationAck(simulatorapi.OperationUpdateStation, station), nil
}

// RemoveStation deletes the station named by id when the expected revision
// matches. The identifier stays reserved for the rest of the run.
func (a *API) RemoveStation(ctx context.Context, id string, command simulatorapi.RemoveStationCommand) (simulatorapi.CommandAck, error) {
	if err := a.guardRun(command.RunID); err != nil {
		return simulatorapi.CommandAck{}, err
	}
	revision, err := a.expectedRevision(command.ExpectedRevision)
	if err != nil {
		return simulatorapi.CommandAck{}, err
	}
	if err := a.runtime.RemoveStation(ctx, id, revision); err != nil {
		return simulatorapi.CommandAck{}, a.serviceError("remove station", err)
	}
	return a.ack(simulatorapi.OperationRemoveStation), nil
}

// guardRun rejects a command written for another engine lifetime before any
// runtime work, including before measured real time would be settled.
func (a *API) guardRun(commandRunID string) error {
	if commandRunID != a.runID {
		return a.runConflict(commandRunID)
	}
	return nil
}

// expectedRevision parses the positive revision a station command must carry.
func (a *API) expectedRevision(text string) (uint64, error) {
	revision, err := simulatorapi.ParseUint64(text)
	if err != nil {
		return 0, a.invalidRequest("$.expectedRevision", err)
	}
	if revision == 0 {
		return 0, a.invalidRequest("$.expectedRevision", errors.New("expectedRevision must be positive"))
	}
	return revision, nil
}

func (a *API) ack(operation string) simulatorapi.CommandAck {
	return simulatorapi.CommandAck{RunID: a.runID, Operation: operation}
}

func (a *API) stationAck(operation string, station simulation.Station) simulatorapi.StationAck {
	return simulatorapi.StationAck{RunID: a.runID, Operation: operation, Station: stationDTO(station)}
}
