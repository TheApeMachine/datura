package dmt

import (
	"bytes"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

/*
strongestSelector is a deterministic stand-in for stochastic selection, so a
generation test asserts on the generator rather than on a sampler's luck.
*/
func strongestSelector(candidates []CandidateToken, _ float64) []byte {
	if len(candidates) == 0 {
		return nil
	}

	best := candidates[0]

	for _, candidate := range candidates[1:] {
		if candidate.Score > best.Score {
			best = candidate
		}
	}

	return best.Token
}

func TestGenerateSequenceFollowsLearnedStructure(t *testing.T) {
	Convey("Given a learned chain", t, func() {
		tree, err := NewTree("")
		So(err, ShouldBeNil)

		for range 10 {
			tree.TrainSensorySequence([]byte("coil_ignition_exhaustion"))
		}

		Convey("When generating from its opening token", func() {
			generated := tree.GenerateSequence(
				[]byte("coil"), nil, 0, 3, strongestSelector,
			)

			Convey("Then it produces the learned continuation", func() {
				So(len(generated), ShouldBeGreaterThan, 0)
				So(string(generated), ShouldContainSubstring, "ignition")
			})
		})
	})
}

func TestGenerationGuards(t *testing.T) {
	Convey("Given a model", t, func() {
		tree, err := NewTree("")
		So(err, ShouldBeNil)

		tree.TrainSensorySequence([]byte("a_b"))

		Convey("Then a non-positive token budget generates nothing", func() {
			So(tree.GenerateSequence([]byte("a"), nil, 0, 0, strongestSelector), ShouldBeNil)
		})

		Convey("Then a missing selector generates nothing", func() {
			So(tree.GenerateSequence([]byte("a"), nil, 0, 4, nil), ShouldBeNil)
		})

		Convey("Then a model with nothing to say generates nothing", func() {
			empty, emptyErr := NewTree("")
			So(emptyErr, ShouldBeNil)
			So(
				empty.GenerateSequence([]byte("a"), nil, 0, 4, strongestSelector),
				ShouldBeNil,
			)
		})
	})
}

func TestRepetitionPenaltyBreaksCycles(t *testing.T) {
	Convey("Given a model whose strongest continuation loops", t, func() {
		tree, err := NewTree("")
		So(err, ShouldBeNil)

		// A mutually reinforcing pair: without a penalty, always taking the
		// strongest continuation orbits between these two forever.
		for range 10 {
			tree.TrainSensorySequence([]byte("ping_pong"))
			tree.TrainSensorySequence([]byte("pong_ping"))
		}

		Convey("When generating a long sequence deterministically", func() {
			generated := tree.GenerateSequence(
				[]byte("ping"), nil, 0, 6, strongestSelector,
			)

			Convey("Then it does not emit the same token back to back", func() {
				tokens := splitUnderscoreTokens(generated)

				for index := 1; index < len(tokens); index++ {
					So(bytes.Equal(tokens[index], tokens[index-1]), ShouldBeFalse)
				}
			})
		})
	})
}

func TestDreamConsolidationGuards(t *testing.T) {
	Convey("Given a model", t, func() {
		tree, err := NewTree("")
		So(err, ShouldBeNil)

		var scratch ClassificationScratch

		Convey("Then a missing selector consolidates nothing", func() {
			So(
				tree.ExecuteDreamConsolidation(0.8, 4, &scratch, nil),
				ShouldBeNil,
			)
		})

		Convey("Then a model with no classes has nothing to dream from", func() {
			So(
				tree.ExecuteDreamConsolidation(0.8, 4, &scratch, strongestSelector),
				ShouldBeNil,
			)
		})
	})
}

func TestDreamConsolidationRejectsKnownSequences(t *testing.T) {
	Convey("Given a class with exactly one learned path", t, func() {
		tree, err := NewTree("")
		So(err, ShouldBeNil)

		var scratch ClassificationScratch

		for range 10 {
			_ = tree.TeachSequence([]byte("coil_ignition"), []byte("bullish"))
		}

		before := tree.EffectiveSensoryWeight([]byte("coil_ignition")).Count

		Convey("When consolidation regenerates that same path", func() {
			outcomes := tree.ExecuteDreamConsolidation(
				0, 2, &scratch, strongestSelector,
			)

			Convey("Then an already-known sequence is not consolidated", func() {
				for _, outcome := range outcomes {
					So(string(outcome.Sequence), ShouldNotEqual, "coil_ignition")
				}
			})

			Convey("Then lived experience is not re-counted", func() {
				So(
					tree.GetSensoryWeight([]byte("coil_ignition")).Count,
					ShouldBeLessThanOrEqualTo,
					before+1,
				)
			})
		})
	})
}

func TestREMSleepWithDreamingRunsReplayFirst(t *testing.T) {
	Convey("Given an episodic window awaiting replay", t, func() {
		tree, err := NewTree("")
		So(err, ShouldBeNil)

		var scratch ClassificationScratch

		for range 6 {
			_ = tree.TeachSequence([]byte("drive_ignition"), []byte("bullish"))
		}

		_, _ = tree.CommitToEpisodicBuffer(10, []byte("drive_ignition"))

		Convey("When consolidation runs", func() {
			outcomes, _ := tree.ExecuteREMSleepWithDreaming(
				1, 100, 0, 3, &scratch, strongestSelector,
			)

			Convey("Then it completes and reports whatever it invented", func() {
				for _, outcome := range outcomes {
					So(len(outcome.Sequence), ShouldBeGreaterThan, 0)
					So(outcome.Confidence, ShouldBeGreaterThanOrEqualTo, dreamNoveltyConfidence)
				}
			})

			Convey("Then the replayed path survived the decay pass", func() {
				So(
					tree.GetSensoryWeight([]byte("drive_ignition")).Count,
					ShouldBeGreaterThan,
					0,
				)
			})
		})
	})
}
