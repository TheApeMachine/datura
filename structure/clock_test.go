package structure

import (
	"strconv"
	"sync"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

/*
TestClockRing_ConcurrentObserveAndRead reproduces the production access pattern:
one producer Observes across many symbols while several consumers concurrently
Cut, EventsAfter, Frame, and Aligned. Run under -race, it fails on any data race
in the clock's shared sequence, timeline size, track map, or track fields.
*/
func TestClockRing_ConcurrentObserveAndRead(t *testing.T) {
	clock := newTestClock[string, float64](4096, 64)
	start := time.Date(2026, 7, 16, 10, 0, 0, 0, time.UTC)

	const symbols = 64
	const observations = 4000
	const readers = 6

	var waitGroup sync.WaitGroup
	done := make(chan struct{})

	waitGroup.Add(1)
	go func() {
		defer waitGroup.Done()
		defer close(done)

		for index := range observations {
			symbol := "symbol-" + strconv.Itoa(index%symbols)
			wall := start.Add(time.Duration(index) * time.Millisecond)

			if err := clock.Observe(symbol, wall, float64(index)); err != nil {
				t.Error(err)
				return
			}
		}
	}()

	for range readers {
		waitGroup.Add(1)
		go func() {
			defer waitGroup.Done()

			cursor := ClockCursor{}
			at := start.Add(time.Duration(observations) * time.Millisecond)

			for {
				select {
				case <-done:
					return
				default:
				}

				cut, err := clock.Cut(at)

				if err != nil {
					t.Error(err)
					return
				}

				if _, next, err := clock.EventsAfter(cursor, cut); err == nil {
					cursor = next
				}

				_, _ = clock.Frame(at)
				_, _ = clock.Aligned()
			}
		}()
	}

	waitGroup.Wait()
}

func TestClockRing_Ring(t *testing.T) {
	Convey("Given a keyed clock constructed entirely from Ring implementations", t, func() {
		clock := newTestClock[string, float64](8, 4)
		var timeline Ring[ClockSlot[string, float64]] = clock
		start := time.Date(2026, 7, 16, 10, 0, 0, 0, time.UTC)

		pushed := timeline.Push(ClockSlot[string, float64]{
			Track: "BTC/USD", Wall: start, Payload: 100,
		})

		Convey("It should satisfy and operate through Ring directly", func() {
			So(pushed, ShouldBeTrue)
			slot, ok := timeline.Pop()
			So(ok, ShouldBeTrue)
			So(slot.Payload, ShouldEqual, 100)
			slot, ok = timeline.Select(-1).Pop()
			So(ok, ShouldBeTrue)
			So(slot.Track, ShouldEqual, "BTC/USD")
			track, found := clock.Track("BTC/USD")
			So(found, ShouldBeTrue)
			slot, ok = track.Select(-1).Pop()
			So(ok, ShouldBeTrue)
			So(slot.Payload, ShouldEqual, 100)
		})
	})
}

func TestClockRing_Observe(t *testing.T) {
	Convey("Given a bounded symbol track", t, func() {
		clock := newTestClock[string, float64](8, 2)
		start := time.Date(2026, 7, 16, 10, 0, 0, 0, time.UTC)

		for index := range 4 {
			So(clock.Observe(
				"BTC/USD", start.Add(time.Duration(index)*time.Second), float64(index),
			), ShouldBeNil)
		}

		Convey("It should overwrite only the oldest items on that track", func() {
			track, found := clock.Track("BTC/USD")
			So(found, ShouldBeTrue)
			slot, ok := track.Select(-2).Pop()
			So(ok, ShouldBeTrue)
			So(slot.Payload, ShouldEqual, 2)
			slot, ok = track.Select(-1).Pop()
			So(ok, ShouldBeTrue)
			So(slot.Payload, ShouldEqual, 3)
			slot, ok = track.Select(-1).Pop()
			So(ok, ShouldBeTrue)
			So(slot.Sequence, ShouldEqual, 4)
		})
	})
}

func TestClockRing_Frame(t *testing.T) {
	Convey("Given symbol tracks with different arrival frequencies", t, func() {
		clock := newTestClock[string, float64](16, 8)
		start := time.Date(2026, 7, 16, 10, 0, 0, 0, time.UTC)

		So(clock.Observe("BTC/USD", start, 100), ShouldBeNil)
		So(clock.Observe("BTC/USD", start.Add(time.Second), 101), ShouldBeNil)
		So(clock.Observe("BTC/USD", start.Add(2*time.Second), 102), ShouldBeNil)
		So(clock.Observe("ILLIQUID/USD", start.Add(500*time.Millisecond), 10), ShouldBeNil)

		frame, err := clock.Frame(start.Add(1500 * time.Millisecond))

		Convey("It should choose each track's newest nonfuture item", func() {
			So(err, ShouldBeNil)
			So(frame.Tracks, ShouldHaveLength, 2)
			So(frame.Tracks["BTC/USD"].Payload, ShouldEqual, 101)
			So(frame.Tracks["ILLIQUID/USD"].Payload, ShouldEqual, 10)
			So(frame.Tracks["ILLIQUID/USD"].Wall,
				ShouldEqual, start.Add(500*time.Millisecond))
		})
	})
}

func TestClockRing_FrameThrough(t *testing.T) {
	Convey("Given a cut captured before a late ticker arrives", t, func() {
		clock := newTestClock[string, float64](8, 4)
		start := time.Date(2026, 7, 16, 10, 0, 0, 0, time.UTC)
		So(clock.Observe("BTC/USD", start, 100), ShouldBeNil)
		cut, err := clock.Cut(start.Add(time.Second))
		So(err, ShouldBeNil)
		So(clock.Observe("BTC/USD", start.Add(500*time.Millisecond), 101), ShouldBeNil)

		frame, err := clock.FrameThrough(cut)

		Convey("It excludes the later ingress even when its event time is earlier than the cut", func() {
			So(err, ShouldBeNil)
			So(frame.Tracks["BTC/USD"].Payload, ShouldEqual, 100)
			So(frame.Tracks["BTC/USD"].IngestSequence, ShouldEqual, 1)
		})
	})

	Convey("Given out-of-order event times retained on one track", t, func() {
		clock := newTestClock[string, float64](8, 8)
		start := time.Date(2026, 7, 16, 10, 0, 0, 0, time.UTC)
		So(clock.Observe("BTC/USD", start.Add(2*time.Second), 102), ShouldBeNil)
		So(clock.Observe("BTC/USD", start.Add(time.Second), 101), ShouldBeNil)
		cut, err := clock.Cut(start.Add(3 * time.Second))
		So(err, ShouldBeNil)

		frame, err := clock.FrameThrough(cut)

		Convey("It selects the greatest eligible event time rather than latest arrival", func() {
			So(err, ShouldBeNil)
			So(frame.Tracks["BTC/USD"].Payload, ShouldEqual, 102)
		})
	})

	Convey("Given a captured state replaced by a hot symbol's bounded track", t, func() {
		clock := newTestClock[string, int](8, 2)
		start := time.Date(2026, 7, 16, 10, 0, 0, 0, time.UTC)
		So(clock.Observe("BTC/USD", start, 1), ShouldBeNil)
		cut, err := clock.Cut(start)
		So(err, ShouldBeNil)
		So(clock.Observe("BTC/USD", start, 2), ShouldBeNil)
		So(clock.Observe("BTC/USD", start, 3), ShouldBeNil)
		frame, err := clock.FrameThrough(cut)

		Convey("It resumes from state still present in the retained window", func() {
			So(err, ShouldBeNil)
			So(frame.Tracks["BTC/USD"].Payload, ShouldEqual, 3)
		})
	})
}

func TestClockRing_EventsAfter(t *testing.T) {
	Convey("Given a captured ingress range", t, func() {
		clock := newTestClock[string, int](4, 4)
		start := time.Date(2026, 7, 16, 10, 0, 0, 0, time.UTC)
		So(clock.Observe("BTC/USD", start, 1), ShouldBeNil)
		So(clock.Observe("ETH/USD", start, 2), ShouldBeNil)
		cut, err := clock.Cut(start)
		So(err, ShouldBeNil)
		So(clock.Observe("BTC/USD", start, 3), ShouldBeNil)

		events, next, err := clock.EventsAfter(ClockCursor{}, cut)

		Convey("It drains every event through the cut and returns a logical cursor", func() {
			So(err, ShouldBeNil)
			So(events, ShouldHaveLength, 2)
			So(events[0].Payload, ShouldEqual, 1)
			So(events[1].Payload, ShouldEqual, 2)
			So(next.After, ShouldEqual, 2)
		})
	})

	Convey("Given a consumer whose cursor precedes the retained window", t, func() {
		clock := newTestClock[string, int](2, 2)
		start := time.Date(2026, 7, 16, 10, 0, 0, 0, time.UTC)

		for index := range 3 {
			So(clock.Observe("BTC/USD", start, index), ShouldBeNil)
		}

		cut, err := clock.Cut(start)
		So(err, ShouldBeNil)
		events, next, err := clock.EventsAfter(ClockCursor{}, cut)

		Convey("It consumes the current window and advances from there", func() {
			So(err, ShouldBeNil)
			So(events, ShouldHaveLength, 2)
			So(events[0].Payload, ShouldEqual, 1)
			So(events[1].Payload, ShouldEqual, 2)
			So(next.After, ShouldEqual, 3)
		})
	})
}

func TestClockRing_Aligned(t *testing.T) {
	Convey("Given populated tracks with unequal latest event times", t, func() {
		clock := newTestClock[string, int](16, 8)
		start := time.Date(2026, 7, 16, 10, 0, 0, 0, time.UTC)

		So(clock.Observe("BTC/USD", start, 1), ShouldBeNil)
		So(clock.Observe("BTC/USD", start.Add(2*time.Second), 2), ShouldBeNil)
		So(clock.Observe("ILLIQUID/USD", start.Add(time.Second), 3), ShouldBeNil)

		frame, err := clock.Aligned()

		Convey("It should cut all tracks at the common watermark", func() {
			So(err, ShouldBeNil)
			So(frame.Wall, ShouldEqual, start.Add(time.Second))
			So(frame.Tracks["BTC/USD"].Payload, ShouldEqual, 1)
			So(frame.Tracks["ILLIQUID/USD"].Payload, ShouldEqual, 3)
		})
	})
}

func TestClockRing_Register(t *testing.T) {
	Convey("Given a universe-sized keyed clock", t, func() {
		clock := newTestClock[string, float64](1024, 16)

		for index := range 640 {
			So(clock.Register("symbol-"+strconv.Itoa(index)), ShouldBeNil)
		}

		Convey("It should allocate every track through the Ring factory", func() {
			trackCount := 0
			clock.tracks.Range(func(_, _ any) bool {
				trackCount++
				return true
			})
			So(trackCount, ShouldEqual, 640)
			track, found := clock.Track("symbol-639")
			So(found, ShouldBeTrue)
			So(track.Len(), ShouldEqual, 0)
		})
	})
}

func TestClockRing_Merge(t *testing.T) {
	Convey("Given clocks with disjoint item tracks", t, func() {
		left := newTestClock[string, float64](2, 2)
		right := newTestClock[string, float64](2, 2)
		start := time.Date(2026, 7, 16, 10, 0, 0, 0, time.UTC)
		So(left.Observe("BTC/USD", start, 100), ShouldBeNil)
		So(right.Observe("ETH/USD", start, 50), ShouldBeNil)

		Convey("It should merge through Ring without losing the keyed index", func() {
			So(left.Merge(right), ShouldBeTrue)
			So(left.Len(), ShouldEqual, 2)
			frame, err := left.Frame(start)
			So(err, ShouldBeNil)
			So(frame.Tracks, ShouldHaveLength, 2)
			cut, err := left.Cut(start)
			So(err, ShouldBeNil)
			events, _, err := left.EventsAfter(ClockCursor{}, cut)
			So(err, ShouldBeNil)
			So(events, ShouldHaveLength, 2)
			So(events[0].IngestSequence, ShouldEqual, 1)
			So(events[1].IngestSequence, ShouldEqual, 2)
		})
	})

	Convey("Given clocks with an overlapping item track", t, func() {
		left := newTestClock[string, float64](2, 2)
		right := newTestClock[string, float64](2, 2)
		start := time.Date(2026, 7, 16, 10, 0, 0, 0, time.UTC)
		So(left.Observe("BTC/USD", start, 100), ShouldBeNil)
		So(right.Observe("BTC/USD", start.Add(time.Second), 101), ShouldBeNil)

		Convey("It should reject a splice that would corrupt event-time order", func() {
			So(left.Merge(right), ShouldBeFalse)
			So(left.Len(), ShouldEqual, 1)
		})
	})
}

func BenchmarkClockRing_Observe640Tracks(b *testing.B) {
	clock := newTestClock[string, float64](8192, 256)
	tracks := make([]string, 640)

	for index := range tracks {
		tracks[index] = "symbol-" + strconv.Itoa(index)
	}

	start := time.Date(2026, 7, 16, 10, 0, 0, 0, time.UTC)
	b.ReportAllocs()
	b.ResetTimer()

	for index := range b.N {
		if err := clock.Observe(
			tracks[index%len(tracks)], start.Add(time.Duration(index)), float64(index),
		); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkClockRing_Frame640Tracks(b *testing.B) {
	clock := newTestClock[string, float64](1024, 256)
	start := time.Date(2026, 7, 16, 10, 0, 0, 0, time.UTC)

	for index := range 640 {
		if err := clock.Observe(
			"symbol-"+strconv.Itoa(index), start, float64(index),
		); err != nil {
			b.Fatal(err)
		}
	}

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if _, err := clock.Frame(start); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkClockRing_EventsAfter8192(b *testing.B) {
	start := time.Date(2026, 7, 16, 10, 0, 0, 0, time.UTC)
	clock := newTestClock[string, int](8192, 128)

	for index := range 8192 {
		if err := clock.Observe(strconv.Itoa(index%64), start, index); err != nil {
			b.Fatal(err)
		}
	}

	cut, err := clock.Cut(start)

	if err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if _, _, err := clock.EventsAfter(ClockCursor{}, cut); err != nil {
			b.Fatal(err)
		}
	}
}

func newTestClock[K comparable, T any](
	timelineCapacity int,
	trackCapacity int,
) *ClockRing[K, T] {
	timeline := NewSPSCRing[ClockSlot[K, T]](timelineCapacity, true)
	factory := func() Ring[ClockSlot[K, T]] {
		return NewSPSCRing[ClockSlot[K, T]](trackCapacity, true)
	}
	clock, err := NewClockRing(timeline, factory)

	if err != nil {
		panic(err)
	}

	return clock
}
