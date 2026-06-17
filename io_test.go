package datura

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestArtifactWriteSetMetaValue(t *testing.T) {
	Convey("Given a writable artifact round-trip", t, func() {
		source := Acquire("io-test", Artifact_Type_json).Poke("input", "seed")
		wire, err := source.Message().Marshal()
		So(err, ShouldBeNil)

		target := Acquire("io-test-target", Artifact_Type_json)
		_, err = target.Write(wire)
		So(err, ShouldBeNil)

		err = target.SetMetaValue("output", "processed")
		So(err, ShouldBeNil)
		So(Peek[string](target, "output"), ShouldEqual, "processed")
	})
}
