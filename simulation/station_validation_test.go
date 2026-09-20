package simulation

import (
	"context"
	"math"
	"testing"

	"github.com/stretchr/testify/require"
)

// Station command validators share their rules with the commands, so each
// case asserts that validator and command agree, and that validation alone
// changes nothing visible or hidden.

func TestStationValidatorsAgreeWithCommands(t *testing.T) {
	t.Parallel()

	invalid := namedStation("bad id")
	cases := []struct {
		name     string
		reserved int
		active   []string
		validate func(context.Context, *Engine) error
		command  func(context.Context, *Engine) error
		err      error
	}{
		{
			name: "valid addition", active: []string{"primary"},
			validate: func(ctx context.Context, e *Engine) error {
				return e.ValidateStationAddition(ctx, namedStation("second"))
			},
			command: func(ctx context.Context, e *Engine) error {
				_, err := e.AddStation(ctx, namedStation("second"))
				return err
			},
		},
		{
			name: "invalid settings", active: []string{"primary"}, err: ErrInvalid,
			validate: func(ctx context.Context, e *Engine) error { return e.ValidateStationAddition(ctx, invalid) },
			command: func(ctx context.Context, e *Engine) error {
				_, err := e.AddStation(ctx, invalid)
				return err
			},
		},
		{
			name: "reused removed identifier", reserved: 3, err: ErrInvalid,
			validate: func(ctx context.Context, e *Engine) error {
				return e.ValidateStationAddition(ctx, namedStation("used-0001"))
			},
			command: func(ctx context.Context, e *Engine) error {
				_, err := e.AddStation(ctx, namedStation("used-0001"))
				return err
			},
		},
		{
			name: "lifetime budget", reserved: MaxStationIDsPerRun, err: ErrLimit,
			validate: func(ctx context.Context, e *Engine) error { return e.ValidateStationAddition(ctx, namedStation("new")) },
			command: func(ctx context.Context, e *Engine) error {
				_, err := e.AddStation(ctx, namedStation("new"))
				return err
			},
		},
		{
			name: "unknown update", active: []string{"primary"}, err: ErrNotFound,
			validate: func(ctx context.Context, e *Engine) error {
				return e.ValidateStationUpdate(ctx, 1, namedStation("missing"))
			},
			command: func(ctx context.Context, e *Engine) error {
				_, err := e.UpdateStation(ctx, 1, namedStation("missing"))
				return err
			},
		},
		{
			name: "stale update", active: []string{"primary"}, err: ErrConflict,
			validate: func(ctx context.Context, e *Engine) error {
				return e.ValidateStationUpdate(ctx, 7, namedStation("primary"))
			},
			command: func(ctx context.Context, e *Engine) error {
				_, err := e.UpdateStation(ctx, 7, namedStation("primary"))
				return err
			},
		},
		{
			name: "stale removal", active: []string{"primary"}, err: ErrConflict,
			validate: func(ctx context.Context, e *Engine) error { return e.ValidateStationRemoval(ctx, "primary", 7) },
			command:  func(ctx context.Context, e *Engine) error { return e.RemoveStation(ctx, "primary", 7) },
		},
		{
			name: "unknown removal", active: []string{"primary"}, err: ErrNotFound,
			validate: func(ctx context.Context, e *Engine) error { return e.ValidateStationRemoval(ctx, "missing", 1) },
			command:  func(ctx context.Context, e *Engine) error { return e.RemoveStation(ctx, "missing", 1) },
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			subject, control := arrangedPair(t, tc.reserved, tc.active...)
			validateErr := tc.validate(t.Context(), subject)
			requireIdenticalFuture(t, subject, control)

			subject, _ = arrangedPair(t, tc.reserved, tc.active...)
			commandErr := tc.command(t.Context(), subject)
			if tc.err == nil {
				require.NoError(t, validateErr)
				require.NoError(t, commandErr)
				return
			}
			require.ErrorIs(t, validateErr, tc.err)
			require.ErrorIs(t, commandErr, tc.err)
		})
	}
}

func TestStationValidatorsRejectExhaustedRevisions(t *testing.T) {
	t.Parallel()

	subject, control := arrangedPair(t, 0, "primary")
	for _, engine := range []*Engine{subject, control} {
		engine.state.stations.active[0].revision = math.MaxUint64
	}
	require.ErrorIs(t, subject.ValidateStationUpdate(t.Context(), math.MaxUint64, namedStation("primary")), ErrLimit)
	require.NoError(t, subject.ValidateStationRemoval(t.Context(), "primary", math.MaxUint64),
		"removal does not increment the revision")
	requireIdenticalFuture(t, subject, control)
}

func TestStationValidatorsCheckContextFirst(t *testing.T) {
	t.Parallel()

	engine := newEngine(t, validConfig())
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	require.ErrorIs(t, engine.ValidateStationAddition(ctx, namedStation("bad id")), context.Canceled)
	require.ErrorIs(t, engine.ValidateStationUpdate(ctx, 1, namedStation("missing")), context.Canceled)
	require.ErrorIs(t, engine.ValidateStationRemoval(ctx, "missing", 1), context.Canceled)
}
