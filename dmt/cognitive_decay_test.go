package dmt

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestStampedWeightRoundTrip(t *testing.T) {
	Convey("Given a stamped weight", t, func() {
		encoded := MarshalStampedWeight(9, 0.75, 1234)

		Convey("Then the count and probability decode through the legacy reader", func() {
			decoded := UnmarshalCognitive(encoded)
			So(decoded.Count, ShouldEqual, 9)
			So(decoded.Probability, ShouldEqual, 0.75)
		})

		Convey("Then the write step decodes", func() {
			step, stamped := UnmarshalStampedStep(encoded)
			So(stamped, ShouldBeTrue)
			So(step, ShouldEqual, 1234)
		})
	})

	Convey("Given a legacy unstamped weight", t, func() {
		encoded := MarshalWeight(4, 0.5)

		Convey("Then it still decodes its counts", func() {
			decoded := UnmarshalCognitive(encoded)
			So(decoded.Count, ShouldEqual, 4)
			So(decoded.Probability, ShouldEqual, 0.5)
		})

		Convey("Then it reports no provenance rather than a false one", func() {
			_, stamped := UnmarshalStampedStep(encoded)
			So(stamped, ShouldBeFalse)
		})
	})
}

func TestEffectiveWeightDecaysWithExperience(t *testing.T) {
	Convey("Given a weight written at the current step", t, func() {
		tree, err := NewTree("")
		So(err, ShouldBeNil)

		_, _ = tree.InsertSensoryWeight([]byte("a"), CognitiveState{
			Count:       1000,
			Probability: 1.0,
		})

		Convey("When no further experience has accumulated", func() {
			state := tree.EffectiveSensoryWeight([]byte("a"))

			Convey("Then it reads at full strength", func() {
				So(state.Count, ShouldEqual, 1000)
				So(state.Probability, ShouldEqual, 1.0)
			})
		})

		Convey("When the model accumulates experience elsewhere", func() {
			for range 200 {
				tree.AdvanceCognitiveStep()
			}

			state := tree.EffectiveSensoryWeight([]byte("a"))

			Convey("Then the untouched weight reads as staler", func() {
				So(state.Count, ShouldBeLessThan, 1000)
				So(state.Probability, ShouldBeLessThan, 1.0)
			})

			Convey("Then the stored weight itself is unchanged", func() {
				So(tree.GetSensoryWeight([]byte("a")).Count, ShouldEqual, 1000)
			})

			Convey("Then decay is monotone in elapsed experience", func() {
				before := tree.EffectiveSensoryWeight([]byte("a")).Probability

				for range 200 {
					tree.AdvanceCognitiveStep()
				}

				So(
					tree.EffectiveSensoryWeight([]byte("a")).Probability,
					ShouldBeLessThan,
					before,
				)
			})
		})
	})
}

func TestTrainingAdvancesTheCognitiveClock(t *testing.T) {
	Convey("Given a fresh tree", t, func() {
		tree, err := NewTree("")
		So(err, ShouldBeNil)
		So(tree.CognitiveStep(), ShouldEqual, 0)

		Convey("When a sequence is trained", func() {
			tree.TrainSensorySequence([]byte("alpha_beta_gamma"))

			Convey("Then the clock advances once for the observation, not once per prefix", func() {
				So(tree.CognitiveStep(), ShouldEqual, 1)
			})
		})
	})
}

func TestRecentEvidenceOutranksStaleEvidence(t *testing.T) {
	Convey("Given an old pattern and a newer weaker one", t, func() {
		tree, err := NewTree("")
		So(err, ShouldBeNil)

		// The stale pattern is written heavily, then left alone while the model
		// accumulates a great deal of unrelated experience.
		_, _ = tree.InsertSensoryWeight([]byte("stale"), CognitiveState{
			Count:       500,
			Probability: 1.0,
		})

		for range 1000 {
			tree.AdvanceCognitiveStep()
		}

		_, _ = tree.InsertSensoryWeight([]byte("fresh"), CognitiveState{
			Count:       100,
			Probability: 1.0,
		})

		Convey("Then raw counts still favour the stale pattern", func() {
			So(
				tree.GetSensoryWeight([]byte("stale")).Count,
				ShouldBeGreaterThan,
				tree.GetSensoryWeight([]byte("fresh")).Count,
			)
		})

		Convey("Then effective counts favour the fresh one", func() {
			So(
				tree.EffectiveSensoryWeight([]byte("fresh")).Count,
				ShouldBeGreaterThan,
				tree.EffectiveSensoryWeight([]byte("stale")).Count,
			)
		})
	})
}
