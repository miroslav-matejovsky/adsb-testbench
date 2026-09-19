package simulation

// Snapshot returns one detached, coherent view of the engine.
//
// Configuration, current time, aircraft truth, and retained history are all
// read under the same lock, so they always describe the same instant. Every
// aircraft is evaluated at that instant. The returned slices and frame arrays
// are copies: editing them cannot change engine state or later output.
//
// Snapshot.Config.SpeedHundredths reflects the current speed, while
// Snapshot.Config.InitialAircraftCount keeps the count given to New. The
// current count is len(Snapshot.Aircraft).
//
// Reading a snapshot draws no random values and moves no deadline.
func (e *Engine) Snapshot() Snapshot {
	e.mu.Lock()
	defer e.mu.Unlock()

	fleet := make([]Aircraft, 0, len(e.state.fleet))
	for i := range e.state.fleet {
		fleet = append(fleet, e.state.fleet[i].public(e.state.clock.start, e.state.clock.elapsed))
	}

	return Snapshot{
		Config:   e.state.cfg,
		Now:      e.state.clock.now(),
		Elapsed:  e.state.clock.elapsed,
		Aircraft: fleet,
		Stations: e.state.stations.snapshot(e.state.clock.start),
		History:  e.state.history.snapshot(),
	}
}
