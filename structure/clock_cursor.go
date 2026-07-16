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

	clock.access.RLock()
	defer clock.access.RUnlock()

	if clock.err != nil {
		return ClockCut{}, clock.err
	}

	return ClockCut{
		Through: clock.sequence,
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

	clock.access.RLock()
	defer clock.access.RUnlock()

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

	if cut.Through > clock.sequence {
		return ClockFrame[K, T]{}, fmt.Errorf(
			"structure: clock cut %d exceeds high-water %d",
			cut.Through,
			clock.sequence,
		)
	}

	frame := ClockFrame[K, T]{
		Wall:   cut.At,
		Tracks: make(map[K]ClockSlot[K, T], len(clock.tracks)),
	}

	for key, track := range clock.tracks {
		slot, found := track.at(cut)

		if !found && track.first <= cut.Through {
			slot, found = track.at(ClockCut{
				Through: clock.sequence,
				At:      cut.At,
			})
		}

		if found {
			frame.Tracks[key] = slot
		}
	}

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

	clock.access.RLock()
	defer clock.access.RUnlock()

	if clock.err != nil {
		return nil, cursor, clock.err
	}

	if cut.Through > clock.sequence {
		return nil, cursor, fmt.Errorf(
			"structure: clock cut %d exceeds high-water %d",
			cut.Through,
			clock.sequence,
		)
	}

	if cursor.After >= cut.Through {
		return nil, cursor, nil
	}

	after := cursor.After

	if clock.timelineSize > 0 {
		oldest := clock.sequence - uint64(clock.timelineSize) + 1

		if after < oldest-1 {
			after = oldest - 1
		}
	}

	capacity := 0

	if cut.Through > after {
		capacity = int(cut.Through - after)
	}

	if capacity > clock.timelineSize {
		capacity = clock.timelineSize
	}

	events := make([]ClockSlot[K, T], 0, capacity)
	appendSlot := func(slot ClockSlot[K, T]) {
		if slot.IngestSequence <= after || slot.IngestSequence > cut.Through {
			return
		}

		events = append(events, slot)
	}

	if clock.rewritten {
		for _, slot := range clock.retained() {
			appendSlot(slot)
		}
	} else {
		clock.timeline.Do(appendSlot)
	}

	return events, ClockCursor{After: cut.Through}, nil
}

/*
retained returns the populated global timeline in ingress order while the
caller holds a clock lock, excluding unused ring capacity and stale rewrites.
*/
func (clock *ClockRing[K, T]) retained() []ClockSlot[K, T] {
	slots := make([]ClockSlot[K, T], 0, clock.timelineSize)

	for step := clock.timelineSize; step >= 1; step-- {
		slots = append(slots, clock.timeline.Select(-step).Pop())
	}

	return slots
}
