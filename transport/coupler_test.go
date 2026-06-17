package transport

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/datura"
)

func TestNewCoupler(t *testing.T) {
	Convey("Given a new coupler", t, func() {
		coupler := NewCoupler()

		Convey("Then it should start unconnected", func() {
			So(coupler, ShouldNotBeNil)
			So(coupler.origin, ShouldBeNil)
			So(coupler.destination, ShouldBeNil)
		})
	})
}

func TestCouplerConnect(t *testing.T) {
	Convey("Given a coupler and two buffers", t, func() {
		origin := newTestBuffer(nil)
		destination := newTestBuffer(nil)
		coupler := NewCoupler().
			Connect(origin).
			Connect(destination)

		Convey("Then both ends should be bound", func() {
			So(coupler.origin, ShouldEqual, origin)
			So(coupler.destination, ShouldEqual, destination)
		})
	})
}

func TestCouplerWrite(t *testing.T) {
	Convey("Given a coupler routing to destination for median frames", t, func() {
		origin := newTestBuffer(nil)
		destination := newTestBuffer(nil)
		coupler := NewCoupler().Connect(origin).Connect(destination)

		frame := datura.Acquire("coupler", datura.Artifact_Type_json).
			Poke("destination", "median")

		wire, err := frame.Message().Marshal()
		So(err, ShouldBeNil)

		n, writeErr := coupler.Write(wire)

		Convey("Then the frame should land on the destination", func() {
			So(writeErr, ShouldBeNil)
			So(n, ShouldEqual, len(wire))
			So(destination.Len(), ShouldEqual, len(wire))
			So(origin.Len(), ShouldEqual, 0)
		})
	})

	Convey("Given a coupler routing to origin for generic frames", t, func() {
		origin := newTestBuffer(nil)
		destination := newTestBuffer(nil)
		coupler := NewCoupler().Connect(origin).Connect(destination)

		frame := datura.Acquire("coupler", datura.Artifact_Type_json).
			Poke("role", "features")

		wire, err := frame.Message().Marshal()
		So(err, ShouldBeNil)

		n, writeErr := coupler.Write(wire)

		Convey("Then the frame should land on the origin", func() {
			So(writeErr, ShouldBeNil)
			So(n, ShouldEqual, len(wire))
			So(origin.Len(), ShouldEqual, len(wire))
			So(destination.Len(), ShouldEqual, 0)
		})
	})
}

func TestCouplerRead(t *testing.T) {
	Convey("Given a coupler with data on the origin", t, func() {
		origin := newTestBuffer([]byte("origin payload"))
		destination := newTestBuffer(nil)
		coupler := NewCoupler().Connect(origin).Connect(destination)

		buffer := make([]byte, 64)
		n, err := coupler.Read(buffer)

		Convey("Then it should copy origin to destination and read the copy", func() {
			So(err, ShouldBeNil)
			So(n, ShouldEqual, len("origin payload"))
			So(string(buffer[:n]), ShouldEqual, "origin payload")
			So(origin.Len(), ShouldEqual, 0)
		})
	})
}

func TestCouplerClose(t *testing.T) {
	Convey("Given a coupler", t, func() {
		coupler := NewCoupler()

		Convey("When closing", func() {
			So(coupler.Close(), ShouldBeNil)
		})
	})
}

func BenchmarkCouplerWrite(b *testing.B) {
	frame := datura.Acquire("coupler-bench", datura.Artifact_Type_json).
		Poke("destination", "median")

	wire, err := frame.Message().Marshal()

	if err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()

	for b.Loop() {
		origin := newTestBuffer(nil)
		destination := newTestBuffer(nil)
		coupler := NewCoupler().Connect(origin).Connect(destination)

		if _, writeErr := coupler.Write(wire); writeErr != nil {
			b.Fatal(writeErr)
		}
	}
}
