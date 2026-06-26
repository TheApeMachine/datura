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
The ring carries a derived context cancelled by Close.
*/
type MPMCRing[T any] struct {
	ctx        context.Context
	cancel     context.CancelFunc
	err        error
	mask       uint64
	buffer     []MPMCRingCell[T]
	enqueuePos atomic.Uint64
	dequeuePos atomic.Uint64
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

		cell.sequence.Store(position + ring.mask + 1)

		return cell.data.Swap(nil)
	}
}

/*
Push enqueues one value at the producer edge.

Returns false when value is the nil sentinel for T, or the ring is full under
contention (pushpop returns nil). Callers that must not drop spin with
runtime.Gosched.
*/
func (ring *MPMCRing[T]) Push(value T) bool {
	stored := value
	return ring.pushpop(&ring.enqueuePos, 0, pushpopProducer, &stored) != nil
}

/*
Pop dequeues the oldest value from the consumer edge.

Returns the zero value of T when the queue is empty at the instant of the
dequeue attempt.
*/
func (ring *MPMCRing[T]) Pop() T {
	value := ring.pushpop(&ring.dequeuePos, 1, pushpopConsumer, nil)

	var zero T

	if value == nil {
		return zero
	}

	return *value
}

/*
Select returns an mpmcNavigator Ring[T] positioned step logical elements forward
from the current dequeue edge (dequeuePos). step may be negative to walk
backward in sequence space.

enqueuePos and dequeuePos on the parent are not modified.
*/
func (ring *MPMCRing[T]) Select(step int) Ring[T] {
	return &mpmcNavigator[T]{
		parent:   ring,
		position: ring.dequeuePos.Load() + uint64(step),
	}
}

/*
Merge absorbs other into ring.

When other is *MPMCRing[T], every queued value is moved into ring. If the
combined length exceeds ring's capacity, a new larger MPMCRing is allocated on
ring.ctx, both rings are drained into it, and ring adopts the grown buffer.

When other is *mpmcNavigator[T], values from the navigator's span are pushed
into ring.

Call while quiescent (no concurrent Push or Pop on the participating rings).
Returns false when other is not a compatible Ring[T].
*/
func (ring *MPMCRing[T]) Merge(other Ring[T]) bool {
	switch typed := other.(type) {
	case *MPMCRing[T]:
		return ring.mergeMPMC(typed)
	case *mpmcNavigator[T]:
		return ring.mergeNavigator(typed)
	default:
		return false
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
mergeNavigator copies values from the navigator's logical span [position,
enqueuePos) into ring via Push. Cells in the parent buffer are not cleared.
*/
func (ring *MPMCRing[T]) mergeNavigator(navigator *mpmcNavigator[T]) bool {
	enqueue := navigator.parent.enqueuePos.Load()

	for position := navigator.position; position < enqueue; position++ {
		cell := &navigator.parent.buffer[position&navigator.parent.mask]
		value := cell.data.Load()

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
Slice detaches up to count elements from the dequeue edge into a new MPMCRing.

Each element is Pop'd from ring and Push'd onto the returned ring. When count <=
0, returns nil.
*/
func (ring *MPMCRing[T]) Slice(count int) Ring[T] {
	if count <= 0 {
		return nil
	}

	sliced, err := NewMPMCRing[T](ring.ctx, 1<<uint(bits.Len(uint(max(count, 2)))))

	if errnie.Error(err) != nil {
		return nil
	}

	for index := 0; index < count; index++ {
		value := ring.Pop()
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

	if ring.Len() == 0 {
		return 0, io.EOF
	}

	value := ring.Pop()
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

	if !ring.Push(datura.As[T](ring.artifact)) {
		return written, errors.New("structure: MPMCRing Push failed")
	}

	return written, nil
}

/*
Close cancels the ring's derived context. Queued values are not drained.
*/
func (ring *MPMCRing[T]) Close() error {
	ring.cancel()
	return ring.err
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
	for ring.Len() > 0 {
		visitor(ring.Pop())
	}
}

/*
drainInto Pop's every value from ring and Push'es each onto target until ring is
empty.
*/
func (ring *MPMCRing[T]) drainInto(target *MPMCRing[T]) bool {
	for ring.Len() > 0 {
		value := ring.Pop()

		if !target.Push(value) {
			return false
		}
	}

	return true
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

/*
mpmcNavigator is a Ring[T] view anchored at one logical sequence position inside
an MPMCRing. position indexes the same monotonic space as enqueuePos and
dequeuePos; the physical cell is buffer[position & parent.mask].

Navigators share the parent's cell buffer. Mutating cells through a navigator
while the parent is live requires quiescence or acceptance of races with
lock-free Push and Pop.
*/
type mpmcNavigator[T any] struct {
	parent   *MPMCRing[T]
	position uint64
}

/*
Push stores value in the navigator's cell data pointer without updating Vyukov
sequence state.

Intended for quiescent repair or bulk setup; concurrent use with live Push/Pop on
the parent violates the queue protocol.
*/
func (navigator *mpmcNavigator[T]) Push(value T) bool {
	cell := &navigator.parent.buffer[navigator.position&navigator.parent.mask]
	stored := value

	cell.data.Store(&stored)

	return true
}

/*
Pop Swap's the navigator's cell payload to nil without advancing dequeuePos.

Returns the zero value of T when the cell is empty or navigator is invalid.
*/
func (navigator *mpmcNavigator[T]) Pop() T {
	cell := &navigator.parent.buffer[navigator.position&navigator.parent.mask]
	value := cell.data.Swap(nil)

	return *value
}

/*
Select returns a new navigator at position+step on the same parent MPMCRing.
*/
func (navigator *mpmcNavigator[T]) Select(step int) Ring[T] {
	return &mpmcNavigator[T]{
		parent:   navigator.parent,
		position: navigator.position + uint64(step),
	}
}

/*
Merge delegates to parent.mergeMPMC or parent.mergeNavigator depending on other
's concrete type.
*/
func (navigator *mpmcNavigator[T]) Merge(other Ring[T]) bool {
	switch typed := other.(type) {
	case *MPMCRing[T]:
		return navigator.parent.mergeMPMC(typed)
	case *mpmcNavigator[T]:
		return navigator.parent.mergeNavigator(typed)
	default:
		return false
	}
}

/*
Slice removes up to count values from the navigator's span [position, enqueuePos)
into a new MPMCRing. Each cell in that span is Swap'd to nil in the parent.
*/
func (navigator *mpmcNavigator[T]) Slice(count int) Ring[T] {
	if count <= 0 {
		return nil
	}

	sliced, err := NewMPMCRing[T](navigator.parent.ctx, 1<<uint(bits.Len(uint(max(count, 2)))))

	if err != nil {
		return nil
	}

	enqueue := navigator.parent.enqueuePos.Load()

	for offset := 0; offset < count; offset++ {
		position := navigator.position + uint64(offset)

		if position >= enqueue {
			break
		}

		cell := &navigator.parent.buffer[position&navigator.parent.mask]
		value := cell.data.Swap(nil)

		if value == nil {
			continue
		}

		sliced.Push(*value)
	}

	return sliced
}

/*
Len returns the number of logical positions from navigator.position up to but not
including parent.enqueuePos at the instant of the call.
*/
func (navigator *mpmcNavigator[T]) Len() int {
	enqueue := navigator.parent.enqueuePos.Load()

	if navigator.position >= enqueue {
		return 0
	}

	return int(enqueue - navigator.position)
}

/*
Do visits each non-nil value in [position, enqueuePos) without dequeuing through
the Vyukov consumer path.

For a consuming walk, use Slice or Pop on the parent queue instead.
*/
func (navigator *mpmcNavigator[T]) Do(visitor func(T)) {
	enqueue := navigator.parent.enqueuePos.Load()

	for position := navigator.position; position < enqueue; position++ {
		cell := &navigator.parent.buffer[position&navigator.parent.mask]
		value := cell.data.Load()

		if value == nil {
			continue
		}

		visitor(*value)
	}
}

/*
Read implements io.Reader. It reads the navigator cell through the parent
artifact.
*/
func (navigator *mpmcNavigator[T]) Read(p []byte) (int, error) {
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
		return 0, errors.New("structure: mpmcNavigator artifact acquire failed")
	}

	if scope, scopeErr := navigator.parent.artifact.Scope(); scopeErr == nil {
		outbound.WithScope(scope)
	}

	outbound.WithPayload(payload)

	return outbound.PackInto(p)
}

/*
Write implements io.Writer. It unmarshals p through the parent artifact and
stores at the navigator cell.
*/
func (navigator *mpmcNavigator[T]) Write(p []byte) (int, error) {
	if navigator.parent.artifact == nil {
		return 0, errors.New("structure: mpmcNavigator has no artifact")
	}

	written, err := navigator.parent.artifact.Unpack(p)

	if err != nil {
		return written, err
	}

	if !navigator.Push(datura.As[T](navigator.parent.artifact)) {
		return written, errors.New("structure: mpmcNavigator Push failed")
	}

	return written, nil
}

/*
Close cancels the parent MPMCRing context via parent.Close.
*/
func (navigator *mpmcNavigator[T]) Close() error {
	return navigator.parent.Close()
}

/*
Error returns parent.Error when navigator and parent are valid.
*/
func (navigator *mpmcNavigator[T]) Error() error {
	return navigator.parent.Error()
}

/*
max returns the larger of left and right.
*/
func max(left, right int) int {
	if left > right {
		return left
	}

	return right
}
