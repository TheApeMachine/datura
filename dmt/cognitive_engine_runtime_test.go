package dmt

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestBuildContextTrainingMutations(t *testing.T) {
	Convey("Given context training mutations", t, func() {
		Convey("When a parent path is skipped as an empty token", func() {
			tree, err := NewTree("")
			So(err, ShouldBeNil)
			engine := NewCognitiveEngine(tree)

			Convey("It should reject division by an empty parent sample", func() {
				_, err := engine.buildContextTrainingMutations([]byte("a__b"))
				So(err, ShouldNotBeNil)
				So(err.Error(), ShouldContainSubstring, "positive parent sample count")
			})
		})

		Convey("When parent and child appear in one sequence", func() {
			tree, err := NewTree("")
			So(err, ShouldBeNil)
			engine := NewCognitiveEngine(tree)

			mutations, err := engine.buildContextTrainingMutations([]byte("alpha_beta"))

			Convey("It should derive child probability from positive parent mass", func() {
				So(err, ShouldBeNil)
				So(len(mutations), ShouldEqual, 2)

				parent := UnmarshalWeight(mutations[0].value)
				child := UnmarshalWeight(mutations[1].value)
				So(parent.Count, ShouldEqual, 1)
				So(child.Count, ShouldEqual, 1)
				So(child.Probability, ShouldEqual, 1.0)
			})
		})
	})
}
