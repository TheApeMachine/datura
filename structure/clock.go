package structure

import (
	"errors"
	"sync"
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
	access       sync.RWMutex
	timeline     Ring[ClockSlot[K, T]]
	newTrack     func() Ring[ClockSlot[K, T]]
	tracks       map[K]*clockTrack[K, T]
	err          error
	sequence     uint64
	timelineSize int
	rewritten    bool
	timelineOpen bool
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
		timeline: timeline, newTrack: newTrack,
		tracks: make(map[K]*clockTrack[K, T]), timelineOpen: true,
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

	clock.access.Lock()
	defer clock.access.Unlock()

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

	if err := clock.register(slot.Track); err != nil {
		clock.err = err
		return false
	}

	track := clock.tracks[slot.Track]

	if slot.Sequence == 0 {
		track.sequence++
		slot.Sequence = track.sequence
	}

	clock.sequence++
	slot.IngestSequence = clock.sequence

	if track.first == 0 {
		track.first = slot.IngestSequence
	}

	if !clock.timeline.Push(slot) || !track.values.Push(slot) {
		clock.err = errors.New(
			"structure: clock ring push failed and clock is no longer readable",
		)
		return false
	}

	if slot.Sequence > track.sequence {
		track.sequence = slot.Sequence
	}

	if slot.Wall.After(track.latest) {
		track.latest = slot.Wall
	}

	track.count++

	if capacity := track.values.Len(); track.count > capacity {
		track.count = capacity
	}

	clock.timelineSize++

	if capacity := clock.timeline.Len(); clock.timelineSize > capacity {
		clock.timelineSize = capacity
	}

	return true
}

/*
Pop returns the latest global timeline observation without advancing its cursor.
*/
func (clock *ClockRing[K, T]) Pop() ClockSlot[K, T] {
	if clock == nil || clock.timeline == nil {
		return ClockSlot[K, T]{}
	}

	clock.access.RLock()
	defer clock.access.RUnlock()

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

	clock.access.Lock()
	defer clock.access.Unlock()
	otherClock.access.Lock()
	defer otherClock.access.Unlock()

	for key := range otherClock.tracks {
		if clock.tracks[key] != nil {
			return false
		}
	}
	leftSequence := clock.sequence
	leftSlots := clock.retained()
	rightSlots := otherClock.retained()

	for index := range rightSlots {
		rightSlots[index].IngestSequence += leftSequence
	}

	if !clock.timeline.Merge(otherClock.timeline) {
		return false
	}

	for _, slot := range append(leftSlots, rightSlots...) {
		if !clock.timeline.Push(slot) {
			clock.err = errors.New("structure: clock timeline merge rewrite failed")
			return false
		}
	}

	for key, incoming := range otherClock.tracks {
		incoming.resequence(leftSequence)
		clock.tracks[key] = incoming
	}

	clock.sequence += otherClock.sequence
	clock.timelineSize += otherClock.timelineSize
	clock.rewritten = true

	if capacity := clock.timeline.Len(); clock.timelineSize > capacity {
		clock.timelineSize = capacity
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

	clock.access.Lock()
	defer clock.access.Unlock()

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

	clock.access.RLock()
	defer clock.access.RUnlock()

	clock.timeline.Do(visitor)
}

/*
Error returns the clock or underlying timeline error.
*/
func (clock *ClockRing[K, T]) Error() error {
	if clock == nil {
		return errClockNil
	}

	clock.access.RLock()
	defer clock.access.RUnlock()

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

	clock.access.Lock()
	defer clock.access.Unlock()

	for _, track := range clock.tracks {
		if err := track.values.Close(); err != nil {
			return err
		}
	}

	if !clock.timelineOpen {
		return nil
	}

	clock.timelineOpen = false

	return clock.timeline.Close()
}

var _ Ring[ClockSlot[string, float64]] = (*ClockRing[string, float64])(nil)
