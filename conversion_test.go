package datura

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestClone(t *testing.T) {
	Convey("Given an artifact with attributes and payload", t, func() {
		source := Acquire("clone-source", APPJSON).
			WithAttributes(Map[any]{
				"outputs": []string{"ignition", "compression"},
				"ignition": Map[any]{
					"leftKey":  "rvol",
					"rightKey": "precursor",
				},
			}).
			WithPayload([]byte(`{"source":true}`))

		clone, err := source.Clone()

		So(err, ShouldBeNil)
		So(clone, ShouldNotBeNil)

		Convey("It should preserve typed access to cloned attributes", func() {
			So(Peek[[]string](clone, "outputs"), ShouldResemble, []string{"ignition", "compression"})
			So(Peek[string](clone, "ignition", "leftKey"), ShouldEqual, "rvol")
			So(Peek[string](clone, "ignition", "rightKey"), ShouldEqual, "precursor")
		})

		Convey("It should not share mutable attributes or payload with the source", func() {
			clone.Poke([]string{"trend"}, "outputs")
			clone.Poke([]float64{1}, "output", "scaleSamples", "rvol")
			clone.WithPayload([]byte(`{"clone":true}`))

			So(Peek[[]string](source, "outputs"), ShouldResemble, []string{"ignition", "compression"})
			So(Peek[[]string](clone, "outputs"), ShouldResemble, []string{"trend"})
			So(Peek[string](clone, "ignition", "leftKey"), ShouldEqual, "rvol")
			So(Peek[string](clone, "ignition", "rightKey"), ShouldEqual, "precursor")
			So(Peek[bool](source, "clone"), ShouldBeFalse)
			So(Peek[bool](clone, "clone"), ShouldBeTrue)
		})
	})
}

func TestAs(t *testing.T) {
	Convey("Given an artifact with malformed payload", t, func() {
		artifact := Acquire("decode", APPJSON).WithPayload([]byte(`{`))

		Convey("When converting into a typed value", func() {
			value, err := As[map[string]any](artifact)

			Convey("Then the decode error should be returned", func() {
				So(err, ShouldNotBeNil)
				So(value, ShouldBeNil)
			})
		})
	})
}
