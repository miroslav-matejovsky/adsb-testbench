//go:build !windows

package main

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// Interrupt delivery to a child process is platform specific; Windows
// cannot deliver os.Interrupt to another process, so this test runs only
// where it can. Portable cancellation is covered by internal/cli tests.
func TestInterruptDrainsAndExitsCleanly(t *testing.T) {
	t.Parallel()

	process := start(t, config(t, func(map[string]any) {}))
	require.NoError(t, process.Interrupt())
	require.Equal(t, 0, process.Wait(t), process.Log())
	require.Contains(t, process.Log(), "msg=stopped")
}
