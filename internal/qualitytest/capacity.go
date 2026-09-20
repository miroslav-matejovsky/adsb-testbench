package qualitytest

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"reflect"
	"strings"
	"time"
)

// Capacity is the content of capacity.json: the supported byte-budget
// profile, the conservative response bounds computed from it, and the
// example response budgets chosen from those bounds.
type Capacity struct {
	Profile CapacityProfile `json:"profile"`
	// Bounds maps a bound name, such as rawSnapshotBytes, to its byte count.
	Bounds  map[string]int  `json:"bounds"`
	Budgets CapacityBudgets `json:"budgets"`
	// Workloads are the named, fully explicit benchmark inputs.
	Workloads []CapacityWorkload `json:"workloads"`
}

// CapacityWorkload is one named benchmark workload. Every dimension is an
// explicit list, so a benchmark enumerates stated cases instead of choosing
// inputs of its own.
type CapacityWorkload struct {
	Name        string
	Description string
	Seed        uint64
	StartTime   time.Time
	Spawn       Spawn
	// AircraftCounts, StationCounts, SpeedHundredths and Advances are the
	// dimensions of the workload matrix.
	AircraftCounts  []int
	StationCounts   []int
	SpeedHundredths []uint16
	Advances        []time.Duration
	// Station is the settings template; benchmarks assign generated IDs.
	Station Station
	// StationVariants are named station settings replacing the template,
	// for workloads that vary the reception outcome.
	StationVariants []CapacityStationVariant
	Expiry          Expiry
	// RetentionFillTarget is the number of records every selected station
	// must hold before a measurement starts. Zero means no fill.
	RetentionFillTarget int
	// Callers are the concurrent caller counts to measure.
	Callers []int
}

// CapacityStationVariant is one named station setting of a workload.
type CapacityStationVariant struct {
	Name    string
	Station Station
}

// Workload returns the workload named name.
func (c Capacity) Workload(name string) (CapacityWorkload, error) {
	for _, workload := range c.Workloads {
		if workload.Name == name {
			return workload, nil
		}
	}
	return CapacityWorkload{}, fmt.Errorf("workload %q is not in the corpus", name)
}

// CapacityProfile states every assumption of the response-size worksheet.
type CapacityProfile struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	// Structural limits of the engine and display.
	MaxSelectedStations      int `json:"maxSelectedStations"`
	MaxStationRecords        int `json:"maxStationRecords"`
	MaxSnapshotRecords       int `json:"maxSnapshotRecords"`
	MaxHistoryPageRecords    int `json:"maxHistoryPageRecords"`
	MaxTruthAircraft         int `json:"maxTruthAircraft"`
	TransmissionHistoryLimit int `json:"transmissionHistoryLimit"`
	// Profile limits on host-chosen text.
	MaxRunIDBytes        int `json:"maxRunIdBytes"`
	RunIDSuffixBytes     int `json:"runIdSuffixBytes"`
	MaxStationIDBytes    int `json:"maxStationIdBytes"`
	MaxErrorMessageBytes int `json:"maxErrorMessageBytes"`
	MaxErrorFieldBytes   int `json:"maxErrorFieldBytes"`
	// Encoding widths.
	JSONEscapeFactor  int `json:"jsonEscapeFactor"`
	Uint64Digits      int `json:"uint64Digits"`
	TimestampBytes    int `json:"timestampBytes"`
	FiniteNumberBytes int `json:"finiteNumberBytes"`
	// Budget rule: bound times HeadroomPercent/100, rounded up to a whole
	// multiple of RoundingBytes.
	HeadroomPercent int `json:"headroomPercent"`
	RoundingBytes   int `json:"roundingBytes"`
	// Declared consumer concurrency of the supported profile.
	SimulatorReaders  int `json:"simulatorReaders"`
	DisplayRefreshers int `json:"displayRefreshers"`
}

// CapacityBudgets are the example configuration response budgets.
type CapacityBudgets struct {
	SimulatorAPIMaxResponseBytes int `json:"simulatorApiMaxResponseBytes"`
	ManagerUIMaxResponseBytes    int `json:"managerUiMaxResponseBytes"`
	SourceMaxResponseBytes       int `json:"sourceMaxResponseBytes"`
	DisplayMaxResponseBytes      int `json:"displayMaxResponseBytes"`
	AircraftUIMaxResponseBytes   int `json:"aircraftUiMaxResponseBytes"`
}

// Budget applies the profile budget rule to a bound.
func (c Capacity) Budget(bound int) int {
	scaled := (bound*c.Profile.HeadroomPercent + 99) / 100
	unit := c.Profile.RoundingBytes
	return (scaled + unit - 1) / unit * unit
}

// LoadCapacity reads and validates a capacity.json file. Every struct key is
// required, none may be null, unknown keys are rejected, and every number
// must be positive.
func LoadCapacity(path string) (Capacity, error) {
	var capacity Capacity
	err := loadFile(path, func(root json.RawMessage) error {
		o, err := strictObject(root, "$", "profile", "bounds", "budgets", "workloads")
		if err != nil {
			return err
		}
		for key, typ := range map[string]reflect.Type{
			"profile": reflect.TypeFor[CapacityProfile](),
			"budgets": reflect.TypeFor[CapacityBudgets](),
		} {
			raw, err := o.raw(key)
			if err != nil {
				return err
			}
			if err := requireStructKeys(raw, "$."+key, typ); err != nil {
				return err
			}
		}
		if err := o.value("profile", &capacity.Profile); err != nil {
			return err
		}
		if err := o.value("bounds", &capacity.Bounds); err != nil {
			return err
		}
		if err := o.value("budgets", &capacity.Budgets); err != nil {
			return err
		}
		var rawWorkloads []json.RawMessage
		if err := o.value("workloads", &rawWorkloads); err != nil {
			return err
		}
		names := map[string]bool{}
		for i, raw := range rawWorkloads {
			workload, err := parseWorkload(raw, fmt.Sprintf("$.workloads[%d]", i))
			if err != nil {
				return err
			}
			if names[workload.Name] {
				return fmt.Errorf("$.workloads: duplicate workload %q", workload.Name)
			}
			names[workload.Name] = true
			capacity.Workloads = append(capacity.Workloads, workload)
		}
		return capacity.validate()
	})
	return capacity, err
}

// parseWorkload reads one benchmark workload.
func parseWorkload(raw json.RawMessage, where string) (CapacityWorkload, error) {
	o, err := strictObject(raw, where, "name", "description", "seed", "startTime", "spawn", "station",
		"stationVariants", "expiry", "retentionFillTarget", "callers",
		"aircraftCounts", "stationCounts", "speedHundredths", "advanceNanoseconds")
	if err != nil {
		return CapacityWorkload{}, err
	}
	var workload CapacityWorkload
	if workload.Name, err = o.text("name"); err != nil {
		return CapacityWorkload{}, err
	}
	where = fmt.Sprintf("workload %q", workload.Name)
	o.where = where
	if workload.Description, err = o.text("description"); err != nil {
		return CapacityWorkload{}, err
	}
	seed, err := o.text("seed")
	if err != nil {
		return CapacityWorkload{}, err
	}
	if workload.Seed, err = parseUint64(seed); err != nil {
		return CapacityWorkload{}, fmt.Errorf("%s.seed: %w", where, err)
	}
	if workload.StartTime, err = timeField(o, "startTime"); err != nil {
		return CapacityWorkload{}, err
	}
	rawSpawn, err := o.raw("spawn")
	if err != nil {
		return CapacityWorkload{}, err
	}
	if err := decodeTagged(rawSpawn, where+".spawn", &workload.Spawn); err != nil {
		return CapacityWorkload{}, err
	}
	if workload.Station, err = stationField(o); err != nil {
		return CapacityWorkload{}, err
	}
	if workload.StationVariants, err = parseStationVariants(o); err != nil {
		return CapacityWorkload{}, err
	}
	rawExpiry, err := o.raw("expiry")
	if err != nil {
		return CapacityWorkload{}, err
	}
	if workload.Expiry, err = parseExpiry(rawExpiry, where+".expiry"); err != nil {
		return CapacityWorkload{}, err
	}
	if err := o.value("retentionFillTarget", &workload.RetentionFillTarget); err != nil {
		return CapacityWorkload{}, err
	}
	for _, field := range []struct {
		key string
		dst *[]int
	}{
		{"callers", &workload.Callers}, {"aircraftCounts", &workload.AircraftCounts},
		{"stationCounts", &workload.StationCounts},
	} {
		if err := o.value(field.key, field.dst); err != nil {
			return CapacityWorkload{}, err
		}
		if len(*field.dst) == 0 {
			return CapacityWorkload{}, fmt.Errorf("%s.%s: no value", where, field.key)
		}
	}
	if err := o.value("speedHundredths", &workload.SpeedHundredths); err != nil {
		return CapacityWorkload{}, err
	}
	var advances []string
	if err := o.value("advanceNanoseconds", &advances); err != nil {
		return CapacityWorkload{}, err
	}
	for _, text := range advances {
		value, err := parseUint64(text)
		if err != nil || value > math.MaxInt64 {
			return CapacityWorkload{}, fmt.Errorf("%s.advanceNanoseconds: %q is not nonnegative decimal nanoseconds",
				where, text)
		}
		workload.Advances = append(workload.Advances, time.Duration(value))
	}
	if len(workload.Advances) == 0 || len(workload.SpeedHundredths) == 0 {
		return CapacityWorkload{}, fmt.Errorf("%s: advanceNanoseconds and speedHundredths need a value", where)
	}
	return workload, nil
}

// parseStationVariants reads the named station settings of a workload.
func parseStationVariants(o object) ([]CapacityStationVariant, error) {
	var rawVariants []json.RawMessage
	if err := o.value("stationVariants", &rawVariants); err != nil {
		return nil, err
	}
	variants := make([]CapacityStationVariant, 0, len(rawVariants))
	for i, rawVariant := range rawVariants {
		where := fmt.Sprintf("%s.stationVariants[%d]", o.where, i)
		fields, err := strictObject(rawVariant, where, "name", "station")
		if err != nil {
			return nil, err
		}
		var variant CapacityStationVariant
		if variant.Name, err = fields.text("name"); err != nil {
			return nil, err
		}
		if variant.Station, err = stationField(fields); err != nil {
			return nil, err
		}
		variants = append(variants, variant)
	}
	return variants, nil
}

func (c Capacity) validate() error {
	if strings.TrimSpace(c.Profile.Name) == "" || strings.TrimSpace(c.Profile.Description) == "" {
		return errors.New("$.profile: name and description must be nonblank")
	}
	if err := positiveInts(reflect.ValueOf(c.Profile), "$.profile"); err != nil {
		return err
	}
	if err := positiveInts(reflect.ValueOf(c.Budgets), "$.budgets"); err != nil {
		return err
	}
	if len(c.Workloads) == 0 {
		return errors.New("$.workloads: no workload")
	}
	if len(c.Bounds) == 0 {
		return errors.New("$.bounds: no bound")
	}
	for name, bytes := range c.Bounds {
		if bytes <= 0 {
			return fmt.Errorf("$.bounds.%s: want a positive byte count", name)
		}
	}
	return nil
}

// positiveInts rejects zero or negative int fields of a struct.
func positiveInts(value reflect.Value, where string) error {
	for i := range value.NumField() {
		field := value.Field(i)
		if field.Kind() == reflect.Int && field.Int() <= 0 {
			return fmt.Errorf("%s.%s: want a positive value", where, value.Type().Field(i).Tag.Get("json"))
		}
	}
	return nil
}

// requireStructKeys checks that raw holds exactly the JSON keys of typ at
// every struct depth and that no struct field is null. Maps may hold any key.
func requireStructKeys(raw json.RawMessage, where string, typ reflect.Type) error {
	if typ.Kind() != reflect.Struct {
		if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
			return fmt.Errorf("%s: null where a value is required", where)
		}
		return nil
	}
	keys := make([]string, 0, typ.NumField())
	for i := range typ.NumField() {
		keys = append(keys, strings.Split(typ.Field(i).Tag.Get("json"), ",")[0])
	}
	o, err := strictObject(raw, where, keys...)
	if err != nil {
		return err
	}
	for i, key := range keys {
		value, err := o.raw(key)
		if err != nil {
			return err
		}
		if err := requireStructKeys(value, where+"."+key, typ.Field(i).Type); err != nil {
			return err
		}
	}
	return nil
}
