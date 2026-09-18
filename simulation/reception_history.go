package simulation

// receptionHistory is one station's fixed-capacity ring of accepted frames.
type receptionHistory struct {
	buffer []Reception
	next   int
	size   int
}

func newReceptionHistory() receptionHistory {
	return receptionHistory{buffer: make([]Reception, ReceptionHistoryLimit)}
}

func (h *receptionHistory) append(reception Reception) {
	h.buffer[h.next] = reception
	h.next = (h.next + 1) % ReceptionHistoryLimit
	if h.size < ReceptionHistoryLimit {
		h.size++
	}
}

func (h receptionHistory) clone() receptionHistory {
	copied := h
	copied.buffer = append([]Reception(nil), h.buffer...)
	return copied
}

// records returns retained receptions oldest first in caller-owned storage.
func (h receptionHistory) records() []Reception {
	records := make([]Reception, 0, h.size)
	oldest := (h.next - h.size + ReceptionHistoryLimit) % ReceptionHistoryLimit
	for i := range h.size {
		records = append(records, h.buffer[(oldest+i)%ReceptionHistoryLimit])
	}
	return records
}

func (h receptionHistory) bounds() (uint64, uint64) {
	if h.size == 0 {
		return 0, 0
	}
	oldest := (h.next - h.size + ReceptionHistoryLimit) % ReceptionHistoryLimit
	latest := (h.next - 1 + ReceptionHistoryLimit) % ReceptionHistoryLimit
	return h.buffer[oldest].Sequence, h.buffer[latest].Sequence
}
