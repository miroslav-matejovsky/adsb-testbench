// Package adsb encodes and decodes the testbench's raw 1090 MHz ADS-B frames.
// It reuses kreklow.us/go/go-adsb v0.4.1 for bit extraction, CRC calculation,
// callsign decoding, and barometric altitude decoding. Encoding, velocity
// semantics, and guarded airborne CPR reconstruction are implemented here.
//
// # Wire subset
//
// Frames contain exactly 14 bytes in transmission order. Bit positions below
// are one-based and inclusive. Bits 1-5 are DF=17, 6-8 capability, 9-32 the
// aircraft address, 33-88 the ME payload, and 89-112 CRC. The polynomial is
// 0x1FFF409 (including its degree-24 term). DF17 uses the CRC directly, without
// an address overlay. Decode compares received and computed CRC before
// interpreting fields and never attempts error correction.
//
// Addresses must be 000001..FFFFFE. The codec checks representation, not real
// aircraft registration or allocation; the simulator must assign unique
// synthetic addresses. Capability is 0 or 4-7; reserved 1-3 are rejected.
//
// Supported ME layouts (bit numbers relative to the complete frame):
//
//   - Identification, TC 1-4: type 33-37, category 38-40, eight six-bit
//     characters 41-88. Letters A-Z, digits, and spaces are accepted. Empty
//     callsigns encode as spaces. Decoding trims trailing spaces only.
//     Category and type are retained without assigning meaning to reserved codes.
//   - Barometric airborne position, TC 9-18: type 33-37, surveillance 38-39,
//     version-dependent supplement 40, altitude 41-52, time synchronization 53,
//     even/odd 54, CPR latitude 55-71, CPR longitude 72-88. Time synchronization
//     is a flag, not an absolute timestamp. Quality/version semantics remain
//     with callers; no integrity category is inferred from this frame alone.
//   - Airborne velocity, TC19, subtypes 1-4: type 33-37, subtype 38-40,
//     intent change 41, legacy IFR capability 42, quality 43-45.
//     Ground subtypes 1/2 have east sign 46 and magnitude 47-56, north sign 57
//     and magnitude 58-67. Airspeed subtypes 3/4 have heading status 46,
//     magnetic heading 47-56, IAS/TAS selection 57, and airspeed 58-67.
//     All share vertical source 68, vertical sign 69, vertical magnitude
//     70-78, reserved zero bits 79-80, altitude-difference sign 81, and
//     magnitude 82-88.
//
// Other downlink formats, surface positions, GNSS-height position TC20-22,
// and other type codes return ErrUnsupported. The supported subset is not a
// complete transponder implementation. Frame has no transport delimiters,
// Beast metadata, receiver timestamps, or RF preamble.
//
// # Units, availability, and quantization
//
// Position altitude is pressure altitude in feet relative to 1013.25 hPa,
// not height above terrain or GNSS height. ALT=0 means unavailable. Encoders
// use Q=1, rounding to 25 feet, ties upward, over [-1000,50175] feet.
// Decoders also accept valid Q=0 Gillham altitude through go-adsb.
//
// Velocity subtypes 1/2 carry signed east/north ground components in knots;
// they do not carry heading. Subtypes 3/4 carry magnetic heading and IAS/TAS,
// not ground track. Heading resolution is 360/1024 degrees. Vertical rate
// is feet/minute, positive upward, with geometric or pressure source retained.
// Altitude difference is GNSS minus barometric altitude in feet.
//
// Zero magnitude codes mean unavailable and decode to nil. Code 1 means
// available zero. Ordinary magnitude codes decode as (code-1)*resolution.
// Resolutions are 1 knot for subtypes 1/3, 4 knots for 2/4, 64 feet/minute for
// vertical rate, and 25 feet for altitude difference. The top code is over-range:
// Measurement.Value contains the signed strict magnitude threshold and
// OverRange is true. Thresholds are 1021.5 knots, 4086 knots, 32608 feet/minute,
// and 3137.5 feet respectively. This includes negative over-range altitude
// difference (all eight final bits set); it is not an unavailable sentinel.
//
// Encoders accept finite physical measurements, round ordinary magnitudes
// to the nearest bin (ties away from zero), and emit over-range codes above
// those thresholds. Exactly at a threshold they retain the highest exact bin.
// An explicit OverRange input must contain the exact signed threshold.
// Nil heading clears its status; unavailable numeric fields encode zero.
// Decoders ignore sign/payload bits whose availability code/status is absent.
//
// # CPR and caller responsibilities
//
// EncodeCPR accepts latitude [-90,90], longitude [-180,180], and an explicit
// odd/even selection. It emits 17-bit fractions using nearest-bin rounding
// and modulo wrapping. Zero fractions are valid. NL at +/-87 degrees is 2.
//
// DecodeGlobal validates two received frames, equal aircraft addresses,
// opposite parity, valid coordinates, and matching longitude-zone counts.
// Both times must be nonzero, not in the future, and at most MaxCPRAge (10s)
// old relative to caller-supplied now. Thus pair separation is at most 10s.
// It selects the newer position, choosing even on timestamp ties.
// Receiver-range and motion-plausibility checks belong to the observation
// consumer. Returned longitude is normalized to [-180,180). These functions
// own no cache or clock; callers retain samples and supply consistent times.
//
// # Emission cadence
//
// The codec generates one frame per call and performs no scheduling.
// ICAO Annex 10 Volume IV, 3.1.2.8.6.4.2/.4/.5, specifies airborne position
// and velocity intervals uniformly distributed over 0.4-0.6s, and airborne
// identification over 4.8-5.2s, with time quantization no coarser than 15ms.
// The simulation engine must schedule these independently in virtual time
// and alternate even/odd airborne CPR. A fixed 0.5s/5s schedule would be an
// explicit testbench simplification, not the randomized transmission schedule.
//
// # Errors and ownership
//
// Errors support errors.Is with ErrInvalid, ErrUnsupported, ErrParity, or
// ErrCPR. Errors from go-adsb are wrapped with their original causes intact.
// Failed operations return zero results. Frames are values; decoded payloads
// are fresh allocations. Concurrent independent calls share no mutable state.
//
// # References
//
// Bit layouts and over-range semantics follow the ICAO Doc 9871 tables
// reproduced in MIT Lincoln Laboratory ATC-334, Appendix A, tables A-1,
// A-4, A-5, and A-6:
// https://archive.ll.mit.edu/mission/aviation/publications/publication-files/atc-reports/Grappel_2007_ATC-334_WW-15318.pdf
//
// Scheduling reference, ICAO Annex 10 Volume IV:
// https://applications.icao.int/tools/ATMiKIT/story_content/external_files/story_content/external_files/Annex10_Volume%204_cons.pdf
//
// CPR equations and arithmetic limitations:
// https://shemesh.larc.nasa.gov/fm/CPR/
//
// Independently published examples, Junzi Sun, The 1090 Megahertz Riddle:
// https://mode-s.org/1090mhz/content/ads-b/2-identification.html
// https://mode-s.org/1090mhz/content/ads-b/3-airborne-position.html
// https://mode-s.org/1090mhz/content/ads-b/5-airborne-velocity.html
package adsb
