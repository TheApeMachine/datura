package dmt

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestEpisodicProbabilities(t *testing.T) {
	Convey("Given a strong statistical continuation", t, func() {
		tree, err := NewTree("")
		So(err, ShouldBeNil)

		for range 30 {
			tree.TrainSensorySequence([]byte("coil_ignition"))
		}

		baseline := tree.InterpolatedProbabilities(tokensOf("coil"), nil)
		So(probabilityFor(baseline, "vacuum"), ShouldEqual, 0)

		Convey("When one contradicting episode is committed", func() {
			_, _ = tree.CommitToEpisodicBuffer(1, []byte("coil_vacuum"))
			distribution := tree.InterpolatedProbabilities(tokensOf("coil"), nil)

			Convey("Then the once-seen continuation becomes possible", func() {
				So(probabilityFor(distribution, "vacuum"), ShouldBeGreaterThan, 0)
			})

			Convey("Then it does not overrule accumulated statistics", func() {
				So(
					probabilityFor(distribution, "ignition"),
					ShouldBeGreaterThan,
					probabilityFor(distribution, "vacuum"),
				)
			})

			Convey("Then recall never claims more than its capped share", func() {
				So(
					probabilityFor(distribution, "vacuum"),
					ShouldAlmostEqual,
					maximumEpisodicShare,
				)
			})
		})
	})

	Convey("Given episodes matching a context to different depths", t, func() {
		deepTree, err := NewTree("")
		So(err, ShouldBeNil)
		_, _ = deepTree.CommitToEpisodicBuffer(1, []byte("a_b_c_deep"))
		_, deepCoverage := deepTree.EpisodicProbabilities(tokensOf("a", "b", "c"))

		shallowTree, err := NewTree("")
		So(err, ShouldBeNil)
		_, _ = shallowTree.CommitToEpisodicBuffer(1, []byte("z_c_shallow"))
		_, shallowCoverage := shallowTree.EpisodicProbabilities(tokensOf("a", "b", "c"))

		Convey("Then a fuller match carries more authority", func() {
			So(deepCoverage, ShouldBeGreaterThan, shallowCoverage)
		})

		Convey("Then coverage never exceeds the cap", func() {
			So(deepCoverage, ShouldBeLessThanOrEqualTo, maximumEpisodicShare)
		})
	})
}
