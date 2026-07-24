package structure

/*
OfflineRing is an immutable-contents ring built from a live queue snapshot.
Navigation (Select, Merge, Slice, cursor Pop) mutates only this offline view —
never the concurrent producer/consumer state of an MPMC or SPSC queue.
*/
type OfflineRing[T any] struct {
	values []T
	cursor int
	err    error
}

/*
NewOfflineRing wraps a copied value slice as an offline ring. The slice is owned
by the ring; callers must not retain a mutable alias.
*/
func NewOfflineRing[T any](values []T) *OfflineRing[T] {
	return &OfflineRing[T]{values: values}
}

/*
Push appends value after the current cursor span, or overwrites at cursor when
still inside the snapshot. Always returns true.
*/
func (ring *OfflineRing[T]) Push(value T) bool {
	if ring.cursor < len(ring.values) {
		ring.values[ring.cursor] = value
		ring.cursor++
		return true
	}

	ring.values = append(ring.values, value)
	ring.cursor = len(ring.values)
	return true
}

/*
Pop returns the value at the cursor without advancing. ok is false when the
cursor is outside the snapshot.
*/
func (ring *OfflineRing[T]) Pop() (T, bool) {
	var zero T

	if ring == nil || ring.cursor < 0 || ring.cursor >= len(ring.values) {
		return zero, false
	}

	return ring.values[ring.cursor], true
}

/*
Select returns a new offline ring sharing the same value slice with cursor
moved by step (negative walks backward).
*/
func (ring *OfflineRing[T]) Select(step int) Ring[T] {
	cursor := ring.cursor + step

	if cursor < 0 {
		cursor = 0
	}

	if cursor > len(ring.values) {
		cursor = len(ring.values)
	}

	return &OfflineRing[T]{
		values: ring.values,
		cursor: cursor,
		err:    ring.err,
	}
}

/*
Merge appends every value from other into this offline ring.
*/
func (ring *OfflineRing[T]) Merge(other Ring[T]) bool {
	if other == nil {
		return false
	}

	other.Do(func(value T) {
		ring.values = append(ring.values, value)
	})

	return true
}

/*
Slice copies up to count values starting at the cursor into a new OfflineRing.
*/
func (ring *OfflineRing[T]) Slice(count int) Ring[T] {
	if count <= 0 || ring.cursor >= len(ring.values) {
		return NewOfflineRing[T](nil)
	}

	end := ring.cursor + count

	if end > len(ring.values) {
		end = len(ring.values)
	}

	copied := append([]T(nil), ring.values[ring.cursor:end]...)
	return NewOfflineRing(copied)
}

/*
Len returns the number of values in the snapshot.
*/
func (ring *OfflineRing[T]) Len() int {
	if ring == nil {
		return 0
	}

	return len(ring.values)
}

/*
Do visits every snapshot value in order without consuming them.
*/
func (ring *OfflineRing[T]) Do(visitor func(T)) {
	for _, value := range ring.values {
		visitor(value)
	}
}

/*
Close is a no-op for offline rings.
*/
func (ring *OfflineRing[T]) Close() error {
	return ring.err
}

/*
Error returns any stored terminal failure.
*/
func (ring *OfflineRing[T]) Error() error {
	return ring.err
}
