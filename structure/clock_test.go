package structure

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

func TestClockRing_ObserveSecond(testingTB *testing.T) {
	Convey("Given a default click clock", testingTB, func() {
		clock, err := NewClockRing[float64](10, 100, 1000)

		So(err, ShouldBeNil)

		start := time.Date(2026, 6, 14, 12, 0, 0, 0, time.UTC)

		Convey("It should cascade to the little hand after one second lap", func() {
			var cascade ClockCascade

			for index := 0; index < 10; index++ {
				var tickErr error
				cascade, tickErr = clock.ObserveSecond(start.Add(time.Duration(index) * time.Second))

				So(tickErr, ShouldBeNil)
			}

			So(cascade.Little, ShouldBeTrue)
			So(cascade.Big, ShouldBeFalse)
			So(clock.Click(), ShouldEqual, 10)
		})
	})
}

func TestClockRing_AdvanceVirtual(testingTB *testing.T) {
	Convey("Given one fresh second-hand observation", testingTB, func() {
		clock, err := NewClockRing[float64](10, 100, 1000)

		So(err, ShouldBeNil)

		start := time.Date(2026, 6, 14, 12, 0, 0, 0, time.UTC)
		_, observeErr := clock.ObserveSecond(start)

		So(observeErr, ShouldBeNil)

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

func TestClockTrack_AdvanceVirtual(testingTB *testing.T) {
	Convey("Given a thin stream with one compression reading", testingTB, func() {
		clock, err := NewClockRing[float64](10, 100, 1000)

		So(err, ShouldBeNil)

		track, trackErr := NewClockTrack[float64](clock)

		So(trackErr, ShouldBeNil)

		start := time.Date(2026, 6, 14, 12, 0, 0, 0, time.UTC)
		_, observeErr := track.ObserveSecond(start, 0.2)

		So(observeErr, ShouldBeNil)

		_, advanceErr := track.AdvanceVirtual(9)

		So(advanceErr, ShouldBeNil)

		secondRing, ringErr := track.ValueRing(HandSecond)

		So(ringErr, ShouldBeNil)

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

func TestClockRing_Ring(testingTB *testing.T) {
	Convey("Given a click clock as Ring[ClockSlot]", testingTB, func() {
		clock, err := NewClockRing[float64](10, 100, 1000)

		So(err, ShouldBeNil)

		ring := NewRing(clock)
		start := time.Date(2026, 6, 14, 12, 0, 0, 0, time.UTC)

		Convey("It should push and pop second-hand slots through the shared interface", func() {
			So(ring.Push(ClockSlot{Wall: start}), ShouldBeTrue)
			So(ring.Len(), ShouldEqual, 10)

			slot := ring.Pop()

			So(slot.Wall, ShouldEqual, start)
			So(clock.Click(), ShouldEqual, 1)
		})
	})
}

func TestClockRing_Freshness(testingTB *testing.T) {
	Convey("Given a fresh little-hand observation", testingTB, func() {
		clock, err := NewClockRing[float64](10, 100, 1000)

		So(err, ShouldBeNil)

		start := time.Date(2026, 6, 14, 12, 0, 0, 0, time.UTC)
		_, observeErr := clock.ObserveLittle(start)

		So(observeErr, ShouldBeNil)

		freshness, freshErr := clock.Freshness(HandLittle)

		Convey("It should report zero freshness on the latest slot", func() {
			So(freshErr, ShouldBeNil)
			So(freshness, ShouldEqual, 0)
		})
	})
}

func BenchmarkClockRing_AdvanceVirtual(testingTB *testing.B) {
	clock, err := NewClockRing[float64](10, 100, 1000)

	if err != nil {
		testingTB.Fatal(err)
	}

	start := time.Date(2026, 6, 14, 12, 0, 0, 0, time.UTC)

	_, err = clock.ObserveSecond(start)

	if err != nil {
		testingTB.Fatal(err)
	}

	for testingTB.Loop() {
		_, err = clock.AdvanceVirtual(9)

		if err != nil {
			testingTB.Fatal(err)
		}
	}
}

func BenchmarkClockTrack_ObserveSecond(testingTB *testing.B) {
	clock, err := NewClockRing[float64](10, 100, 1000)

	if err != nil {
		testingTB.Fatal(err)
	}

	track, err := NewClockTrack[float64](clock)

	if err != nil {
		testingTB.Fatal(err)
	}

	start := time.Date(2026, 6, 14, 12, 0, 0, 0, time.UTC)

	for testingTB.Loop() {
		_, err = track.ObserveSecond(start, 1.5)

		if err != nil {
			testingTB.Fatal(err)
		}
	}
}
