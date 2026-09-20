package qualitytest

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"slices"
	"strconv"
	"strings"
	"time"
)

// Frame classifications. External frames come from a cited publication and
// are independent protocol evidence. Synthetic frames are constructed for a
// test and prove nothing about the protocol on their own.
const (
	External  = "external"
	Synthetic = "synthetic"
)

// Frame families.
const (
	IdentificationFamily = "identification"
	PositionFamily       = "position"
	VelocityFamily       = "velocity"
)

// Scenario kinds.
const (
	// EngineKind scenarios carry explicit engine inputs and runs.
	EngineKind = "engine"
	// ReferenceKind scenarios name a literal reference fixture file.
	ReferenceKind = "reference"
)

// Scenario operations.
const (
	OpAdvance       = "advance"
	OpElapse        = "elapse"
	OpSetCount      = "setCount"
	OpSetSpeed      = "setSpeed"
	OpAddStation    = "addStation"
	OpUpdateStation = "updateStation"
	OpRemoveStation = "removeStation"
)

// Assertions is the closed vocabulary of scenario assertion names. A
// scenario naming anything else is rejected, so a typo cannot silently skip
// a check. The testdata/quality README maps each name to its owning tests.
var Assertions = []string{
	"allocated-empty-arrays", "no-fabricated-tracks",
	"known-fields", "partial-tracks", "cpr-coordinate-tolerance", "distinct-original-icaos",
	"identical-batches", "identical-future",
	"paused-time-discarded", "physical-speed-unchanged",
	"historical-revisions", "receiver-provenance", "unchanged-transmissions",
	"receive-all", "drop-all", "seeded-partial-loss", "reproducible",
	"inclusive-lifetime", "expired-after-lifetime", "cpr-pair-age-bound",
	"bounded-records", "eviction-gap", "removed-aircraft-observed", "new-identities",
	"same-generated-payloads", "old-cursor-rejected", "no-cross-run-fallback",
}

// FrameCorpus is the content of frames.json.
type FrameCorpus struct {
	Frames   []Frame
	CPRPairs []CPRPair
}

// Frame returns the frame with id.
func (c FrameCorpus) Frame(id string) (Frame, error) {
	for _, frame := range c.Frames {
		if frame.ID == id {
			return frame, nil
		}
	}
	return Frame{}, fmt.Errorf("frame %q is not in the corpus", id)
}

// Frame is one literal frame with provenance and expected wire fields.
type Frame struct {
	ID             string
	Classification string
	// Hex is the frame as stored: 28 uppercase hexadecimal digits.
	Hex            string
	Bytes          [14]byte
	SourceURL      string
	SourceLocation string
	Verification   string
	Expected       ExpectedFrame
}

// ExpectedFrame states the fields a frame carries. Exactly one family
// payload is set, matching Family.
type ExpectedFrame struct {
	ICAO           uint32
	Family         string
	Capability     uint8
	Identification *ExpectedIdentification
	Position       *ExpectedPosition
	Velocity       *ExpectedVelocity
}

// ExpectedIdentification is an identification payload.
type ExpectedIdentification struct {
	TypeCode uint8  `json:"typeCode"`
	Category uint8  `json:"category"`
	Callsign string `json:"callsign"`
}

// ExpectedPosition is an airborne position payload. A nil AltitudeFeet is an
// unavailable altitude.
type ExpectedPosition struct {
	TypeCode           uint8       `json:"typeCode"`
	SurveillanceStatus uint8       `json:"surveillanceStatus"`
	Supplement         bool        `json:"supplement"`
	TimeSynchronized   bool        `json:"timeSynchronized"`
	AltitudeFeet       *float64    `json:"altitudeFeet"`
	CPR                ExpectedCPR `json:"cpr"`
}

// ExpectedCPR is one raw CPR half.
type ExpectedCPR struct {
	Odd       bool   `json:"odd"`
	Latitude  uint32 `json:"latitude"`
	Longitude uint32 `json:"longitude"`
}

// Measurement is a value with its over-range flag. Nil pointers to it mean
// unavailable.
type Measurement struct {
	Value     float64 `json:"value"`
	OverRange bool    `json:"overRange"`
}

// ExpectedVelocity is an airborne velocity payload. Nil pointers are
// unavailable wire values or fields of another subtype.
type ExpectedVelocity struct {
	Subtype                   uint8        `json:"subtype"`
	IntentChange              bool         `json:"intentChange"`
	IFRCapability             bool         `json:"ifrCapability"`
	NACv                      uint8        `json:"nacv"`
	EastKnots                 *Measurement `json:"eastKnots"`
	NorthKnots                *Measurement `json:"northKnots"`
	HeadingDegrees            *float64     `json:"headingDegrees"`
	AirspeedKnots             *Measurement `json:"airspeedKnots"`
	TrueAirspeed              bool         `json:"trueAirspeed"`
	BarometricVerticalRate    bool         `json:"barometricVerticalRate"`
	VerticalRateFeetPerMinute *Measurement `json:"verticalRateFeetPerMinute"`
	GNSSMinusBaroFeet         *Measurement `json:"gnssMinusBaroFeet"`
}

// CPRPair is an even and an odd position frame with expected global fixes.
type CPRPair struct {
	ID             string
	Even           string
	Odd            string
	SourceURL      string
	SourceLocation string
	Verification   string
	// TimeProvenance says where the reception instants come from.
	TimeProvenance string
	Cases          []CPRCase
}

// CPRCase is one timing of a pair and the fix it must decode to.
type CPRCase struct {
	Name             string
	EvenTime         time.Time
	OddTime          time.Time
	LatitudeDegrees  float64
	LongitudeDegrees float64
	ToleranceDegrees float64
}

// Scenario is one named entry of scenarios.json.
//
// Engine scenarios set Engine, Stations, Selection, Expiry and Runs. Stations
// are added right after the engine is created and before the operations of
// every run. Each run starts from a fresh engine. Reference scenarios set
// Reference to a fixture file name in the same directory.
type Scenario struct {
	ID             string
	Kind           string
	Classification string
	Description    string
	Engine         Engine
	Stations       []Station
	Selection      []string
	Expiry         Expiry
	Runs           []Run
	Reference      string
	Assertions     []string
}

// Asserts reports whether the scenario names assertion.
func (s Scenario) Asserts(assertion string) bool {
	return slices.Contains(s.Assertions, assertion)
}

// Run returns the run named name.
func (s Scenario) Run(name string) (Run, error) {
	for _, run := range s.Runs {
		if run.Name == name {
			return run, nil
		}
	}
	return Run{}, fmt.Errorf("scenario %q has no run %q", s.ID, name)
}

// Engine is a complete explicit engine configuration without the run ID,
// which each run supplies.
type Engine struct {
	Seed                 uint64
	StartTime            time.Time
	InitialAircraftCount int
	SpeedHundredths      uint16
	Spawn                Spawn
}

// Spawn lists the birth ranges of every aircraft.
type Spawn struct {
	LatitudeDegrees           Range `json:"latitudeDegrees"`
	LongitudeDegrees          Range `json:"longitudeDegrees"`
	AltitudeFeet              Range `json:"altitudeFeet"`
	GroundSpeedKnots          Range `json:"groundSpeedKnots"`
	TrackDegrees              Range `json:"trackDegrees"`
	VerticalRateFeetPerMinute Range `json:"verticalRateFeetPerMinute"`
}

// Range is a sampling interval.
type Range struct {
	Min float64 `json:"min"`
	Max float64 `json:"max"`
}

// Station is a complete station configuration. Its JSON keys match the
// simulator wire settings.
type Station struct {
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

// Expiry holds the four field lifetimes.
type Expiry struct {
	Identity time.Duration
	Position time.Duration
	Altitude time.Duration
	Velocity time.Duration
}

// Run is one named ordered operation list with its run ID.
type Run struct {
	Name       string
	RunID      string
	Operations []Operation
}

// Operation is one scenario command. Only the fields of its Op are set:
// Duration for advance and elapse (virtual and real time respectively),
// Count for setCount, SpeedHundredths for setSpeed, Station for addStation
// and updateStation, StationID for removeStation, and ExpectedRevision for
// updateStation and removeStation.
type Operation struct {
	Op               string
	Duration         time.Duration
	Count            int
	SpeedHundredths  uint16
	Station          Station
	StationID        string
	ExpectedRevision uint64
}

// DisplayReference is the content of display-reference.json.
type DisplayReference struct {
	ID             string
	Classification string
	Description    string
	// Frames lists the frames.json identifiers the snapshot carries.
	Frames []string
	Expiry Expiry
	// Snapshot is the literal raw reception snapshot JSON object, exactly as
	// stored, so transport tests can serve it byte for byte.
	Snapshot json.RawMessage
	// Aircraft are the expected observations in ICAO order.
	Aircraft []ReferenceAircraft
}

// ReferenceAircraft is the expected partial track of one address. Nil
// fields must be absent from the decoded track.
type ReferenceAircraft struct {
	ICAO               string             `json:"icao"`
	LastReceivedAt     string             `json:"lastReceivedAt"`
	Identity           *ReferenceIdentity `json:"identity"`
	Position           *ReferencePosition `json:"position"`
	BarometricAltitude *ReferenceAltitude `json:"barometricAltitude"`
	Velocity           *ReferenceVelocity `json:"velocity"`
}

// ReferenceIdentity is an expected identity field.
type ReferenceIdentity struct {
	Callsign                     string `json:"callsign"`
	ObservedAt                   string `json:"observedAt"`
	EvidenceTransmissionSequence string `json:"evidenceTransmissionSequence"`
}

// ReferencePosition is an expected global fix.
type ReferencePosition struct {
	LatitudeDegrees               float64  `json:"latitudeDegrees"`
	LongitudeDegrees              float64  `json:"longitudeDegrees"`
	ToleranceDegrees              float64  `json:"toleranceDegrees"`
	ObservedAt                    string   `json:"observedAt"`
	EvidenceTransmissionSequences []string `json:"evidenceTransmissionSequences"`
	Derivation                    string   `json:"derivation"`
}

// ReferenceAltitude is an expected pressure altitude.
type ReferenceAltitude struct {
	Feet                         float64 `json:"feet"`
	ObservedAt                   string  `json:"observedAt"`
	EvidenceTransmissionSequence string  `json:"evidenceTransmissionSequence"`
}

// ReferenceVelocity is an expected velocity field. DerivedTolerance applies
// to GroundSpeedKnots and TrackDegrees, which are derived from the wire
// components; wire values compare exactly.
type ReferenceVelocity struct {
	Subtype                      uint8        `json:"subtype"`
	EastKnots                    *Measurement `json:"eastKnots"`
	NorthKnots                   *Measurement `json:"northKnots"`
	GroundSpeedKnots             *float64     `json:"groundSpeedKnots"`
	TrackDegrees                 *float64     `json:"trackDegrees"`
	DerivedTolerance             float64      `json:"derivedTolerance"`
	HeadingDegrees               *float64     `json:"headingDegrees"`
	AirspeedKnots                *Measurement `json:"airspeedKnots"`
	TrueAirspeed                 bool         `json:"trueAirspeed"`
	BarometricVerticalRate       bool         `json:"barometricVerticalRate"`
	VerticalRateFeetPerMinute    *Measurement `json:"verticalRateFeetPerMinute"`
	GNSSMinusBaroFeet            *Measurement `json:"gnssMinusBaroFeet"`
	ObservedAt                   string       `json:"observedAt"`
	EvidenceTransmissionSequence string       `json:"evidenceTransmissionSequence"`
	Derivation                   string       `json:"derivation"`
}

// LoadFrames reads and validates a frames.json file.
func LoadFrames(path string) (FrameCorpus, error) {
	var corpus FrameCorpus
	err := loadFile(path, func(root json.RawMessage) error {
		var err error
		corpus, err = parseFrames(root)
		return err
	})
	return corpus, err
}

// LoadScenarios reads and validates a scenarios.json file.
func LoadScenarios(path string) ([]Scenario, error) {
	var scenarios []Scenario
	err := loadFile(path, func(root json.RawMessage) error {
		var err error
		scenarios, err = parseScenarios(root)
		return err
	})
	return scenarios, err
}

// LoadDisplayReference reads and validates a display-reference.json file.
func LoadDisplayReference(path string) (DisplayReference, error) {
	var reference DisplayReference
	err := loadFile(path, func(root json.RawMessage) error {
		var err error
		reference, err = parseDisplayReference(root)
		return err
	})
	return reference, err
}

// FindScenario returns the scenario with id.
func FindScenario(scenarios []Scenario, id string) (Scenario, error) {
	for _, scenario := range scenarios {
		if scenario.ID == id {
			return scenario, nil
		}
	}
	return Scenario{}, fmt.Errorf("scenario %q is not in the corpus", id)
}

// loadFile reads one JSON document, rejects trailing data, and wraps every
// error with the file path.
func loadFile(path string, parse func(json.RawMessage) error) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("load %s: %w", path, err)
	}
	root, err := single(data)
	if err != nil {
		return fmt.Errorf("load %s: %w", path, err)
	}
	if err := parse(root); err != nil {
		return fmt.Errorf("load %s: %w", path, err)
	}
	return nil
}

// single returns the one JSON value in data and rejects anything after it.
func single(data []byte) (json.RawMessage, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	var root json.RawMessage
	if err := decoder.Decode(&root); err != nil {
		return nil, fmt.Errorf("decode JSON: %w", err)
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		return nil, errors.New("trailing data after the JSON value")
	}
	return root, nil
}

// object is one JSON object whose key set has been checked.
type object struct {
	where  string
	fields map[string]json.RawMessage
}

// strictObject decodes raw as an object holding exactly keys.
func strictObject(raw json.RawMessage, where string, keys ...string) (object, error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil || fields == nil {
		return object{}, fmt.Errorf("%s: want a JSON object", where)
	}
	allowed := make(map[string]bool, len(keys))
	for _, key := range keys {
		allowed[key] = true
		if _, ok := fields[key]; !ok {
			return object{}, fmt.Errorf("%s: missing field %q", where, key)
		}
	}
	for key := range fields {
		if !allowed[key] {
			return object{}, fmt.Errorf("%s: unknown field %q", where, key)
		}
	}
	return object{where: where, fields: fields}, nil
}

// value decodes a required non-null field strictly into dst.
func (o object) value(key string, dst any) error {
	raw := o.fields[key]
	if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return fmt.Errorf("%s.%s: null where a value is required", o.where, key)
	}
	return strictDecode(raw, fmt.Sprintf("%s.%s", o.where, key), dst)
}

// raw returns a required non-null field undecoded.
func (o object) raw(key string) (json.RawMessage, error) {
	raw := o.fields[key]
	if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return nil, fmt.Errorf("%s.%s: null where a value is required", o.where, key)
	}
	return raw, nil
}

// text decodes a required non-blank string field.
func (o object) text(key string) (string, error) {
	var text string
	if err := o.value(key, &text); err != nil {
		return "", err
	}
	if strings.TrimSpace(text) == "" {
		return "", fmt.Errorf("%s.%s: empty string", o.where, key)
	}
	return text, nil
}

// strictDecode decodes raw into dst, rejecting unknown object keys at every
// depth. A null nested inside a struct is permitted only for pointer fields;
// requireFields checks presence of struct keys.
func strictDecode(raw json.RawMessage, where string, dst any) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		return fmt.Errorf("%s: %w", where, err)
	}
	return nil
}

// requireKeys checks that raw is an object holding exactly keys, none of
// them null, for nested payloads decoded by strictDecode into tagged structs.
func requireKeys(raw json.RawMessage, where string, keys ...string) error {
	o, err := strictObject(raw, where, keys...)
	if err != nil {
		return err
	}
	for _, key := range keys {
		if _, err := o.raw(key); err != nil {
			return err
		}
	}
	return nil
}

// nestedKeys checks the keys of the object held in field key of raw.
func nestedKeys(raw json.RawMessage, where, key string, keys ...string) error {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return fmt.Errorf("%s: %w", where, err)
	}
	return requireKeys(fields[key], where+"."+key, keys...)
}

func parseFrames(root json.RawMessage) (FrameCorpus, error) {
	top, err := strictObject(root, "$", "frames", "cprPairs")
	if err != nil {
		return FrameCorpus{}, err
	}
	var rawFrames, rawPairs []json.RawMessage
	if err := top.value("frames", &rawFrames); err != nil {
		return FrameCorpus{}, err
	}
	if err := top.value("cprPairs", &rawPairs); err != nil {
		return FrameCorpus{}, err
	}
	if len(rawFrames) == 0 {
		return FrameCorpus{}, errors.New("$.frames: no frame")
	}
	corpus := FrameCorpus{Frames: make([]Frame, 0, len(rawFrames)), CPRPairs: make([]CPRPair, 0, len(rawPairs))}
	seen := map[string]bool{}
	for i, raw := range rawFrames {
		frame, err := parseFrame(raw, fmt.Sprintf("$.frames[%d]", i))
		if err != nil {
			return FrameCorpus{}, err
		}
		if seen[frame.ID] {
			return FrameCorpus{}, fmt.Errorf("frame %q: duplicate id", frame.ID)
		}
		seen[frame.ID] = true
		corpus.Frames = append(corpus.Frames, frame)
	}
	for i, raw := range rawPairs {
		pair, err := parseCPRPair(raw, fmt.Sprintf("$.cprPairs[%d]", i), corpus)
		if err != nil {
			return FrameCorpus{}, err
		}
		if seen[pair.ID] {
			return FrameCorpus{}, fmt.Errorf("CPR pair %q: duplicate id", pair.ID)
		}
		seen[pair.ID] = true
		corpus.CPRPairs = append(corpus.CPRPairs, pair)
	}
	return corpus, nil
}

func parseFrame(raw json.RawMessage, where string) (Frame, error) {
	o, err := strictObject(raw, where,
		"id", "classification", "hex", "sourceUrl", "sourceLocation", "verification", "expected")
	if err != nil {
		return Frame{}, err
	}
	var frame Frame
	if frame.ID, err = o.text("id"); err != nil {
		return Frame{}, err
	}
	where = fmt.Sprintf("frame %q", frame.ID)
	o.where = where
	if frame.Classification, err = classification(o); err != nil {
		return Frame{}, err
	}
	if frame.Hex, err = o.text("hex"); err != nil {
		return Frame{}, err
	}
	if frame.Bytes, err = parseFrameHex(frame.Hex); err != nil {
		return Frame{}, fmt.Errorf("%s: %w", where, err)
	}
	if err := o.value("sourceUrl", &frame.SourceURL); err != nil {
		return Frame{}, err
	}
	if frame.Classification == External && !strings.HasPrefix(frame.SourceURL, "https://") {
		return Frame{}, fmt.Errorf("%s: an external frame needs an https sourceUrl", where)
	}
	if frame.SourceLocation, err = o.text("sourceLocation"); err != nil {
		return Frame{}, err
	}
	if frame.Verification, err = o.text("verification"); err != nil {
		return Frame{}, err
	}
	expected, err := o.raw("expected")
	if err != nil {
		return Frame{}, err
	}
	if frame.Expected, err = parseExpected(expected, where+".expected"); err != nil {
		return Frame{}, err
	}
	// The address is a fixed byte range of every DF17 frame, so this is a
	// structural consistency check, not a decode.
	address := uint32(frame.Bytes[1])<<16 | uint32(frame.Bytes[2])<<8 | uint32(frame.Bytes[3])
	if address != frame.Expected.ICAO {
		return Frame{}, fmt.Errorf("%s: expected ICAO %06X differs from frame address %06X",
			where, frame.Expected.ICAO, address)
	}
	return frame, nil
}

func parseExpected(raw json.RawMessage, where string) (ExpectedFrame, error) {
	var head struct {
		Family string `json:"family"`
	}
	if err := json.Unmarshal(raw, &head); err != nil {
		return ExpectedFrame{}, fmt.Errorf("%s: %w", where, err)
	}
	switch head.Family {
	case IdentificationFamily, PositionFamily, VelocityFamily:
	default:
		return ExpectedFrame{}, fmt.Errorf("%s.family: unknown family %q", where, head.Family)
	}
	o, err := strictObject(raw, where, "icao", "family", "capability", head.Family)
	if err != nil {
		return ExpectedFrame{}, err
	}
	expected := ExpectedFrame{Family: head.Family}
	icao, err := o.text("icao")
	if err != nil {
		return ExpectedFrame{}, err
	}
	if expected.ICAO, err = parseICAO(icao); err != nil {
		return ExpectedFrame{}, fmt.Errorf("%s.icao: %w", where, err)
	}
	if err := o.value("capability", &expected.Capability); err != nil {
		return ExpectedFrame{}, err
	}
	payload, err := o.raw(head.Family)
	if err != nil {
		return ExpectedFrame{}, err
	}
	payloadWhere := where + "." + head.Family
	switch head.Family {
	case IdentificationFamily:
		expected.Identification = &ExpectedIdentification{}
		err = decodeTagged(payload, payloadWhere, expected.Identification)
	case PositionFamily:
		expected.Position = &ExpectedPosition{}
		if err = decodeTagged(payload, payloadWhere, expected.Position); err == nil {
			err = nestedKeys(payload, payloadWhere, "cpr", "odd", "latitude", "longitude")
		}
	case VelocityFamily:
		expected.Velocity = &ExpectedVelocity{}
		if err = decodeTagged(payload, payloadWhere, expected.Velocity); err == nil {
			err = measurementKeys(payload, payloadWhere, "eastKnots", "northKnots", "airspeedKnots",
				"verticalRateFeetPerMinute", "gnssMinusBaroFeet")
		}
	}
	if err != nil {
		return ExpectedFrame{}, err
	}
	return expected, nil
}

// decodeTagged decodes a struct whose every JSON key is required. Nullable
// fields are pointers; a null on a non-pointer field is rejected.
func decodeTagged(raw json.RawMessage, where string, dst any) error {
	keys, nullable := structKeys(dst)
	o, err := strictObject(raw, where, keys...)
	if err != nil {
		return err
	}
	for key, value := range o.fields {
		if !nullable[key] && bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			return fmt.Errorf("%s.%s: null where a value is required", where, key)
		}
	}
	return strictDecode(raw, where, dst)
}

// measurementKeys checks that every present measurement object carries both
// of its keys.
func measurementKeys(raw json.RawMessage, where string, keys ...string) error {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return fmt.Errorf("%s: %w", where, err)
	}
	for _, key := range keys {
		value := fields[key]
		if value == nil || bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			continue
		}
		if err := requireKeys(value, where+"."+key, "value", "overRange"); err != nil {
			return err
		}
	}
	return nil
}

func parseCPRPair(raw json.RawMessage, where string, corpus FrameCorpus) (CPRPair, error) {
	o, err := strictObject(raw, where, "id", "even", "odd", "sourceUrl", "sourceLocation",
		"verification", "timeProvenance", "cases")
	if err != nil {
		return CPRPair{}, err
	}
	var pair CPRPair
	if pair.ID, err = o.text("id"); err != nil {
		return CPRPair{}, err
	}
	where = fmt.Sprintf("CPR pair %q", pair.ID)
	o.where = where
	for _, field := range []struct {
		key string
		dst *string
	}{
		{"even", &pair.Even}, {"odd", &pair.Odd}, {"sourceUrl", &pair.SourceURL},
		{"sourceLocation", &pair.SourceLocation}, {"verification", &pair.Verification},
		{"timeProvenance", &pair.TimeProvenance},
	} {
		if *field.dst, err = o.text(field.key); err != nil {
			return CPRPair{}, err
		}
	}
	for _, half := range []struct {
		id  string
		odd bool
	}{{pair.Even, false}, {pair.Odd, true}} {
		frame, err := corpus.Frame(half.id)
		if err != nil {
			return CPRPair{}, fmt.Errorf("%s: %w", where, err)
		}
		if frame.Expected.Position == nil || frame.Expected.Position.CPR.Odd != half.odd {
			return CPRPair{}, fmt.Errorf("%s: frame %q is not a position half with odd=%t", where, half.id, half.odd)
		}
	}
	var rawCases []json.RawMessage
	if err := o.value("cases", &rawCases); err != nil {
		return CPRPair{}, err
	}
	if len(rawCases) == 0 {
		return CPRPair{}, fmt.Errorf("%s: no case", where)
	}
	names := map[string]bool{}
	for i, rawCase := range rawCases {
		c, err := parseCPRCase(rawCase, fmt.Sprintf("%s.cases[%d]", where, i))
		if err != nil {
			return CPRPair{}, err
		}
		if names[c.Name] {
			return CPRPair{}, fmt.Errorf("%s: duplicate case %q", where, c.Name)
		}
		names[c.Name] = true
		pair.Cases = append(pair.Cases, c)
	}
	return pair, nil
}

func parseCPRCase(raw json.RawMessage, where string) (CPRCase, error) {
	o, err := strictObject(raw, where, "name", "evenTime", "oddTime", "latitudeDegrees",
		"longitudeDegrees", "toleranceDegrees")
	if err != nil {
		return CPRCase{}, err
	}
	var c CPRCase
	if c.Name, err = o.text("name"); err != nil {
		return CPRCase{}, err
	}
	if c.EvenTime, err = timeField(o, "evenTime"); err != nil {
		return CPRCase{}, err
	}
	if c.OddTime, err = timeField(o, "oddTime"); err != nil {
		return CPRCase{}, err
	}
	for _, field := range []struct {
		key string
		dst *float64
	}{
		{"latitudeDegrees", &c.LatitudeDegrees}, {"longitudeDegrees", &c.LongitudeDegrees},
		{"toleranceDegrees", &c.ToleranceDegrees},
	} {
		if err := o.value(field.key, field.dst); err != nil {
			return CPRCase{}, err
		}
	}
	if !(c.ToleranceDegrees > 0) || math.IsInf(c.ToleranceDegrees, 0) {
		return CPRCase{}, fmt.Errorf("%s.toleranceDegrees: want a positive finite tolerance", where)
	}
	return c, nil
}

func parseScenarios(root json.RawMessage) ([]Scenario, error) {
	var rawScenarios []json.RawMessage
	if err := strictDecode(root, "$", &rawScenarios); err != nil {
		return nil, err
	}
	if len(rawScenarios) == 0 {
		return nil, errors.New("$: no scenario")
	}
	scenarios := make([]Scenario, 0, len(rawScenarios))
	seen := map[string]bool{}
	for i, raw := range rawScenarios {
		scenario, err := parseScenario(raw, fmt.Sprintf("$[%d]", i))
		if err != nil {
			return nil, err
		}
		if seen[scenario.ID] {
			return nil, fmt.Errorf("scenario %q: duplicate id", scenario.ID)
		}
		seen[scenario.ID] = true
		scenarios = append(scenarios, scenario)
	}
	return scenarios, nil
}

func parseScenario(raw json.RawMessage, where string) (Scenario, error) {
	var head struct {
		ID   string `json:"id"`
		Kind string `json:"kind"`
	}
	if err := json.Unmarshal(raw, &head); err != nil {
		return Scenario{}, fmt.Errorf("%s: %w", where, err)
	}
	if head.ID != "" {
		where = fmt.Sprintf("scenario %q", head.ID)
	}
	keys := []string{"id", "kind", "classification", "description", "assertions"}
	switch head.Kind {
	case EngineKind:
		keys = append(keys, "engine", "stations", "selection", "expiry", "runs")
	case ReferenceKind:
		keys = append(keys, "reference")
	default:
		return Scenario{}, fmt.Errorf("%s.kind: unknown kind %q", where, head.Kind)
	}
	o, err := strictObject(raw, where, keys...)
	if err != nil {
		return Scenario{}, err
	}
	scenario := Scenario{Kind: head.Kind}
	if scenario.ID, err = o.text("id"); err != nil {
		return Scenario{}, err
	}
	if scenario.Classification, err = classification(o); err != nil {
		return Scenario{}, err
	}
	if scenario.Description, err = o.text("description"); err != nil {
		return Scenario{}, err
	}
	if scenario.Assertions, err = assertions(o); err != nil {
		return Scenario{}, err
	}
	if head.Kind == ReferenceKind {
		if scenario.Reference, err = o.text("reference"); err != nil {
			return Scenario{}, err
		}
		return scenario, nil
	}
	if err := parseEngineScenario(o, &scenario); err != nil {
		return Scenario{}, err
	}
	return scenario, nil
}

func parseEngineScenario(o object, scenario *Scenario) error {
	rawEngine, err := o.raw("engine")
	if err != nil {
		return err
	}
	if scenario.Engine, err = parseEngine(rawEngine, o.where+".engine"); err != nil {
		return err
	}
	var rawStations []json.RawMessage
	if err := o.value("stations", &rawStations); err != nil {
		return err
	}
	scenario.Stations = make([]Station, 0, len(rawStations))
	ids := map[string]bool{}
	for i, raw := range rawStations {
		station, err := parseStation(raw, fmt.Sprintf("%s.stations[%d]", o.where, i))
		if err != nil {
			return err
		}
		if ids[station.ID] {
			return fmt.Errorf("%s.stations: duplicate station %q", o.where, station.ID)
		}
		ids[station.ID] = true
		scenario.Stations = append(scenario.Stations, station)
	}
	if err := o.value("selection", &scenario.Selection); err != nil {
		return err
	}
	if err := uniqueStrings(scenario.Selection, o.where+".selection"); err != nil {
		return err
	}
	rawExpiry, err := o.raw("expiry")
	if err != nil {
		return err
	}
	if scenario.Expiry, err = parseExpiry(rawExpiry, o.where+".expiry"); err != nil {
		return err
	}
	var rawRuns []json.RawMessage
	if err := o.value("runs", &rawRuns); err != nil {
		return err
	}
	if len(rawRuns) == 0 {
		return fmt.Errorf("%s.runs: no run", o.where)
	}
	names := map[string]bool{}
	for i, raw := range rawRuns {
		run, err := parseRun(raw, fmt.Sprintf("%s.runs[%d]", o.where, i))
		if err != nil {
			return err
		}
		if names[run.Name] {
			return fmt.Errorf("%s.runs: duplicate run %q", o.where, run.Name)
		}
		names[run.Name] = true
		scenario.Runs = append(scenario.Runs, run)
	}
	return nil
}

func parseEngine(raw json.RawMessage, where string) (Engine, error) {
	o, err := strictObject(raw, where, "seed", "startTime", "initialAircraftCount", "speedHundredths", "spawn")
	if err != nil {
		return Engine{}, err
	}
	var engine Engine
	seed, err := o.text("seed")
	if err != nil {
		return Engine{}, err
	}
	if engine.Seed, err = parseUint64(seed); err != nil {
		return Engine{}, fmt.Errorf("%s.seed: %w", where, err)
	}
	if engine.StartTime, err = timeField(o, "startTime"); err != nil {
		return Engine{}, err
	}
	if err := o.value("initialAircraftCount", &engine.InitialAircraftCount); err != nil {
		return Engine{}, err
	}
	if engine.InitialAircraftCount < 0 {
		return Engine{}, fmt.Errorf("%s.initialAircraftCount: negative count", where)
	}
	if err := o.value("speedHundredths", &engine.SpeedHundredths); err != nil {
		return Engine{}, err
	}
	rawSpawn, err := o.raw("spawn")
	if err != nil {
		return Engine{}, err
	}
	if err := decodeTagged(rawSpawn, where+".spawn", &engine.Spawn); err != nil {
		return Engine{}, err
	}
	var ranges map[string]json.RawMessage
	if err := json.Unmarshal(rawSpawn, &ranges); err != nil {
		return Engine{}, fmt.Errorf("%s.spawn: %w", where, err)
	}
	for key, value := range ranges {
		if err := requireKeys(value, where+".spawn."+key, "min", "max"); err != nil {
			return Engine{}, err
		}
	}
	return engine, nil
}

func parseStation(raw json.RawMessage, where string) (Station, error) {
	var station Station
	if err := decodeTagged(raw, where, &station); err != nil {
		return Station{}, err
	}
	if strings.TrimSpace(station.ID) == "" {
		return Station{}, fmt.Errorf("%s.id: empty station id", where)
	}
	return station, nil
}

func parseExpiry(raw json.RawMessage, where string) (Expiry, error) {
	o, err := strictObject(raw, where, "identityNanoseconds", "positionNanoseconds",
		"altitudeNanoseconds", "velocityNanoseconds")
	if err != nil {
		return Expiry{}, err
	}
	var expiry Expiry
	for _, field := range []struct {
		key string
		dst *time.Duration
	}{
		{"identityNanoseconds", &expiry.Identity}, {"positionNanoseconds", &expiry.Position},
		{"altitudeNanoseconds", &expiry.Altitude}, {"velocityNanoseconds", &expiry.Velocity},
	} {
		if *field.dst, err = durationField(o, field.key); err != nil {
			return Expiry{}, err
		}
	}
	return expiry, nil
}

func parseRun(raw json.RawMessage, where string) (Run, error) {
	o, err := strictObject(raw, where, "name", "runId", "operations")
	if err != nil {
		return Run{}, err
	}
	var run Run
	if run.Name, err = o.text("name"); err != nil {
		return Run{}, err
	}
	o.where = fmt.Sprintf("%s (run %q)", where, run.Name)
	if run.RunID, err = o.text("runId"); err != nil {
		return Run{}, err
	}
	var rawOperations []json.RawMessage
	if err := o.value("operations", &rawOperations); err != nil {
		return Run{}, err
	}
	run.Operations = make([]Operation, 0, len(rawOperations))
	for i, rawOperation := range rawOperations {
		operation, err := parseOperation(rawOperation, fmt.Sprintf("%s.operations[%d]", o.where, i))
		if err != nil {
			return Run{}, err
		}
		run.Operations = append(run.Operations, operation)
	}
	return run, nil
}

// operationKeys lists the exact keys of every operation.
var operationKeys = map[string][]string{
	OpAdvance:       {"op", "durationNanoseconds"},
	OpElapse:        {"op", "durationNanoseconds"},
	OpSetCount:      {"op", "count"},
	OpSetSpeed:      {"op", "speedHundredths"},
	OpAddStation:    {"op", "station"},
	OpUpdateStation: {"op", "expectedRevision", "station"},
	OpRemoveStation: {"op", "stationId", "expectedRevision"},
}

func parseOperation(raw json.RawMessage, where string) (Operation, error) {
	var head struct {
		Op string `json:"op"`
	}
	if err := json.Unmarshal(raw, &head); err != nil {
		return Operation{}, fmt.Errorf("%s: %w", where, err)
	}
	keys, ok := operationKeys[head.Op]
	if !ok {
		return Operation{}, fmt.Errorf("%s.op: unknown operation %q", where, head.Op)
	}
	o, err := strictObject(raw, where, keys...)
	if err != nil {
		return Operation{}, err
	}
	operation := Operation{Op: head.Op}
	switch head.Op {
	case OpAdvance, OpElapse:
		operation.Duration, err = durationField(o, "durationNanoseconds")
	case OpSetCount:
		if err = o.value("count", &operation.Count); err == nil && operation.Count < 0 {
			err = fmt.Errorf("%s.count: negative count", where)
		}
	case OpSetSpeed:
		err = o.value("speedHundredths", &operation.SpeedHundredths)
	case OpAddStation:
		operation.Station, err = stationField(o)
	case OpUpdateStation:
		if operation.ExpectedRevision, err = revisionField(o); err == nil {
			operation.Station, err = stationField(o)
		}
	case OpRemoveStation:
		if operation.StationID, err = o.text("stationId"); err == nil {
			operation.ExpectedRevision, err = revisionField(o)
		}
	}
	if err != nil {
		return Operation{}, err
	}
	return operation, nil
}

func stationField(o object) (Station, error) {
	raw, err := o.raw("station")
	if err != nil {
		return Station{}, err
	}
	return parseStation(raw, o.where+".station")
}

func revisionField(o object) (uint64, error) {
	text, err := o.text("expectedRevision")
	if err != nil {
		return 0, err
	}
	revision, err := parseUint64(text)
	if err != nil {
		return 0, fmt.Errorf("%s.expectedRevision: %w", o.where, err)
	}
	return revision, nil
}

func parseDisplayReference(root json.RawMessage) (DisplayReference, error) {
	o, err := strictObject(root, "$", "id", "classification", "description", "frames", "expiry", "snapshot", "expected")
	if err != nil {
		return DisplayReference{}, err
	}
	var reference DisplayReference
	if reference.ID, err = o.text("id"); err != nil {
		return DisplayReference{}, err
	}
	if reference.Classification, err = classification(o); err != nil {
		return DisplayReference{}, err
	}
	if reference.Description, err = o.text("description"); err != nil {
		return DisplayReference{}, err
	}
	if err := o.value("frames", &reference.Frames); err != nil {
		return DisplayReference{}, err
	}
	if len(reference.Frames) == 0 {
		return DisplayReference{}, errors.New("$.frames: no frame")
	}
	if err := uniqueStrings(reference.Frames, "$.frames"); err != nil {
		return DisplayReference{}, err
	}
	rawExpiry, err := o.raw("expiry")
	if err != nil {
		return DisplayReference{}, err
	}
	if reference.Expiry, err = parseExpiry(rawExpiry, "$.expiry"); err != nil {
		return DisplayReference{}, err
	}
	if reference.Snapshot, err = o.raw("snapshot"); err != nil {
		return DisplayReference{}, err
	}
	if _, err := strictObject(reference.Snapshot, "$.snapshot",
		"runId", "now", "stationIds", "retention", "records"); err != nil {
		return DisplayReference{}, err
	}
	rawExpected, err := o.raw("expected")
	if err != nil {
		return DisplayReference{}, err
	}
	expected, err := strictObject(rawExpected, "$.expected", "aircraft")
	if err != nil {
		return DisplayReference{}, err
	}
	var rawAircraft []json.RawMessage
	if err := expected.value("aircraft", &rawAircraft); err != nil {
		return DisplayReference{}, err
	}
	seen := map[string]bool{}
	for i, raw := range rawAircraft {
		aircraft, err := parseReferenceAircraft(raw, fmt.Sprintf("$.expected.aircraft[%d]", i))
		if err != nil {
			return DisplayReference{}, err
		}
		if seen[aircraft.ICAO] {
			return DisplayReference{}, fmt.Errorf("$.expected.aircraft: duplicate ICAO %q", aircraft.ICAO)
		}
		seen[aircraft.ICAO] = true
		reference.Aircraft = append(reference.Aircraft, aircraft)
	}
	return reference, nil
}

func parseReferenceAircraft(raw json.RawMessage, where string) (ReferenceAircraft, error) {
	var aircraft ReferenceAircraft
	if err := decodeTagged(raw, where, &aircraft); err != nil {
		return ReferenceAircraft{}, err
	}
	if _, err := parseICAO(aircraft.ICAO); err != nil {
		return ReferenceAircraft{}, fmt.Errorf("%s.icao: %w", where, err)
	}
	fields, err := strictObject(raw, where, "icao", "lastReceivedAt", "identity", "position",
		"barometricAltitude", "velocity")
	if err != nil {
		return ReferenceAircraft{}, err
	}
	nested := []struct {
		key string
		dst any
	}{
		{"identity", &ReferenceIdentity{}}, {"position", &ReferencePosition{}},
		{"barometricAltitude", &ReferenceAltitude{}}, {"velocity", &ReferenceVelocity{}},
	}
	for _, field := range nested {
		value := fields.fields[field.key]
		if bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			continue
		}
		if err := decodeTagged(value, where+"."+field.key, field.dst); err != nil {
			return ReferenceAircraft{}, err
		}
	}
	if aircraft.Velocity != nil {
		if err := measurementKeys(fields.fields["velocity"], where+".velocity", "eastKnots", "northKnots",
			"airspeedKnots", "verticalRateFeetPerMinute", "gnssMinusBaroFeet"); err != nil {
			return ReferenceAircraft{}, err
		}
	}
	return aircraft, nil
}

// structKeys lists the JSON keys of a tagged struct and which of them are
// pointers, so null is legal.
func structKeys(dst any) (keys []string, nullable map[string]bool) {
	nullable = map[string]bool{}
	switch dst.(type) {
	case *ExpectedIdentification:
		keys = []string{"typeCode", "category", "callsign"}
	case *ExpectedPosition:
		keys = []string{"typeCode", "surveillanceStatus", "supplement", "timeSynchronized", "altitudeFeet", "cpr"}
		nullable["altitudeFeet"] = true
	case *ExpectedVelocity:
		keys = []string{"subtype", "intentChange", "ifrCapability", "nacv", "eastKnots", "northKnots",
			"headingDegrees", "airspeedKnots", "trueAirspeed", "barometricVerticalRate",
			"verticalRateFeetPerMinute", "gnssMinusBaroFeet"}
		for _, key := range []string{"eastKnots", "northKnots", "headingDegrees", "airspeedKnots",
			"verticalRateFeetPerMinute", "gnssMinusBaroFeet"} {
			nullable[key] = true
		}
	case *Spawn:
		keys = []string{"latitudeDegrees", "longitudeDegrees", "altitudeFeet", "groundSpeedKnots",
			"trackDegrees", "verticalRateFeetPerMinute"}
	case *Station:
		keys = []string{"id", "enabled", "latitudeDegrees", "longitudeDegrees", "siteElevationMetres",
			"antennaHeightMetres", "antennaGainDBi", "sensitivityDBm", "systemLossDB", "frameLossProbability"}
	case *ReferenceAircraft:
		keys = []string{"icao", "lastReceivedAt", "identity", "position", "barometricAltitude", "velocity"}
		for _, key := range []string{"identity", "position", "barometricAltitude", "velocity"} {
			nullable[key] = true
		}
	case *ReferenceIdentity:
		keys = []string{"callsign", "observedAt", "evidenceTransmissionSequence"}
	case *ReferencePosition:
		keys = []string{"latitudeDegrees", "longitudeDegrees", "toleranceDegrees", "observedAt",
			"evidenceTransmissionSequences", "derivation"}
	case *ReferenceAltitude:
		keys = []string{"feet", "observedAt", "evidenceTransmissionSequence"}
	case *ReferenceVelocity:
		keys = []string{"subtype", "eastKnots", "northKnots", "groundSpeedKnots", "trackDegrees",
			"derivedTolerance", "headingDegrees", "airspeedKnots", "trueAirspeed", "barometricVerticalRate",
			"verticalRateFeetPerMinute", "gnssMinusBaroFeet", "observedAt", "evidenceTransmissionSequence",
			"derivation"}
		for _, key := range []string{"eastKnots", "northKnots", "groundSpeedKnots", "trackDegrees",
			"headingDegrees", "airspeedKnots", "verticalRateFeetPerMinute", "gnssMinusBaroFeet"} {
			nullable[key] = true
		}
	default:
		panic(fmt.Sprintf("qualitytest: no key list for %T", dst))
	}
	return keys, nullable
}

func classification(o object) (string, error) {
	text, err := o.text("classification")
	if err != nil {
		return "", err
	}
	if text != External && text != Synthetic {
		return "", fmt.Errorf("%s.classification: unknown classification %q", o.where, text)
	}
	return text, nil
}

func assertions(o object) ([]string, error) {
	var names []string
	if err := o.value("assertions", &names); err != nil {
		return nil, err
	}
	if len(names) == 0 {
		return nil, fmt.Errorf("%s.assertions: no assertion", o.where)
	}
	if err := uniqueStrings(names, o.where+".assertions"); err != nil {
		return nil, err
	}
	for _, name := range names {
		if !slices.Contains(Assertions, name) {
			return nil, fmt.Errorf("%s.assertions: unknown assertion %q", o.where, name)
		}
	}
	return names, nil
}

func uniqueStrings(values []string, where string) error {
	seen := make(map[string]bool, len(values))
	for _, value := range values {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("%s: empty entry", where)
		}
		if seen[value] {
			return fmt.Errorf("%s: duplicate entry %q", where, value)
		}
		seen[value] = true
	}
	return nil
}

func timeField(o object, key string) (time.Time, error) {
	text, err := o.text(key)
	if err != nil {
		return time.Time{}, err
	}
	if !strings.HasSuffix(text, "Z") {
		return time.Time{}, fmt.Errorf("%s.%s: time %q must be UTC and end with Z", o.where, key, text)
	}
	value, err := time.Parse(time.RFC3339Nano, text)
	if err != nil {
		return time.Time{}, fmt.Errorf("%s.%s: %w", o.where, key, err)
	}
	return value.UTC(), nil
}

// durationField parses a nonnegative decimal nanosecond string.
func durationField(o object, key string) (time.Duration, error) {
	text, err := o.text(key)
	if err != nil {
		return 0, err
	}
	value, err := parseUint64(text)
	if err != nil {
		return 0, fmt.Errorf("%s.%s: want nonnegative decimal nanoseconds: %w", o.where, key, err)
	}
	if value > math.MaxInt64 {
		return 0, fmt.Errorf("%s.%s: %s nanoseconds overflow a duration", o.where, key, text)
	}
	return time.Duration(value), nil
}

// parseUint64 accepts canonical decimal digits without sign or leading zero.
func parseUint64(text string) (uint64, error) {
	if text == "" || (len(text) > 1 && text[0] == '0') {
		return 0, fmt.Errorf("value %q is not canonical decimal", text)
	}
	for i := range len(text) {
		if text[i] < '0' || text[i] > '9' {
			return 0, fmt.Errorf("value %q must contain only decimal digits", text)
		}
	}
	value, err := strconv.ParseUint(text, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("parse %q: %w", text, err)
	}
	return value, nil
}

func parseICAO(text string) (uint32, error) {
	if len(text) != 6 || strings.ToUpper(text) != text {
		return 0, fmt.Errorf("ICAO %q must be six uppercase hexadecimal digits", text)
	}
	value, err := strconv.ParseUint(text, 16, 32)
	if err != nil {
		return 0, fmt.Errorf("parse ICAO %q: %w", text, err)
	}
	if value < 0x000001 || value > 0xFFFFFE {
		return 0, fmt.Errorf("ICAO %q is outside 000001-FFFFFE", text)
	}
	return uint32(value), nil
}

func parseFrameHex(text string) ([14]byte, error) {
	var frame [14]byte
	if len(text) != 28 || strings.ToUpper(text) != text {
		return frame, fmt.Errorf("hex %q must be 28 uppercase hexadecimal digits", text)
	}
	if _, err := hex.Decode(frame[:], []byte(text)); err != nil {
		return [14]byte{}, fmt.Errorf("hex %q: %w", text, err)
	}
	return frame, nil
}
