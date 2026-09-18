// Package simulatorapi defines transport-safe simulator contracts.
//
// The implemented contract covers received frames, station provenance,
// restart-aware history cursors, partial aircraft observations, independent
// field expiry, and bounded-retention metadata. Aircraft truth and received
// observations have distinct representations.
//
// Integer sequences and revisions use decimal strings. Timestamps use UTC
// RFC3339Nano strings, durations use decimal nanosecond strings, ICAO addresses
// use six uppercase hexadecimal characters, and frames use 28 uppercase
// hexadecimal characters. These representations prevent JSON precision loss.
// Optional received fields use pointers so unknown and known zero values remain
// distinct. Collections are arrays, including when empty.
//
// Conversion, request parsing, validation, errors, and HTTP behavior belong to
// the simulator package and are implemented by the simulator API backlog item.
package simulatorapi
