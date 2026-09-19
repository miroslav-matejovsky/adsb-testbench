package display

import (
	"fmt"
	"net/http"
	"time"

	"github.com/miroslav-matejovsky/adsb-testbench/internal/urlpath"
	"github.com/miroslav-matejovsky/adsb-testbench/simulatorapi"
)

// Config is the complete explicit configuration of one display backend.
// Every field must be assigned; no constructor inserts an application default.
//
// The four expiry durations are independent virtual-time lifetimes of the
// received identity, position, altitude, and velocity fields. A field is
// fresh at exactly its lifetime and absent once its age exceeds it. They are
// never applied to wall-clock time.
//
// MaxRequestBytes and MaxResponseBytes bound one browser-facing request and
// one complete browser-facing response. RequestTimeout bounds the work of one
// backend operation.
//
// ReportError is a Go dependency, not a serializable setting. The backend
// calls it for actionable write, encoding, and internal failures no browser
// can observe. It must not be nil and must not panic.
type Config struct {
	IdentityExpiry   time.Duration
	PositionExpiry   time.Duration
	AltitudeExpiry   time.Duration
	VelocityExpiry   time.Duration
	MaxRequestBytes  int
	MaxResponseBytes int
	RequestTimeout   time.Duration
	ReportError      func(error)
}

// Validate checks every field without building a backend.
func (c Config) Validate() error {
	lifetimes := []struct {
		name  string
		value time.Duration
	}{
		{"IdentityExpiry", c.IdentityExpiry},
		{"PositionExpiry", c.PositionExpiry},
		{"AltitudeExpiry", c.AltitudeExpiry},
		{"VelocityExpiry", c.VelocityExpiry},
	}
	for _, lifetime := range lifetimes {
		if lifetime.value <= 0 {
			return fmt.Errorf("%w: %s %s must be positive",
				simulatorapi.CategoryInvalid, lifetime.name, lifetime.value)
		}
	}
	if c.MaxRequestBytes <= 0 {
		return fmt.Errorf("%w: MaxRequestBytes %d must be positive",
			simulatorapi.CategoryInvalid, c.MaxRequestBytes)
	}
	if c.MaxResponseBytes < simulatorapi.MinResponseBytes {
		return fmt.Errorf("%w: MaxResponseBytes %d is below the %d byte error envelope budget",
			simulatorapi.CategoryInvalid, c.MaxResponseBytes, simulatorapi.MinResponseBytes)
	}
	if c.RequestTimeout <= 0 {
		return fmt.Errorf("%w: RequestTimeout %s must be positive",
			simulatorapi.CategoryInvalid, c.RequestTimeout)
	}
	if c.ReportError == nil {
		return fmt.Errorf("%w: ReportError callback is nil", simulatorapi.CategoryInvalid)
	}
	return nil
}

// HTTPSourceConfig is the complete explicit configuration of one HTTP
// observation source. Every field must be assigned.
//
// BaseURL is the absolute http or https URL the simulator handler is mounted
// at, including any mount prefix. Route paths are appended to it, so a nested
// mount such as https://host/bench/a/simulator/ keeps its whole prefix.
//
// Timeout bounds one complete call including reading the response body.
// MaxRequestBytes and MaxResponseBytes are hard bounds on the encoded request
// and the bytes actually read from the response.
//
// Client is a Go dependency supplied by the host, which retains ownership of
// its transport. The source never closes idle connections and never follows a
// redirect, because a redirect would silently change the source identity.
type HTTPSourceConfig struct {
	BaseURL          string
	Timeout          time.Duration
	MaxRequestBytes  int
	MaxResponseBytes int
	Client           *http.Client
}

// Validate checks every field without building a source.
func (c HTTPSourceConfig) Validate() error {
	if _, err := urlpath.ParseSourceBase(c.BaseURL); err != nil {
		return fmt.Errorf("%w: %w", simulatorapi.CategoryInvalid, err)
	}
	if c.Timeout <= 0 {
		return fmt.Errorf("%w: Timeout %s must be positive", simulatorapi.CategoryInvalid, c.Timeout)
	}
	if c.MaxRequestBytes <= 0 {
		return fmt.Errorf("%w: MaxRequestBytes %d must be positive",
			simulatorapi.CategoryInvalid, c.MaxRequestBytes)
	}
	if c.MaxResponseBytes < simulatorapi.MinResponseBytes {
		return fmt.Errorf("%w: MaxResponseBytes %d is below the %d byte error envelope budget",
			simulatorapi.CategoryInvalid, c.MaxResponseBytes, simulatorapi.MinResponseBytes)
	}
	if c.Client == nil {
		return fmt.Errorf("%w: Client is nil", simulatorapi.CategoryInvalid)
	}
	return nil
}
