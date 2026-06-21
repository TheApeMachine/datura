package stream

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/datura"
)

func TestBufferWriteRead(t *testing.T) {
	Convey("Given a buffer stage", t, func() {
		source := datura.Acquire("buffer-test", datura.Artifact_Type_json).
			WithAttributes(datura.Map[any]{"input": "seed"})

		wire, err := source.MarshalPacked()
		So(err, ShouldBeNil)

		buffer := NewBuffer(func(processed *datura.Artifact) error {
			processed.WithAttributes(datura.Map[any]{"output": "processed"})
			return nil
		})

		_, err = buffer.Write(wire)
		So(err, ShouldBeNil)

		readBuf := make([]byte, 4096)
		readCount, readErr := buffer.Read(readBuf)
		So(readErr, ShouldBeNil)
		So(readCount, ShouldBeGreaterThan, 0)

		target := datura.Acquire("buffer-target", datura.Artifact_Type_json)
		_, err = target.Write(readBuf[:readCount])
		So(err, ShouldBeNil)
		So(datura.Peek[string](target, "output"), ShouldEqual, "processed")
	})
}
