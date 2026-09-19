package simulation

// history is a fixed-capacity ring of the most recent transmissions.
// Eviction only affects retention: the batch returned by the call that emitted
// a transmission stays complete even when history no longer holds it.
type history struct {
	buffer []Transmission
	// next is the index that will receive the following transmission.
	next int
	// size is the number of retained transmissions, at most HistoryLimit.
	size int
}

// newHistory allocates an empty ring.
func newHistory() history {
	return history{buffer: make([]Transmission, HistoryLimit)}
}

// append retains one transmission, evicting the oldest when full.
func (h *history) append(t Transmission) {
	h.buffer[h.next] = t
	h.next = (h.next + 1) % HistoryLimit
	if h.size < HistoryLimit {
		h.size++
	}
}

// clone copies the ring, including its backing storage, so a staged mutation
// shares nothing with committed state.
func (h history) clone() history {
	copied := h
	copied.buffer = append([]Transmission(nil), h.buffer...)
	return copied
}

// snapshot returns the retained transmissions oldest first, in a slice owned
// by the caller. Bounds are zero only when nothing is retained.
func (h history) snapshot() HistorySnapshot {
	messages := make([]Transmission, 0, h.size)
	oldest := (h.next - h.size + HistoryLimit) % HistoryLimit
	for i := range h.size {
		messages = append(messages, h.buffer[(oldest+i)%HistoryLimit])
	}

	snapshot := HistorySnapshot{Messages: messages, Limit: HistoryLimit}
	if h.size > 0 {
		snapshot.OldestSequence = messages[0].Sequence
		snapshot.LatestSequence = messages[h.size-1].Sequence
	}
	return snapshot
}
