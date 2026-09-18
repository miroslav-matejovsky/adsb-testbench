package simulation

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func validObservationExpiry() ObservationExpiry {
	return ObservationExpiry{
		Identity: 30 * time.Second, Position: 15 * time.Second,
		Altitude: 20 * time.Second, Velocity: 10 * time.Second,
	}
}

func TestObservationExpiryValidation(t *testing.T) {
	t.Parallel()

	require.NoError(t, validateObservationExpiry(validObservationExpiry()))
	for _, field := range []string{"identity", "position", "altitude", "velocity"} {
		t.Run(field, func(t *testing.T) {
			expiry := validObservationExpiry()
			switch field {
			case "identity":
				expiry.Identity = 0
			case "position":
				expiry.Position = -1
			case "altitude":
				expiry.Altitude = 0
			case "velocity":
				expiry.Velocity = -time.Second
			}
			require.ErrorIs(t, validateObservationExpiry(expiry), ErrInvalid)
		})
	}
}

func TestObservationsEmptySelection(t *testing.T) {
	t.Parallel()

	engine := newEngine(t, validConfig())
	snapshot, err := engine.Observations(t.Context(), ObservationRequest{Expiry: validObservationExpiry()})
	require.NoError(t, err)
	require.Equal(t, engine.Snapshot().Now, snapshot.Now)
	require.Empty(t, snapshot.StationIDs)
	require.NotNil(t, snapshot.StationIDs)
	require.Empty(t, snapshot.Retention)
	require.NotNil(t, snapshot.Retention)
	require.Empty(t, snapshot.Aircraft)
	require.NotNil(t, snapshot.Aircraft)
}
