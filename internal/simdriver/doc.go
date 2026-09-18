// Package simdriver serializes real-time pacing and control of one simulation
// engine.
//
// Driver measures elapsed time when a ticker wakeup or command is processed,
// settles it at the previous speed, and then applies the command. Long valid
// backlogs are split to respect simulation.MaxAdvance. Backlogs beyond
// MaxCatchUp fail before delivery. Paused time is discarded.
//
// Clock makes pacing deterministic in tests. Driver.Run owns its ticker, runs
// once, and makes all later mutation commands fail with ErrStopped. Reads remain
// available directly through the concurrency-safe engine.
package simdriver
