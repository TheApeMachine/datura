package structure

import (
	"errors"
	"io"
	"math/bits"
	"sync/atomic"

	"github.com/bytedance/sonic"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/errnie"
)

/*
SPSCRing is a single-producer single-consumer FIFO queue backed by atomic.Pointer
slots indexed with a power-of-two mask.

Exactly one goroutine may Push and exactly one may Pop without external
synchronization. head and tail are monotonic sequence counters; the physical slot
is index = sequence & mask. Capacity must be a positive power of two.

When dropOldestOnFull is true, Push evicts the oldest queued value (via Pop) when
the ring is full instead of returning false.
*/
type SPSCRing[T any] struct {
	slots            []atomic.Pointer[T]
	mask             uint64
	head             atomic.Uint64
	tail             atomic.Uint64
	dropOldestOnFull bool
	err              error
	artifact         *datura.Artifact
}

/*
NewSPSCRing allocates a single-producer single-consumer ring of the given
capacity.

Returns a validation error when capacity is not a power of two. When
dropOldestOnFull is true, a full Push drops the oldest element instead of
failing.
*/
func NewSPSCRing[T any](
	capacity int,
	dropOldestOnFull bool,
	artifact *datura.Artifact,
) (*SPSCRing[T], error) {
	if (capacity & (capacity - 1)) != 0 {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			"SPSCRing capacity must be a power of two",
			errors.New("SPSCRing capacity must be a power of two"),
		))
	}

	ring := &SPSCRing[T]{
		slots:            make([]atomic.Pointer[T], capacity),
		mask:             uint64(capacity - 1),
		dropOldestOnFull: dropOldestOnFull,
		artifact:         artifact,
	}

	return ring, errnie.Error(errnie.Require(map[string]any{
		"slots": ring.slots,
	}))
}

/*
Push enqueues one value at the producer edge (head).

Returns false when value is the nil sentinel for T, or the ring is full and
dropOldestOnFull is false. The enqueue loop retries on CAS failure under
contention between producer steps.
*/
func (ring *SPSCRing[T]) Push(value T) bool {
	for {
		head := ring.head.Load()
		tail := ring.tail.Load()

		if head-tail >= uint64(len(ring.slots)) {
			if ring.dropOldestOnFull {
				ring.Pop()

				continue
			}

			return false
		}

		index := head & ring.mask
		stored := value

		if !ring.slots[index].CompareAndSwap(nil, &stored) {
			continue
		}

		if ring.head.CompareAndSwap(head, head+1) {
			return true
		}

		ring.slots[index].Store(nil)
	}
}

/*
Pop dequeues the oldest value from the consumer edge (tail).

Returns the zero value of T when the queue is empty. The dequeue
loop retries on CAS failure when the consumer races the producer's slot publish.
*/
func (ring *SPSCRing[T]) Pop() T {
	for {
		tail := ring.tail.Load()
		head := ring.head.Load()

		if tail >= head {
			var zero T
			return zero
		}

		index := tail & ring.mask
		value := ring.slots[index].Swap(nil)

		if value == nil {
			continue
		}

		if ring.tail.CompareAndSwap(tail, tail+1) {
			return *value
		}

		ring.slots[index].Store(value)
	}
}

/*
Empty reports whether the ring currently holds no values. This is a hint for idle
polling; head and tail may change immediately after the call returns.
*/
func (ring *SPSCRing[T]) Empty() bool {
	return ring.tail.Load() >= ring.head.Load()
}

/*
Select returns an spscNavigator Ring[T] positioned step logical elements forward
from the current dequeue edge (tail). step may be negative to walk backward in
sequence space.

The parent SPSCRing's head and tail are not modified.
*/
func (ring *SPSCRing[T]) Select(step int) Ring[T] {
	return &spscNavigator[T]{
		parent:   ring,
		position: ring.tail.Load() + uint64(step),
	}
}

/*
Merge absorbs other into ring.

When other is *SPSCRing[T], every queued value is moved into ring. If the
combined length exceeds ring's capacity, a new larger SPSCRing is allocated,
both rings are drained into it, and ring adopts the grown storage.

When other is *spscNavigator[T], values from the navigator's span are pushed into
ring without removing them from the parent queue.

Call while quiescent (no concurrent Push or Pop on the participating rings).
Returns false when other is not a compatible Ring[T].
*/
func (ring *SPSCRing[T]) Merge(other Ring[T]) bool {
	switch typed := other.(type) {
	case *SPSCRing[T]:
		return ring.mergeSPSC(typed)
	case *spscNavigator[T]:
		return ring.mergeNavigator(typed)
	default:
		return false
	}
}

/*
mergeSPSC moves every value from other into ring, reallocating ring when the
union does not fit in the current slot store.
*/
func (ring *SPSCRing[T]) mergeSPSC(other *SPSCRing[T]) bool {
	combined := ring.Len() + other.Len()

	if combined > len(ring.slots) {
		newRing, err := NewSPSCRing[T](
			1<<uint(bits.Len(uint(max(combined, 2)))),
			ring.dropOldestOnFull,
			ring.artifact,
		)

		if err != nil {
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
mergeNavigator copies values from the navigator's logical span [position, head)
into ring via Push. Slots in the parent queue are not cleared.
*/
func (ring *SPSCRing[T]) mergeNavigator(navigator *spscNavigator[T]) bool {
	head := navigator.parent.head.Load()

	for position := navigator.position; position < head; position++ {
		index := position & navigator.parent.mask
		value := navigator.parent.slots[index].Load()

		if value == nil {
			continue
		}

		if !ring.Push(*value) {
			return false
		}
	}

	return true
}

/*
Slice detaches up to count elements from the dequeue edge into a new SPSCRing.

Each element is Pop'd from ring and Push'd onto the returned ring. When count <=
0, returns nil. The returned ring owns the detached values; ring's queue shrinks
accordingly.
*/
func (ring *SPSCRing[T]) Slice(count int) Ring[T] {
	if count <= 0 {
		return nil
	}

	sliced, err := NewSPSCRing[T](
		1<<uint(bits.Len(uint(max(count, 2)))),
		false,
		ring.artifact,
	)

	if err != nil {
		return nil
	}

	for index := 0; index < count; index++ {
		value := ring.Pop()
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

This consumes the queue; it is not a snapshot. Call while quiescent if other
goroutines must not observe an emptying queue mid-Do.
*/
func (ring *SPSCRing[T]) Do(visitor func(T)) {
	for !ring.Empty() {
		visitor(ring.Pop())
	}
}

/*
Read implements io.Reader. It Pop's one queued value and marshals it through the
bound artifact.
*/
func (ring *SPSCRing[T]) Read(p []byte) (int, error) {
	if ring.artifact == nil {
		return 0, io.EOF
	}

	if ring.Empty() {
		return 0, io.EOF
	}

	value := ring.Pop()
	payload, err := sonic.Marshal(value)

	if err != nil {
		return 0, err
	}

	outbound := datura.Acquire("structure", datura.Artifact_Type_json)

	if outbound == nil {
		return 0, errors.New("structure: SPSCRing artifact acquire failed")
	}

	if scope, scopeErr := ring.artifact.Scope(); scopeErr == nil {
		outbound.WithScope(scope)
	}

	outbound.WithPayload(payload)

	return outbound.Read(p)
}

/*
Write implements io.Writer. It unmarshals p into the bound artifact and Push'es
the decoded value.
*/
func (ring *SPSCRing[T]) Write(p []byte) (int, error) {
	if ring.artifact == nil {
		return 0, errors.New("structure: SPSCRing has no artifact")
	}

	written, err := ring.artifact.Write(p)

	if err != nil {
		return written, err
	}

	if !ring.Push(datura.As[T](ring.artifact)) {
		return written, errors.New("structure: SPSCRing Push failed")
	}

	return written, nil
}

/*
Close drains all queued values by calling Do with a no-op visitor, then returns
ring.err.
*/
func (ring *SPSCRing[T]) Close() error {
	ring.Do(func(T) {})
	return ring.err
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
	for !ring.Empty() {
		value := ring.Pop()

		if !target.Push(value) {
			return false
		}
	}

	return true
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

/*
spscNavigator is a Ring[T] view anchored at one logical sequence position inside
an SPSCRing. position indexes the same monotonic space as head and tail; the
physical slot is position & parent.mask.

Navigators share the parent's slot array. Safe use matches the parent's SPSC
concurrency contract; mutating slots through a navigator while the parent is
live requires quiescence.
*/
type spscNavigator[T any] struct {
	parent   *SPSCRing[T]
	position uint64
}

/*
Push stores value in the navigator's slot when that slot is empty (CAS from nil).

Does not advance parent head or tail. Returns false when value is the nil
sentinel for T.
*/
func (navigator *spscNavigator[T]) Push(value T) bool {
	index := navigator.position & navigator.parent.mask
	stored := value

	return navigator.parent.slots[index].CompareAndSwap(nil, &stored)
}

/*
Pop loads and clears the navigator's slot without advancing parent tail.

Returns the zero value of T when the slot is empty.
*/
func (navigator *spscNavigator[T]) Pop() T {
	index := navigator.position & navigator.parent.mask
	value := navigator.parent.slots[index].Swap(nil)

	if value == nil {
		var zero T
		return zero
	}

	return *value
}

/*
Select returns a new navigator at position+step on the same parent SPSCRing.
*/
func (navigator *spscNavigator[T]) Select(step int) Ring[T] {
	return &spscNavigator[T]{
		parent:   navigator.parent,
		position: navigator.position + uint64(step),
	}
}

/*
Merge delegates to parent.mergeSPSC or parent.mergeNavigator depending on other
's concrete type.
*/
func (navigator *spscNavigator[T]) Merge(other Ring[T]) bool {
	switch typed := other.(type) {
	case *SPSCRing[T]:
		return navigator.parent.mergeSPSC(typed)
	case *spscNavigator[T]:
		return navigator.parent.mergeNavigator(typed)
	default:
		return false
	}
}

/*
Slice removes up to count values from the navigator's span [position, head) into
a new SPSCRing. Each slot in that span is Swap'd to nil in the parent. Values
that were nil are skipped.
*/
func (navigator *spscNavigator[T]) Slice(count int) Ring[T] {
	if count <= 0 {
		return nil
	}

	sliced, err := NewSPSCRing[T](
		1<<uint(bits.Len(uint(max(count, 2)))),
		false,
		navigator.parent.artifact,
	)

	if err != nil {
		return nil
	}

	head := navigator.parent.head.Load()

	for offset := 0; offset < count; offset++ {
		position := navigator.position + uint64(offset)

		if position >= head {
			break
		}

		index := position & navigator.parent.mask
		value := navigator.parent.slots[index].Swap(nil)

		if value == nil {
			continue
		}

		sliced.Push(*value)
	}

	return sliced
}

/*
Len returns the number of logical positions from navigator.position up to but not
including parent.head at the instant of the call.
*/
func (navigator *spscNavigator[T]) Len() int {
	head := navigator.parent.head.Load()

	if navigator.position >= head {
		return 0
	}

	return int(head - navigator.position)
}

/*
Do visits each non-nil value in [position, head) without removing slots.

For a consuming walk, use Slice or Pop on the parent queue instead.
*/
func (navigator *spscNavigator[T]) Do(visitor func(T)) {
	head := navigator.parent.head.Load()

	for position := navigator.position; position < head; position++ {
		index := position & navigator.parent.mask
		value := navigator.parent.slots[index].Load()

		if value == nil {
			continue
		}

		visitor(*value)
	}
}

/*
Read implements io.Reader. It reads the navigator slot through the parent
artifact.
*/
func (navigator *spscNavigator[T]) Read(p []byte) (int, error) {
	if navigator.parent.artifact == nil {
		return 0, io.EOF
	}

	if navigator.Len() == 0 {
		return 0, io.EOF
	}

	value := navigator.Pop()
	payload, err := sonic.Marshal(value)

	if err != nil {
		return 0, err
	}

	outbound := datura.Acquire("structure", datura.Artifact_Type_json)

	if outbound == nil {
		return 0, errors.New("structure: spscNavigator artifact acquire failed")
	}

	if scope, scopeErr := navigator.parent.artifact.Scope(); scopeErr == nil {
		outbound.WithScope(scope)
	}

	outbound.WithPayload(payload)

	return outbound.Read(p)
}

/*
Write implements io.Writer. It unmarshals p through the parent artifact and
stores at the navigator slot.
*/
func (navigator *spscNavigator[T]) Write(p []byte) (int, error) {
	if navigator.parent.artifact == nil {
		return 0, errors.New("structure: spscNavigator has no artifact")
	}

	written, err := navigator.parent.artifact.Write(p)

	if err != nil {
		return written, err
	}

	if !navigator.Push(datura.As[T](navigator.parent.artifact)) {
		return written, errors.New("structure: spscNavigator Push failed")
	}

	return written, nil
}

/*
Close drains the parent SPSCRing via parent.Close.
*/
func (navigator *spscNavigator[T]) Close() error {
	return navigator.parent.Close()
}

/*
Error returns parent.Error when navigator and parent are valid.
*/
func (navigator *spscNavigator[T]) Error() error {
	return navigator.parent.Error()
}
