package simulation

import (
	"crypto/sha256"
	"math"
	"math/rand/v2"
	"testing"

	"github.com/stretchr/testify/require"
)

// expectedSource rebuilds the documented seed tuple by hand, so the test does
// not simply call the production helper it is checking.
func expectedSource(seed, ordinal uint64, domain byte) rand.PCG {
	tuple := make([]byte, 0, 17)
	for i := range 8 {
		tuple = append(tuple, byte(seed>>(8*i)))
	}
	for i := range 8 {
		tuple = append(tuple, byte(ordinal>>(8*i)))
	}
	tuple = append(tuple, domain)

	sum := sha256.Sum256(tuple)
	var hi, lo uint64
	for i := range 8 {
		hi |= uint64(sum[i]) << (8 * i)
		lo |= uint64(sum[8+i]) << (8 * i)
	}
	return *rand.NewPCG(hi, lo)
}

func draws(src rand.PCG, n int) []uint64 {
	out := make([]uint64, n)
	for i := range out {
		out[i] = src.Uint64()
	}
	return out
}

func TestRandomSeedDerivation(t *testing.T) {
	t.Parallel()

	cases := []struct {
		seed, ordinal uint64
		domain        byte
	}{
		{0, 1, birthDomain},
		{0, 1, identificationDomain},
		{1 << 63, 42, positionDomain},
		{0xffffffffffffffff, 0xffffffffffffffff, velocityDomain},
	}

	for _, c := range cases {
		want := draws(expectedSource(c.seed, c.ordinal, c.domain), 4)
		got := draws(newSource(c.seed, c.ordinal, c.domain), 4)
		require.Equal(t, want, got)
	}
}

// Every stream must differ from the others for the same aircraft, and the same
// stream must reproduce exactly.
func TestRandomStreamsAreIndependent(t *testing.T) {
	t.Parallel()

	const seed, ordinal = 12345, 7
	seen := make(map[uint64]byte)
	for _, domain := range []byte{birthDomain, identificationDomain, positionDomain, velocityDomain} {
		first := draws(newSource(seed, ordinal, domain), 1)[0]
		_, duplicate := seen[first]
		require.False(t, duplicate, "domain %d repeats another stream", domain)
		seen[first] = domain

		require.Equal(t, draws(newSource(seed, ordinal, domain), 8), draws(newSource(seed, ordinal, domain), 8))
	}

	require.NotEqual(t,
		draws(newSource(seed, ordinal, positionDomain), 4),
		draws(newSource(seed, ordinal+1, positionDomain), 4))
	require.NotEqual(t,
		draws(newSource(seed, ordinal, positionDomain), 4),
		draws(newSource(seed+1, ordinal, positionDomain), 4))
}

func TestRandomFraction(t *testing.T) {
	t.Parallel()

	src := newSource(99, 1, birthDomain)
	draw := rand.New(&src)
	for range 10000 {
		f := fraction(draw)
		require.GreaterOrEqual(t, f, 0.0)
		require.Less(t, f, 1.0)
	}
}

func TestRandomSampleRange(t *testing.T) {
	t.Parallel()

	src := newSource(5, 2, birthDomain)
	draw := rand.New(&src)

	r := Range{Min: -17.5, Max: 42.25}
	for range 10000 {
		v := sampleRange(draw, r)
		require.GreaterOrEqual(t, v, r.Min)
		require.Less(t, v, r.Max)
	}
}

// An exact range returns its value but still consumes exactly one draw, so the
// stream position never depends on how wide a configured range is.
func TestRandomSampleRangeExactConsumesOneDraw(t *testing.T) {
	t.Parallel()

	exactSrc := newSource(5, 2, birthDomain)
	exactDraw := rand.New(&exactSrc)
	require.Equal(t, 3.5, sampleRange(exactDraw, Range{Min: 3.5, Max: 3.5}))
	after := exactDraw.Uint64()

	wideSrc := newSource(5, 2, birthDomain)
	wideDraw := rand.New(&wideSrc)
	sampleRange(wideDraw, Range{Min: 0, Max: 100})
	require.Equal(t, after, wideDraw.Uint64())
}

// A fraction that rounds up to the upper endpoint is pulled back one ULP.
func TestRandomSampleRangeNeverReachesMax(t *testing.T) {
	t.Parallel()

	// The widest representable interval magnifies rounding at the top end.
	r := Range{Min: -math.MaxFloat64 / 4, Max: math.MaxFloat64 / 4}
	src := newSource(11, 3, birthDomain)
	draw := rand.New(&src)
	for range 10000 {
		require.Less(t, sampleRange(draw, r), r.Max)
	}
}

func TestRandomSampleInterval(t *testing.T) {
	t.Parallel()

	src := newSource(21, 4, positionDomain)
	draw := rand.New(&src)

	seen := make(map[uint64]int)
	for range 20000 {
		v := sampleInterval(draw, 400, 600)
		require.GreaterOrEqual(t, v, uint64(400))
		require.LessOrEqual(t, v, uint64(600))
		seen[v]++
	}
	require.Len(t, seen, 201, "every value in the inclusive interval is reachable")

	single := sampleInterval(draw, 7, 7)
	require.Equal(t, uint64(7), single)
}
