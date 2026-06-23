package transport

import (
	"bytes"
	"io"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestCopy(t *testing.T) {
	Convey("Given a reader and writer", t, func() {
		source := bytes.NewBuffer([]byte("copied payload"))
		destination := bytes.NewBuffer(nil)

		Convey("When copying between them", func() {
			written, err := Copy(destination, source)

			Convey("Then all bytes should transfer", func() {
				So(err, ShouldBeNil)
				So(written, ShouldEqual, int64(len("copied payload")))
				So(destination.String(), ShouldEqual, "copied payload")
			})
		})
	})

	Convey("Given an empty reader", t, func() {
		source := bytes.NewBuffer(nil)
		destination := bytes.NewBuffer(nil)

		written, err := Copy(destination, source)

		Convey("Then copy should report no output", func() {
			So(err, ShouldEqual, io.EOF)
			So(written, ShouldEqual, 0)
		})
	})
}

func BenchmarkCopy(b *testing.B) {
	payload := bytes.Repeat([]byte("x"), 4096)
	source := bytes.NewReader(payload)

	b.ResetTimer()

	for b.Loop() {
		destination := bytes.NewBuffer(make([]byte, 0, len(payload)))
		source.Reset(payload)

		if _, err := Copy(destination, source); err != nil && err != io.EOF {
			b.Fatal(err)
		}
	}
}
