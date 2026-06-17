package dmt

import (
	"sync/atomic"
	"time"
)

/*
Latency maintains a rolling window of operation latencies using atomic slots.
*/
type Latency struct {
	slots []atomic.Uint64
	size  int
	head  atomic.Uint64
}

/*
NewLatency creates a new latency with the specified window size.
*/
func NewLatency(size int) *Latency {
	return &Latency{
		slots: make([]atomic.Uint64, size),
		size:  size,
	}
}

/*
Record adds a new latency measurement to the tracker.
*/
func (latency *Latency) Record(duration time.Duration) {
	index := latency.head.Add(1) - 1
	slot := int(index % uint64(latency.size))
	latency.slots[slot].Store(uint64(duration))
}

/*
Average returns the average latency over the window.
*/
func (latency *Latency) Average() time.Duration {
	var sum time.Duration
	count := 0

	for index := range latency.slots {
		value := latency.slots[index].Load()

		if value == 0 {
			continue
		}

		sum += time.Duration(value)
		count++
	}

	if count == 0 {
		return 0
	}

	return sum / time.Duration(count)
}
