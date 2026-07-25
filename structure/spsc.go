package structure

import (
	"errors"
	"math/bits"
	"sync/atomic"

	"github.com/theapemachine/errnie"
)

/*
SPSCRing is a single-producer single-consumer FIFO queue backed by atomic.Pointer
slots indexed with a power-of-two mask.

Exactly one goroutine may Push and exactly one may Pop without external
synchronization. head and tail are monotonic sequence counters; the physical slot
is index = sequence & mask. Capacity must be a positive power of two.

When dropOldestOnFull is true, a full Push advances an eviction cursor (tail)
and clears the oldest slot without executing the consumer Pop path, then
enqueues the new value.
*/
type spscSlot[T any] struct {
	value T
	ready atomic.Bool
}

type SPSCRing[T any] struct {
	slots            []spscSlot[T]
	mask             uint64
	head             atomic.Uint64
	tail             atomic.Uint64
	dropped          atomic.Uint64
	rejected         atomic.Uint64
	closed           atomic.Bool
	dropOldestOnFull bool
	err              error
}

/*
NewSPSCRing allocates a single-producer single-consumer ring of the given
capacity.

Returns a validation error when capacity is not a positive power of two. When
dropOldestOnFull is true, a full Push drops the oldest element instead of
failing.
*/
func NewSPSCRing[T any](
	capacity int,
	dropOldestOnFull bool,
) *SPSCRing[T] {
	if capacity < 1 || (capacity&(capacity-1)) != 0 {
		errnie.Error(errnie.Err(
			errnie.Validation,
			"SPSCRing capacity must be a positive power of two",
			errors.New("SPSCRing capacity must be a positive power of two"),
		))

		return nil
	}

	ring := &SPSCRing[T]{
		slots:            make([]spscSlot[T], capacity),
		mask:             uint64(capacity - 1),
		dropOldestOnFull: dropOldestOnFull,
	}

	errnie.Error(errnie.Require(map[string]any{
		"slots": ring.slots,
	}))

	return ring
}

/*
Push enqueues one value at the producer edge (head).

Returns false when the ring is closed, or the ring is full and dropOldestOnFull
is false. On drop-oldest overflow the producer advances the eviction cursor
without calling Pop.
*/
func (ring *SPSCRing[T]) Push(value T) bool {
	if ring.closed.Load() {
		return false
	}

	head := ring.head.Load()
	tail := ring.tail.Load()

	if head-tail >= uint64(len(ring.slots)) {
		if !ring.dropOldestOnFull {
			ring.rejected.Add(1)
			return false
		}

		evictIndex := tail & ring.mask
		var zero T
		ring.slots[evictIndex].value = zero
		ring.slots[evictIndex].ready.Store(false)
		ring.tail.Store(tail + 1)
		ring.dropped.Add(1)
		tail++
	}

	index := head & ring.mask
	ring.slots[index].value = value
	ring.slots[index].ready.Store(true)
	ring.head.Store(head + 1)

	return true
}

/*
Pop dequeues the oldest value from the consumer edge (tail).

ok is false when the queue is empty.
*/
func (ring *SPSCRing[T]) Pop() (T, bool) {
	var zero T

	tail := ring.tail.Load()
	head := ring.head.Load()

	if tail >= head {
		return zero, false
	}

	index := tail & ring.mask
	if !ring.slots[index].ready.Load() {
		return zero, false
	}

	value := ring.slots[index].value
	ring.slots[index].value = zero
	ring.slots[index].ready.Store(false)
	ring.tail.Store(tail + 1)

	return value, true
}

/*
Empty reports whether the ring currently holds no values. This is a hint for idle
polling; head and tail may change immediately after the call returns.
*/
func (ring *SPSCRing[T]) Empty() bool {
	return ring.tail.Load() >= ring.head.Load()
}

/*
Dropped reports values explicitly evicted by drop-oldest overflow policy.
*/
func (ring *SPSCRing[T]) Dropped() uint64 {
	return ring.dropped.Load()
}

/*
Rejected reports pushes refused because a non-evicting ring was full.
*/
func (ring *SPSCRing[T]) Rejected() uint64 {
	return ring.rejected.Load()
}

/*
Snapshot copies currently queued values into an OfflineRing without dequeuing.
*/
func (ring *SPSCRing[T]) Snapshot() *OfflineRing[T] {
	tail := ring.tail.Load()
	head := ring.head.Load()
	values := make([]T, 0, int(head-tail))

	for position := tail; position < head; position++ {
		idx := position & ring.mask
		if ring.slots[idx].ready.Load() {
			values = append(values, ring.slots[idx].value)
		}
	}

	return NewOfflineRing(values)
}

/*
Select returns an OfflineRing snapshot positioned relative to the ring's logical
edges. Negative steps walk backward from the write edge; non-negative steps walk
forward from the read edge.
*/
func (ring *SPSCRing[T]) Select(step int) Ring[T] {
	offline := ring.Snapshot()

	if step < 0 {
		cursor := offline.Len() + step

		if cursor < 0 {
			cursor = 0
		}

		return &OfflineRing[T]{values: offline.values, cursor: cursor}
	}

	return offline.Select(step)
}

/*
Merge absorbs other into ring while quiescent.
*/
func (ring *SPSCRing[T]) Merge(other Ring[T]) bool {
	switch typed := other.(type) {
	case *SPSCRing[T]:
		return ring.mergeSPSC(typed)
	default:
		if other == nil {
			return false
		}

		ok := true

		other.Do(func(value T) {
			if !ring.Push(value) {
				ok = false
			}
		})

		return ok
	}
}

/*
mergeSPSC moves every value from other into ring, reallocating ring when the
union does not fit in the current slot store.
*/
func (ring *SPSCRing[T]) mergeSPSC(other *SPSCRing[T]) bool {
	combined := ring.Len() + other.Len()

	if combined > len(ring.slots) {
		newRing := NewSPSCRing[T](
			1<<uint(bits.Len(uint(max(combined, 2)))),
			ring.dropOldestOnFull,
		)

		if newRing == nil {
			return false
		}

		ring.drainInto(newRing)
		other.drainInto(newRing)
		ring.adopt(newRing)

		return true
	}

	return other.drainInto(ring)
}

/*
Slice detaches up to count elements from the dequeue edge into a new SPSCRing.
Stops when Pop reports empty so overlong counts do not manufacture zero values.
*/
func (ring *SPSCRing[T]) Slice(count int) Ring[T] {
	if count <= 0 {
		return nil
	}

	sliced := NewSPSCRing[T](
		1<<uint(bits.Len(uint(max(count, 2)))),
		false,
	)

	if sliced == nil {
		return nil
	}

	for range count {
		value, ok := ring.Pop()

		if !ok {
			break
		}

		sliced.Push(value)
	}

	return sliced
}

/*
Len returns the number of values queued between tail and head at the instant of
the call. The count may change immediately under concurrent Push or Pop.
*/
func (ring *SPSCRing[T]) Len() int {
	queued := ring.head.Load() - ring.tail.Load()

	if queued > uint64(len(ring.slots)) {
		return len(ring.slots)
	}

	return int(queued)
}

/*
Do drains the queue in FIFO order, invoking visitor for each value until empty.
*/
func (ring *SPSCRing[T]) Do(visitor func(T)) {
	for {
		value, ok := ring.Pop()

		if !ok {
			return
		}

		visitor(value)
	}
}

/*
Close marks the ring closed so later Push fails, drains remaining values, and
returns ring.err.
*/
func (ring *SPSCRing[T]) Close() error {
	ring.closed.Store(true)
	ring.Do(func(T) {})
	return ring.err
}

/*
Closed reports whether Close has been called.
*/
func (ring *SPSCRing[T]) Closed() bool {
	return ring.closed.Load()
}

/*
Error returns the stored terminal failure for this ring, if any.
*/
func (ring *SPSCRing[T]) Error() error {
	return ring.err
}

/*
drainInto Pop's every value from ring and Push'es each onto target until ring is
empty. Returns false when a Push onto target fails mid-drain.
*/
func (ring *SPSCRing[T]) drainInto(target *SPSCRing[T]) bool {
	for {
		value, ok := ring.Pop()

		if !ok {
			return true
		}

		if !target.Push(value) {
			return false
		}
	}
}

/*
adopt replaces ring's slot store and head/tail counters with those from grownRing
after a merge reallocation. grownRing is abandoned after adopt.
*/
func (ring *SPSCRing[T]) adopt(grownRing *SPSCRing[T]) {
	ring.slots = grownRing.slots
	ring.mask = grownRing.mask
	ring.head.Store(grownRing.head.Load())
	ring.tail.Store(grownRing.tail.Load())
}
