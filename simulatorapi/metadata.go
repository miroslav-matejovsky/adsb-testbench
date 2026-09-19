package simulatorapi

// Truth and metadata responses.
//
// Truth describes what the simulation decided, not what any receiver heard.
// It is manager-facing diagnostic data. A display builds tracks only from
// received evidence, so truth types are deliberately distinct from the
// received Aircraft type in this package.

// TruthAircraft is the exact simulated state of one aircraft at the snapshot
// instant. Values are not wire-quantized.
//
// Units are degrees, pressure-altitude feet relative to 1013.25 hPa, knots,
// and feet per minute positive upward.
type TruthAircraft struct {
	ICAO                      string  `json:"icao"`
	Callsign                  string  `json:"callsign"`
	CreatedAt                 string  `json:"createdAt"`
	LatitudeDegrees           float64 `json:"latitudeDegrees"`
	LongitudeDegrees          float64 `json:"longitudeDegrees"`
	BarometricAltitudeFeet    float64 `json:"barometricAltitudeFeet"`
	GroundSpeedKnots          float64 `json:"groundSpeedKnots"`
	TrackDegrees              float64 `json:"trackDegrees"`
	VerticalRateFeetPerMinute float64 `json:"verticalRateFeetPerMinute"`
}

// Transmission is one generated frame as emitted, before any reception model
// is applied. Sequence is a canonical decimal string starting at 1.
type Transmission struct {
	Sequence  string `json:"sequence"`
	ICAO      string `json:"icao"`
	Timestamp string `json:"timestamp"`
	Kind      string `json:"kind"`
	Frame     string `json:"frame"`
}

// TransmissionHistory is the engine's bounded generated-frame history.
// Messages are oldest first. Bounds are "0" when the history is empty.
type TransmissionHistory struct {
	Messages       []Transmission `json:"messages"`
	OldestSequence string         `json:"oldestSequence"`
	LatestSequence string         `json:"latestSequence"`
	Limit          int            `json:"limit"`
}

// TruthSnapshot is one coherent manager-facing view of simulated state.
//
// InitialAircraftCount is the count the run was configured with;
// AircraftCount is the live count. SpeedHundredths is the current speed of
// the committed engine state, not the configured one.
type TruthSnapshot struct {
	RunID                string              `json:"runId"`
	Now                  string              `json:"now"`
	ElapsedNanoseconds   string              `json:"elapsedNanoseconds"`
	InitialAircraftCount int                 `json:"initialAircraftCount"`
	AircraftCount        int                 `json:"aircraftCount"`
	SpeedHundredths      int                 `json:"speedHundredths"`
	Aircraft             []TruthAircraft     `json:"aircraft"`
	History              TransmissionHistory `json:"history"`
}

// Coverage is the synthetic reach of one station at one reference altitude.
// All three radii are great-circle surface radii in nautical miles.
type Coverage struct {
	ReferenceAltitudeFeet         float64 `json:"referenceAltitudeFeet"`
	HorizonRadiusNauticalMiles    float64 `json:"horizonRadiusNauticalMiles"`
	LinkBudgetRadiusNauticalMiles float64 `json:"linkBudgetRadiusNauticalMiles"`
	EffectiveRadiusNauticalMiles  float64 `json:"effectiveRadiusNauticalMiles"`
}

// StationState is one active station with its estimated coverage at the
// service's configured reference altitude.
type StationState struct {
	Station  Station  `json:"station"`
	Coverage Coverage `json:"coverage"`
}

// StationsSnapshot lists active stations in creation order at one read
// instant, so every coverage estimate describes the same state.
type StationsSnapshot struct {
	RunID    string         `json:"runId"`
	Now      string         `json:"now"`
	Stations []StationState `json:"stations"`
}

// SpawnRange is one sampling interval in its own field's unit. Equal
// endpoints mean an exact value.
type SpawnRange struct {
	Min float64 `json:"min"`
	Max float64 `json:"max"`
}

// SpawnSettings is the complete birth-state sampling configuration.
type SpawnSettings struct {
	LatitudeDegrees           SpawnRange `json:"latitudeDegrees"`
	LongitudeDegrees          SpawnRange `json:"longitudeDegrees"`
	AltitudeFeet              SpawnRange `json:"altitudeFeet"`
	GroundSpeedKnots          SpawnRange `json:"groundSpeedKnots"`
	TrackDegrees              SpawnRange `json:"trackDegrees"`
	VerticalRateFeetPerMinute SpawnRange `json:"verticalRateFeetPerMinute"`
}

// SimulationSettings is the effective configuration of the served run. Seed
// is a canonical decimal string. SpeedHundredths is the current speed.
type SimulationSettings struct {
	ID                   string        `json:"id"`
	StartTime            string        `json:"startTime"`
	Seed                 string        `json:"seed"`
	InitialAircraftCount int           `json:"initialAircraftCount"`
	SpeedHundredths      int           `json:"speedHundredths"`
	Spawn                SpawnSettings `json:"spawn"`
}

// ReceptionModel is the fixed synthetic propagation model. It describes this
// testbench, not any real receiver or propagation environment.
type ReceptionModel struct {
	TransmitPowerDBm            float64 `json:"transmitPowerDBm"`
	FrequencyMHz                float64 `json:"frequencyMHz"`
	FreeSpacePathLossConstantDB float64 `json:"freeSpacePathLossConstantDB"`
	RefractionFactor            float64 `json:"refractionFactor"`
	HorizonMetresPerSqrtMetre   float64 `json:"horizonMetresPerSqrtMetre"`
	EarthRadiusMetres           float64 `json:"earthRadiusMetres"`
}

// EngineLimits are fixed engine policy bounds. They are published so a client
// can validate input before sending it.
type EngineLimits struct {
	MaxAircraft           int    `json:"maxAircraft"`
	MaxStations           int    `json:"maxStations"`
	MaxSpeedHundredths    int    `json:"maxSpeedHundredths"`
	HistoryLimit          int    `json:"historyLimit"`
	ReceptionHistoryLimit int    `json:"receptionHistoryLimit"`
	MaxHistoryPageSize    int    `json:"maxHistoryPageSize"`
	MaxBatchFrames        int    `json:"maxBatchFrames"`
	MaxBatchReceptions    int    `json:"maxBatchReceptions"`
	MaxAdvanceNanoseconds string `json:"maxAdvanceNanoseconds"`
}

// DriverLimits are the fixed real-time pacing bounds of the running driver.
type DriverLimits struct {
	HeartbeatNanoseconds  string `json:"heartbeatNanoseconds"`
	MaxCatchUpNanoseconds string `json:"maxCatchUpNanoseconds"`
}

// ServiceSettings are the explicit transport settings of the serving API.
// They are host choices, never implicit defaults.
type ServiceSettings struct {
	MaxRequestBytes               int     `json:"maxRequestBytes"`
	MaxResponseBytes              int     `json:"maxResponseBytes"`
	RequestTimeoutNanoseconds     string  `json:"requestTimeoutNanoseconds"`
	CoverageReferenceAltitudeFeet float64 `json:"coverageReferenceAltitudeFeet"`
}

// Metadata is everything a client needs to configure itself against one run:
// effective settings, fixed policy, and the current virtual instant.
type Metadata struct {
	RunID              string             `json:"runId"`
	Now                string             `json:"now"`
	ElapsedNanoseconds string             `json:"elapsedNanoseconds"`
	AircraftCount      int                `json:"aircraftCount"`
	StationCount       int                `json:"stationCount"`
	Simulation         SimulationSettings `json:"simulation"`
	Model              ReceptionModel     `json:"model"`
	Limits             EngineLimits       `json:"limits"`
	Driver             DriverLimits       `json:"driver"`
	Service            ServiceSettings    `json:"service"`
}
