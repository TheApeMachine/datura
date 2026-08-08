package dmt

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

/*
probabilityFor is a test reader for one token's share of a distribution.
*/
func probabilityFor(distribution []TokenProbability, token string) float64 {
	for _, entry := range distribution {
		if string(entry.Token) == token {
			return entry.Probability
		}
	}

	return 0
}

func tokensOf(values ...string) [][]byte {
	tokens := make([][]byte, 0, len(values))

	for _, value := range values {
		tokens = append(tokens, []byte(value))
	}

	return tokens
}

func TestInterpolationFallsBackToShorterContext(t *testing.T) {
	Convey("Given a model that has only ever seen a short transition", t, func() {
		tree, err := NewTree("")
		So(err, ShouldBeNil)

		for range 5 {
			tree.TrainSensorySequence([]byte("beta_gamma"))
		}

		Convey("When asked to continue a longer context it has never seen", func() {
			distribution := tree.InterpolatedProbabilities(
				tokensOf("alpha", "beta"), nil,
			)

			Convey("Then the known suffix still supplies a prediction", func() {
				So(len(distribution), ShouldBeGreaterThan, 0)
				So(probabilityFor(distribution, "gamma"), ShouldBeGreaterThan, 0)
			})
		})
	})
}

func TestSmoothingKeepsUnseenContinuationsPossible(t *testing.T) {
	Convey("Given two continuations of one prefix at very different counts", t, func() {
		tree, err := NewTree("")
		So(err, ShouldBeNil)

		for range 20 {
			tree.TrainSensorySequence([]byte("root_common"))
		}

		tree.TrainSensorySequence([]byte("root_rare"))

		distribution := tree.InterpolatedProbabilities(tokensOf("root"), nil)

		Convey("Then the common continuation dominates", func() {
			So(
				probabilityFor(distribution, "common"),
				ShouldBeGreaterThan,
				probabilityFor(distribution, "rare"),
			)
		})

		Convey("Then the rare continuation is improbable rather than impossible", func() {
			So(probabilityFor(distribution, "rare"), ShouldBeGreaterThan, 0)
		})
	})
}

func TestInterpolationSurvivesACorruptedToken(t *testing.T) {
	Convey("Given a learned transition", t, func() {
		tree, err := NewTree("")
		So(err, ShouldBeNil)

		for range 5 {
			tree.TrainSensorySequence([]byte("frenzy_exhaustion"))
		}

		Convey("When the context token arrives with one character wrong", func() {
			distribution := tree.InterpolatedProbabilities(
				tokensOf("frenzz"), nil,
			)

			Convey("Then the walk resolves onto the known token", func() {
				So(probabilityFor(distribution, "exhaustion"), ShouldBeGreaterThan, 0)
			})
		})

		Convey("When the context token is unrelated", func() {
			distribution := tree.InterpolatedProbabilities(
				tokensOf("completelyunrelated"), nil,
			)

			Convey("Then it does not silently resolve onto the known token", func() {
				So(probabilityFor(distribution, "exhaustion"), ShouldEqual, 0)
			})
		})
	})
}

func TestDistributionIsNormalised(t *testing.T) {
	Convey("Given several learned continuations", t, func() {
		tree, err := NewTree("")
		So(err, ShouldBeNil)

		for _, sequence := range []string{
			"root_one", "root_two", "root_three", "root_one",
		} {
			tree.TrainSensorySequence([]byte(sequence))
		}

		distribution := tree.InterpolatedProbabilities(tokensOf("root"), nil)

		Convey("Then the probabilities sum to one", func() {
			total := 0.0

			for _, entry := range distribution {
				total += entry.Probability
			}

			So(total, ShouldAlmostEqual, 1.0, 1e-9)
		})

		Convey("Then it is ordered by descending probability", func() {
			for index := 1; index < len(distribution); index++ {
				So(
					distribution[index-1].Probability,
					ShouldBeGreaterThanOrEqualTo,
					distribution[index].Probability,
				)
			}
		})
	})
}

func TestEmptyModelYieldsNoDistribution(t *testing.T) {
	Convey("Given a model that has learned nothing", t, func() {
		tree, err := NewTree("")
		So(err, ShouldBeNil)

		Convey("Then it proposes nothing rather than inventing a uniform", func() {
			So(
				len(tree.InterpolatedProbabilities(tokensOf("anything"), nil)),
				ShouldEqual,
				0,
			)
		})
	})
}

func TestEpisodicRecallSurfacesASingleObservation(t *testing.T) {
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

			Convey("Then it does not overrule the accumulated statistics", func() {
				So(
					probabilityFor(distribution, "ignition"),
					ShouldBeGreaterThan,
					probabilityFor(distribution, "vacuum"),
				)
			})

			Convey("Then recall never claims more than its capped share", func() {
				So(
					probabilityFor(distribution, "vacuum"),
					ShouldBeLessThanOrEqualTo,
					maximumEpisodicShare,
				)
			})
		})
	})
}

func TestEpisodicCoverageScalesWithMatchLength(t *testing.T) {
	Convey("Given episodes matching a context to different depths", t, func() {
		tree, err := NewTree("")
		So(err, ShouldBeNil)

		_, _ = tree.CommitToEpisodicBuffer(1, []byte("a_b_c_deep"))

		_, coverageDeep := tree.EpisodicProbabilities(tokensOf("a", "b", "c"))

		tree2, err := NewTree("")
		So(err, ShouldBeNil)

		_, _ = tree2.CommitToEpisodicBuffer(1, []byte("z_c_shallow"))

		_, coverageShallow := tree2.EpisodicProbabilities(tokensOf("a", "b", "c"))

		Convey("Then a fuller match carries more authority", func() {
			So(coverageDeep, ShouldBeGreaterThan, coverageShallow)
		})

		Convey("Then coverage never exceeds the cap", func() {
			So(coverageDeep, ShouldBeLessThanOrEqualTo, maximumEpisodicShare)
		})
	})
}

func TestClassConditioningSeparatesContinuations(t *testing.T) {
	Convey("Given two classes that continue the same prefix differently", t, func() {
		tree, err := NewTree("")
		So(err, ShouldBeNil)

		for range 6 {
			_, _ = tree.InsertAttractorBasin(
				[]byte("bullish"), []byte("drive_ignition"),
				CognitiveState{Count: 10, Probability: 0.9},
			)
			_, _ = tree.InsertAttractorBasin(
				[]byte("bearish"), []byte("drive_absorption"),
				CognitiveState{Count: 10, Probability: 0.9},
			)
			tree.TrainSensorySequence([]byte("drive_ignition"))
			tree.TrainSensorySequence([]byte("drive_absorption"))
		}

		Convey("Then each class predicts its own continuation", func() {
			bullish := tree.InterpolatedProbabilities(
				tokensOf("drive"), []byte("bullish"),
			)
			bearish := tree.InterpolatedProbabilities(
				tokensOf("drive"), []byte("bearish"),
			)

			So(
				probabilityFor(bullish, "ignition"),
				ShouldBeGreaterThan,
				probabilityFor(bullish, "absorption"),
			)
			So(
				probabilityFor(bearish, "absorption"),
				ShouldBeGreaterThan,
				probabilityFor(bearish, "ignition"),
			)
		})
	})
}
