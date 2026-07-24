package structure

import (
	"context"
	"errors"
	"io"
	"math/bits"
	"sync/atomic"

	"github.com/bytedance/sonic"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/errnie"
)

/*
MPMCRingCell is one slot in a Dmitry Vyukov bounded MPMC queue (rigtorp
MPMCQueue layout): a sequence counter plus an atomic.Pointer payload slot.
Producers and consumers coordinate only through atomics on sequence, enqueuePos,
and dequeuePos — no mutex.

Reference: Vyukov "Bounded MPMC queue" and rigtorp/MPMCQueue (1024cores).
*/
type MPMCRingCell[T any] struct {
	sequence atomic.Uint64
	data     atomic.Pointer[T]
}

/*
MPMCRing is a fixed-capacity multi-producer multi-consumer queue used as spill
storage where multiple goroutines may Push and Pop concurrently.

Push and Pop are lock-free. Capacity must be at least two and a power of two.
Close sets an atomic closed flag that rejects further Push; queued values remain
until drained. Select, Merge, and Slice operate on OfflineRing snapshots or
require quiescence so they never mutate live Vyukov cells through navigators.
*/
type MPMCRing[T any] struct {
	ctx        context.Context
	cancel     context.CancelFunc
	err        error
	mask       uint64
	buffer     []MPMCRingCell[T]
	enqueuePos atomic.Uint64
	dequeuePos atomic.Uint64
	closed     atomic.Bool
	artifact   *datura.Artifact
}

/*
NewMPMCRing allocates an MPMC ring with the given capacity on a derived context
from ctx.

Returns a validation error when capacity is not a power of two or is less than
two. Each cell's sequence is initialized to its index so the first enqueue on
that slot can proceed.
*/
func NewMPMCRing[T any](ctx context.Context, capacity int) (*MPMCRing[T], error) {
	if capacity < 2 || (capacity&(capacity-1)) != 0 {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			"MPMCRing capacity must be a power of two >= 2",
			errors.New("MPMCRing capacity must be a power of two >= 2"),
		))
	}

	ctx, cancel := context.WithCancel(ctx)

	ring := &MPMCRing[T]{
		ctx:    ctx,
		cancel: cancel,
		mask:   uint64(capacity - 1),
		buffer: make([]MPMCRingCell[T], capacity),
	}

	for index := range ring.buffer {
		ring.buffer[index].sequence.Store(uint64(index))
	}

	return ring, errnie.Error(errnie.Require(map[string]any{
		"ctx":    ring.ctx,
		"cancel": ring.cancel,
		"mask":   ring.mask,
		"buffer": ring.buffer,
	}))
}

/*
pushpopRole selects which side of the Vyukov queue executes after a successful
slot claim: producer publishes a value; consumer takes one out.
*/
type pushpopRole uint8

const (
	pushpopProducer pushpopRole = 1
	pushpopConsumer pushpopRole = 0
)

/*
pushpop is the shared wait and CAS loop for Push and Pop.

Producers claim enqueuePos, Store into the cell, and advance sequence by one.
Consumers claim dequeuePos, Swap the payload out, and advance sequence by
mask+1 so the slot becomes available for a future lap. Returns nil when the
queue is empty (consumer) or full (producer) at the claimed position.
*/
func (ring *MPMCRing[T]) pushpop(
	queuePos *atomic.Uint64,
	positionAdd uint64,
	role pushpopRole,
	value *T,
) *T {
	for {
		position := queuePos.Load()
		cell := &ring.buffer[position&ring.mask]
		sequence := cell.sequence.Load()
		diff := int64(sequence) - int64(position+positionAdd)

		if diff < 0 {
			return nil
		}

		if diff != 0 || !queuePos.CompareAndSwap(position, position+1) {
			continue
		}

		if role == pushpopProducer {
			cell.data.Store(value)
			cell.sequence.Store(position + 1)

			return value
		}

		value := cell.data.Swap(nil)
		cell.sequence.Store(position + ring.mask + 1)

		return value
	}
}

/*
Push enqueues one value at the producer edge.

Returns false when the ring is closed, or the ring is full under contention
(pushpop returns nil). Callers that must not drop spin with runtime.Gosched.
*/
func (ring *MPMCRing[T]) Push(value T) bool {
	if ring.closed.Load() {
		return false
	}

	stored := value
	return ring.pushpop(&ring.enqueuePos, 0, pushpopProducer, &stored) != nil
}

/*
Pop dequeues the oldest value from the consumer edge.

ok is false when the queue is empty at the instant of the dequeue attempt.
*/
func (ring *MPMCRing[T]) Pop() (T, bool) {
	var zero T
	value := ring.pushpop(&ring.dequeuePos, 1, pushpopConsumer, nil)

	if value == nil {
		return zero, false
	}

	return *value, true
}

/*
Snapshot copies currently queued values into an OfflineRing without dequeuing.
The copy may tear under concurrent Push/Pop; it never mutates Vyukov cells.
*/
func (ring *MPMCRing[T]) Snapshot() *OfflineRing[T] {
	dequeue := ring.dequeuePos.Load()
	enqueue := ring.enqueuePos.Load()
	values := make([]T, 0, int(enqueue-dequeue))

	for position := dequeue; position < enqueue; position++ {
		cell := &ring.buffer[position&ring.mask]
		value := cell.data.Load()

		if value == nil {
			continue
		}

		values = append(values, *value)
	}

	return NewOfflineRing(values)
}

/*
Select returns an OfflineRing snapshot positioned step elements from the
dequeue edge. Live queue cells are not shared with the returned ring.
*/
func (ring *MPMCRing[T]) Select(step int) Ring[T] {
	return ring.Snapshot().Select(step)
}

/*
Merge absorbs other into ring while quiescent. other is drained via Pop when it
is an *MPMCRing, or visited via Do otherwise. Combined length may reallocate.
*/
func (ring *MPMCRing[T]) Merge(other Ring[T]) bool {
	switch typed := other.(type) {
	case *MPMCRing[T]:
		return ring.mergeMPMC(typed)
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
mergeMPMC moves every value from other into ring, reallocating ring when the
union does not fit in the current buffer.
*/
func (ring *MPMCRing[T]) mergeMPMC(other *MPMCRing[T]) bool {
	combined := ring.Len() + other.Len()

	if combined > len(ring.buffer) {
		grownRing, err := NewMPMCRing[T](
			ring.ctx, 1<<uint(bits.Len(uint(max(combined, 2)))),
		)

		if errnie.Error(err) != nil {
			return false
		}

		ring.drainInto(grownRing)
		other.drainInto(grownRing)
		ring.adopt(grownRing)

		return true
	}

	return other.drainInto(ring)
}

/*
Slice detaches up to count elements from the dequeue edge into a new MPMCRing.
Stops when Pop reports empty so overlong counts do not manufacture zero values.
*/
func (ring *MPMCRing[T]) Slice(count int) Ring[T] {
	if count <= 0 {
		return nil
	}

	sliced, err := NewMPMCRing[T](ring.ctx, 1<<uint(bits.Len(uint(max(count, 2)))))

	if errnie.Error(err) != nil {
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
SetScope applies scope to the bound artifact before Read or Write.
*/
func (ring *MPMCRing[T]) SetScope(scope string) {
	if ring.artifact != nil {
		ring.artifact.WithScope(scope)
	}
}

/*
WithArtifact binds artifact I/O on this ring.
*/
func (ring *MPMCRing[T]) WithArtifact(artifact *datura.Artifact) *MPMCRing[T] {
	ring.artifact = artifact

	return ring
}

/*
WithPayload replaces the bound artifact payload before Read.
*/
func (ring *MPMCRing[T]) WithPayload(payload []byte) *MPMCRing[T] {
	if ring.artifact != nil {
		ring.artifact.WithPayload(payload)
	}

	return ring
}

/*
Read implements io.Reader. It Pop's one queued value and marshals it through the
bound artifact.
*/
func (ring *MPMCRing[T]) Read(p []byte) (int, error) {
	if ring.artifact == nil {
		return 0, io.EOF
	}

	value, ok := ring.Pop()

	if !ok {
		return 0, io.EOF
	}

	payload, err := sonic.Marshal(value)

	if err != nil {
		return 0, err
	}

	outbound := datura.Acquire("structure", datura.Artifact_Type_json)

	if outbound == nil {
		return 0, errors.New("structure: MPMCRing artifact acquire failed")
	}

	if scope, scopeErr := ring.artifact.Scope(); scopeErr == nil {
		outbound.WithScope(scope)
	}

	outbound.WithPayload(payload)

	return outbound.PackInto(p)
}

/*
Write implements io.Writer. It unmarshals p into the bound artifact and Push'es
the decoded value.
*/
func (ring *MPMCRing[T]) Write(p []byte) (int, error) {
	if ring.artifact == nil {
		return 0, errors.New("structure: MPMCRing has no artifact")
	}

	written, err := ring.artifact.Unpack(p)

	if err != nil {
		return written, err
	}

	value, err := datura.As[T](ring.artifact)

	if err != nil {
		return written, err
	}

	if !ring.Push(value) {
		return written, errors.New("structure: MPMCRing Push failed")
	}

	return written, nil
}

/*
Close marks the ring closed so Push fails, then cancels the derived context.
Queued values are left for the consumer to drain.
*/
func (ring *MPMCRing[T]) Close() error {
	ring.closed.Store(true)
	ring.cancel()
	return ring.err
}

/*
Closed reports whether Close has been called.
*/
func (ring *MPMCRing[T]) Closed() bool {
	return ring.closed.Load()
}

/*
Error returns the stored terminal failure for this ring, if any.
*/
func (ring *MPMCRing[T]) Error() error {
	return ring.err
}

/*
Len returns the number of values queued between dequeuePos and enqueuePos at the
instant of the call. The count may change immediately under concurrent Push and
Pop.
*/
func (ring *MPMCRing[T]) Len() int {
	queued := ring.enqueuePos.Load() - ring.dequeuePos.Load()

	if queued > uint64(len(ring.buffer)) {
		return len(ring.buffer)
	}

	return int(queued)
}

/*
Do drains the queue in FIFO order by repeated Pop, invoking visitor for each
value until empty.

Call while quiescent; concurrent Push or Pop during Do races the drain loop.
*/
func (ring *MPMCRing[T]) Do(visitor func(T)) {
	for {
		value, ok := ring.Pop()

		if !ok {
			return
		}

		visitor(value)
	}
}

/*
drainInto Pop's every value from ring and Push'es each onto target until ring is
empty.
*/
func (ring *MPMCRing[T]) drainInto(target *MPMCRing[T]) bool {
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
adopt replaces ring's buffer, mask, and position counters with those from
grownRing after a merge reallocation.
*/
func (ring *MPMCRing[T]) adopt(grownRing *MPMCRing[T]) {
	ring.mask = grownRing.mask
	ring.buffer = grownRing.buffer
	ring.enqueuePos.Store(grownRing.enqueuePos.Load())
	ring.dequeuePos.Store(grownRing.dequeuePos.Load())
}
