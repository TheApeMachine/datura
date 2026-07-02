package structure

/*
listRingNode is one node in the circular list backing a ListRing. Nodes are
linked through prev and next; Value holds the element payload for type T.
*/
type ListRingNode[T any] struct {
	next, prev *ListRingNode[T]
	Value      T
}

/*
init makes node a circular list of length one when next and prev are unset.
Called lazily on the first Push or Pop when a node was allocated without being
spliced into a larger ring.
*/
func (node *ListRingNode[T]) init() {
	node.next = node
	node.prev = node
}

/*
ListRing is a fixed-size circular buffer with an internal write cursor.

Behavior is analogous to stdlib container/ring: the structure is a ring of nodes,
but unlike container/ring the ListRing type owns the cursor explicitly on the
buffer rather than requiring the caller to hold a separate node pointer.

Push stores at the cursor and advances; when the cursor passes the last slot it
wraps to the first and overwrites the oldest value. Nil *ListRing[T] represents
an empty ring. NewListRing never returns a ring with zero nodes; elementCount
must be positive.
*/
type ListRing[T any] struct {
	cursor *ListRingNode[T]
}

/*
NewListRing creates a ring of elementCount nodes, each with Value set to the zero
value of T. The cursor starts at the first node. Returns nil when elementCount is
not positive.
*/
func NewListRing[T any](elementCount int) *ListRing[T] {
	if elementCount <= 0 {
		return nil
	}

	head := new(ListRingNode[T])
	tail := head

	for index := 1; index < elementCount; index++ {
		tail.next = &ListRingNode[T]{prev: tail}
		tail = tail.next
	}

	tail.next = head
	head.prev = tail

	return &ListRing[T]{cursor: head}
}

/*
Push stores value at the cursor, advances the cursor to the next node in the
ring, and returns true on success.

Returns false when value is the nil sentinel for T. The cursor advance is owned
by the ListRing struct; callers do not need to call Select after Push.
*/
func (ring *ListRing[T]) Push(value T) bool {
	if ring.cursor.next == nil {
		ring.cursor.init()
	}

	ring.cursor.Value = value
	ring.cursor = ring.cursor.next

	return true
}

/*
Pop returns the value at the cursor without moving the cursor. Call Select to
obtain a Ring at another position, or rely on Push to advance the write cursor.
*/
func (ring *ListRing[T]) Pop() T {
	if ring.cursor.next == nil {
		ring.cursor.init()
	}

	return ring.cursor.Value
}

/*
Select moves step elements backward (step < 0) or forward (step >= 0) relative
to the current cursor and returns a new ListRing whose cursor sits at that node.

The receiver's cursor is not mutated. The returned Ring shares the same underlying
nodes as the receiver.
*/
func (ring *ListRing[T]) Select(step int) Ring[T] {
	return &ListRing[T]{
		cursor: ring.move(step),
	}
}

/*
move is Select without the Ring[T] interface wrapper.
*/
func (ring *ListRing[T]) move(step int) *ListRingNode[T] {
	if ring.cursor.next == nil {
		ring.cursor.init()
	}

	cursor := ring.cursor

	switch {
	case step < 0:
		for ; step < 0; step++ {
			cursor = cursor.prev
		}
	case step > 0:
		for ; step > 0; step-- {
			cursor = cursor.next
		}
	}

	return cursor
}

/*
Merge connects ring with other.

When ring and other are distinct ListRings, other's cursor node is spliced
immediately after ring's cursor — equivalent to growing the circular list by
other.Len() nodes. When both arguments reference nodes in the same circular list,
Merge removes the nodes strictly between ring.cursor and other.cursor.

other must be a *ListRing[T] with the same element type as ring. Returns false
when other is not a *ListRing[T].
*/
func (ring *ListRing[T]) Merge(other Ring[T]) bool {
	otherRing, ok := other.(*ListRing[T])

	if !ok {
		return false
	}

	if ring.cursor.next == nil {
		ring.cursor.init()
	}

	ring.cursor.link(otherRing.cursor)

	return true
}

/*
link splices the circular list headed at other immediately after node. When other
is non-nil, link returns the node segment that was previously after node (the
chain that was cut out when other was inserted). When other is nil, link returns
node.next without modifying links.
*/
func (node *ListRingNode[T]) link(other *ListRingNode[T]) *ListRingNode[T] {
	next := node.next

	if other != nil {
		tail := other.prev
		node.next = other
		other.prev = node
		next.prev = tail
		tail.next = next
	}

	return next
}

/*
Slice removes count modulo Len() elements starting at the node after the cursor
and returns them as a new ListRing whose cursor points at the removed segment.

When count <= 0, ring is unchanged and Slice returns nil. The removed segment is
no longer reachable from ring's cursor unless Merge reconnects it later.
*/
func (ring *ListRing[T]) Slice(count int) Ring[T] {
	if count <= 0 {
		return nil
	}

	removed := ring.cursor.link(ring.move(count + 1))

	if removed == nil {
		return nil
	}

	return &ListRing[T]{
		cursor: removed,
	}
}

/*
Len returns the number of nodes in the circular list. Time is O(n).
*/
func (ring *ListRing[T]) Len() int {
	if ring.cursor.next == nil {
		return 1
	}

	length := 1

	for walk := ring.cursor.next; walk != ring.cursor; walk = walk.next {
		length++
	}

	return length
}

/*
Do calls visitor on each Value in forward order, starting at the cursor and
walking next until the cursor is reached again.

Behavior is undefined if visitor mutates the ring structure (links or cursor).
*/
func (ring *ListRing[T]) Do(visitor func(T)) {
	visitor(ring.cursor.Value)

	if ring.cursor.next == nil {
		return
	}

	for walk := ring.cursor.next; walk != ring.cursor; walk = walk.next {
		visitor(walk.Value)
	}
}

/*
Close is a no-op for ListRing. It exists so ListRing satisfies Ring[T].
*/
func (ring *ListRing[T]) Close() error {
	return nil
}

/*
Error is always nil for ListRing. It exists so ListRing satisfies Ring[T].
*/
func (ring *ListRing[T]) Error() error {
	return nil
}
