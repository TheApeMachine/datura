package structure

import "time"

/*
ClockFrame is an event-time cut across the clock's populated item tracks. Every
entry is the newest retained observation at or before Wall.
*/
type ClockFrame[K comparable, T any] struct {
	Wall   time.Time
	Tracks map[K]ClockSlot[K, T]
}

/*
clockTrack owns one bounded item history through the shared Ring interface.
*/
type clockTrack[K comparable, T any] struct {
	values   Ring[ClockSlot[K, T]]
	count    int
	sequence uint64
	first    uint64
	latest   time.Time
}

/*
Register creates one empty item track. Repeated registration is idempotent.
*/
func (clock *ClockRing[K, T]) Register(key K) error {
	if clock == nil {
		return errClockNil
	}

	clock.access.Lock()
	defer clock.access.Unlock()

	return clock.register(key)
}

/*
register creates a track while the caller holds the clock lock so observation
and explicit registration share one allocation path.
*/
func (clock *ClockRing[K, T]) register(key K) error {

	if clock.tracks[key] != nil {
		return nil
	}

	values := clock.newTrack()

	if values == nil {
		return errClockTrack
	}

	clock.tracks[key] = &clockTrack[K, T]{values: values}

	return nil
}

/*
Frame returns the newest retained nonfuture observation from every item track.
Tracks without an eligible observation remain absent.
*/
func (clock *ClockRing[K, T]) Frame(
	wall time.Time,
) (ClockFrame[K, T], error) {
	if clock == nil {
		return ClockFrame[K, T]{}, errClockNil
	}

	if wall.IsZero() {
		return ClockFrame[K, T]{}, errClockTime
	}

	clock.access.RLock()
	defer clock.access.RUnlock()

	return clock.frame(ClockCut{
		Through: clock.sequence,
		At:      wall,
	})
}

/*
Aligned returns a frame at the minimum latest timestamp across populated tracks.
This is the greatest event-time cut every populated track can satisfy.
*/
func (clock *ClockRing[K, T]) Aligned() (ClockFrame[K, T], error) {
	if clock == nil {
		return ClockFrame[K, T]{}, errClockNil
	}

	clock.access.RLock()
	defer clock.access.RUnlock()

	watermark := time.Time{}

	for _, track := range clock.tracks {
		if track.count == 0 || (!watermark.IsZero() && !track.latest.Before(watermark)) {
			continue
		}

		watermark = track.latest
	}

	if watermark.IsZero() {
		return ClockFrame[K, T]{}, errClockEmpty
	}

	return clock.frame(ClockCut{
		Through: clock.sequence,
		At:      watermark,
	})
}

/*
Track returns the Ring that owns one item's bounded observation history.
*/
func (clock *ClockRing[K, T]) Track(key K) (Ring[ClockSlot[K, T]], bool) {
	if clock == nil {
		return nil, false
	}

	clock.access.RLock()
	defer clock.access.RUnlock()

	if clock.tracks[key] == nil {
		return nil, false
	}

	return clock.tracks[key].values, true
}

func (track *clockTrack[K, T]) at(cut ClockCut) (ClockSlot[K, T], bool) {
	selected := ClockSlot[K, T]{}
	found := false

	track.values.Do(func(slot ClockSlot[K, T]) {
		if slot.IngestSequence < track.first ||
			slot.IngestSequence > cut.Through || slot.Wall.After(cut.At) {
			return
		}

		if found && slot.Wall.Before(selected.Wall) {
			return
		}

		if found && slot.Wall.Equal(selected.Wall) &&
			slot.IngestSequence < selected.IngestSequence {
			return
		}

		selected = slot
		found = true
	})

	return selected, found
}

/*
overrun detects a captured ingress cut older than this track's retained history.
The track existed by the cut, but every state eligible by ingress was replaced.
*/
func (track *clockTrack[K, T]) overrun(cut ClockCut) (uint64, bool) {
	if track.count == 0 {
		return 0, false
	}

	oldest := uint64(0)
	track.values.Do(func(slot ClockSlot[K, T]) {
		if slot.IngestSequence < track.first {
			return
		}

		if oldest == 0 || slot.IngestSequence < oldest {
			oldest = slot.IngestSequence
		}
	})

	return oldest, track.first <= cut.Through && oldest > cut.Through
}

/*
resequence offsets retained ingress identities after two disjoint clocks merge,
while preserving track-local sequence and event-time order.
*/
func (track *clockTrack[K, T]) resequence(offset uint64) {
	slots := make([]ClockSlot[K, T], 0, track.count)

	for step := track.count; step >= 1; step-- {
		slot := track.values.Select(-step).Pop()
		slot.IngestSequence += offset
		slots = append(slots, slot)
	}

	for _, slot := range slots {
		track.values.Push(slot)
	}

	track.first += offset
}
