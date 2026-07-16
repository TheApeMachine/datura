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
ClockOverrunError reports that a bounded timeline overwrote observations before
a consumer advanced. Path-dependent callers must reset or resynchronize rather
than silently measuring an incomplete event path.
*/
type ClockOverrunError struct {
	Track    any
	Expected uint64
	Oldest   uint64
}

/*
Error describes the exact missing ingress boundary so callers can make the
overrun visible and choose a domain-specific recovery policy.
*/
func (err ClockOverrunError) Error() string {
	if err.Track != nil {
		return fmt.Sprintf(
			"structure: clock track %v overrun at cut %d, oldest retained is %d",
			err.Track,
			err.Expected,
			err.Oldest,
		)
	}

	return fmt.Sprintf(
		"structure: clock cursor overrun: expected sequence %d, oldest retained is %d",
		err.Expected,
		err.Oldest,
	)
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
ingress cut. Event time chooses the state; ingress sequence prevents late
arrivals from entering a measurement cycle after that cycle was captured.
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

		if found {
			frame.Tracks[key] = slot
			continue
		}

		oldest, overrun := track.overrun(cut)

		if overrun {
			return ClockFrame[K, T]{}, ClockOverrunError{
				Track:    key,
				Expected: cut.Through,
				Oldest:   oldest,
			}
		}
	}

	return frame, nil
}

/*
EventsAfter returns every retained observation after cursor and through cut in
global ingress order. It advances only the returned cursor value; the clock's
bounded journal remains intact for other consumers.
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

	oldest := clock.sequence - uint64(clock.timelineSize) + 1
	expected := cursor.After + 1

	if clock.timelineSize > 0 && expected < oldest && cut.Through >= oldest {
		return nil, cursor, ClockOverrunError{
			Expected: expected,
			Oldest:   oldest,
		}
	}

	capacity := int(cut.Through - cursor.After)

	if capacity > clock.timelineSize {
		capacity = clock.timelineSize
	}

	events := make([]ClockSlot[K, T], 0, capacity)
	appendSlot := func(slot ClockSlot[K, T]) {
		if slot.IngestSequence <= cursor.After || slot.IngestSequence > cut.Through {
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
