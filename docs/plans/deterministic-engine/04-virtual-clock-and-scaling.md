---
title: "04 - Virtual clock, exact scaling, and overflow checks"
dependencies: ["01-contract-and-configuration.md"]
effort: "M"
complexity: "high"
---

# 04 - Virtual clock, exact scaling, and overflow checks

## Objective

Implement deterministic duration conversion and time validation that preserve fractional nanoseconds across split Elapse calls.

## Target Artifacts

- Create `simulation/clock.go` and `simulation/clock_test.go`.

## Implementation Tasks

1. Represent engine time as a normalized start instant plus nonnegative elapsed time.Duration. Keep the scaling remainder as an integer from 0 through 99.

2. Implement private virtual-advance planning for durations from zero through MaxAdvance. Reject negative duration as ErrInvalid and excessive positive virtual work as ErrLimit.

3. Implement checked real-to-virtual scaling in hundredths. Test the scaled-duration bound before multiplying real nanoseconds by speed or adding carry.

4. Validate elapsed-duration addition and the final UTC year before exposing the candidate time. Deadline additions from step 03 must use equivalent checked duration arithmetic.

5. Define paused conversion as accepting nonnegative real duration, advancing no virtual time, and retaining existing carry. SetSpeed and direct Advance preserve carry.

6. Add exact integer-arithmetic tests covering fractional carry, speed changes, pause, direct Advance interleaving, zero durations, maximum durations, and date/time overflow.

## Technical Details

For positive speed s, the desired virtual nanoseconds are floor((realNS*s +
carry)/100), with the new carry equal to the remainder modulo 100.
Before multiplication, compare realNS with the integer bound
floor((MaxAdvanceNS*100 + 99 - carry)/s). MaxAdvanceNS*100 is small enough for
int64. A value beyond the bound returns ErrLimit without touching carry.

For speed 0, skip multiplication altogether and discard supplied real duration.
A previously earned sub-nanosecond fraction remains available after resume.
For example, one real nanosecond at speed 1 earns one hundredth of a virtual
nanosecond. Pausing and resuming does not erase that fraction, and real time
supplied while paused adds nothing.

Direct Advance applies a virtual duration exactly and does not consume or clear
the carry. SetSpeed changes only subsequent scaling. No engine method reads
time.Now, starts a ticker, sleeps, or catches up from wall time.

Keep the event clock in integer nanoseconds. Conversion to float seconds is
confined to absolute motion evaluation. Return ErrLimit before elapsed addition
would overflow time.Duration or before resulting dates leave years 1-9999.
Elapsed remains valid even when the aircraft count is zero.

## Verification

These commands apply to subsequent implementation, not this planning task.

- `go test ./simulation -run 'TestClock|TestScale'`
- Use a test-only big.Int arithmetic oracle for scaling edge cases; do not duplicate the production overflow formula as the only expectation.

## Acceptance Criteria

- Combined and split real durations produce identical elapsed nanoseconds and remainder.
- 0.01x, 0.33x, 1x, and 100x cases have exact expected integer results.
- Paused elapsed input causes no future catch-up.
- Negative, excessive, and overflowing durations leave candidate clock state unchanged.
- Direct virtual advancement remains usable while speed is zero.

## Non-Goals

Wall-clock sampling, heartbeat pacing, suspension catch-up, HTTP deadlines, and real-time driver lifecycle.
