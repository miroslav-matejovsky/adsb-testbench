// Package simulatorapi defines transport-safe simulator contracts.
//
// The package holds data, wire representations, strict JSON parsing
// primitives, and failure categories. It contains no engine, codec, HTTP
// client, or service implementation, so both a local caller and an HTTP
// client describe the same operations with the same types.
//
// # Contents
//
// Truth data ([TruthSnapshot], [TruthAircraft], [Transmission]) describes what
// the simulation decided. Received data ([ReceptionSnapshot], [Reception],
// [ObservationSnapshot], [Aircraft]) describes what a station heard. The two
// are deliberately separate types: a display builds tracks only from received
// evidence. [Metadata] publishes effective settings and fixed policy limits.
// Control commands and acknowledgements are in control.go.
//
// # Representations
//
// Unsigned 64-bit values, including seeds, sequences, and revisions, are
// canonical decimal strings: "0", or digits without a leading zero. Durations
// are decimal nanosecond strings within signed time.Duration bounds.
// Timestamps are UTC RFC3339Nano strings within years 1-9999. ICAO addresses
// are six uppercase hexadecimal characters within 000001..FFFFFE and frames
// are exactly 28 uppercase hexadecimal characters. These representations
// prevent JSON precision loss. [FormatUint64], [ParseUint64], and their
// siblings are the single implementation of each rule.
//
// Units are degrees, knots, pressure-altitude feet relative to 1013.25 hPa,
// vertical feet per minute, metres, dBi, dBm, dB, and nautical miles.
//
// Optional received fields use pointers so unknown and known zero values stay
// distinct; an unavailable value is an explicit JSON null. Collections are
// arrays, including when empty. Non-finite floating-point values have no
// representation and are rejected on every path, including local Go input.
//
// # Presence
//
// Plain Go scalars cannot express omission, so presence is the responsibility
// of parsers. [ParseJSON] returns a [Value] tree that records presence and
// rejects duplicate object keys, trailing values, and oversized input.
// [Object.Field] rejects a missing or null key, [Object.Nullable] accepts an
// explicit null, and [Object.Done] rejects unknown keys. An explicit empty
// array, false, or numeric zero is a value, never an omission.
//
// Shared data supplied as ordinary Go values is validated semantically by its
// consumer, because a local caller can supply values no JSON parser would
// have produced.
//
// # Failures
//
// [APIError] carries a stable [Category], actionable message, optional field
// path, and the run identifier when known. errors.Is matches a category
// directly. A local error also keeps its original cause through Unwrap; the
// cause is never serialized, because an HTTP client cannot rebuild a remote Go
// error object and must recreate the category, code, and message instead.
package simulatorapi
