package structure

/*
Ring is the shared generic surface for every ring implementation in this
package. All concrete rings — ListRing, SPSCRing, MPMCRing, and OfflineRing —
implement the same verbs so callers can navigate, grow, and splice without
switching APIs.

Push writes at the current position and advances (or enqueues for FIFO rings).
Pop dequeues or peeks per the implementation and reports whether a value was
present; ok is false for an empty FIFO or an out-of-range offline cursor.
Select returns a new Ring[T] view at an offset from the current position without
mutating the receiver's cursor. Merge combines two rings of the same element
type, growing capacity when the union exceeds the current buffer. Slice
detaches a contiguous segment into a new Ring[T].

Live MPMC and SPSC queues treat Select as a copy-on-read snapshot into an
OfflineRing so navigation never mutates concurrent queue cells. Merge and Slice
on live queues require quiescence (no concurrent Push or Pop) unless noted.
*/
type Ring[T any] interface {
	Push(T) bool
	Pop() (T, bool)
	Select(int) Ring[T]
	Merge(Ring[T]) bool
	Slice(int) Ring[T]
	Len() int
	Do(func(T))
	Error() error
	Close() error
}

/*
NewRing returns ring unchanged. It exists so construction reads explicitly as
Ring[T] at call sites (for example structure.NewRing(structure.NewListRing[T](n))).
*/
func NewRing[T any](ring Ring[T]) Ring[T] {
	return ring
}
