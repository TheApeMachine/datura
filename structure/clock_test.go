package structure

import (
	"io"
	"testing"
	"time"

	"github.com/bytedance/sonic"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/datura"
)

func TestClockRing_ReadWrite(testingTB *testing.T) {
	Convey("Given a click clock with a bound artifact", testingTB, func() {
		clock := NewClockRing[float64](10, 100, 1000, datura.Acquire("clock", datura.Artifact_Type_json))

		source := datura.Acquire("clock", datura.Artifact_Type_json)
		So(source, ShouldNotBeNil)

		slot := ClockSlot[float64]{Payload: 1.5}
		payload, err := sonic.Marshal(slot)
		So(err, ShouldBeNil)
		source.WithPayload(payload)

		wire := source.Marshal()
		written, writeErr := clock.Write(wire)

		Convey("Write should store through the second hand", func() {
			So(writeErr, ShouldBeNil)
			So(written, ShouldEqual, len(wire))
		})

		buffer := make([]byte, 4096)
		readCount, readErr := clock.Read(buffer)

		Convey("Read should marshal through the second hand", func() {
			So(readErr, ShouldEqual, io.EOF)
			So(readCount, ShouldBeGreaterThan, 0)

			decoded := datura.Acquire("clock", datura.Artifact_Type_json)
			So(decoded.Unmarshal(buffer[:readCount]), ShouldNotBeNil)

			out, payloadErr := decoded.Payload()
			So(payloadErr, ShouldBeNil)
			So(string(out), ShouldContainSubstring, "1.5")
		})
	})
}

func TestClockRing_ObserveSecond(testingTB *testing.T) {
	Convey("Given a default click clock", testingTB, func() {
		clock := NewClockRing[float64](10, 100, 1000, datura.Acquire("clock", datura.Artifact_Type_json))
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

func TestClockRing_AdvanceVirtual(testingTB *testing.T) {
	Convey("Given one fresh second-hand observation", testingTB, func() {
		clock := NewClockRing[float64](10, 100, 1000, datura.Acquire("clock", datura.Artifact_Type_json))
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

func TestClockTrack_AdvanceVirtual(testingTB *testing.T) {
	Convey("Given a thin stream with one compression reading", testingTB, func() {
		clock := NewClockRing[float64](10, 100, 1000, datura.Acquire("clock", datura.Artifact_Type_json))

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

func TestClockRing_Ring(testingTB *testing.T) {
	Convey("Given a click clock as Ring[ClockSlot]", testingTB, func() {
		clock := NewClockRing[float64](10, 100, 1000, datura.Acquire("clock", datura.Artifact_Type_json))
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

func TestClockRing_Freshness(testingTB *testing.T) {
	Convey("Given a fresh little-hand observation", testingTB, func() {
		clock := NewClockRing[float64](10, 100, 1000, datura.Acquire("clock", datura.Artifact_Type_json))
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

func BenchmarkClockRing_ReadWrite(testingTB *testing.B) {
	clock := NewClockRing[float64](10, 100, 1000, datura.Acquire("clock", datura.Artifact_Type_json))

	source := datura.Acquire("clock", datura.Artifact_Type_json)

	if source == nil {
		testingTB.Fatal("Acquire returned nil")
	}

	payload, err := sonic.Marshal(1.5)

	if err != nil {
		testingTB.Fatal(err)
	}

	source.WithPayload(payload)
	wire := source.Marshal()
	buffer := make([]byte, 4096)

	testingTB.ReportAllocs()
	testingTB.ResetTimer()

	for testingTB.Loop() {
		if _, err := clock.Write(wire); err != nil {
			testingTB.Fatal(err)
		}

		if _, err := clock.Read(buffer); err != io.EOF && err != io.ErrShortBuffer {
			testingTB.Fatal(err)
		}
	}
}

func BenchmarkClockRing_AdvanceVirtual(testingTB *testing.B) {
	clock := NewClockRing[float64](10, 100, 1000, datura.Acquire("clock", datura.Artifact_Type_json))

	start := time.Date(2026, 6, 14, 12, 0, 0, 0, time.UTC)

	_, err := clock.ObserveSecond(start, 0)

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
	clock := NewClockRing[float64](10, 100, 1000, datura.Acquire("clock", datura.Artifact_Type_json))

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
