package structure

import (
	"fmt"
	"time"
)

/*
ClockCut freezes one clock at an ingress boundary and wall-time boundary so a
consumer can perform work without admitting observations that arrive later.
*/
type ClockCut struct {
	Through uint64
	At      time.Time
}

/*
ClockCursor records the last ingress sequence processed by one consumer. The
clock retains observations independently, so multiple consumers may advance
without destructively removing each other's input.
*/
type ClockCursor struct {
	After uint64
}

/*
Cut captures the clock's current ingress high-water mark at the caller's chosen
measurement time. The caller supplies time so replay and live operation share
the same deterministic boundary mechanism.
*/
func (clock *ClockRing[K, T]) Cut(at time.Time) (ClockCut, error) {
	if clock == nil {
		return ClockCut{}, errClockNil
	}

	if at.IsZero() {
		return ClockCut{}, errClockTime
	}

	if clock.err != nil {
		return ClockCut{}, clock.err
	}

	return ClockCut{
		Through: clock.sequence.Load(),
		At:      at,
	}, nil
}

/*
FrameThrough returns one newest eligible state per populated track at a stable
ingress cut. When a track has already cycled past that cut, it supplies its
newest retained state at the requested event time instead of disappearing.
*/
func (clock *ClockRing[K, T]) FrameThrough(
	cut ClockCut,
) (ClockFrame[K, T], error) {
	if clock == nil {
		return ClockFrame[K, T]{}, errClockNil
	}

	if cut.At.IsZero() {
		return ClockFrame[K, T]{}, errClockTime
	}

	return clock.frame(cut)
}

/*
frame builds an immutable state surface while the caller holds the clock read
lock, choosing the greatest event-time observation within the ingress cut.
*/
func (clock *ClockRing[K, T]) frame(
	cut ClockCut,
) (ClockFrame[K, T], error) {
	if clock.err != nil {
		return ClockFrame[K, T]{}, clock.err
	}

	sequence := clock.sequence.Load()

	if cut.Through > sequence {
		return ClockFrame[K, T]{}, fmt.Errorf(
			"structure: clock cut %d exceeds high-water %d",
			cut.Through,
			sequence,
		)
	}

	frame := ClockFrame[K, T]{
		Wall:   cut.At,
		Tracks: make(map[K]ClockSlot[K, T]),
	}

	clock.rangeTracks(func(key K, track *clockTrack[K, T]) bool {
		slot, found := track.at(cut)

		if !found && track.first.Load() <= cut.Through {
			slot, found = track.at(ClockCut{
				Through: sequence,
				At:      cut.At,
			})
		}

		if found {
			frame.Tracks[key] = slot
		}

		return true
	})

	return frame, nil
}

/*
EventsAfter returns the retained observations after cursor and through cut in
global ingress order. A cursor older than the retained window resumes at that
window's first slot because overwritten history is no longer actionable.
*/
func (clock *ClockRing[K, T]) EventsAfter(
	cursor ClockCursor,
	cut ClockCut,
) ([]ClockSlot[K, T], ClockCursor, error) {
	if clock == nil {
		return nil, cursor, errClockNil
	}

	if clock.err != nil {
		return nil, cursor, clock.err
	}

	sequence := clock.sequence.Load()

	if cut.Through > sequence {
		return nil, cursor, fmt.Errorf(
			"structure: clock cut %d exceeds high-water %d",
			cut.Through,
			sequence,
		)
	}

	if cursor.After >= cut.Through {
		return nil, cursor, nil
	}

	after := cursor.After
	timelineSize := clock.timelineSize.Load()

	if timelineSize > 0 {
		oldest := sequence - uint64(timelineSize) + 1

		if after < oldest-1 {
			after = oldest - 1
		}
	}

	capacity := 0

	if cut.Through > after {
		capacity = int(cut.Through - after)
	}

	if int64(capacity) > timelineSize {
		capacity = int(timelineSize)
	}

	events := make([]ClockSlot[K, T], 0, capacity)
	appendSlot := func(slot ClockSlot[K, T]) {
		if slot.IngestSequence <= after || slot.IngestSequence > cut.Through {
			return
		}

		events = append(events, slot)
	}

	if clock.rewritten.Load() {
		for _, slot := range clock.retained() {
			appendSlot(slot)
		}
	} else {
		// Select(0).Do is a non-consuming, lock-free walk of the live timeline;
		// plain Do consumes on the FIFO rings and would drain the clock.
		clock.timeline.Select(0).Do(appendSlot)
	}

	return events, ClockCursor{After: cut.Through}, nil
}

/*
retained returns the populated global timeline in ingress order while the
caller holds a clock lock, excluding unused ring capacity and stale rewrites.
*/
func (clock *ClockRing[K, T]) retained() []ClockSlot[K, T] {
	timelineSize := int(clock.timelineSize.Load())
	slots := make([]ClockSlot[K, T], 0, timelineSize)

	for step := timelineSize; step >= 1; step-- {
		slots = append(slots, clock.timeline.Select(-step).Pop())
	}

	return slots
}
