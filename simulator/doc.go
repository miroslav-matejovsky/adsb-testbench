// Package simulator owns a simulation engine and its serialized real-time
// driver.
//
// New validates configuration without starting background work. Simulator.Run
// owns pacing and must be supervised by the caller. Control methods settle
// measured time at the previous speed before editing engine state. Read methods
// return committed snapshots without advancing time.
//
// HTTP contracts, handlers, and the manager UI are separate backlog work.
package simulator
