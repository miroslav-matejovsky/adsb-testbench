package simulatorapi_test

import (
	"math"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/miroslav-matejovsky/adsb-testbench/simulatorapi"
)

func TestParseUint64AcceptsCanonicalDigitsOnly(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		text  string
		want  uint64
		valid bool
	}{
		{name: "zero", text: "0", want: 0, valid: true},
		{name: "one", text: "1", want: 1, valid: true},
		{name: "above float53", text: "9007199254740993", want: 9007199254740993, valid: true},
		{name: "max", text: "18446744073709551615", want: math.MaxUint64, valid: true},
		{name: "overflow", text: "18446744073709551616"},
		{name: "leading zero", text: "01"},
		{name: "empty", text: ""},
		{name: "signed", text: "-1"},
		{name: "space", text: " 1"},
		{name: "hex", text: "0x1"},
		{name: "float", text: "1.0"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := simulatorapi.ParseUint64(tc.text)
			if !tc.valid {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.want, got)
			require.Equal(t, tc.text, simulatorapi.FormatUint64(got))
		})
	}
}

func TestParseDurationAcceptsSignedCanonicalNanoseconds(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		text  string
		want  time.Duration
		valid bool
	}{
		{name: "zero", text: "0", want: 0, valid: true},
		{name: "second", text: "1000000000", want: time.Second, valid: true},
		{name: "negative", text: "-1", want: -time.Nanosecond, valid: true},
		{name: "min", text: "-9223372036854775808", want: math.MinInt64, valid: true},
		{name: "max", text: "9223372036854775807", want: math.MaxInt64, valid: true},
		{name: "overflow", text: "9223372036854775808"},
		{name: "negative zero", text: "-0"},
		{name: "leading zero", text: "007"},
		{name: "suffix", text: "1s"},
		{name: "empty", text: ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := simulatorapi.ParseDuration(tc.text)
			if !tc.valid {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.want, got)
			require.Equal(t, tc.text, simulatorapi.FormatDuration(got))
		})
	}
}

func TestParseTimeRequiresUTCWithinYearBounds(t *testing.T) {
	t.Parallel()

	valid, err := simulatorapi.ParseTime("2024-03-05T12:00:00.123456789Z")
	require.NoError(t, err)
	require.Equal(t, time.Date(2024, 3, 5, 12, 0, 0, 123456789, time.UTC), valid)
	require.Equal(t, "2024-03-05T12:00:00.123456789Z", simulatorapi.FormatTime(valid))

	for _, text := range []string{
		"2024-03-05T12:00:00+01:00",
		"2024-03-05 12:00:00Z",
		"10000-01-01T00:00:00Z",
		"0000-01-01T00:00:00Z",
		"",
	} {
		_, err := simulatorapi.ParseTime(text)
		require.Error(t, err, text)
	}
}

func TestParseICAOAndFrameRequireUppercaseHex(t *testing.T) {
	t.Parallel()

	icao, err := simulatorapi.ParseICAO("00ABCD")
	require.NoError(t, err)
	require.Equal(t, uint32(0x00ABCD), icao)
	require.Equal(t, "00ABCD", simulatorapi.FormatICAO(icao))

	for _, text := range []string{"000000", "FFFFFF", "00abcd", "ABCDE", "ABCDEF0"} {
		_, err := simulatorapi.ParseICAO(text)
		require.Error(t, err, text)
	}

	frameText := "8D4840D6202CC371C32CE0576098"
	frame, err := simulatorapi.ParseFrame(frameText)
	require.NoError(t, err)
	require.Equal(t, frameText, simulatorapi.FormatFrame(frame))

	for _, text := range []string{"", "8d4840d6202cc371c32ce0576098", frameText + "00", frameText[:26]} {
		_, err := simulatorapi.ParseFrame(text)
		require.Error(t, err, text)
	}
}

func TestClampMessageBoundsExternalText(t *testing.T) {
	t.Parallel()

	short := strings.Repeat("a", simulatorapi.MaxMessageBytes)
	require.Equal(t, short, simulatorapi.ClampMessage(short))

	long := simulatorapi.ClampMessage(strings.Repeat("a", simulatorapi.MaxMessageBytes+1))
	require.Len(t, long, simulatorapi.MaxMessageBytes)
	require.True(t, strings.HasSuffix(long, "..."))
}

func TestFiniteRejectsNaNAndInfinity(t *testing.T) {
	t.Parallel()

	require.True(t, simulatorapi.Finite(0))
	require.False(t, simulatorapi.Finite(math.NaN()))
	require.False(t, simulatorapi.Finite(math.Inf(1)))
	require.False(t, simulatorapi.Finite(math.Inf(-1)))
}
