package dmt

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestAnalyzeInterpolated(t *testing.T) {
	Convey("Given two classes separated by one continuation", t, func() {
		tree, err := NewTree("")
		So(err, ShouldBeNil)

		for range 10 {
			So(
				tree.TeachSequence([]byte("drive_ignition"), []byte("bullish")),
				ShouldBeNil,
			)
			So(
				tree.TeachSequence([]byte("drive_absorption"), []byte("bearish")),
				ShouldBeNil,
			)
		}

		Convey("When DMT analyzes the sequence in one operation", func() {
			analysis := tree.AnalyzeInterpolated([]byte("drive_ignition"))

			Convey("Then it returns the complete generative explanation", func() {
				So(string(analysis.Classification.Winner), ShouldEqual, "bullish")
				So(len(analysis.Surprisal), ShouldEqual, 2)
				So(analysis.AverageSurprisal, ShouldBeGreaterThanOrEqualTo, 0)
				So(len(analysis.Contributions), ShouldEqual, 2)
			})

			Convey("Then the distinguishing token carries different evidence", func() {
				shared := analysis.Contributions[0].Bits
				distinguishing := analysis.Contributions[1].Bits
				So(distinguishing, ShouldNotAlmostEqual, shared, 1e-9)
			})

			Convey("Then DMT owns reuse while its immutable root is unchanged", func() {
				again := tree.AnalyzeInterpolated([]byte("drive_ignition"))
				So(again, ShouldResemble, analysis)
				So(len(tree.CognitiveInference.analysisCache), ShouldEqual, 1)
			})

			Convey("Then a new root invalidates the prior derived reading", func() {
				previousRoot := tree.CognitiveInference.analysisRoot
				So(tree.TeachSequence(
					[]byte("drive_absorption"), []byte("bearish"),
				), ShouldBeNil)
				_ = tree.AnalyzeInterpolated([]byte("drive_ignition"))
				So(tree.CognitiveInference.analysisRoot, ShouldNotEqual, previousRoot)
				So(len(tree.CognitiveInference.analysisCache), ShouldEqual, 1)
			})
		})

		Convey("When only one class exists", func() {
			single, singleErr := NewTree("")
			So(singleErr, ShouldBeNil)
			So(single.TeachSequence([]byte("a_b"), []byte("only")), ShouldBeNil)

			Convey("Then there is nothing to contrast against", func() {
				analysis := single.AnalyzeInterpolated([]byte("a_b"))
				So(analysis.Contributions, ShouldBeNil)
			})
		})
	})
}

func TestInterpolatedSurprisal(t *testing.T) {
	Convey("Given a model with a learned transition", t, func() {
		tree, err := NewTree("")
		So(err, ShouldBeNil)

		for range 10 {
			tree.TrainSensorySequence([]byte("coil_ignition"))
		}

		Convey("When scoring a sequence the model has never seen", func() {
			novel := tree.InterpolatedSurprisal([]byte("coil_catastrophe"))
			familiar := tree.InterpolatedSurprisal([]byte("coil_ignition"))

			Convey("Then novelty is finite and more surprising", func() {
				So(len(novel), ShouldEqual, 2)

				for _, item := range novel {
					So(item.Surprisal, ShouldBeGreaterThanOrEqualTo, 0)
					So(item.Surprisal, ShouldBeLessThan, 20)
				}

				So(novel[1].Surprisal, ShouldBeGreaterThan, familiar[1].Surprisal)
			})
		})
	})
}

func BenchmarkAnalyzeInterpolated(b *testing.B) {
	tree, err := NewTree("")

	if err != nil {
		b.Fatal(err)
	}

	// The fixture uses several competing classes and repeated episodes so the
	// benchmark exercises class-conditioned basins and the episodic snapshot.
	for repetition := range 16 {
		for classIndex, class := range []string{
			"bullish", "bearish", "compression", "exhaustion",
		} {
			sequence := []byte("drive_phase_continuation_" + class)

			if err = tree.TeachSequence(sequence, []byte(class)); err != nil {
				b.Fatal(err)
			}

			_, _ = tree.CommitToEpisodicBuffer(
				uint64(repetition*4+classIndex+1), sequence,
			)
		}
	}

	b.ReportAllocs()
	b.ResetTimer()

	for range b.N {
		tree.AnalyzeInterpolated([]byte("drive_phase_continuation_bullish"))
	}
}
