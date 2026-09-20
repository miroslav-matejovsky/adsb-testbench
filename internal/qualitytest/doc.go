// Package qualitytest loads the shared test corpus stored below
// testdata/quality.
//
// The corpus has three files:
//
//   - frames.json holds literal ADS-B frames with provenance and
//     independently stated expected wire fields, plus CPR position pairs with
//     their expected global coordinates.
//   - scenarios.json holds named engine scenarios: explicit engine inputs,
//     stations, a station selection, field lifetimes, named runs of ordered
//     operations, and the names of the assertions tests execute.
//   - display-reference.json holds one complete raw reception snapshot built
//     from the external frames and the observations a display must derive
//     from it.
//
// The loader only reads and validates data. It rejects missing fields,
// unknown fields, null where a value is required, duplicate identifiers,
// unknown operations and assertions, malformed frames, negative durations,
// and trailing JSON, and names the file and entry in every error. Each
// consuming package executes scenario operations and assertions itself.
//
// Tests pass an explicit path relative to their own package directory. The
// loader holds no global state, reads no network resource, changes no working
// directory, and imports only the standard library. Production packages must
// not import it; it is test support, not a scenario or replay API.
package qualitytest
