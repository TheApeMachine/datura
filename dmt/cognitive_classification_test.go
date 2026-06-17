package dmt

import (
	"errors"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestClassify(t *testing.T) {
	Convey("Given competing attractor basins", t, func() {
		tree := NewTree("")

		sequence := []byte("the_blue")

		_, _ = tree.InsertAttractorBasin(
			[]byte("Concept_3"),
			[]byte("the_blue"),
			CognitiveState{Count: 12, Probability: 0.737},
		)
		_, _ = tree.InsertAttractorBasin(
			[]byte("Truck"),
			[]byte("the"),
			CognitiveState{Count: 2, Probability: 0.058},
		)
		_, _ = tree.InsertAttractorBasin(
			[]byte("Car"),
			[]byte("the_blue"),
			CognitiveState{Count: 4, Probability: 0.12},
		)

		var scratch ClassificationScratch

		Convey("When classifying the sensory sequence", func() {
			result := tree.Classify(sequence, &scratch)

			Convey("Then Concept_3 should dominate the posterior matrix", func() {
				So(len(result.Scores), ShouldEqual, 3)
				So(string(result.Winner), ShouldEqual, "Concept_3")
				So(result.Highest, ShouldBeGreaterThan, result.Scores[1].Value)
			})
		})
	})
}

func TestUnsupervisedLearn(t *testing.T) {
	Convey("Given a trained attractor basin", t, func() {
		tree := NewTree("")

		sequence := []byte("the_blue")

		_, _ = tree.InsertAttractorBasin(
			[]byte("Concept_3"),
			[]byte("the"),
			CognitiveState{Count: 4, Probability: 0.6},
		)

		var scratch ClassificationScratch

		Convey("When running unsupervised learning", func() {
			inferredClass, confidence, learnErr := tree.UnsupervisedLearn(sequence, &scratch)

			Convey("Then it should infer the dominant class and strengthen basin weights", func() {
				So(learnErr, ShouldBeNil)
				So(string(inferredClass), ShouldEqual, "Concept_3")
				So(confidence, ShouldBeGreaterThan, 0)

				basinState := tree.GetAttractorBasin([]byte("Concept_3"), []byte("the"))
				sensoryState := tree.GetSensoryWeight([]byte("the"))

				So(basinState.Count, ShouldBeGreaterThan, 4)
				So(sensoryState.Count, ShouldBeGreaterThan, 0)
			})
		})
	})
}

func TestUnsupervisedLearnNoAttractor(t *testing.T) {
	Convey("Given an empty attractor landscape", t, func() {
		tree := NewTree("")

		var scratch ClassificationScratch

		Convey("When learning without matching basins", func() {
			_, _, learnErr := tree.UnsupervisedLearn([]byte("novel"), &scratch)

			Convey("Then it should return a no-match error", func() {
				So(errors.Is(learnErr, ErrNoAttractorMatch), ShouldBeTrue)
			})
		})
	})
}

func TestClassifyZeroAlloc(t *testing.T) {
	Convey("Given packed basin weights", t, func() {
		tree := NewTree("")

		_, _ = tree.InsertAttractorBasin(
			[]byte("Concept_3"),
			[]byte("the_blue"),
			CognitiveState{Count: 12, Probability: 0.737},
		)
		_, _ = tree.InsertAttractorBasin(
			[]byte("Truck"),
			[]byte("the"),
			CognitiveState{Count: 2, Probability: 0.058},
		)

		var scratch ClassificationScratch

		Convey("When classifying repeatedly", func() {
			allocs := testing.AllocsPerRun(100, func() {
				_ = tree.Classify([]byte("the_blue"), &scratch)
			})

			Convey("Then it should avoid heap churn beyond radix iterator internals", func() {
				So(allocs, ShouldBeLessThanOrEqualTo, 2)
			})
		})
	})
}

func BenchmarkClassify(b *testing.B) {
	tree := NewTree("")

	_, _ = tree.InsertAttractorBasin(
		[]byte("Concept_3"),
		[]byte("the_blue"),
		CognitiveState{Count: 12, Probability: 0.737},
	)
	_, _ = tree.InsertAttractorBasin(
		[]byte("Truck"),
		[]byte("the"),
		CognitiveState{Count: 2, Probability: 0.058},
	)

	var scratch ClassificationScratch
	sequence := []byte("the_blue")

	for b.Loop() {
		_ = tree.Classify(sequence, &scratch)
	}
}

func BenchmarkUnsupervisedLearn(b *testing.B) {
	tree := NewTree("")

	_, _ = tree.InsertAttractorBasin(
		[]byte("Concept_3"),
		[]byte("the"),
		CognitiveState{Count: 4, Probability: 0.6},
	)

	var scratch ClassificationScratch
	sequence := []byte("the_blue")

	for b.Loop() {
		_, _, _ = tree.UnsupervisedLearn(sequence, &scratch)
	}
}
