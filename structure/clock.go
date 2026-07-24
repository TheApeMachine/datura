package structure

import (
	"errors"
	"sync"
	"sync/atomic"
	"time"
)

var (
	errClockNil      = errors.New("structure: clock ring is nil")
	errClockTimeline = errors.New("structure: clock timeline ring is nil")
	errClockFactory  = errors.New("structure: clock track ring factory is nil")
	errClockTrack    = errors.New("structure: clock track ring allocation failed")
	errClockTime     = errors.New("structure: clock observation time is required")
	errClockEmpty    = errors.New("structure: clock has no observations")
)

/*
ClockSlot is one real observation on an item track. Sequence is monotonic within
Track and makes overwritten or skipped observations detectable by consumers.
*/
type ClockSlot[K comparable, T any] struct {
	Track          K
	Wall           time.Time
	IngestSequence uint64
	Sequence       uint64
	Payload        T
}

/*
ClockRing is a bounded observation timeline with independently paced item
tracks. It implements Ring itself while Frame provides aligned event-time reads.
*/
type ClockRing[K comparable, T any] struct {
	timeline Ring[ClockSlot[K, T]]
	newTrack func() Ring[ClockSlot[K, T]]
	// tracks maps each key to its bounded history. Keys are added once, when a
	// key first appears, then only read; sync.Map is the accepted concurrent map
	// for that add-once/read-many pattern and removes the map read/write race.
	tracks       sync.Map
	err          error
	sequence     atomic.Uint64
	timelineSize atomic.Int64
	rewritten    atomic.Bool
	timelineOpen bool
}

/*
track loads one registered track, or nil when the key is not yet registered.
*/
func (clock *ClockRing[K, T]) track(key K) *clockTrack[K, T] {
	value, ok := clock.tracks.Load(key)

	if !ok {
		return nil
	}

	return value.(*clockTrack[K, T])
}

/*
rangeTracks visits every registered track. Order is unspecified, matching the
prior map iteration contract.
*/
func (clock *ClockRing[K, T]) rangeTracks(visit func(key K, track *clockTrack[K, T]) bool) {
	clock.tracks.Range(func(rawKey, rawValue any) bool {
		return visit(rawKey.(K), rawValue.(*clockTrack[K, T]))
	})
}

/*
NewClockRing creates a clock from the caller's timeline Ring and track Ring
factory. The factory is invoked lazily once for each observed or registered key.
*/
func NewClockRing[K comparable, T any](
	timeline Ring[ClockSlot[K, T]],
	newTrack func() Ring[ClockSlot[K, T]],
) (*ClockRing[K, T], error) {
	if timeline == nil {
		return nil, errClockTimeline
	}

	if newTrack == nil {
		return nil, errClockFactory
	}

	return &ClockRing[K, T]{
		timeline: timeline, newTrack: newTrack, timelineOpen: true,
	}, nil
}

/*
Observe records one real observation after validating its event time.
*/
func (clock *ClockRing[K, T]) Observe(key K, wall time.Time, payload T) error {
	if clock == nil {
		return errClockNil
	}

	if wall.IsZero() {
		return errClockTime
	}

	if !clock.Push(ClockSlot[K, T]{
		Track:   key,
		Wall:    wall,
		Payload: payload,
	}) {
		return clock.Error()
	}

	return nil
}

/*
Push writes a slot to the global timeline and its keyed track.
*/
func (clock *ClockRing[K, T]) Push(slot ClockSlot[K, T]) bool {
	if clock == nil || clock.timeline == nil {
		return false
	}

	return clock.push(slot)
}

/*
push writes one slot while the caller holds the clock lock so the global
timeline, keyed track, and both sequence counters advance as one operation.
*/
func (clock *ClockRing[K, T]) push(slot ClockSlot[K, T]) bool {
	if clock.err != nil {
		return false
	}

	if slot.Wall.IsZero() {
		clock.err = errClockTime
		return false
	}

	track, err := clock.registerTrack(slot.Track)

	if err != nil {
		clock.err = err
		return false
	}

	if slot.Sequence == 0 {
		slot.Sequence = track.sequence.Add(1)
	}

	slot.IngestSequence = clock.sequence.Add(1)

	if track.first.Load() == 0 {
		track.first.Store(slot.IngestSequence)
	}

	if !clock.timeline.Push(slot) || !track.values.Push(slot) {
		clock.err = errors.New(
			"structure: clock ring push failed and clock is no longer readable",
		)
		return false
	}

	if slot.Sequence > track.sequence.Load() {
		track.sequence.Store(slot.Sequence)
	}

	if slot.Wall.After(track.latestWall()) {
		track.storeLatest(slot.Wall)
	}

	trackCount := track.count.Add(1)

	if capacity := int64(track.values.Len()); trackCount > capacity {
		track.count.Store(capacity)
	}

	timelineSize := clock.timelineSize.Add(1)

	if capacity := int64(clock.timeline.Len()); timelineSize > capacity {
		clock.timelineSize.Store(capacity)
	}

	return true
}

/*
Pop returns the latest global timeline observation without advancing its cursor.
ok is false when the timeline has no retained observation at that cursor.
*/
func (clock *ClockRing[K, T]) Pop() (ClockSlot[K, T], bool) {
	if clock == nil || clock.timeline == nil {
		return ClockSlot[K, T]{}, false
	}

	return clock.timeline.Select(-1).Pop()
}

/*
Select returns a global timeline Ring view at step from its write cursor.
*/
func (clock *ClockRing[K, T]) Select(step int) Ring[ClockSlot[K, T]] {
	if clock == nil || clock.timeline == nil {
		return nil
	}

	return clock.timeline.Select(step)
}

/*
Merge combines clocks whose keyed tracks are disjoint. Overlapping tracks are
rejected because structural Ring splicing cannot preserve their event-time order.
*/
func (clock *ClockRing[K, T]) Merge(other Ring[ClockSlot[K, T]]) bool {
	if clock == nil || clock.timeline == nil {
		return false
	}

	otherClock, ok := other.(*ClockRing[K, T])

	if !ok || otherClock == clock {
		return false
	}

	disjoint := true

	otherClock.rangeTracks(func(key K, _ *clockTrack[K, T]) bool {
		if clock.track(key) != nil {
			disjoint = false
			return false
		}

		return true
	})

	if !disjoint {
		return false
	}

	leftSequence := clock.sequence.Load()
	leftSlots := clock.retained()
	rightSlots := otherClock.retained()

	for index := range rightSlots {
		rightSlots[index].IngestSequence += leftSequence
	}

	// Merge grows the underlying SPSC ring to hold both timelines, then drain
	// the stale (un-resequenced) items so the re-push below lands cleanly.
	if !clock.timeline.Merge(otherClock.timeline) {
		return false
	}

	clock.timeline.Do(func(ClockSlot[K, T]) {})

	for _, slot := range append(leftSlots, rightSlots...) {
		if !clock.timeline.Push(slot) {
			clock.err = errors.New("structure: clock timeline merge rewrite failed")
			return false
		}
	}

	otherClock.rangeTracks(func(key K, incoming *clockTrack[K, T]) bool {
		incoming.resequence(leftSequence)
		clock.tracks.Store(key, incoming)
		return true
	})

	clock.sequence.Store(leftSequence + otherClock.sequence.Load())
	clock.timelineSize.Store(clock.timelineSize.Load() + otherClock.timelineSize.Load())
	clock.rewritten.Store(true)

	if capacity := int64(clock.timeline.Len()); clock.timelineSize.Load() > capacity {
		clock.timelineSize.Store(capacity)
	}

	return true
}

/*
Slice detaches count slots from the global timeline as a Ring view.
*/
func (clock *ClockRing[K, T]) Slice(count int) Ring[ClockSlot[K, T]] {
	if clock == nil || clock.timeline == nil {
		return nil
	}

	return clock.timeline.Slice(count)
}

/*
Len returns the global timeline Ring capacity.
*/
func (clock *ClockRing[K, T]) Len() int {
	if clock == nil || clock.timeline == nil {
		return 0
	}

	return clock.timeline.Len()
}

/*
Do visits the global timeline in its Ring-defined logical order.
*/
func (clock *ClockRing[K, T]) Do(visitor func(ClockSlot[K, T])) {
	if clock == nil || clock.timeline == nil {
		return
	}

	clock.timeline.Do(visitor)
}

/*
Error returns the clock or underlying timeline error.
*/
func (clock *ClockRing[K, T]) Error() error {
	if clock == nil {
		return errClockNil
	}

	if clock.err != nil {
		return clock.err
	}

	return clock.timeline.Error()
}

/*
Close closes every track Ring and the global timeline Ring.
*/
func (clock *ClockRing[K, T]) Close() error {
	if clock == nil {
		return errClockNil
	}

	var closeErr error

	clock.rangeTracks(func(_ K, track *clockTrack[K, T]) bool {
		if err := track.values.Close(); err != nil {
			closeErr = err
			return false
		}

		return true
	})

	if closeErr != nil {
		return closeErr
	}

	if !clock.timelineOpen {
		return nil
	}

	clock.timelineOpen = false

	return clock.timeline.Close()
}

var _ Ring[ClockSlot[string, float64]] = (*ClockRing[string, float64])(nil)
