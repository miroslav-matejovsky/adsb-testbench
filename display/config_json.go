package display

import (
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/miroslav-matejovsky/adsb-testbench/simulatorapi"
)

// ParseConfig reads a complete JSON display configuration from r, reading at
// most maxBytes. Every key is required and unknown keys are rejected.
// reportError is supplied separately because a Go callback has no JSON
// representation.
//
// Required keys: identityExpiryNanoseconds, positionExpiryNanoseconds,
// altitudeExpiryNanoseconds, velocityExpiryNanoseconds, maxRequestBytes,
// maxResponseBytes, requestTimeoutNanoseconds.
func ParseConfig(r io.Reader, maxBytes int, reportError func(error)) (Config, error) {
	object, err := parseObject(r, maxBytes, "display configuration")
	if err != nil {
		return Config{}, err
	}

	var config Config
	if config.IdentityExpiry, err = readDuration(object, "identityExpiryNanoseconds"); err != nil {
		return Config{}, configError("display configuration", err)
	}
	if config.PositionExpiry, err = readDuration(object, "positionExpiryNanoseconds"); err != nil {
		return Config{}, configError("display configuration", err)
	}
	if config.AltitudeExpiry, err = readDuration(object, "altitudeExpiryNanoseconds"); err != nil {
		return Config{}, configError("display configuration", err)
	}
	if config.VelocityExpiry, err = readDuration(object, "velocityExpiryNanoseconds"); err != nil {
		return Config{}, configError("display configuration", err)
	}
	if config.MaxRequestBytes, err = readInt(object, "maxRequestBytes"); err != nil {
		return Config{}, configError("display configuration", err)
	}
	if config.MaxResponseBytes, err = readInt(object, "maxResponseBytes"); err != nil {
		return Config{}, configError("display configuration", err)
	}
	if config.RequestTimeout, err = readDuration(object, "requestTimeoutNanoseconds"); err != nil {
		return Config{}, configError("display configuration", err)
	}
	if err := object.Done(); err != nil {
		return Config{}, configError("display configuration", err)
	}
	config.ReportError = reportError
	if err := config.Validate(); err != nil {
		return Config{}, configError("display configuration", err)
	}
	return config, nil
}

// ParseHTTPSourceConfig reads a complete JSON HTTP source configuration from
// r, reading at most maxBytes. Every key is required and unknown keys are
// rejected. client is supplied separately because the host owns the transport.
//
// Required keys: baseUrl, timeoutNanoseconds, maxRequestBytes,
// maxResponseBytes.
func ParseHTTPSourceConfig(r io.Reader, maxBytes int, client *http.Client) (HTTPSourceConfig, error) {
	object, err := parseObject(r, maxBytes, "display HTTP source configuration")
	if err != nil {
		return HTTPSourceConfig{}, err
	}

	var config HTTPSourceConfig
	if config.BaseURL, err = readText(object, "baseUrl"); err != nil {
		return HTTPSourceConfig{}, configError("display HTTP source configuration", err)
	}
	if config.Timeout, err = readDuration(object, "timeoutNanoseconds"); err != nil {
		return HTTPSourceConfig{}, configError("display HTTP source configuration", err)
	}
	if config.MaxRequestBytes, err = readInt(object, "maxRequestBytes"); err != nil {
		return HTTPSourceConfig{}, configError("display HTTP source configuration", err)
	}
	if config.MaxResponseBytes, err = readInt(object, "maxResponseBytes"); err != nil {
		return HTTPSourceConfig{}, configError("display HTTP source configuration", err)
	}
	if err := object.Done(); err != nil {
		return HTTPSourceConfig{}, configError("display HTTP source configuration", err)
	}
	config.Client = client
	if err := config.Validate(); err != nil {
		return HTTPSourceConfig{}, configError("display HTTP source configuration", err)
	}
	return config, nil
}

// parseObject reads exactly one strict JSON object from r.
func parseObject(r io.Reader, maxBytes int, what string) (*simulatorapi.Object, error) {
	value, err := simulatorapi.ParseJSON(r, maxBytes)
	if err != nil {
		return nil, configError(what, err)
	}
	object, err := value.Object()
	if err != nil {
		return nil, configError(what, err)
	}
	return object, nil
}

// configError marks every configuration parse failure with the shared invalid
// category while keeping the original cause.
func configError(what string, err error) error {
	return fmt.Errorf("parse %s: %w: %w", what, simulatorapi.CategoryInvalid, err)
}

// Package-local accessors for one required key of a known wire type.

func readText(object *simulatorapi.Object, key string) (string, error) {
	field, err := object.Field(key)
	if err != nil {
		return "", err
	}
	return field.Text()
}

func readInt(object *simulatorapi.Object, key string) (int, error) {
	field, err := object.Field(key)
	if err != nil {
		return 0, err
	}
	return field.Int()
}

func readDuration(object *simulatorapi.Object, key string) (time.Duration, error) {
	text, err := readText(object, key)
	if err != nil {
		return 0, err
	}
	value, err := simulatorapi.ParseDuration(text)
	if err != nil {
		return 0, fmt.Errorf("%s.%s: %w", object.Path(), key, err)
	}
	return value, nil
}
