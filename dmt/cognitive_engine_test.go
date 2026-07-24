package dmt

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestSensoryNamespace(t *testing.T) {
	Convey("Given sensory storage keys", t, func() {
		tree, err := NewTree("")
		So(err, ShouldBeNil)
		_, _, _ = tree.InsertSensoryWeight([]byte("blue_cab"), CognitiveState{
			Count:       5,
			Probability: 0.5,
		})

		Convey("When reading sensory weights", func() {
			state := tree.GetSensoryWeight([]byte("blue_cab"))

			Convey("Then it should resolve under s/ namespace", func() {
				So(state.Count, ShouldEqual, 5)
				So(state.Probability, ShouldEqual, 0.5)
			})
		})
	})
}

func TestAttractorBasin(t *testing.T) {
	Convey("Given attractor basin entries", t, func() {
		tree, err := NewTree("")
		So(err, ShouldBeNil)
		_, _, _ = tree.InsertAttractorBasin(
			[]byte("Concept_2"),
			[]byte("big_cab_test"),
			CognitiveState{Count: 3, Probability: 0.8},
		)

		Convey("When reading basin posteriors", func() {
			state := tree.GetAttractorBasin([]byte("Concept_2"), []byte("big_cab_test"))

			Convey("Then it should resolve under b/ namespace", func() {
				So(state.Count, ShouldEqual, 3)
				So(state.Probability, ShouldEqual, 0.8)
			})
		})
	})
}

func TestSelectStochasticToken(t *testing.T) {
	Convey("Given candidate tokens", t, func() {
		candidates := []CandidateToken{
			{Token: []byte("cab"), Score: 0.2},
			{Token: []byte("truck"), Score: 0.9},
		}

		Convey("When temperature is zero", func() {
			selected := SelectStochasticToken(candidates, 0, nil)

			Convey("Then it should pick the highest score deterministically", func() {
				So(string(selected), ShouldEqual, "truck")
			})
		})
	})
}

func TestTrainSensorySequenceProbability(t *testing.T) {
	Convey("Given a sensory sequence observed once", t, func() {
		tree, err := NewTree("")
		So(err, ShouldBeNil)
		tree.TrainSensorySequence([]byte("blue_cab"))

		Convey("When reading child probability", func() {
			parent := tree.GetSensoryWeight([]byte("blue"))
			child := tree.GetSensoryWeight([]byte("blue_cab"))

			Convey("Then it should be child count over parent count", func() {
				So(parent.Count, ShouldEqual, uint64(1))
				So(child.Count, ShouldEqual, uint64(1))
				So(child.Probability, ShouldEqual, 1.0)
			})
		})
	})
}

func TestExecuteBeamSearch(t *testing.T) {
	Convey("Given trained sensory branches", t, func() {
		tree, err := NewTree("")
		So(err, ShouldBeNil)
		_, _, _ = tree.InsertSensoryWeight([]byte("blue"), CognitiveState{Count: 10, Probability: 1.0})
		_, _, _ = tree.InsertSensoryWeight([]byte("blue_cab"), CognitiveState{Count: 6, Probability: 0.75})
		_, _, _ = tree.InsertSensoryWeight([]byte("blue_cab_big"), CognitiveState{Count: 4, Probability: 0.8})
		_, _, _ = tree.InsertSensoryWeight([]byte("blue_truck"), CognitiveState{Count: 2, Probability: 0.2})

		scratch := &BeamSearchScratch{
			NextBeams:    make([]BeamPath, 0, 4),
			CurrentBeams: make([]BeamPath, 0, 4),
			LookupBuffer: make([]LookaheadPrediction, 0, 8),
		}

		Convey("When executing beam search", func() {
			paths := tree.ExecuteBeamSearch([]byte("blue"), 2, 2, scratch)

			Convey("Then it should rank multi-hop sensory paths", func() {
				So(len(paths), ShouldBeGreaterThan, 0)
				So(string(paths[0].Sequence), ShouldEqual, "blue_cab_big")

				if len(paths) > 1 {
					So(paths[0].Score, ShouldBeGreaterThanOrEqualTo, paths[len(paths)-1].Score)
				}
			})
		})
	})
}

func TestEpisodicREMConsolidation(t *testing.T) {
	Convey("Given episodic observations", t, func() {
		tree, err := NewTree("")
		So(err, ShouldBeNil)
		_, _, _ = tree.CommitToEpisodicBuffer(100, []byte("blue_cab_big"))
		_, _, _ = tree.CommitToEpisodicBuffer(150, []byte("blue_cab_big"))
		_, _, _ = tree.CommitToEpisodicBuffer(200, []byte("blue_cab_big"))
		_, _, _ = tree.CommitToEpisodicBuffer(250, []byte("blue_cab_big"))

		Convey("When running REM consolidation", func() {
			err := tree.ExecuteREMSleepConsolidation(100, 200)

			Convey("Then sensory weights should be trained once and the interval consumed", func() {
				rootState := tree.GetSensoryWeight([]byte("blue"))
				leafState := tree.GetSensoryWeight([]byte("blue_cab_big"))

				So(err, ShouldBeNil)
				So(rootState.Count, ShouldEqual, 3)
				So(leafState.Count, ShouldEqual, 3)
				So(leafState.Probability, ShouldBeGreaterThan, 0)
				So(tree.ExecuteREMSleepConsolidation(100, 200), ShouldBeNil)
				So(tree.GetSensoryWeight([]byte("blue")).Count, ShouldEqual, 3)
				_, replayedStartFound := tree.Get(episodicStorageKey(100, []byte("blue_cab_big")))
				_, replayedMiddleFound := tree.Get(episodicStorageKey(150, []byte("blue_cab_big")))
				_, replayedEndFound := tree.Get(episodicStorageKey(200, []byte("blue_cab_big")))
				_, futureFound := tree.Get(episodicStorageKey(250, []byte("blue_cab_big")))
				So(replayedStartFound, ShouldBeFalse)
				So(replayedMiddleFound, ShouldBeFalse)
				So(replayedEndFound, ShouldBeFalse)
				So(futureFound, ShouldBeTrue)
			})
		})
	})
}

func TestEpisodicREMConsolidationPersistence(t *testing.T) {
	Convey("Given a durable episodic observation", t, func() {
		persistDir := t.TempDir()
		sequence := []byte("blue_cab_big")
		tree, err := NewTree(persistDir)
		So(err, ShouldBeNil)
		_, _, err = tree.CommitToEpisodicBuffer(100, sequence)
		So(err, ShouldBeNil)

		Convey("When REM consumes it and the tree is reopened", func() {
			So(tree.ExecuteREMSleepConsolidation(100, 100), ShouldBeNil)
			So(tree.Close(), ShouldBeNil)

			reopened, err := NewTree(persistDir)

			So(err, ShouldBeNil)
			_, found := reopened.Get(episodicStorageKey(100, sequence))

			Convey("Then the consumed episode should stay absent", func() {
				So(found, ShouldBeFalse)
				So(reopened.Close(), ShouldBeNil)
			})
		})
	})
}

func BenchmarkExecuteREMSleepConsolidation(b *testing.B) {
	tree, err := NewTree("")
	if err != nil {
		b.Fatal(err)
	}
	sequence := []byte("symbol-btc-usd_pressure-positive_stress-negative")
	timestamp := uint64(0)

	for b.Loop() {
		timestamp++
		_, _, err := tree.CommitToEpisodicBuffer(timestamp, sequence)

		if err != nil {
			b.Fatal(err)
		}

		if err := tree.ExecuteREMSleepConsolidation(timestamp, timestamp); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkExecuteBeamSearch(b *testing.B) {
	tree, err := NewTree("")
	if err != nil {
		b.Fatal(err)
	}
	_, _, _ = tree.InsertSensoryWeight([]byte("blue"), CognitiveState{Count: 10, Probability: 1.0})
	_, _, _ = tree.InsertSensoryWeight([]byte("blue_cab"), CognitiveState{Count: 6, Probability: 0.75})
	_, _, _ = tree.InsertSensoryWeight([]byte("blue_cab_big"), CognitiveState{Count: 4, Probability: 0.8})
	_, _, _ = tree.InsertSensoryWeight([]byte("blue_truck"), CognitiveState{Count: 2, Probability: 0.2})

	scratch := &BeamSearchScratch{
		NextBeams:    make([]BeamPath, 0, 8),
		CurrentBeams: make([]BeamPath, 0, 8),
		LookupBuffer: make([]LookaheadPrediction, 0, 16),
	}

	for b.Loop() {
		_ = tree.ExecuteBeamSearch([]byte("blue"), 4, 3, scratch)
	}
}

func BenchmarkTrainSensorySequence(b *testing.B) {
	tree, err := NewTree("")
	if err != nil {
		b.Fatal(err)
	}
	sequence := []byte("blue_cab_big")

	for b.Loop() {
		tree.TrainSensorySequence(sequence)
	}
}
