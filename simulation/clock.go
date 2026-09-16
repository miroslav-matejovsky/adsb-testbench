package simulation

import (
	"fmt"
	"time"
)

// scaleDivisor converts SpeedHundredths into a plain ratio.
const scaleDivisor = 100

// clock is the engine's virtual time. It is a value: planning a change
// produces a candidate clock that the caller commits only after the rest of
// the mutation succeeds.
//
// No method here reads the wall clock, sleeps, or catches up from real time.
// Callers supply every duration explicitly.
type clock struct {
	// start is the normalized UTC instant of zero elapsed time.
	start time.Time
	// elapsed is nonnegative virtual time since start.
	elapsed time.Duration
	// carry is the fractional virtual nanosecond earned but not yet granted
	// by real-time scaling, in hundredths of a nanosecond, 0 through 99.
	carry int64
}

// now returns the current virtual instant.
func (c clock) now() time.Time { return c.start.Add(c.elapsed) }

// planAdvance returns the clock after a direct virtual advance.
// The carry is preserved: direct advancement neither consumes nor clears the
// fraction earned by earlier real-time scaling.
func (c clock) planAdvance(d time.Duration) (clock, error) {
	if d < 0 {
		return clock{}, fmt.Errorf("%w: duration %s is negative", ErrInvalid, d)
	}
	if d > MaxAdvance {
		return clock{}, fmt.Errorf("%w: duration %s exceeds the %s limit for one call",
			ErrLimit, d, MaxAdvance)
	}

	elapsed, err := addDuration(c.elapsed, d)
	if err != nil {
		return clock{}, err
	}
	next := clock{start: c.start, elapsed: elapsed, carry: c.carry}
	if !withinYearBounds(next.now()) {
		return clock{}, fmt.Errorf("%w: advancing by %s leaves years %d-%d", ErrLimit, d, minYear, maxYear)
	}
	return next, nil
}

// planElapse converts a supplied real duration into virtual time at the given
// speed in hundredths and returns the resulting clock.
//
// The desired virtual nanoseconds are floor((realNS*speed + carry)/100), and
// the new carry is that expression's remainder, so splitting one real duration
// into parts yields exactly the same total virtual time.
//
// At speed 0 the supplied real duration is discarded, no virtual time passes,
// and the existing carry is preserved. There is no later catch-up.
func (c clock) planElapse(real time.Duration, speed uint16) (clock, error) {
	if real < 0 {
		return clock{}, fmt.Errorf("%w: real duration %s is negative", ErrInvalid, real)
	}
	if speed == 0 {
		return c, nil
	}

	// Reject before multiplying, so the product itself can never overflow.
	bound := (int64(MaxAdvance)*scaleDivisor + scaleDivisor - 1 - c.carry) / int64(speed)
	if int64(real) > bound {
		return clock{}, fmt.Errorf("%w: real duration %s at speed %d exceeds the %s virtual limit for one call",
			ErrLimit, real, speed, MaxAdvance)
	}

	scaled := int64(real)*int64(speed) + c.carry
	next, err := c.planAdvance(time.Duration(scaled / scaleDivisor))
	if err != nil {
		return clock{}, err
	}
	next.carry = scaled % scaleDivisor
	return next, nil
}

// addDuration adds two nonnegative durations and reports int64 overflow.
// Schedule deadlines use it too, so every duration sum is checked the same way.
func addDuration(a, b time.Duration) (time.Duration, error) {
	sum := a + b
	if b > 0 && sum < a {
		return 0, fmt.Errorf("%w: virtual lifetime %s plus %s overflows a duration", ErrLimit, a, b)
	}
	return sum, nil
}
