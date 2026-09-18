package display_test

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/miroslav-matejovsky/adsb-testbench/display"
	"github.com/miroslav-matejovsky/adsb-testbench/simulatorapi"
)

// validDisplayConfigJSON holds explicit fixture settings, not defaults.
const validDisplayConfigJSON = `{
  "identityExpiryNanoseconds": "60000000000",
  "positionExpiryNanoseconds": "30000000000",
  "altitudeExpiryNanoseconds": "30000000000",
  "velocityExpiryNanoseconds": "30000000000",
  "maxRequestBytes": 65536,
  "maxResponseBytes": 16777216,
  "requestTimeoutNanoseconds": "5000000000"
}`

// validSourceConfigJSON holds explicit fixture settings, not defaults.
const validSourceConfigJSON = `{
  "baseUrl": "http://127.0.0.1:8080/bench/a/simulator",
  "timeoutNanoseconds": "5000000000",
  "maxRequestBytes": 65536,
  "maxResponseBytes": 16777216
}`

func TestParseConfigAcceptsCompleteSettings(t *testing.T) {
	t.Parallel()

	config, err := display.ParseConfig(strings.NewReader(validDisplayConfigJSON), 4096, func(error) {})
	require.NoError(t, err)
	require.Equal(t, time.Minute, config.IdentityExpiry)
	require.Equal(t, 30*time.Second, config.PositionExpiry)
	require.Equal(t, 30*time.Second, config.AltitudeExpiry)
	require.Equal(t, 30*time.Second, config.VelocityExpiry)
	require.Equal(t, 65536, config.MaxRequestBytes)
	require.Equal(t, 16777216, config.MaxResponseBytes)
	require.Equal(t, 5*time.Second, config.RequestTimeout)
	require.NotNil(t, config.ReportError)
}

func TestParseConfigRejectsIncompleteOrInvalidSettings(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		body string
	}{
		{name: "missing identity expiry", body: strings.Replace(validDisplayConfigJSON, `"identityExpiryNanoseconds": "60000000000",`, ``, 1)},
		{name: "null position expiry", body: strings.Replace(validDisplayConfigJSON, `"30000000000"`, `null`, 1)},
		{name: "unknown key", body: strings.Replace(validDisplayConfigJSON, `"maxRequestBytes": 65536`, `"maxRequestBytes": 65536, "extra": 1`, 1)},
		{name: "duplicate key", body: strings.Replace(validDisplayConfigJSON, `"maxRequestBytes": 65536`, `"maxRequestBytes": 65536, "maxRequestBytes": 1`, 1)},
		{name: "zero expiry", body: strings.Replace(validDisplayConfigJSON, `"60000000000"`, `"0"`, 1)},
		{name: "negative expiry", body: strings.Replace(validDisplayConfigJSON, `"60000000000"`, `"-1"`, 1)},
		{name: "expiry as number", body: strings.Replace(validDisplayConfigJSON, `"60000000000"`, `60000000000`, 1)},
		{name: "zero request bytes", body: strings.Replace(validDisplayConfigJSON, `"maxRequestBytes": 65536`, `"maxRequestBytes": 0`, 1)},
		{name: "response below envelope budget", body: strings.Replace(validDisplayConfigJSON, `16777216`, `16`, 1)},
		{name: "zero timeout", body: strings.Replace(validDisplayConfigJSON, `"requestTimeoutNanoseconds": "5000000000"`, `"requestTimeoutNanoseconds": "0"`, 1)},
		{name: "trailing value", body: validDisplayConfigJSON + `{}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			_, err := display.ParseConfig(strings.NewReader(tc.body), 4096, func(error) {})
			require.ErrorIs(t, err, simulatorapi.CategoryInvalid)
		})
	}
}

func TestParseConfigRequiresErrorCallback(t *testing.T) {
	t.Parallel()

	_, err := display.ParseConfig(strings.NewReader(validDisplayConfigJSON), 4096, nil)
	require.ErrorIs(t, err, simulatorapi.CategoryInvalid)
	require.ErrorContains(t, err, "ReportError")
}

func TestParseHTTPSourceConfigAcceptsCompleteSettings(t *testing.T) {
	t.Parallel()

	client := &http.Client{}
	config, err := display.ParseHTTPSourceConfig(strings.NewReader(validSourceConfigJSON), 4096, client)
	require.NoError(t, err)
	require.Equal(t, "http://127.0.0.1:8080/bench/a/simulator", config.BaseURL)
	require.Equal(t, 5*time.Second, config.Timeout)
	require.Equal(t, 65536, config.MaxRequestBytes)
	require.Equal(t, 16777216, config.MaxResponseBytes)
	require.Same(t, client, config.Client)
}

func TestParseHTTPSourceConfigRejectsIncompleteOrInvalidSettings(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		body string
	}{
		{name: "missing base url", body: strings.Replace(validSourceConfigJSON, `"baseUrl": "http://127.0.0.1:8080/bench/a/simulator",`, ``, 1)},
		{name: "null timeout", body: strings.Replace(validSourceConfigJSON, `"5000000000"`, `null`, 1)},
		{name: "relative base url", body: strings.Replace(validSourceConfigJSON, `"http://127.0.0.1:8080/bench/a/simulator"`, `"/bench/a/simulator"`, 1)},
		{name: "base url with query", body: strings.Replace(validSourceConfigJSON, `simulator"`, `simulator?a=1"`, 1)},
		{name: "unknown key", body: strings.Replace(validSourceConfigJSON, `"timeoutNanoseconds"`, `"extra": 1, "timeoutNanoseconds"`, 1)},
		{name: "zero timeout", body: strings.Replace(validSourceConfigJSON, `"timeoutNanoseconds": "5000000000"`, `"timeoutNanoseconds": "0"`, 1)},
		{name: "zero request bytes", body: strings.Replace(validSourceConfigJSON, `"maxRequestBytes": 65536`, `"maxRequestBytes": 0`, 1)},
		{name: "response below envelope budget", body: strings.Replace(validSourceConfigJSON, `16777216`, `16`, 1)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			_, err := display.ParseHTTPSourceConfig(strings.NewReader(tc.body), 4096, &http.Client{})
			require.ErrorIs(t, err, simulatorapi.CategoryInvalid)
		})
	}
}

func TestParseHTTPSourceConfigRequiresClient(t *testing.T) {
	t.Parallel()

	_, err := display.ParseHTTPSourceConfig(strings.NewReader(validSourceConfigJSON), 4096, nil)
	require.ErrorIs(t, err, simulatorapi.CategoryInvalid)
	require.ErrorContains(t, err, "Client is nil")
}
