package transport

import (
	"io"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestNewStream(t *testing.T) {
	Convey("Given a stage without hooks", t, func() {
		pipeline := newTestBuffer(nil)

		Convey("Then NewStream should return a buffered wrapper", func() {
			So(NewStream(pipeline), ShouldNotEqual, pipeline)
		})
	})
}

func TestStreamRead(t *testing.T) {
	Convey("Given a stream over a pipeline", t, func() {
		pipeline := newTestBuffer([]byte("stream data"))
		stream := NewStream(pipeline)

		buffer := make([]byte, 16)
		n, err := stream.Read(buffer)

		Convey("Then it should delegate to the pipeline", func() {
			So(err, ShouldBeNil)
			So(n, ShouldEqual, len("stream data"))
			So(string(buffer[:n]), ShouldEqual, "stream data")
		})
	})
}

func TestStreamWrite(t *testing.T) {
	Convey("Given a stream over a pipeline", t, func() {
		pipeline := newTestBuffer(nil)
		stream := NewStream(pipeline)

		payload := []byte("written")
		n, err := stream.Write(payload)

		Convey("Then it should delegate to the pipeline after flush", func() {
			So(err, ShouldBeNil)
			So(n, ShouldEqual, len(payload))
			So(stream.Flush(), ShouldBeNil)
			So(pipeline.String(), ShouldEqual, "written")
		})
	})
}

func TestStreamClose(t *testing.T) {
	Convey("Given a stream over a pipeline", t, func() {
		pipeline := newTestBuffer(nil)
		stream := NewStream(pipeline)

		Convey("When closing", func() {
			So(stream.Close(), ShouldBeNil)
		})
	})
}

func BenchmarkStreamRead(b *testing.B) {
	payload := []byte("stream benchmark payload")

	b.ResetTimer()

	for b.Loop() {
		pipeline := newTestBuffer(payload)
		stream := NewStream(pipeline)
		buffer := make([]byte, len(payload))

		if _, err := stream.Read(buffer); err != nil && err != io.EOF {
			b.Fatal(err)
		}
	}
}

func BenchmarkStreamWrite(b *testing.B) {
	payload := []byte("stream benchmark payload")

	b.ResetTimer()

	for b.Loop() {
		pipeline := newTestBuffer(nil)
		stream := NewStream(pipeline)

		if _, err := stream.Write(payload); err != nil {
			b.Fatal(err)
		}

		if err := stream.Flush(); err != nil {
			b.Fatal(err)
		}
	}
}
