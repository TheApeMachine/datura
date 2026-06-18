package datura

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestPeek(t *testing.T) {
	Convey("Given an artifact with attributes", t, func() {
		artifact := Acquire("test", Artifact_Type_json).
			WithAttributes(Map[any]{"test": "test"})
		So(artifact, ShouldNotBeNil)

		Convey("It should peek the attribute", func() {
			So(Peek[string](artifact, "test"), ShouldEqual, "test")
		})
	})
}

func TestPoke(t *testing.T) {
	Convey("Given an artifact with attributes", t, func() {
		artifact := Acquire("test", Artifact_Type_json).
			WithAttributes(Map[any]{"test": "test"})

		Convey("It should poke the attribute", func() {
			So(artifact.Poke("test", "test"), ShouldEqual, artifact)
			So(Peek[string](artifact, "test"), ShouldEqual, "test")

			Convey("When poking the attribute again", func() {
				So(artifact.Poke("toast", "test"), ShouldEqual, artifact)
				So(Peek[string](artifact, "test"), ShouldEqual, "toast")
			})
		})
	})
}
