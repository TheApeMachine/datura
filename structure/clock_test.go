package structure

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

func TestClockRing_ObserveSecond(t *testing.T) {
	Convey("Given a default click clock", t, func() {
		clock := NewClockRing[float64](10, 100, 1000)
		start := time.Date(2026, 6, 14, 12, 0, 0, 0, time.UTC)

		Convey("It should cascade to the little hand after one second lap", func() {
			var cascade ClockCascade

			for index := 0; index < 10; index++ {
				var tickErr error
				cascade, tickErr = clock.ObserveSecond(start.Add(time.Duration(index)*time.Second), 0)

				So(tickErr, ShouldBeNil)
			}

			So(cascade.Little, ShouldBeTrue)
			So(cascade.Big, ShouldBeFalse)
			So(clock.Click(), ShouldEqual, 10)
		})
	})
}

func TestClockRing_AdvanceVirtual(t *testing.T) {
	Convey("Given one fresh second-hand observation", t, func() {
		clock := NewClockRing[float64](10, 100, 1000)
		start := time.Date(2026, 6, 14, 12, 0, 0, 0, time.UTC)

		_, err := clock.ObserveSecond(start, 0)
		So(err, ShouldBeNil)

		cascades, advanceErr := clock.AdvanceVirtual(9)

		Convey("It should fill click space without wall timestamps", func() {
			So(advanceErr, ShouldBeNil)
			So(len(cascades), ShouldEqual, 9)

			freshness, freshErr := clock.Freshness(HandSecond)

			So(freshErr, ShouldBeNil)
			So(freshness, ShouldEqual, 9)
		})
	})
}

func TestClockTrack_AdvanceVirtual(t *testing.T) {
	Convey("Given a thin stream with one compression reading", t, func() {
		clock := NewClockRing[float64](10, 100, 1000)

		track, err := NewClockTrack(clock)

		So(err, ShouldBeNil)

		start := time.Date(2026, 6, 14, 12, 0, 0, 0, time.UTC)
		_, err = track.ObserveSecond(start, 0.2)

		So(err, ShouldBeNil)

		_, err = track.AdvanceVirtual(9)

		So(err, ShouldBeNil)

		secondRing, err := track.ValueRing(HandSecond)

		So(err, ShouldBeNil)

		values := make([]float64, 0, 10)

		secondRing.Do(func(value float64) {
			values = append(values, value)
		})

		Convey("It should hold the last value across virtual clicks", func() {
			So(len(values), ShouldEqual, 10)

			for _, value := range values {
				So(value, ShouldEqual, 0.2)
			}
		})
	})
}

func TestClockRing_Ring(t *testing.T) {
	Convey("Given a click clock as Ring[ClockSlot]", t, func() {
		clock := NewClockRing[float64](10, 100, 1000)
		ring := NewRing(clock)
		start := time.Date(2026, 6, 14, 12, 0, 0, 0, time.UTC)

		Convey("It should push and pop second-hand slots through the shared interface", func() {
			So(ring.Push(ClockSlot[float64]{Wall: start}), ShouldBeTrue)
			So(ring.Len(), ShouldEqual, 10)

			slot := ring.Pop()

			So(slot.Wall, ShouldEqual, start)
			So(clock.Click(), ShouldEqual, 1)
		})
	})
}

func TestClockRing_Freshness(t *testing.T) {
	Convey("Given a fresh little-hand observation", t, func() {
		clock := NewClockRing[float64](10, 100, 1000)
		start := time.Date(2026, 6, 14, 12, 0, 0, 0, time.UTC)

		_, err := clock.ObserveLittle(start)
		So(err, ShouldBeNil)

		freshness, freshErr := clock.Freshness(HandLittle)

		Convey("It should report zero freshness on the latest slot", func() {
			So(freshErr, ShouldBeNil)
			So(freshness, ShouldEqual, 0)
		})
	})
}

func BenchmarkClockRing_AdvanceVirtual(t *testing.B) {
	clock := NewClockRing[float64](10, 100, 1000)

	start := time.Date(2026, 6, 14, 12, 0, 0, 0, time.UTC)

	_, err := clock.ObserveSecond(start, 0)

	if err != nil {
		t.Fatal(err)
	}

	for t.Loop() {
		_, err = clock.AdvanceVirtual(9)

		if err != nil {
			t.Fatal(err)
		}
	}
}

func BenchmarkClockTrack_ObserveSecond(t *testing.B) {
	clock := NewClockRing[float64](10, 100, 1000)

	track, err := NewClockTrack(clock)

	if err != nil {
		t.Fatal(err)
	}

	start := time.Date(2026, 6, 14, 12, 0, 0, 0, time.UTC)

	for t.Loop() {
		_, err = track.ObserveSecond(start, 1.5)

		if err != nil {
			t.Fatal(err)
		}
	}
}
