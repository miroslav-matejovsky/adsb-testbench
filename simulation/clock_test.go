package simulation

import (
	"math"
	"math/big"
	"math/rand/v2"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func baseClock() clock { return clock{start: fixtureStart} }

// oracleScale computes the documented conversion with arbitrary-precision
// arithmetic, independently of the production overflow guard and formula.
func oracleScale(realNS int64, speed uint16, carry int64) (virtualNS, nextCarry int64) {
	scaled := new(big.Int).Mul(big.NewInt(realNS), big.NewInt(int64(speed)))
	scaled.Add(scaled, big.NewInt(carry))
	quotient, remainder := new(big.Int).QuoRem(scaled, big.NewInt(100), new(big.Int))
	return quotient.Int64(), remainder.Int64()
}

func TestClockAdvanceValidates(t *testing.T) {
	t.Parallel()

	c := baseClock()

	_, err := c.planAdvance(-time.Nanosecond)
	require.ErrorIs(t, err, ErrInvalid)

	_, err = c.planAdvance(MaxAdvance + time.Nanosecond)
	require.ErrorIs(t, err, ErrLimit)

	at := func(d time.Duration) clock {
		next, err := c.planAdvance(d)
		require.NoError(t, err)
		return next
	}
	require.Equal(t, time.Duration(0), at(0).elapsed)
	require.Equal(t, MaxAdvance, at(MaxAdvance).elapsed)
	require.True(t, at(time.Second).now().Equal(fixtureStart.Add(time.Second)))
}

// Direct virtual advancement never consumes or clears the scaling carry.
func TestClockAdvancePreservesCarry(t *testing.T) {
	t.Parallel()

	c := clock{start: fixtureStart, carry: 73}
	next, err := c.planAdvance(5 * time.Second)
	require.NoError(t, err)
	require.Equal(t, int64(73), next.carry)
	require.Equal(t, 5*time.Second, next.elapsed)
}

func TestScaleExactValues(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		speed       uint16
		carry       int64
		real        time.Duration
		wantElapsed time.Duration
		wantCarry   int64
	}{
		{"hundredth speed loses fractions", 1, 0, time.Nanosecond, 0, 1},
		{"hundredth speed over a second", 1, 0, time.Second, 10 * time.Millisecond, 0},
		{"third speed", 33, 0, time.Second, 330 * time.Millisecond, 0},
		{"third speed odd nanoseconds", 33, 0, 7 * time.Nanosecond, 2, 31},
		{"real time", 100, 0, 1234567 * time.Nanosecond, 1234567 * time.Nanosecond, 0},
		{"hundred times", 10000, 0, time.Millisecond, 100 * time.Millisecond, 0},
		{"carry completes a nanosecond", 1, 99, time.Nanosecond, time.Nanosecond, 0},
		{"zero real duration keeps carry", 100, 42, 0, 0, 42},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			c := clock{start: fixtureStart, carry: test.carry}
			next, err := c.planElapse(test.real, test.speed)
			require.NoError(t, err)
			require.Equal(t, test.wantElapsed, next.elapsed)
			require.Equal(t, test.wantCarry, next.carry)

			wantNS, wantCarry := oracleScale(int64(test.real), test.speed, test.carry)
			require.Equal(t, wantNS, int64(next.elapsed))
			require.Equal(t, wantCarry, next.carry)
		})
	}
}

// Splitting a real duration must produce exactly the same virtual time and
// carry as supplying it in one call.
func TestScaleSplitEquivalence(t *testing.T) {
	t.Parallel()

	// 500 ms keeps even 100x scaling inside MaxAdvance.
	const total = 500 * time.Millisecond

	speeds := []uint16{0, 1, 33, 99, 100, 250, 10000}
	partitions := [][]time.Duration{
		{total},
		{250 * time.Millisecond, 250 * time.Millisecond},
		{1, 1, 1, 1, 1, total - 5},
		{166666666, 166666667, 166666667},
		{0, total, 0},
	}

	for _, speed := range speeds {
		combined := baseClock()
		combined, err := combined.planElapse(total, speed)
		require.NoError(t, err)

		for _, parts := range partitions {
			split := baseClock()
			for _, part := range parts {
				split, err = split.planElapse(part, speed)
				require.NoError(t, err)
			}
			require.Equal(t, combined.elapsed, split.elapsed, "speed %d", speed)
			require.Equal(t, combined.carry, split.carry, "speed %d", speed)
		}
	}
}

// Randomized partitions of nanosecond-scale durations, checked against the
// arbitrary-precision oracle.
func TestScaleRandomPartitions(t *testing.T) {
	t.Parallel()

	source := rand.New(rand.NewPCG(1, 2))
	for range 500 {
		speed := uint16(source.UintN(10001))
		total := time.Duration(source.Int64N(1_000_000))

		wantNS, wantCarry := oracleScale(int64(total), speed, 0)
		if speed == 0 {
			wantNS, wantCarry = 0, 0
		}

		split := baseClock()
		remaining := total
		var err error
		for remaining > 0 {
			part := time.Duration(source.Int64N(int64(remaining)) + 1)
			split, err = split.planElapse(part, speed)
			require.NoError(t, err)
			remaining -= part
		}
		require.Equal(t, wantNS, int64(split.elapsed))
		require.Equal(t, wantCarry, split.carry)
	}
}

// A paused engine discards supplied real time and keeps its earned fraction.
func TestScalePauseHasNoCatchUp(t *testing.T) {
	t.Parallel()

	c := baseClock()
	c, err := c.planElapse(time.Nanosecond, 1)
	require.NoError(t, err)
	require.Equal(t, time.Duration(0), c.elapsed)
	require.Equal(t, int64(1), c.carry)

	paused, err := c.planElapse(time.Hour, 0)
	require.NoError(t, err)
	require.Equal(t, c, paused, "paused conversion changes nothing")

	// Resuming continues from the preserved fraction: the 99 hundredths earned
	// now complete the nanosecond that the first call started.
	resumed, err := paused.planElapse(99*time.Nanosecond, 1)
	require.NoError(t, err)
	require.Equal(t, time.Nanosecond, resumed.elapsed)
	require.Equal(t, int64(0), resumed.carry)
}

// Direct stepping still works while paused.
func TestScalePausedAdvanceStillWorks(t *testing.T) {
	t.Parallel()

	c := clock{start: fixtureStart, carry: 5}
	stepped, err := c.planAdvance(2 * time.Second)
	require.NoError(t, err)

	paused, err := stepped.planElapse(time.Minute, 0)
	require.NoError(t, err)
	require.Equal(t, 2*time.Second, paused.elapsed)
	require.Equal(t, int64(5), paused.carry)
}

func TestScaleRejectsInvalidAndExcessive(t *testing.T) {
	t.Parallel()

	c := baseClock()

	_, err := c.planElapse(-time.Nanosecond, 100)
	require.ErrorIs(t, err, ErrInvalid)

	_, err = c.planElapse(-time.Nanosecond, 0)
	require.ErrorIs(t, err, ErrInvalid)

	// At one hundred times speed the largest accepted real duration is the
	// one whose scaled value still floors to MaxAdvance.
	const bound = 600_000_000 * time.Nanosecond
	accepted, err := c.planElapse(bound, 10000)
	require.NoError(t, err)
	require.Equal(t, MaxAdvance, accepted.elapsed)

	_, err = c.planElapse(bound+time.Nanosecond, 10000)
	require.ErrorIs(t, err, ErrLimit)

	_, err = c.planElapse(time.Duration(math.MaxInt64), 10000)
	require.ErrorIs(t, err, ErrLimit)

	// A rejected conversion leaves the source clock untouched.
	require.Equal(t, baseClock(), c)
}

func TestClockOverflowAndDateBounds(t *testing.T) {
	t.Parallel()

	near := clock{start: fixtureStart, elapsed: time.Duration(math.MaxInt64) - time.Second}
	_, err := near.planAdvance(2 * time.Second)
	require.ErrorIs(t, err, ErrLimit)

	endOfTime := clock{start: time.Date(9999, time.December, 31, 23, 59, 59, 0, time.UTC)}
	_, err = endOfTime.planAdvance(2 * time.Second)
	require.ErrorIs(t, err, ErrLimit)

	ok, err := endOfTime.planAdvance(500 * time.Millisecond)
	require.NoError(t, err)
	require.Equal(t, 500*time.Millisecond, ok.elapsed)
}

func TestAddDuration(t *testing.T) {
	t.Parallel()

	sum, err := addDuration(time.Second, 2*time.Second)
	require.NoError(t, err)
	require.Equal(t, 3*time.Second, sum)

	_, err = addDuration(time.Duration(math.MaxInt64), time.Nanosecond)
	require.ErrorIs(t, err, ErrLimit)

	sum, err = addDuration(time.Duration(math.MaxInt64), 0)
	require.NoError(t, err)
	require.Equal(t, time.Duration(math.MaxInt64), sum)
}
