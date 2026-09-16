package simulation

import (
	"crypto/sha256"
	"encoding/binary"
	"math"
	"math/rand/v2"
)

// Domain tags separate the independent random streams of the run.
// They are part of the documented seed derivation and must not change.
//
// Aircraft ordinals and station ordinals are independent counters, so the tag
// is what keeps an aircraft stream and a station stream disjoint even when the
// two ordinals coincide.
const (
	birthDomain          byte = 0
	identificationDomain byte = 1
	positionDomain       byte = 2
	velocityDomain       byte = 3
	stationDomain        byte = 4
)

// newSource derives an explicitly seeded PCG generator for one aircraft
// stream. The two seed words are the first 16 bytes of SHA-256 over the
// binary tuple of the run seed (8 bytes, little-endian), the aircraft
// creation ordinal (8 bytes, little-endian), and the domain tag (one byte),
// read as two little-endian 64-bit words.
//
// The generator is returned by value. Streams are stored as values so that a
// staged copy of engine state never shares generator state with the committed
// engine.
func newSource(seed uint64, ordinal uint64, domain byte) rand.PCG {
	var tuple [17]byte
	binary.LittleEndian.PutUint64(tuple[0:8], seed)
	binary.LittleEndian.PutUint64(tuple[8:16], ordinal)
	tuple[16] = domain

	sum := sha256.Sum256(tuple[:])
	hi := binary.LittleEndian.Uint64(sum[0:8])
	lo := binary.LittleEndian.Uint64(sum[8:16])
	return *rand.NewPCG(hi, lo)
}

// fraction draws one uniform value in [0,1) using a fixed 53-bit mantissa.
// Every floating field consumes exactly one draw, including exact ranges,
// so the stream position never depends on the configured range widths.
func fraction(r *rand.Rand) float64 {
	return float64(r.Uint64()>>11) * 0x1p-53
}

// sampleRange draws one value from r. Equal endpoints return that exact value.
// Otherwise the result lies in the half-open interval [Min, Max); a value that
// rounds up to Max is moved to the previous representable value toward Min.
func sampleRange(src *rand.Rand, r Range) float64 {
	f := fraction(src)
	if r.exact() {
		return r.Min
	}
	v := r.Min + f*(r.Max-r.Min)
	if v >= r.Max {
		v = math.Nextafter(r.Max, r.Min)
	}
	return v
}

// sampleInterval draws one inclusive integer from [lo,hi] without modulo bias.
func sampleInterval(src *rand.Rand, lo, hi uint64) uint64 {
	return lo + src.Uint64N(hi-lo+1)
}
