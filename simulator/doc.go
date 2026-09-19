// Package simulator owns a simulation engine, its serialized real-time
// driver, and one transport-neutral service over both.
//
// [New] validates configuration without starting background work.
// [Simulator.Run] owns pacing and must be supervised by the caller. Control
// methods settle measured time at the previous speed before editing engine
// state. Read methods return committed snapshots without advancing time.
//
// # Service
//
// [NewAPI] wraps a runtime in an [API], the single implementation behind both
// direct Go calls and HTTP. Its reads and controls use the transport-safe data
// of the simulatorapi package, so a local caller and a remote client observe
// the same operations, values, and failure categories. An API is permanently
// attached to one run, and every mutation must name that run, so a command
// written for a replacement run is a conflict.
//
// [API.Handler] returns the relative HTTP routes documented on that method.
// A host mounts them wherever it likes with http.StripPrefix and keeps
// ownership of listeners, logging, and lifecycle.
//
// # Explicit configuration
//
// [Config], [APIConfig], and their parsers insert no application default.
// [ParseConfig] reads a complete JSON simulation configuration and
// [ParseAPIConfig] a complete JSON service configuration; both reject missing
// keys, explicit nulls, unknown keys, duplicate keys, and trailing values.
// The host error callback is a Go dependency passed separately, because a
// callback has no JSON representation.
//
// The browser-facing manager UI remains separate backlog work.
package simulator
