// Package simdriver serializes real-time pacing and control of one simulation
// engine.
//
// Driver measures elapsed time when a ticker wakeup or command is processed,
// settles it at the previous speed, and then applies the command. Long valid
// backlogs are split to respect simulation.MaxAdvance. Backlogs beyond
// MaxCatchUp fail before delivery. Paused time is discarded.
//
// Commands are checked before any time is settled, so a rejected command
// leaves the pending real time in place for the next accepted one. Count and
// speed are range checked locally. Station commands call the engine's
// read-only validators (simulation.Engine.ValidateStationAddition,
// ValidateStationUpdate and ValidateStationRemoval), which apply the same
// registry rules as the mutations: identifier reuse, active capacity, the
// per-run identifier budget, unknown stations, stale and exhausted revisions.
// A successful validation reserves nothing; it is sufficient only because the
// driver is the sole mutator and holds its mutex from validation through the
// mutation. Error precedence is stopped driver, canceled context, invalid
// settings, registry eligibility, settlement, then mutation.
//
// Clock makes pacing deterministic in tests. Driver.Run owns its ticker, runs
// once, and makes all later mutation commands fail with ErrStopped. Reads remain
// available directly through the concurrency-safe engine.
package simdriver
