package structure

import (
	"sync/atomic"
	"time"
)

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
Scalar fields are atomic because the single producer mutates them in push while
concurrent readers observe them in at, Aligned, and Frame.
*/
type clockTrack[K comparable, T any] struct {
	values      Ring[ClockSlot[K, T]]
	count       atomic.Int64
	sequence    atomic.Uint64
	first       atomic.Uint64
	latestUnix  atomic.Int64
	latestNanos atomic.Int64
}

/*
latestWall returns the track's newest observed wall time as one value.
*/
func (track *clockTrack[K, T]) latestWall() time.Time {
	seconds := track.latestUnix.Load()
	nanos := track.latestNanos.Load()

	if seconds == 0 && nanos == 0 {
		return time.Time{}
	}

	return time.Unix(seconds, nanos).UTC()
}

/*
storeLatest publishes a newer wall time for this track.
*/
func (track *clockTrack[K, T]) storeLatest(wall time.Time) {
	track.latestUnix.Store(wall.Unix())
	track.latestNanos.Store(int64(wall.Nanosecond()))
}

/*
Register creates one empty item track. Repeated registration is idempotent.
*/
func (clock *ClockRing[K, T]) Register(key K) error {
	if clock == nil {
		return errClockNil
	}

	_, err := clock.registerTrack(key)

	return err
}

/*
registerTrack returns the track for key, allocating it once on first appearance.
Concurrent producers cannot occur per the single-producer contract, but
LoadOrStore keeps the add-once semantics exact under sync.Map.
*/
func (clock *ClockRing[K, T]) registerTrack(key K) (*clockTrack[K, T], error) {
	if existing := clock.track(key); existing != nil {
		return existing, nil
	}

	values := clock.newTrack()

	if values == nil {
		return nil, errClockTrack
	}

	actual, _ := clock.tracks.LoadOrStore(key, &clockTrack[K, T]{values: values})

	return actual.(*clockTrack[K, T]), nil
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

	return clock.frame(ClockCut{
		Through: clock.sequence.Load(),
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

	watermark := time.Time{}

	clock.rangeTracks(func(_ K, track *clockTrack[K, T]) bool {
		latest := track.latestWall()

		if track.count.Load() == 0 ||
			(!watermark.IsZero() && !latest.Before(watermark)) {
			return true
		}

		watermark = latest
		return true
	})

	if watermark.IsZero() {
		return ClockFrame[K, T]{}, errClockEmpty
	}

	return clock.frame(ClockCut{
		Through: clock.sequence.Load(),
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

	track := clock.track(key)

	if track == nil {
		return nil, false
	}

	return track.values, true
}

func (track *clockTrack[K, T]) at(cut ClockCut) (ClockSlot[K, T], bool) {
	selected := ClockSlot[K, T]{}
	found := false
	first := track.first.Load()

	// Select(0).Do is a non-consuming, lock-free walk; plain Do consumes on the
	// FIFO rings and would drain the track.
	track.values.Select(0).Do(func(slot ClockSlot[K, T]) {
		if slot.IngestSequence < first ||
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
resequence offsets retained ingress identities after two disjoint clocks merge,
while preserving track-local sequence and event-time order.
*/
func (track *clockTrack[K, T]) resequence(offset uint64) {
	count := int(track.count.Load())
	slots := make([]ClockSlot[K, T], 0, count)

	for range count {
		slot, ok := track.values.Pop()

		if !ok {
			break
		}

		slot.IngestSequence += offset
		slots = append(slots, slot)
	}

	for _, slot := range slots {
		track.values.Push(slot)
	}

	track.first.Store(track.first.Load() + offset)
}
