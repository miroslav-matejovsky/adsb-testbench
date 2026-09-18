package simulator_test

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/miroslav-matejovsky/adsb-testbench/simulator"
	"github.com/miroslav-matejovsky/adsb-testbench/simulatorapi"
)

// validAPIConfigJSON holds explicit fixture settings, not defaults.
const validAPIConfigJSON = `{
  "maxRequestBytes": 65536,
  "maxResponseBytes": 16777216,
  "requestTimeoutNanoseconds": "5000000000",
  "coverageReferenceAltitudeFeet": 0
}`

func TestParseAPIConfigAcceptsCompleteSettings(t *testing.T) {
	t.Parallel()

	report := func(error) {}
	config, err := simulator.ParseAPIConfig(strings.NewReader(validAPIConfigJSON), 4096, report)
	require.NoError(t, err)
	require.Equal(t, 65536, config.MaxRequestBytes)
	require.Equal(t, 16777216, config.MaxResponseBytes)
	require.Equal(t, 5*time.Second, config.RequestTimeout)
	require.Equal(t, float64(0), config.CoverageReferenceAltitudeFeet)
	require.NotNil(t, config.ReportError)
}

func TestParseAPIConfigRejectsIncompleteOrInvalidSettings(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		body string
	}{
		{name: "missing request bytes", body: `{"maxResponseBytes":16777216,"requestTimeoutNanoseconds":"1","coverageReferenceAltitudeFeet":0}`},
		{name: "null timeout", body: strings.Replace(validAPIConfigJSON, `"5000000000"`, `null`, 1)},
		{name: "unknown key", body: strings.Replace(validAPIConfigJSON, `"maxRequestBytes": 65536`, `"maxRequestBytes": 65536, "extra": 1`, 1)},
		{name: "zero request bytes", body: strings.Replace(validAPIConfigJSON, `65536`, `0`, 1)},
		{name: "response below envelope budget", body: strings.Replace(validAPIConfigJSON, `16777216`, `16`, 1)},
		{name: "zero timeout", body: strings.Replace(validAPIConfigJSON, `"5000000000"`, `"0"`, 1)},
		{name: "negative timeout", body: strings.Replace(validAPIConfigJSON, `"5000000000"`, `"-1"`, 1)},
		{name: "timeout as number", body: strings.Replace(validAPIConfigJSON, `"5000000000"`, `5000000000`, 1)},
		{name: "altitude below coverage domain", body: strings.Replace(validAPIConfigJSON, `"coverageReferenceAltitudeFeet": 0`, `"coverageReferenceAltitudeFeet": -1001`, 1)},
		{name: "altitude above coverage domain", body: strings.Replace(validAPIConfigJSON, `"coverageReferenceAltitudeFeet": 0`, `"coverageReferenceAltitudeFeet": 50176`, 1)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			_, err := simulator.ParseAPIConfig(strings.NewReader(tc.body), 4096, func(error) {})
			require.ErrorIs(t, err, simulatorapi.CategoryInvalid)
		})
	}
}

func TestParseAPIConfigRequiresErrorCallback(t *testing.T) {
	t.Parallel()

	_, err := simulator.ParseAPIConfig(strings.NewReader(validAPIConfigJSON), 4096, nil)
	require.ErrorIs(t, err, simulatorapi.CategoryInvalid)
	require.ErrorContains(t, err, "ReportError")
}

func TestAPIConfigValidateAcceptsExtremeCoverageAltitudes(t *testing.T) {
	t.Parallel()

	for _, altitude := range []float64{-1000, 0, 50175} {
		config := simulator.APIConfig{
			MaxRequestBytes: 1, MaxResponseBytes: simulatorapi.MinResponseBytes,
			RequestTimeout: time.Nanosecond, CoverageReferenceAltitudeFeet: altitude,
			ReportError: func(error) {},
		}
		require.NoError(t, config.Validate(), altitude)
	}
}
