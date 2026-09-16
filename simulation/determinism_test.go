package simulation

import (
	"context"
	"math/rand/v2"
	"sync"
	"testing"
	"time"

	"github.com/miroslav-matejovsky/adsb-testbench/internal/adsb"
	"github.com/stretchr/testify/require"
)

// operation is one scripted engine command. Scripts are fixed and ordered, so
// two independently constructed engines must produce identical output.
type operation struct {
	name  string
	apply func(*Engine) ([]Transmission, error)
}

func advanceOp(d time.Duration) operation {
	return operation{"advance " + d.String(), func(e *Engine) ([]Transmission, error) {
		return e.Advance(context.Background(), d)
	}}
}

func elapseOp(d time.Duration) operation {
	return operation{"elapse " + d.String(), func(e *Engine) ([]Transmission, error) {
		return e.Elapse(context.Background(), d)
	}}
}

func countOp(count int) operation {
	return operation{"set count", func(e *Engine) ([]Transmission, error) {
		return e.SetCount(context.Background(), count)
	}}
}

func speedOp(speed uint16) operation {
	return operation{"set speed", func(e *Engine) ([]Transmission, error) {
		return nil, e.SetSpeed(context.Background(), speed)
	}}
}

// runScript applies every operation and returns the concatenated output.
func runScript(t testing.TB, engine *Engine, script []operation) []Transmission {
	t.Helper()

	all := []Transmission{}
	for _, op := range script {
		batch, err := engine.apply(op)
		require.NoError(t, err, op.name)
		all = append(all, batch...)
	}
	return all
}

func (e *Engine) apply(op operation) ([]Transmission, error) { return op.apply(e) }

// partition splits total into the durations of a fixed plan.
func advanceAll(t testing.TB, engine *Engine, parts []time.Duration) []Transmission {
	t.Helper()

	all := []Transmission{}
	for _, part := range parts {
		batch, err := engine.Advance(context.Background(), part)
		require.NoError(t, err)
		all = append(all, batch...)
	}
	return all
}

func elapseAll(t testing.TB, engine *Engine, parts []time.Duration) []Transmission {
	t.Helper()

	all := []Transmission{}
	for _, part := range parts {
		batch, err := engine.Elapse(context.Background(), part)
		require.NoError(t, err)
		all = append(all, batch...)
	}
	return all
}

// TestDeterminismReplay covers identical configuration and ordered calls
// producing identical frames and identical final state.
func TestDeterminismReplay(t *testing.T) {
	t.Parallel()

	script := []operation{
		advanceOp(3 * time.Second),
		countOp(5),
		elapseOp(2 * time.Second),
		speedOp(250),
		elapseOp(1500 * time.Millisecond),
		countOp(3),
		advanceOp(4500 * time.Millisecond),
		countOp(4),
		advanceOp(MaxAdvance),
	}

	first := newEngine(t, validConfig())
	second := newEngine(t, validConfig())

	require.Equal(t, second.Snapshot(), first.Snapshot(), "creation frames and state")
	require.Equal(t, runScript(t, second, script), runScript(t, first, script))
	require.Equal(t, second.Snapshot(), first.Snapshot())

	// A different non-degenerate seed changes observable output.
	other := validConfig()
	other.Seed++
	different := newEngine(t, other)
	require.NotEqual(t, first.Snapshot().History.Messages, different.Snapshot().History.Messages)
	require.NotEqual(t, first.Snapshot().Aircraft[0].LatitudeDegrees, different.Snapshot().Aircraft[0].LatitudeDegrees)
}

// TestDeterminismVirtualPartitions covers split and combined virtual time.
func TestDeterminismVirtualPartitions(t *testing.T) {
	t.Parallel()

	const total = 30 * time.Second

	control := newEngine(t, validConfig())
	combined, err := control.Advance(context.Background(), total)
	require.NoError(t, err)
	want := control.Snapshot()

	// An exact deadline of a fresh engine, used as one partition boundary.
	probe := newEngine(t, validConfig())
	exact := probe.state.fleet[0].positionDeadline

	partitions := [][]time.Duration{
		{total},
		{10 * time.Second, 10 * time.Second, 10 * time.Second},
		{0, total, 0},
		{time.Nanosecond, 7*time.Second - time.Nanosecond, 23 * time.Second},
		{exact, total - exact},
		{exact - time.Nanosecond, time.Nanosecond, total - exact},
	}
	for i := range 30 {
		step := time.Second + time.Duration(i)*17*time.Millisecond
		var plan []time.Duration
		remaining := total
		for remaining > 0 {
			part := min(step, remaining)
			plan = append(plan, part)
			remaining -= part
		}
		partitions = append(partitions, plan)
	}

	for _, plan := range partitions {
		split := newEngine(t, validConfig())
		require.Equal(t, combined, advanceAll(t, split, plan), "plan %v", plan)
		require.Equal(t, want, split.Snapshot(), "plan %v", plan)
	}

	requireBothParities(t, combined)
}

// requireBothParities asserts that position reports alternate per aircraft
// with no repeated parity.
func requireBothParities(t testing.TB, batch []Transmission) {
	t.Helper()

	previous := make(map[uint32]bool)
	seen := make(map[uint32]int)
	for _, report := range batch {
		if report.Kind != PositionMessage {
			continue
		}
		message, err := adsb.Decode(report.Frame[:])
		require.NoError(t, err)
		if seen[report.ICAO] > 0 {
			require.NotEqual(t, previous[report.ICAO], message.Position.CPR.Odd, "parity repeats")
		}
		previous[report.ICAO] = message.Position.CPR.Odd
		seen[report.ICAO]++
	}
	require.NotEmpty(t, seen)
}

// TestDeterminismRealPartitions covers speed scaling with fractional carry.
func TestDeterminismRealPartitions(t *testing.T) {
	t.Parallel()

	// 500 ms keeps 100x scaling inside MaxAdvance.
	const total = 500 * time.Millisecond

	for _, speed := range []uint16{1, 33, 100, 10000} {
		cfg := validConfig()
		cfg.SpeedHundredths = speed

		control := newEngine(t, cfg)
		combined, err := control.Elapse(context.Background(), total)
		require.NoError(t, err)
		want := control.Snapshot()

		source := rand.New(rand.NewPCG(uint64(speed), 7))
		partitions := [][]time.Duration{
			{total},
			{250 * time.Millisecond, 250 * time.Millisecond},
			{1, 1, 1, total - 3},
			{0, total, 0},
		}
		for range 5 {
			var plan []time.Duration
			remaining := total
			for remaining > 0 {
				part := time.Duration(source.Int64N(int64(remaining)) + 1)
				plan = append(plan, part)
				remaining -= part
			}
			partitions = append(partitions, plan)
		}

		for _, plan := range partitions {
			split := newEngine(t, cfg)
			require.Equal(t, combined, elapseAll(t, split, plan), "speed %d", speed)
			require.Equal(t, want, split.Snapshot(), "speed %d", speed)
			require.Equal(t, control.state.clock.carry, split.state.clock.carry, "speed %d", speed)
		}
	}
}

// TestDeterminismControls covers pause, resume, direct stepping while paused,
// and the absence of hidden settling in SetSpeed or SetCount.
func TestDeterminismControls(t *testing.T) {
	t.Parallel()

	cfg := validConfig()
	cfg.SpeedHundredths = 1

	// Carry earned before a pause survives the pause and the speed changes.
	engine := newEngine(t, cfg)
	_, err := engine.Elapse(context.Background(), 37*time.Nanosecond)
	require.NoError(t, err)
	require.Equal(t, int64(37), engine.state.clock.carry)

	require.NoError(t, engine.SetSpeed(context.Background(), 0))
	batch, err := engine.Elapse(context.Background(), time.Hour)
	require.NoError(t, err)
	require.Empty(t, batch)
	require.Equal(t, int64(37), engine.state.clock.carry)
	require.Equal(t, time.Duration(0), engine.Snapshot().Elapsed)

	// Direct stepping works while paused and preserves the carry.
	stepped, err := engine.Advance(context.Background(), 2*time.Second)
	require.NoError(t, err)
	require.NotEmpty(t, stepped)
	require.Equal(t, int64(37), engine.state.clock.carry)

	// SetCount emits creation reports but settles no time.
	before := engine.Snapshot().Elapsed
	_, err = engine.SetCount(context.Background(), 4)
	require.NoError(t, err)
	require.Equal(t, before, engine.Snapshot().Elapsed)
	require.Equal(t, int64(37), engine.state.clock.carry)

	require.NoError(t, engine.SetSpeed(context.Background(), 1))
	_, err = engine.Elapse(context.Background(), 63*time.Nanosecond)
	require.NoError(t, err)
	require.Equal(t, before+time.Nanosecond, engine.Snapshot().Elapsed)
	require.Equal(t, int64(0), engine.state.clock.carry)

	// A paused engine and one that never received real time agree exactly.
	paused := newEngine(t, cfg)
	require.NoError(t, paused.SetSpeed(context.Background(), 0))
	pausedBatch, err := paused.Elapse(context.Background(), 5*time.Second)
	require.NoError(t, err)
	require.Empty(t, pausedBatch)

	untouched := newEngine(t, cfg)
	require.Equal(t, untouched.Snapshot().History, paused.Snapshot().History)

	pausedStep, err := paused.Advance(context.Background(), 10*time.Second)
	require.NoError(t, err)
	untouchedStep, err := untouched.Advance(context.Background(), 10*time.Second)
	require.NoError(t, err)
	require.Equal(t, untouchedStep, pausedStep)
}

// byAircraft groups a batch by address, dropping global sequences, which
// legitimately differ when other aircraft emit or are removed.
type report struct {
	Timestamp time.Time
	Kind      MessageKind
	Frame     [14]byte
}

func byAircraft(batch []Transmission) map[uint32][]report {
	grouped := make(map[uint32][]report)
	for _, t := range batch {
		grouped[t.ICAO] = append(grouped[t.ICAO], report{t.Timestamp, t.Kind, t.Frame})
	}
	return grouped
}

// TestDeterminismSurvivors covers aircraft removal leaving the remaining
// aircraft's own output unchanged.
func TestDeterminismSurvivors(t *testing.T) {
	t.Parallel()

	cfg := validConfig()
	cfg.InitialAircraftCount = 4

	control := newEngine(t, cfg)
	controlBatch, err := control.Advance(context.Background(), 20*time.Second)
	require.NoError(t, err)

	reduced := newEngine(t, cfg)
	_, err = reduced.SetCount(context.Background(), 2)
	require.NoError(t, err)
	reducedBatch, err := reduced.Advance(context.Background(), 20*time.Second)
	require.NoError(t, err)

	survivors := byAircraft(reducedBatch)
	full := byAircraft(controlBatch)
	require.Len(t, survivors, 2)
	for icao, got := range survivors {
		require.Equal(t, full[icao], got, "survivor %06X", icao)
	}

	// Growing again never reuses a removed address.
	used := make(map[uint32]bool)
	for _, craft := range control.Snapshot().Aircraft {
		used[craft.ICAO] = true
	}
	for icao := range full {
		used[icao] = true
	}
	added, err := reduced.SetCount(context.Background(), 5)
	require.NoError(t, err)
	require.Len(t, added, 9)
	for _, craft := range reduced.Snapshot().Aircraft[2:] {
		require.False(t, used[craft.ICAO], "address %06X was reused", craft.ICAO)
	}
}

// TestEngineZeroAircraft covers time advancing with an empty fleet.
func TestEngineZeroAircraft(t *testing.T) {
	t.Parallel()

	cfg := validConfig()
	cfg.InitialAircraftCount = 0
	engine := newEngine(t, cfg)

	batch, err := engine.Advance(context.Background(), 45*time.Second)
	require.NoError(t, err)
	require.NotNil(t, batch)
	require.Empty(t, batch)
	require.Equal(t, 45*time.Second, engine.Snapshot().Elapsed)

	batch, err = engine.Elapse(context.Background(), 5*time.Second)
	require.NoError(t, err)
	require.Empty(t, batch)

	now := engine.Snapshot().Now
	created, err := engine.SetCount(context.Background(), 2)
	require.NoError(t, err)
	require.Len(t, created, 6)
	for _, craft := range engine.Snapshot().Aircraft {
		require.True(t, craft.CreatedAt.Equal(now))
	}

	// Emptying the fleet again keeps time moving and history intact.
	retained := engine.Snapshot().History.Messages
	_, err = engine.SetCount(context.Background(), 0)
	require.NoError(t, err)
	batch, err = engine.Advance(context.Background(), 10*time.Second)
	require.NoError(t, err)
	require.Empty(t, batch)
	require.Equal(t, retained, engine.Snapshot().History.Messages)
}

// TestEngineIdentityLifetime covers address and callsign uniqueness across
// count changes.
func TestEngineIdentityLifetime(t *testing.T) {
	t.Parallel()

	cfg := validConfig()
	cfg.InitialAircraftCount = 3
	engine := newEngine(t, cfg)

	addresses := make(map[uint32]bool)
	callsigns := make(map[string]bool)
	observe := func() {
		for _, craft := range engine.Snapshot().Aircraft {
			require.NotEqual(t, uint32(0x000000), craft.ICAO)
			require.NotEqual(t, uint32(0xffffff), craft.ICAO)
			require.GreaterOrEqual(t, craft.ICAO, minAddress)
			require.LessOrEqual(t, craft.ICAO, maxAddress)
			require.Equal(t, callsign(craft.ICAO), craft.Callsign)
			addresses[craft.ICAO] = true
			callsigns[craft.Callsign] = true
		}
	}
	observe()

	for _, count := range []int{1, 6, 0, 4, 2, 7} {
		before := len(addresses)
		_, err := engine.SetCount(context.Background(), count)
		require.NoError(t, err)
		_, err = engine.Advance(context.Background(), time.Second)
		require.NoError(t, err)
		observe()

		// Growing must introduce genuinely new identities.
		if count > 0 {
			require.GreaterOrEqual(t, len(addresses), before)
		}
	}

	require.Len(t, callsigns, len(addresses))
	require.Equal(t, uint64(len(addresses)+1), engine.state.identity.nextOrdinal)
}

// TestEngineConcurrentAccess asserts safety, not a concurrent command order.
func TestEngineConcurrentAccess(t *testing.T) {
	t.Parallel()

	engine := newEngine(t, validConfig())

	var wg sync.WaitGroup
	for range 4 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 50 {
				got := engine.Snapshot()
				require.True(t, got.Now.Equal(got.Config.StartTime.Add(got.Elapsed)))
				require.LessOrEqual(t, len(got.History.Messages), HistoryLimit)
				if len(got.History.Messages) > 0 {
					require.Equal(t, got.History.Messages[0].Sequence, got.History.OldestSequence)
					last := got.History.Messages[len(got.History.Messages)-1]
					require.Equal(t, last.Sequence, got.History.LatestSequence)
				}
			}
		}()
	}
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 25 {
				_, err := engine.Advance(context.Background(), 100*time.Millisecond)
				require.NoError(t, err)
			}
		}()
	}
	wg.Wait()

	require.Equal(t, 5*time.Second, engine.Snapshot().Elapsed)
}
