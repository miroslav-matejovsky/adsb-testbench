package qualitytest

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

const corpusDir = "../../testdata/quality"

func TestShippedCorpusLoads(t *testing.T) {
	t.Parallel()

	frames, err := LoadFrames(filepath.Join(corpusDir, "frames.json"))
	require.NoError(t, err)
	require.Len(t, frames.Frames, 5)
	for _, frame := range frames.Frames {
		require.Equal(t, External, frame.Classification, frame.ID)
	}
	even, err := frames.Frame("position-40621d-even")
	require.NoError(t, err)
	require.Equal(t, uint32(0x40621D), even.Expected.ICAO)
	require.Equal(t, uint32(93000), even.Expected.Position.CPR.Latitude)
	require.Len(t, frames.CPRPairs, 1)
	require.Len(t, frames.CPRPairs[0].Cases, 2)

	scenarios, err := LoadScenarios(filepath.Join(corpusDir, "scenarios.json"))
	require.NoError(t, err)
	ids := make([]string, 0, len(scenarios))
	for _, scenario := range scenarios {
		ids = append(ids, scenario.ID)
	}
	require.Equal(t, []string{
		"empty-selection", "reference-fields", "partitioned-time", "fractional-speed",
		"pause-and-accelerate", "receiver-revisions", "loss-extremes", "expiry-boundaries",
		"retention-and-churn", "restart",
	}, ids)

	partitioned, err := FindScenario(scenarios, "partitioned-time")
	require.NoError(t, err)
	require.Equal(t, uint64(0x0123456789abcdef), partitioned.Engine.Seed)
	split, err := partitioned.Run("split")
	require.NoError(t, err)
	var total time.Duration
	for _, operation := range split.Operations {
		total += operation.Duration
	}
	require.Equal(t, 10*time.Second, total)

	reference, err := LoadDisplayReference(filepath.Join(corpusDir, "display-reference.json"))
	require.NoError(t, err)
	require.Len(t, reference.Aircraft, 4)
	for _, id := range reference.Frames {
		_, err := frames.Frame(id)
		require.NoError(t, err, "the reference snapshot names only corpus frames")
	}
}

// minimalScenario is one valid engine scenario document.
const minimalScenario = `[{
  "id": "tiny", "kind": "engine", "classification": "synthetic", "description": "d",
  "engine": {"seed": "0", "startTime": "2024-03-05T12:00:00Z", "initialAircraftCount": 0, "speedHundredths": 100,
    "spawn": {"latitudeDegrees": {"min": 0, "max": 0}, "longitudeDegrees": {"min": 0, "max": 0},
      "altitudeFeet": {"min": 0, "max": 0}, "groundSpeedKnots": {"min": 0, "max": 0},
      "trackDegrees": {"min": 0, "max": 0}, "verticalRateFeetPerMinute": {"min": 0, "max": 0}}},
  "stations": [], "selection": [],
  "expiry": {"identityNanoseconds": "1", "positionNanoseconds": "1", "altitudeNanoseconds": "1", "velocityNanoseconds": "1"},
  "runs": [{"name": "only", "runId": "tiny", "operations": [{"op": "advance", "durationNanoseconds": "5"}]}],
  "assertions": ["identical-batches"]
}]`

// minimalFrames is one valid frame document.
const minimalFrames = `{"frames": [{
  "id": "ident", "classification": "external", "hex": "8D4840D6202CC371C32CE0576098",
  "sourceUrl": "https://example.test/", "sourceLocation": "l", "verification": "v",
  "expected": {"icao": "4840D6", "family": "identification", "capability": 5,
    "identification": {"typeCode": 4, "category": 0, "callsign": "KLM1023"}}
}], "cprPairs": []}`

func write(t *testing.T, text string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "corpus.json")
	require.NoError(t, os.WriteFile(path, []byte(text), 0o600))
	return path
}

func TestLoadScenariosRejectsMalformedData(t *testing.T) {
	t.Parallel()

	_, err := LoadScenarios(write(t, minimalScenario))
	require.NoError(t, err, "the base document is valid")

	cases := []struct {
		name, old, new, message string
	}{
		{"corrupt JSON", `"kind": "engine"`, `"kind": "engine`, "decode JSON"},
		{"unknown operation", `"op": "advance"`, `"op": "warp"`, `unknown operation "warp"`},
		{"unknown field", `"selection": []`, `"selection": [], "extra": 1`, `unknown field "extra"`},
		{"missing field", `"selection": [],`, ``, `missing field "selection"`},
		{"null value", `"initialAircraftCount": 0`, `"initialAircraftCount": null`, "null where a value is required"},
		{"negative duration", `"durationNanoseconds": "5"`, `"durationNanoseconds": "-5"`, "nonnegative decimal nanoseconds"},
		{"unknown assertion", `"identical-batches"`, `"identical-batchs"`, `unknown assertion "identical-batchs"`},
		{"operation field of another op", `"durationNanoseconds": "5"`, `"durationNanoseconds": "5", "count": 1`, `unknown field "count"`},
		{"nested null", `"altitudeFeet": {"min": 0, "max": 0}`, `"altitudeFeet": {"min": null, "max": 0}`, "null where a value is required"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			mutated := strings.Replace(minimalScenario, tc.old, tc.new, 1)
			require.NotEqual(t, minimalScenario, mutated, "the mutation applies")
			_, err := LoadScenarios(write(t, mutated))
			require.ErrorContains(t, err, tc.message)
			require.ErrorContains(t, err, "corpus.json", "errors name the file")
		})
	}

	duplicateScenario := "[" + strings.Trim(minimalScenario, "[]") + "," + strings.Trim(minimalScenario, "[]") + "]"
	_, err = LoadScenarios(write(t, duplicateScenario))
	require.ErrorContains(t, err, `scenario "tiny": duplicate id`)

	duplicateRun := strings.Replace(minimalScenario, `"runs": [`,
		`"runs": [{"name": "only", "runId": "tiny", "operations": []}, `, 1)
	_, err = LoadScenarios(write(t, duplicateRun))
	require.ErrorContains(t, err, `duplicate run "only"`)

	_, err = LoadScenarios(write(t, minimalScenario+" []"))
	require.ErrorContains(t, err, "trailing data")
}

func TestLoadFramesRejectsMalformedData(t *testing.T) {
	t.Parallel()

	_, err := LoadFrames(write(t, minimalFrames))
	require.NoError(t, err, "the base document is valid")

	cases := []struct {
		name, old, new, message string
	}{
		{"short hex", `"8D4840D6202CC371C32CE0576098"`, `"8D4840D6202CC371C32CE05760"`, "28 uppercase hexadecimal digits"},
		{"lowercase hex", `"8D4840D6202CC371C32CE0576098"`, `"8d4840d6202cc371c32ce0576098"`, "28 uppercase hexadecimal digits"},
		{"address mismatch", `"icao": "4840D6"`, `"icao": "4840D7"`, "differs from frame address"},
		{"payload of another family", `"identification": {`, `"velocity": {`, `missing field "identification"`},
		{"unknown classification", `"classification": "external"`, `"classification": "captured"`, "unknown classification"},
		{"external without source", `"https://example.test/"`, `""`, "https sourceUrl"},
		{"unknown payload field", `"callsign": "KLM1023"`, `"callsign": "KLM1023", "squawk": 7000`, `unknown field "squawk"`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			mutated := strings.Replace(minimalFrames, tc.old, tc.new, 1)
			require.NotEqual(t, minimalFrames, mutated, "the mutation applies")
			_, err := LoadFrames(write(t, mutated))
			require.ErrorContains(t, err, tc.message)
		})
	}

	duplicate := strings.Replace(minimalFrames, `"frames": [{`, `"frames": [{
  "id": "ident", "classification": "external", "hex": "8D4840D6202CC371C32CE0576098",
  "sourceUrl": "https://example.test/", "sourceLocation": "l", "verification": "v",
  "expected": {"icao": "4840D6", "family": "identification", "capability": 5,
    "identification": {"typeCode": 4, "category": 0, "callsign": "KLM1023"}}
}, {`, 1)
	_, err = LoadFrames(write(t, duplicate))
	require.ErrorContains(t, err, `frame "ident": duplicate id`)

	_, err = LoadFrames(filepath.Join(t.TempDir(), "missing.json"))
	require.ErrorContains(t, err, "missing.json")
}

func TestLoadDisplayReferenceRejectsMalformedData(t *testing.T) {
	t.Parallel()

	data, err := os.ReadFile(filepath.Join(corpusDir, "display-reference.json"))
	require.NoError(t, err)
	valid := string(data)

	cases := []struct {
		name, old, new, message string
	}{
		{"missing snapshot key", `"stationIds": [`, `"stations": [`, `missing field "stationIds"`},
		{"bad ICAO", `"icao": "40621D",
        "lastReceivedAt"`, `"icao": "40621d",
        "lastReceivedAt"`, "six uppercase hexadecimal digits"},
		{"nested missing key", `"toleranceDegrees": 1e-10,`, ``, `missing field "toleranceDegrees"`},
		{"negative lifetime", `"identityNanoseconds": "60000000000"`, `"identityNanoseconds": "-1"`, "nonnegative"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			mutated := strings.Replace(valid, tc.old, tc.new, 1)
			require.NotEqual(t, valid, mutated, "the mutation applies")
			_, err := LoadDisplayReference(write(t, mutated))
			require.ErrorContains(t, err, tc.message)
		})
	}
}
