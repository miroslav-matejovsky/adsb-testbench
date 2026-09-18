package simulator_test

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/miroslav-matejovsky/adsb-testbench/simulation"
	"github.com/miroslav-matejovsky/adsb-testbench/simulator"
)

// validConfigJSON is a complete simulation configuration. Every value is an
// explicit fixture choice; the parser supplies no default.
const validConfigJSON = `{
  "simulation": {
    "id": "run-1",
    "startTime": "2024-03-05T12:00:00Z",
    "seed": "18446744073709551615",
    "initialAircraftCount": 0,
    "speedHundredths": 0,
    "spawn": {
      "latitudeDegrees": {"min": 49, "max": 51},
      "longitudeDegrees": {"min": 13, "max": 15},
      "altitudeFeet": {"min": 0, "max": 40000},
      "groundSpeedKnots": {"min": 0, "max": 0},
      "trackDegrees": {"min": 0, "max": 360},
      "verticalRateFeetPerMinute": {"min": 0, "max": 0}
    }
  }
}`

func TestParseConfigAcceptsCompleteDocumentWithExplicitZeros(t *testing.T) {
	t.Parallel()

	config, err := simulator.ParseConfig(strings.NewReader(validConfigJSON), 4096)
	require.NoError(t, err)
	require.Equal(t, simulation.Config{
		ID:                   "run-1",
		StartTime:            time.Date(2024, 3, 5, 12, 0, 0, 0, time.UTC),
		Seed:                 ^uint64(0),
		InitialAircraftCount: 0,
		SpeedHundredths:      0,
		Spawn: simulation.SpawnConfig{
			LatitudeDegrees:           simulation.Range{Min: 49, Max: 51},
			LongitudeDegrees:          simulation.Range{Min: 13, Max: 15},
			AltitudeFeet:              simulation.Range{Min: 0, Max: 40000},
			GroundSpeedKnots:          simulation.Range{Min: 0, Max: 0},
			TrackDegrees:              simulation.Range{Min: 0, Max: 360},
			VerticalRateFeetPerMinute: simulation.Range{Min: 0, Max: 0},
		},
	}, config.Simulation)
}

func TestParseConfigRejectsEveryMissingOrNullLeaf(t *testing.T) {
	t.Parallel()

	for _, path := range leafPaths(t, validConfigJSON) {
		t.Run("missing "+path, func(t *testing.T) {
			t.Parallel()

			_, err := simulator.ParseConfig(strings.NewReader(editLeaf(t, validConfigJSON, path, nil)), 4096)
			require.ErrorContains(t, err, "missing required field")
		})
		t.Run("null "+path, func(t *testing.T) {
			t.Parallel()

			null := json.RawMessage("null")
			_, err := simulator.ParseConfig(strings.NewReader(editLeaf(t, validConfigJSON, path, &null)), 4096)
			require.ErrorContains(t, err, "is null")
		})
	}
}

func TestParseConfigRejectsMalformedDocuments(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		body string
	}{
		{name: "unknown root key", body: `{"simulation":{},"extra":1}`},
		{name: "unknown nested key", body: strings.Replace(validConfigJSON, `"id": "run-1"`, `"id": "run-1", "extra": 1`, 1)},
		{name: "duplicate key", body: strings.Replace(validConfigJSON, `"seed": "18446744073709551615"`, `"seed": "1", "seed": "2"`, 1)},
		{name: "trailing value", body: validConfigJSON + `{"simulation":{}}`},
		{name: "seed as number", body: strings.Replace(validConfigJSON, `"18446744073709551615"`, `18446744073709551615`, 1)},
		{name: "seed not canonical", body: strings.Replace(validConfigJSON, `"18446744073709551615"`, `"007"`, 1)},
		{name: "seed overflow", body: strings.Replace(validConfigJSON, `"18446744073709551615"`, `"18446744073709551616"`, 1)},
		{name: "start time offset", body: strings.Replace(validConfigJSON, `"2024-03-05T12:00:00Z"`, `"2024-03-05T12:00:00+01:00"`, 1)},
		{name: "speed above policy", body: strings.Replace(validConfigJSON, `"speedHundredths": 0`, `"speedHundredths": 10001`, 1)},
		{name: "speed negative", body: strings.Replace(validConfigJSON, `"speedHundredths": 0`, `"speedHundredths": -1`, 1)},
		{name: "count above policy", body: strings.Replace(validConfigJSON, `"initialAircraftCount": 0`, `"initialAircraftCount": 101`, 1)},
		{name: "latitude out of domain", body: strings.Replace(validConfigJSON, `{"min": 49, "max": 51}`, `{"min": -86, "max": 51}`, 1)},
		{name: "empty id", body: strings.Replace(validConfigJSON, `"run-1"`, `"  "`, 1)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			_, err := simulator.ParseConfig(strings.NewReader(tc.body), 8192)
			require.Error(t, err)
		})
	}
}

func TestParseConfigBoundsInputBytes(t *testing.T) {
	t.Parallel()

	_, err := simulator.ParseConfig(strings.NewReader(validConfigJSON), len(validConfigJSON)-1)
	require.Error(t, err)

	_, err = simulator.ParseConfig(strings.NewReader(validConfigJSON), len(validConfigJSON))
	require.NoError(t, err)
}

// leafPaths lists every scalar path of a JSON document in stable order.
func leafPaths(t *testing.T, document string) []string {
	t.Helper()

	var root map[string]any
	require.NoError(t, json.Unmarshal([]byte(document), &root))

	var paths []string
	var walk func(prefix string, value any)
	walk = func(prefix string, value any) {
		object, ok := value.(map[string]any)
		if !ok {
			paths = append(paths, prefix)
			return
		}
		for key, child := range object {
			walk(strings.TrimPrefix(prefix+"."+key, "."), child)
		}
	}
	walk("", root)
	sort.Strings(paths)
	return paths
}

// editLeaf removes the named leaf when replacement is nil, or replaces it.
func editLeaf(t *testing.T, document, path string, replacement *json.RawMessage) string {
	t.Helper()

	var root map[string]any
	require.NoError(t, json.Unmarshal([]byte(document), &root))

	keys := strings.Split(path, ".")
	parent := root
	for _, key := range keys[:len(keys)-1] {
		child, ok := parent[key].(map[string]any)
		require.True(t, ok, "path %s is not an object at %s", path, key)
		parent = child
	}
	last := keys[len(keys)-1]
	if replacement == nil {
		delete(parent, last)
	} else {
		parent[last] = *replacement
	}

	edited, err := json.Marshal(root)
	require.NoError(t, err, fmt.Sprintf("re-encode %s", path))
	return string(edited)
}
