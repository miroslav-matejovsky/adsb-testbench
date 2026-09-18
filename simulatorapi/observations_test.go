package simulatorapi

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestObservationJSONPreservesStringIdentitiesAndNulls(t *testing.T) {
	t.Parallel()

	wantSequence := "9007199254740993"
	value := ObservationSnapshot{
		RunID: "run-2", Now: "2024-03-05T12:00:00Z",
		StationIDs: []string{"zero"}, Retention: []StationRetention{},
		Expiry: ObservationExpiry{
			IdentityNanoseconds: "0", PositionNanoseconds: "1",
			AltitudeNanoseconds: "2", VelocityNanoseconds: "3",
		},
		Aircraft: []Aircraft{{
			ICAO: "000001", LastReceivedAt: "2024-03-05T12:00:00Z",
			Identity: &IdentityObservation{
				Callsign: "", ObservedAt: "2024-03-05T12:00:00Z",
				Evidence: Evidence{
					TransmissionSequence: wantSequence, ICAO: "000001", Kind: "identification",
					Timestamp: "2024-03-05T12:00:00Z", Frame: "8D00000120000000000000000000",
					Receptions: []Reception{},
				},
			},
		}},
	}

	data, err := json.Marshal(value)
	require.NoError(t, err)
	require.Contains(t, string(data), `"transmissionSequence":"`+wantSequence+`"`)
	require.Contains(t, string(data), `"callsign":""`)
	require.Contains(t, string(data), `"position":null`)
	require.Contains(t, string(data), `"velocity":null`)

	var roundTrip ObservationSnapshot
	require.NoError(t, json.Unmarshal(data, &roundTrip))
	require.Equal(t, value, roundTrip)
}
