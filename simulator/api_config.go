package simulator

import (
	"fmt"
	"io"
	"time"

	"github.com/miroslav-matejovsky/adsb-testbench/simulation"
	"github.com/miroslav-matejovsky/adsb-testbench/simulatorapi"
)

// APIConfig is the complete explicit configuration of one simulator service.
// Every field must be assigned; no constructor inserts an application default.
//
// MaxRequestBytes and MaxResponseBytes are hard byte bounds on one request
// body and one complete response body. RequestTimeout bounds the work of one
// operation. CoverageReferenceAltitudeFeet is the pressure altitude every
// published station coverage estimate is made for.
//
// ReportError is a Go dependency, not a serializable setting. The service
// calls it for actionable write, encoding, and internal failures that no
// client can observe. It must not be nil and must not panic.
type APIConfig struct {
	MaxRequestBytes               int
	MaxResponseBytes              int
	RequestTimeout                time.Duration
	CoverageReferenceAltitudeFeet float64
	ReportError                   func(error)
}

// coverageProbe is a minimal valid station used only to check a reference
// altitude against the engine's own coverage rules.
var coverageProbe = simulation.StationConfig{ID: "coverage-probe", Enabled: true, SensitivityDBm: -100}

// Validate checks every field without building a service.
func (c APIConfig) Validate() error {
	if c.MaxRequestBytes <= 0 {
		return fmt.Errorf("%w: MaxRequestBytes %d must be positive", simulatorapi.CategoryInvalid, c.MaxRequestBytes)
	}
	if c.MaxResponseBytes < simulatorapi.MinResponseBytes {
		return fmt.Errorf("%w: MaxResponseBytes %d is below the %d byte error envelope budget",
			simulatorapi.CategoryInvalid, c.MaxResponseBytes, simulatorapi.MinResponseBytes)
	}
	if c.RequestTimeout <= 0 {
		return fmt.Errorf("%w: RequestTimeout %s must be positive", simulatorapi.CategoryInvalid, c.RequestTimeout)
	}
	if _, err := simulation.EstimateCoverage(coverageProbe, c.CoverageReferenceAltitudeFeet); err != nil {
		return fmt.Errorf("%w: CoverageReferenceAltitudeFeet is not an accepted coverage altitude: %w",
			simulatorapi.CategoryInvalid, err)
	}
	if c.ReportError == nil {
		return fmt.Errorf("%w: ReportError callback is nil", simulatorapi.CategoryInvalid)
	}
	return nil
}

// ParseAPIConfig reads a complete JSON service configuration from r, reading
// at most maxBytes. Every key is required and unknown keys are rejected.
// reportError is supplied separately because a Go callback has no JSON
// representation.
//
// Required keys: maxRequestBytes, maxResponseBytes, requestTimeoutNanoseconds,
// coverageReferenceAltitudeFeet.
func ParseAPIConfig(r io.Reader, maxBytes int, reportError func(error)) (APIConfig, error) {
	value, err := simulatorapi.ParseJSON(r, maxBytes)
	if err != nil {
		return APIConfig{}, invalidConfig("simulator API configuration", err)
	}
	object, err := value.Object()
	if err != nil {
		return APIConfig{}, invalidConfig("simulator API configuration", err)
	}
	config, err := readAPIConfig(object)
	if err != nil {
		return APIConfig{}, invalidConfig("simulator API configuration", err)
	}
	if err := object.Done(); err != nil {
		return APIConfig{}, invalidConfig("simulator API configuration", err)
	}
	config.ReportError = reportError
	if err := config.Validate(); err != nil {
		return APIConfig{}, invalidConfig("simulator API configuration", err)
	}
	return config, nil
}

// readAPIConfig binds every required service key without validating domains.
func readAPIConfig(object *simulatorapi.Object) (APIConfig, error) {
	var config APIConfig
	var err error
	if config.MaxRequestBytes, err = readInt(object, "maxRequestBytes"); err != nil {
		return APIConfig{}, err
	}
	if config.MaxResponseBytes, err = readInt(object, "maxResponseBytes"); err != nil {
		return APIConfig{}, err
	}
	if config.RequestTimeout, err = readDuration(object, "requestTimeoutNanoseconds"); err != nil {
		return APIConfig{}, err
	}
	if config.CoverageReferenceAltitudeFeet, err = readFloat(object, "coverageReferenceAltitudeFeet"); err != nil {
		return APIConfig{}, err
	}
	return config, nil
}
