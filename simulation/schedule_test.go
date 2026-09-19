package simulation

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func scheduledAircraft(t testing.TB, ordinal uint64, createdAt time.Duration) aircraft {
	t.Helper()

	cfg, err := normalizeConfig(validConfig())
	require.NoError(t, err)
	craft := newAircraft(cfg, uint32(ordinal), ordinal, createdAt)
	require.NoError(t, craft.scheduleFirst())
	return craft
}

func TestScheduleIntervalBounds(t *testing.T) {
	t.Parallel()

	craft := scheduledAircraft(t, 1, 0)
	bounds := map[MessageKind][2]time.Duration{
		IdentificationMessage: {4800 * time.Millisecond, 5200 * time.Millisecond},
		PositionMessage:       {400 * time.Millisecond, 600 * time.Millisecond},
		VelocityMessage:       {400 * time.Millisecond, 600 * time.Millisecond},
	}

	for kind, want := range bounds {
		for range 2000 {
			got := craft.drawInterval(kind)
			require.GreaterOrEqual(t, got, want[0], kind)
			require.LessOrEqual(t, got, want[1], kind)
			require.Zero(t, got%time.Millisecond, "%s stays on the one millisecond grid", kind)
		}
	}
}

// The first deadline of each family is one interval after creation, and the
// first scheduled position report is the odd half of the CPR pair.
func TestScheduleFirstDeadlines(t *testing.T) {
	t.Parallel()

	const created = 12 * time.Second
	craft := scheduledAircraft(t, 4, created)

	require.True(t, craft.nextPositionOdd)
	for _, kind := range families {
		low, high := intervalBounds(kind)
		at := craft.deadline(kind)
		require.GreaterOrEqual(t, at, created+time.Duration(low)*time.Millisecond, kind)
		require.LessOrEqual(t, at, created+time.Duration(high)*time.Millisecond, kind)
	}
}

// Deadlines chain from the previous deadline, never from an arbitrary later
// instant, so splitting a caller's duration cannot shift the schedule.
func TestScheduleChainsFromPreviousDeadline(t *testing.T) {
	t.Parallel()

	craft := scheduledAircraft(t, 2, 0)
	previous := craft.positionDeadline
	require.NoError(t, craft.rescheduleAfter(PositionMessage))

	delta := craft.positionDeadline - previous
	require.GreaterOrEqual(t, delta, 400*time.Millisecond)
	require.LessOrEqual(t, delta, 600*time.Millisecond)
}

// Draining one family must not consume another family's stream.
func TestScheduleStreamsAreIndependent(t *testing.T) {
	t.Parallel()

	quiet := scheduledAircraft(t, 3, 0)
	busy := scheduledAircraft(t, 3, 0)
	require.Equal(t, quiet, busy)

	for range 500 {
		require.NoError(t, busy.rescheduleAfter(PositionMessage))
	}

	require.NoError(t, quiet.rescheduleAfter(VelocityMessage))
	require.NoError(t, busy.rescheduleAfter(VelocityMessage))
	require.Equal(t, quiet.velocityDeadline, busy.velocityDeadline)

	require.NoError(t, quiet.rescheduleAfter(IdentificationMessage))
	require.NoError(t, busy.rescheduleAfter(IdentificationMessage))
	require.Equal(t, quiet.identificationDeadline, busy.identificationDeadline)
}

func TestScheduleNextEventOrdering(t *testing.T) {
	t.Parallel()

	fleet := []aircraft{scheduledAircraft(t, 1, 0), scheduledAircraft(t, 2, 0)}

	// Deadline wins first.
	fleet[0].identificationDeadline = 3 * time.Second
	fleet[0].positionDeadline = 2 * time.Second
	fleet[0].velocityDeadline = 4 * time.Second
	fleet[1].identificationDeadline = time.Second
	fleet[1].positionDeadline = 5 * time.Second
	fleet[1].velocityDeadline = 5 * time.Second

	got, ok := nextEvent(fleet, time.Minute)
	require.True(t, ok)
	require.Equal(t, event{index: 1, kind: IdentificationMessage, at: time.Second}, got)

	// Equal deadlines break by creation ordinal, then by family order.
	for i := range fleet {
		for _, kind := range families {
			fleet[i].setDeadline(kind, 7*time.Second)
		}
	}
	got, ok = nextEvent(fleet, time.Minute)
	require.True(t, ok)
	require.Equal(t, event{index: 0, kind: IdentificationMessage, at: 7 * time.Second}, got)

	fleet[0].identificationDeadline = 8 * time.Second
	got, ok = nextEvent(fleet, time.Minute)
	require.True(t, ok)
	require.Equal(t, event{index: 0, kind: PositionMessage, at: 7 * time.Second}, got)
}

func TestScheduleNextEventRespectsLimit(t *testing.T) {
	t.Parallel()

	fleet := []aircraft{scheduledAircraft(t, 1, 0)}
	earliest := fleet[0].positionDeadline
	for _, kind := range families {
		if at := fleet[0].deadline(kind); at < earliest {
			earliest = at
		}
	}

	_, ok := nextEvent(fleet, earliest-time.Nanosecond)
	require.False(t, ok)

	got, ok := nextEvent(fleet, earliest)
	require.True(t, ok)
	require.Equal(t, earliest, got.at)

	_, ok = nextEvent(nil, time.Hour)
	require.False(t, ok)
}

func TestScheduleOverflowIsRejected(t *testing.T) {
	t.Parallel()

	craft := scheduledAircraft(t, 1, 0)
	craft.positionDeadline = time.Duration(1<<63 - 1)
	require.ErrorIs(t, craft.rescheduleAfter(PositionMessage), ErrLimit)
}
