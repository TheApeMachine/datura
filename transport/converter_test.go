package transport

import (
	"io"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/datura"
)

func TestNewConverter(t *testing.T) {
	Convey("Given a need for a new Converter", t, func() {
		Convey("When creating a new Converter", func() {
			conv := NewConverter()

			Convey("Then it should not be nil", func() {
				So(conv, ShouldNotBeNil)
			})

			Convey("Then it should have initialized buffers", func() {
				So(conv.buffer, ShouldNotBeNil)
				So(conv.out, ShouldNotBeNil)
			})
		})
	})
}

func TestConverterRead(t *testing.T) {
	Convey("Given a Converter instance", t, func() {
		conv := NewConverter()

		Convey("When reading from an empty converter", func() {
			buf := make([]byte, 10)
			n, err := conv.Read(buf)

			Convey("Then it should return 0 bytes", func() {
				So(n, ShouldEqual, 0)
			})

			Convey("Then it should return EOF as there's no data", func() {
				So(err, ShouldEqual, io.EOF)
			})
		})

		Convey("When reading after writing data", func() {
			// Create a test artifact with output metadata
			artifact := datura.Acquire(
				"converter-test", datura.Artifact_Type_json,
			).Poke("output", "test data")

			// Write the artifact
			data, err := artifact.Message().Marshal()
			So(err, ShouldBeNil)

			_, err = conv.Write(data)
			So(err, ShouldBeNil)

			// Read the processed data
			buf := make([]byte, 100)
			n, err := conv.Read(buf)

			Convey("Then it should read the correct data", func() {
				So(n, ShouldEqual, len("test data"))
				So(string(buf[:n]), ShouldEqual, "test data")
			})

			Convey("Then it should not return an error", func() {
				So(err, ShouldBeNil)
			})
		})
	})
}

func TestConverterWrite(t *testing.T) {
	Convey("Given a Converter instance", t, func() {
		conv := NewConverter()

		Convey("When writing valid artifact data", func() {
			artifact := datura.Acquire(
				"converter-test", datura.Artifact_Type_json,
			).Poke("output", "test data")
			data, err := artifact.Message().Marshal()
			So(err, ShouldBeNil)

			n, err := conv.Write(data)

			Convey("Then it should write all bytes", func() {
				So(n, ShouldEqual, len(data))
			})

			Convey("Then it should not return an error", func() {
				So(err, ShouldBeNil)
			})
		})

		Convey("When writing empty data", func() {
			n, err := conv.Write([]byte{})

			Convey("Then it should report zero bytes written", func() {
				So(n, ShouldEqual, 0)
			})

			Convey("Then it should return empty input error", func() {
				So(err.Error(), ShouldEqual, "empty input")
			})
		})

		Convey("When writing nil data", func() {
			n, err := conv.Write(nil)

			Convey("Then it should report zero bytes written", func() {
				So(n, ShouldEqual, 0)
			})

			Convey("Then it should return empty input error", func() {
				So(err.Error(), ShouldEqual, "empty input")
			})
		})
	})
}

func TestConverterClose(t *testing.T) {
	Convey("Given a Converter instance", t, func() {
		conv := NewConverter()

		Convey("When closing the converter", func() {
			err := conv.Close()

			Convey("Then it should not return an error", func() {
				So(err, ShouldBeNil)
			})
		})

		Convey("When using the converter after closing", func() {
			conv.Close()

			Convey("Then writing should still work", func() {
				artifact := datura.Acquire(
					"converter-test", datura.Artifact_Type_json,
				).Poke("output", "test data")
				data, marshalErr := artifact.Message().Marshal()
				So(marshalErr, ShouldBeNil)

				n, err := conv.Write(data)
				So(err, ShouldBeNil)
				So(n, ShouldEqual, len(data))
			})

			Convey("Then reading should return EOF when no data is available", func() {
				buf := make([]byte, 100)
				n, err := conv.Read(buf)
				So(err, ShouldEqual, io.EOF)
				So(n, ShouldEqual, 0)
			})
		})
	})
}

func BenchmarkConverterWrite(b *testing.B) {
	artifact := datura.Acquire(
		"converter-bench", datura.Artifact_Type_json,
	).Poke("output", "test data")

	data, err := artifact.Message().Marshal()

	if err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()

	for b.Loop() {
		conv := NewConverter()

		if _, writeErr := conv.Write(data); writeErr != nil {
			b.Fatal(writeErr)
		}
	}
}
