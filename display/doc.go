// Package display builds received-aircraft tracks from raw ADS-B evidence.
//
// A display never sees simulator truth. It reads retained receptions through
// an [ObservationSource], validates them semantically, decodes the frames with
// the testbench codec, and publishes partial aircraft state with field
// availability, age, and receiver provenance.
//
// # Sources
//
// [NewInProcessSource] adapts a provider in the same process and
// [NewHTTPSource] reads the same data from a mounted simulator over HTTP.
// Station discovery is a separate [StationSource]: [NewInProcessStationSource]
// adapts a local provider, and an [HTTPSource] also satisfies it by reading
// the simulator's stations route. Station discovery carries no aircraft truth.
// Both transports return the same data and the same failure categories for
// the same upstream state, because both run the same semantic validators. Local calls
// are validated too: a Go provider can supply non-finite numbers, nil
// collections, or inconsistent metadata that no JSON document could carry.
//
// An HTTP source bounds its request and response bytes, refuses redirects,
// owns a copy of the host's client configuration, and never retries. The host
// keeps ownership of the transport.
//
// # Refresh and retention
//
// [Display.Refresh] fetches one complete bounded raw snapshot, validates it,
// decodes it, and publishes the result atomically. Every refresh rebuilds
// identification, altitude, velocity, and even/odd CPR state from the evidence
// the source still retains; no decoded field is ever merged across refreshes.
// A late joiner and a continuously connected display therefore see the same
// tracks, and neither an evicted CPR half nor an expired field can survive in
// a cache. Track count follows the distinct received addresses in that
// evidence, not the simulator's aircraft count.
//
// Position comes only from global CPR pairing of two received frames for one
// address, honoring the codec's ten-second inclusive age bound. No simulator
// coordinate, receiver coordinate, or stale field is ever used as a local CPR
// reference, and no motion is extrapolated. Pair validity and display lifetime
// are separate: a valid historical pair supports a position until the
// configured position lifetime expires.
//
// Each of the four field lifetimes is applied independently at the snapshot's
// virtual instant. A field is fresh at exactly its lifetime and absent once
// its age exceeds it. Wall time never ages a received field.
//
// # Outage and restart
//
// At most one successful snapshot is retained, with the exact selection that
// produced it. This state is shared by every caller of one display, so the
// GET /snapshot route is diagnostic: each browser component renders the
// response of its own refresh request. An invalid selection or a canceled
// admission fails before the source is contacted and returns request-scoped
// unavailable state without touching the published snapshot. A failed refresh returns an error and, for the same selection
// within the same run, that snapshot explicitly marked stale; a failed
// selection change shows no other selection's aircraft. [Display.Snapshot]
// reports [StatusUnavailable], [StatusFresh], or [StatusStale], together with
// the real instant of the last successful refresh, kept separate from the
// source's virtual time.
//
// A validated run identifier that differs from the published one clears the
// previous run's tracks and fallback before the new run's payload is decoded,
// so corrupt new-run data can never fall back to an obsolete run. A conflict
// error carrying a different current run does the same, and so does a station
// catalog read from a replacement run. An unknown or
// malformed source identity is an error and never resets state. Within one
// run, a snapshot whose virtual time or whose per-station latest sequence
// regressed is rejected.
//
// # Handler and configuration
//
// [Display.Handler] serves the relative browser-facing routes documented on
// that method; the browser calls this backend, never the upstream simulator.
// [Config] and [HTTPSourceConfig] are complete and explicit: their
// constructors and parsers insert no application default. The host error
// callback and the HTTP client are Go dependencies, supplied separately from
// serializable settings.
//
// Nothing in this package starts a goroutine, a listener, or a polling loop.
package display
