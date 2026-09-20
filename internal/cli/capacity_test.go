package cli_test

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/miroslav-matejovsky/adsb-testbench/internal/cli"
	"github.com/miroslav-matejovsky/adsb-testbench/internal/qualitytest"
	"github.com/miroslav-matejovsky/adsb-testbench/testbench"
)

// TestShippedResponseBudgetsFollowTheCapacityProfile parses the three
// example files with the command parser and requires every producer and
// consumer response budget to be the one derived in
// testdata/quality/capacity.json, where tests prove it carries the
// supported profile's largest responses.
func TestShippedResponseBudgetsFollowTheCapacityProfile(t *testing.T) {
	t.Parallel()

	capacity, err := qualitytest.LoadCapacity(filepath.Join("..", "..", "testdata", "quality", "capacity.json"))
	require.NoError(t, err)
	budgets := capacity.Budgets
	parse := func(mode testbench.Mode) cli.File {
		file, err := cli.ParseFile(shippedConfig(t, mode), mode, deps())
		require.NoError(t, err)
		return file
	}

	for _, mode := range []testbench.Mode{testbench.ModeCombined, testbench.ModeSimulator} {
		file := parse(mode)
		require.Equal(t, budgets.SimulatorAPIMaxResponseBytes, file.API.MaxResponseBytes, "%s simulatorApi", mode)
		require.Equal(t, budgets.ManagerUIMaxResponseBytes, file.Manager.MaxResponseBytes, "%s managerUi", mode)
		require.GreaterOrEqual(t, file.Manager.MaxResponseBytes, file.API.MaxResponseBytes,
			"a manager reads every simulator response")
	}
	for _, mode := range []testbench.Mode{testbench.ModeCombined, testbench.ModeDisplay} {
		file := parse(mode)
		require.Equal(t, budgets.DisplayMaxResponseBytes, file.Display.MaxResponseBytes, "%s display", mode)
		require.Equal(t, budgets.AircraftUIMaxResponseBytes, file.Aircraft.MaxResponseBytes, "%s aircraftUi", mode)
		require.GreaterOrEqual(t, file.Aircraft.MaxResponseBytes, file.Display.MaxResponseBytes,
			"an aircraft page reads every display response")
	}

	// The separate display reads the separate simulator over HTTP.
	simulatorFile, displayFile := parse(testbench.ModeSimulator), parse(testbench.ModeDisplay)
	require.Equal(t, budgets.SourceMaxResponseBytes, displayFile.Source.MaxResponseBytes)
	require.GreaterOrEqual(t, displayFile.Source.MaxResponseBytes, capacity.Bounds["rawSnapshotBytes"],
		"the source budget carries the largest raw snapshot")
	require.GreaterOrEqual(t, simulatorFile.API.MaxResponseBytes, displayFile.Source.MaxResponseBytes,
		"the simulator may send everything the source accepts")
	require.LessOrEqual(t, displayFile.Source.Timeout, displayFile.Display.RequestTimeout,
		"a source read fits inside one display request")
	require.LessOrEqual(t, displayFile.Source.MaxRequestBytes, simulatorFile.API.MaxRequestBytes,
		"the simulator accepts every request the source may send")
}
