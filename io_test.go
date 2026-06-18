package datura

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestArtifactWriteSetMetaValue(t *testing.T) {
	Convey("Given a writable artifact round-trip", t, func() {
		source := Acquire("io-test", Artifact_Type_json).
			WithAttributes(Map[any]{"input": "seed"})
		So(source, ShouldNotBeNil)

		target := Acquire("io-test-target", Artifact_Type_json).
			WithAttributes(Map[any]{"output": "processed"})
		So(target, ShouldNotBeNil)

		So(Peek[string](source, "input"), ShouldEqual, "seed")
		So(Peek[string](target, "output"), ShouldEqual, "processed")
	})
}
