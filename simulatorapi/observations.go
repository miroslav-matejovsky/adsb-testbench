package simulatorapi

// StationSettings is the complete explicit configuration of one receiving
// station. Every field is required on the wire; there is no partial update
// and no default. Zero values are meaningful settings.
//
// Units are degrees, metres, dBi, dBm, dB, and a probability within [0,1].
type StationSettings struct {
	ID                   string  `json:"id"`
	Enabled              bool    `json:"enabled"`
	LatitudeDegrees      float64 `json:"latitudeDegrees"`
	LongitudeDegrees     float64 `json:"longitudeDegrees"`
	SiteElevationMetres  float64 `json:"siteElevationMetres"`
	AntennaHeightMetres  float64 `json:"antennaHeightMetres"`
	AntennaGainDBi       float64 `json:"antennaGainDBi"`
	SensitivityDBm       float64 `json:"sensitivityDBm"`
	SystemLossDB         float64 `json:"systemLossDB"`
	FrameLossProbability float64 `json:"frameLossProbability"`
}

// Station is one receiving station with its settings, revision, and creation
// instant. Embedded settings are flattened on the wire, so the JSON object
// holds the settings keys alongside revision and createdAt.
//
// Revision is a canonical decimal string starting at 1. CreatedAt is a UTC
// RFC3339Nano virtual instant.
type Station struct {
	StationSettings
	Revision  string `json:"revision"`
	CreatedAt string `json:"createdAt"`
}

// Reception is one exact received ADS-B frame with reception-time provenance.
type Reception struct {
	Sequence                string  `json:"sequence"`
	TransmissionSequence    string  `json:"transmissionSequence"`
	StationID               string  `json:"stationId"`
	StationRevision         string  `json:"stationRevision"`
	ICAO                    string  `json:"icao"`
	Kind                    string  `json:"kind"`
	Timestamp               string  `json:"timestamp"`
	Frame                   string  `json:"frame"`
	SlantRangeNauticalMiles float64 `json:"slantRangeNauticalMiles"`
	ReceivedPowerDBm        float64 `json:"receivedPowerDBm"`
	Receiver                Station `json:"receiver"`
}

// ReceptionCursor resumes one station's history after Sequence.
type ReceptionCursor struct {
	RunID         string `json:"runId"`
	StationID     string `json:"stationId"`
	AfterSequence string `json:"afterSequence"`
}

// HistoryRequest requests an explicit number of records from one station.
type HistoryRequest struct {
	StationID string           `json:"stationId"`
	Cursor    *ReceptionCursor `json:"cursor"`
	Limit     int              `json:"limit"`
}

// ReceptionPage is one transport-safe page of retained receptions.
type ReceptionPage struct {
	RunID          string          `json:"runId"`
	StationID      string          `json:"stationId"`
	Now            string          `json:"now"`
	Records        []Reception     `json:"records"`
	OldestSequence string          `json:"oldestSequence"`
	LatestSequence string          `json:"latestSequence"`
	NextCursor     ReceptionCursor `json:"nextCursor"`
	Gap            bool            `json:"gap"`
	HasMore        bool            `json:"hasMore"`
	RetentionLimit int             `json:"retentionLimit"`
}

// ReceptionSnapshotRequest selects the stations whose complete retained
// receptions are captured. An explicit empty array selects no station and is
// not an omitted field.
type ReceptionSnapshotRequest struct {
	StationIDs []string `json:"stationIds"`
}

// ReceptionSnapshot is every retained reception of an explicit station
// selection, captured at one virtual instant.
//
// Records are ordered by transmission sequence, then station ID. Retention
// describes exactly the records captured for each selected station. The
// snapshot carries raw evidence only: it contains no decoded field and no
// aircraft truth.
type ReceptionSnapshot struct {
	RunID      string             `json:"runId"`
	Now        string             `json:"now"`
	StationIDs []string           `json:"stationIds"`
	Retention  []StationRetention `json:"retention"`
	Records    []Reception        `json:"records"`
}

// ObservationExpiry uses explicit decimal nanosecond strings.
type ObservationExpiry struct {
	IdentityNanoseconds string `json:"identityNanoseconds"`
	PositionNanoseconds string `json:"positionNanoseconds"`
	AltitudeNanoseconds string `json:"altitudeNanoseconds"`
	VelocityNanoseconds string `json:"velocityNanoseconds"`
}

// ObservationRequest selects explicit station IDs and field lifetimes.
type ObservationRequest struct {
	StationIDs []string          `json:"stationIds"`
	Expiry     ObservationExpiry `json:"expiry"`
}

// Evidence identifies one received transmission. Receptions includes every
// selected receiver copy used by the snapshot.
type Evidence struct {
	TransmissionSequence string      `json:"transmissionSequence"`
	ICAO                 string      `json:"icao"`
	Kind                 string      `json:"kind"`
	Timestamp            string      `json:"timestamp"`
	Frame                string      `json:"frame"`
	Receptions           []Reception `json:"receptions"`
}

// Measurement preserves availability by pointer use at its containing field.
type Measurement struct {
	Value     float64 `json:"value"`
	OverRange bool    `json:"overRange"`
}

// IdentityObservation is one received identity field.
type IdentityObservation struct {
	Callsign   string   `json:"callsign"`
	ObservedAt string   `json:"observedAt"`
	Evidence   Evidence `json:"evidence"`
}

// PositionObservation is a received global CPR fix.
type PositionObservation struct {
	LatitudeDegrees  float64    `json:"latitudeDegrees"`
	LongitudeDegrees float64    `json:"longitudeDegrees"`
	ObservedAt       string     `json:"observedAt"`
	Evidence         []Evidence `json:"evidence"`
}

// AltitudeObservation is one received pressure-altitude field.
type AltitudeObservation struct {
	Feet       float64  `json:"feet"`
	ObservedAt string   `json:"observedAt"`
	Evidence   Evidence `json:"evidence"`
}

// VelocityObservation retains raw wire availability and derived ground motion.
type VelocityObservation struct {
	Subtype                   uint8        `json:"subtype"`
	IntentChange              bool         `json:"intentChange"`
	IFRCapability             bool         `json:"ifrCapability"`
	NACv                      uint8        `json:"nacv"`
	EastKnots                 *Measurement `json:"eastKnots"`
	NorthKnots                *Measurement `json:"northKnots"`
	GroundSpeedKnots          *float64     `json:"groundSpeedKnots"`
	TrackDegrees              *float64     `json:"trackDegrees"`
	HeadingDegrees            *float64     `json:"headingDegrees"`
	AirspeedKnots             *Measurement `json:"airspeedKnots"`
	TrueAirspeed              bool         `json:"trueAirspeed"`
	BarometricVerticalRate    bool         `json:"barometricVerticalRate"`
	VerticalRateFeetPerMinute *Measurement `json:"verticalRateFeetPerMinute"`
	GNSSMinusBaroFeet         *Measurement `json:"gnssMinusBaroFeet"`
	ObservedAt                string       `json:"observedAt"`
	Evidence                  Evidence     `json:"evidence"`
}

// Aircraft is partial received state for one ICAO address.
type Aircraft struct {
	ICAO               string               `json:"icao"`
	LastReceivedAt     string               `json:"lastReceivedAt"`
	Identity           *IdentityObservation `json:"identity"`
	Position           *PositionObservation `json:"position"`
	BarometricAltitude *AltitudeObservation `json:"barometricAltitude"`
	Velocity           *VelocityObservation `json:"velocity"`
}

// StationRetention describes bounded evidence for a selected receiver.
type StationRetention struct {
	StationID      string `json:"stationId"`
	OldestSequence string `json:"oldestSequence"`
	LatestSequence string `json:"latestSequence"`
	Truncated      bool   `json:"truncated"`
	Limit          int    `json:"limit"`
}

// ObservationSnapshot is a transport-safe received-data snapshot.
type ObservationSnapshot struct {
	RunID      string             `json:"runId"`
	Now        string             `json:"now"`
	StationIDs []string           `json:"stationIds"`
	Retention  []StationRetention `json:"retention"`
	Expiry     ObservationExpiry  `json:"expiry"`
	Aircraft   []Aircraft         `json:"aircraft"`
}
