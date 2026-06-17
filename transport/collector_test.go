package transport

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/datura"
)

func TestNewCollector(t *testing.T) {
	Convey("Given a new collector", t, func() {
		collector := NewCollector()

		Convey("Then it should initialize", func() {
			So(collector, ShouldNotBeNil)
			So(collector.rb, ShouldNotBeNil)
			So(collector.buf, ShouldNotBeNil)
		})
	})
}

func TestCollectorWrite(t *testing.T) {
	Convey("Given a collector", t, func() {
		collector := NewCollector()

		frame := datura.Acquire("collector-test", datura.Artifact_Type_json).
			Poke("role", "frame")

		wire, err := frame.Message().Marshal()
		So(err, ShouldBeNil)

		n, writeErr := collector.Write(wire)

		Convey("Then it should accept the frame", func() {
			So(writeErr, ShouldBeNil)
			So(n, ShouldEqual, len(wire))
			So(collector.Len(), ShouldEqual, 1)
		})
	})
}

func TestCollectorRead(t *testing.T) {
	Convey("Given a collector with piped data", t, func() {
		collector := NewCollector()

		payload := []byte("pipe payload")
		_, err := collector.pw.Write(payload)
		So(err, ShouldBeNil)

		buffer := make([]byte, len(payload))
		n, readErr := collector.Read(buffer)

		Convey("Then it should read from the pipe", func() {
			So(readErr, ShouldBeNil)
			So(n, ShouldEqual, len(payload))
			So(string(buffer), ShouldEqual, string(payload))
		})
	})
}

func TestCollectorClose(t *testing.T) {
	Convey("Given a collector", t, func() {
		collector := NewCollector()

		Convey("When closing", func() {
			So(collector.Close(), ShouldBeNil)
		})

		Convey("When writing after close", func() {
			collector.Close()

			_, err := collector.Write([]byte("late"))
			So(err, ShouldNotBeNil)
		})
	})
}

func TestCollectorError(t *testing.T) {
	Convey("Given a collector without error", t, func() {
		collector := NewCollector()

		Convey("Then Error should be empty", func() {
			So(collector.Error(), ShouldEqual, "")
		})
	})
}

func TestCollectorNext(t *testing.T) {
	Convey("Given a collector with a queued artifact", t, func() {
		collector := NewCollector()

		frame := datura.Acquire("collector-next", datura.Artifact_Type_json)
		wire, err := frame.Message().Marshal()
		So(err, ShouldBeNil)

		_, writeErr := collector.Write(wire)
		So(writeErr, ShouldBeNil)

		artifactCh := collector.Next(1)

		Convey("Then Next should expose the artifact channel", func() {
			So(artifactCh, ShouldNotBeNil)

			queued := <-artifactCh
			So(queued, ShouldNotBeNil)
		})
	})
}

func BenchmarkCollectorWrite(b *testing.B) {
	frame := datura.Acquire("collector-bench", datura.Artifact_Type_json).
		Poke("role", "frame")

	wire, err := frame.Message().Marshal()

	if err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()

	for b.Loop() {
		collector := NewCollector()

		if _, writeErr := collector.Write(wire); writeErr != nil {
			b.Fatal(writeErr)
		}

		collector.Close()
	}
}
