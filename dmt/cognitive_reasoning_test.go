package dmt

import (
	"fmt"
	"math"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestComputeContrastiveEvidence(t *testing.T) {
	Convey("Given competing context weights", t, func() {
		tree := NewTree("")

		winnerPath := sensoryStorageKey([]byte("Truck_blue_cab_big_wheel"))
		runnerPath := sensoryStorageKey([]byte("Car_blue_hood_spoiler"))

		_, _ = tree.Insert(winnerPath, MarshalCognitive(CognitiveState{
			Count:       10,
			Probability: 0.8,
		}))
		_, _ = tree.Insert(runnerPath, MarshalCognitive(CognitiveState{
			Count:       4,
			Probability: 0.2,
		}))

		Convey("When computing contrastive evidence", func() {
			evidence := tree.ComputeContrastiveEvidence(winnerPath, runnerPath)

			Convey("Then winner surprisal should be lower than runner-up surprisal", func() {
				So(evidence.WinnerBits, ShouldBeLessThan, evidence.RunnerUpBits)
				So(evidence.Divergence, ShouldBeGreaterThan, 0)
			})
		})
	})
}

func TestComputeBasinContrastiveEvidence(t *testing.T) {
	Convey("Given competing attractor basins", t, func() {
		tree := NewTree("")

		_, _ = tree.InsertAttractorBasin(
			[]byte("Concept_2"),
			[]byte("big_wheel"),
			CognitiveState{Count: 8, Probability: 0.74},
		)
		_, _ = tree.InsertAttractorBasin(
			[]byte("Car"),
			[]byte("big_wheel"),
			CognitiveState{Count: 2, Probability: 0.26},
		)

		Convey("When contrasting basin posteriors", func() {
			evidence := tree.ComputeBasinContrastiveEvidence(
				[]byte("Concept_2"),
				[]byte("Car"),
				[]byte("big_wheel"),
			)

			Convey("Then divergence should separate winner and runner-up evidence", func() {
				So(evidence.Divergence, ShouldBeGreaterThan, 0)
				So(evidence.WinnerBits, ShouldBeLessThan, evidence.RunnerUpBits)
			})
		})
	})
}

func TestExecuteDecayConsolidation(t *testing.T) {
	Convey("Given stale sensory weights", t, func() {
		tree := NewTree("")

		staleKey := sensoryStorageKey([]byte("obsolete_path"))
		activeKey := sensoryStorageKey([]byte("active_path"))

		_, _ = tree.Insert(staleKey, MarshalCognitive(CognitiveState{
			Count:       1,
			Probability: 0.01,
		}))
		_, _ = tree.Insert(activeKey, MarshalCognitive(CognitiveState{
			Count:       20,
			Probability: 0.9,
		}))

		Convey("When applying decay consolidation", func() {
			tree.ExecuteDecayConsolidation([]byte(sensoryNamespace), 0.5)

			_, staleFound := tree.Get(staleKey)
			activeState := tree.GetSensoryWeight([]byte("active_path"))

			Convey("Then stale branches should be pruned while active branches survive", func() {
				So(staleFound, ShouldBeFalse)
				So(activeState.Probability, ShouldEqual, 0.45)
			})
		})
	})
}

func TestCalculateBranchEntropy(t *testing.T) {
	Convey("Given flat and peaked branch distributions", t, func() {
		tree := NewTree("")

		_, _ = tree.InsertContextWeight([]byte("ctx/a"), PackedWeight{Count: 5, Probability: 0.5})
		_, _ = tree.InsertContextWeight([]byte("ctx/b"), PackedWeight{Count: 5, Probability: 0.5})

		peakedTree := NewTree("")

		_, _ = peakedTree.InsertContextWeight([]byte("ctx/a"), PackedWeight{Count: 9, Probability: 0.9})
		_, _ = peakedTree.InsertContextWeight([]byte("ctx/b"), PackedWeight{Count: 1, Probability: 0.1})

		Convey("When measuring branch entropy", func() {
			flatEntropy := tree.CalculateBranchEntropy([]byte("ctx"))
			peakedEntropy := peakedTree.CalculateBranchEntropy([]byte("ctx"))

			Convey("Then flat branches should exceed peaked branch entropy", func() {
				So(flatEntropy, ShouldBeGreaterThan, peakedEntropy)
			})
		})
	})
}

func TestMeasureBranchAmbiguity(t *testing.T) {
	Convey("Given a flat sensory branch split", t, func() {
		tree := NewTree("")

		_, _ = tree.InsertSensoryWeight([]byte("blue"), CognitiveState{Count: 5, Probability: 1.0})
		_, _ = tree.InsertSensoryWeight([]byte("blue_cab"), CognitiveState{Count: 3, Probability: 0.5})
		_, _ = tree.InsertSensoryWeight([]byte("blue_truck"), CognitiveState{Count: 3, Probability: 0.5})

		Convey("When evaluating ambiguity", func() {
			ambiguity := tree.MeasureBranchAmbiguity(sensoryStorageKey([]byte("blue")))

			Convey("Then the branch should register as ambiguous", func() {
				So(ambiguity.Ambiguous, ShouldBeTrue)
				So(ambiguity.EntropyBits, ShouldBeGreaterThanOrEqualTo, ambiguity.Threshold)
			})
		})
	})
}

func TestCompareSensoryBranches(t *testing.T) {
	Convey("Given two related sensory prefixes", t, func() {
		tree := NewTree("")

		leftPrefix := []byte("blue_cab_big_wheel")
		rightPrefix := []byte("blue_hood_spoiler")

		Convey("When comparing branch topology", func() {
			similarity := tree.CompareSensoryBranches(leftPrefix, rightPrefix)

			Convey("Then shared token structure should produce non-zero similarity", func() {
				So(similarity, ShouldEqual, 0.25)
			})
		})
	})
}

func TestFindStructuralAnalog(t *testing.T) {
	Convey("Given keys with shared structural prefixes", t, func() {
		tree := NewTree("")

		knownKey := []byte("blue_cab_big")
		unknownKey := []byte("blue_drone_rotor")

		_, _ = tree.Insert(knownKey, []byte("payload"))

		Convey("When searching for a structural analog", func() {
			analog, found := tree.FindStructuralAnalog(unknownKey)

			Convey("Then it should return the closest shared-prefix sibling", func() {
				So(found, ShouldBeTrue)
				So(string(analog.ClosestKey), ShouldEqual, string(knownKey))
				So(analog.Score, ShouldEqual, len("blue_"))
			})
		})
	})
}

func TestGetAnalogousFallback(t *testing.T) {
	Convey("Given a forest with a structural sibling", t, func() {
		forest, _ := NewForest(ForestConfig{})

		knownKey := []byte("blue_cab_big")
		unknownKey := []byte("blue_drone_rotor")

		forest.Insert(knownKey, []byte("fallback-payload"))

		Convey("When resolving an unknown key", func() {
			value, found := forest.GetAnalogousFallback(unknownKey)

			Convey("Then it should route through the closest analog", func() {
				So(found, ShouldBeTrue)
				So(string(value), ShouldEqual, "fallback-payload")
			})
		})
	})
}

func TestExecuteREMSleepConsolidationDecay(t *testing.T) {
	Convey("Given episodic replay with stale sensory clutter", t, func() {
		tree := NewTree("")

		_, _ = tree.Insert(sensoryStorageKey([]byte("stale")), MarshalCognitive(CognitiveState{
			Count:       1,
			Probability: 0.01,
		}))
		_, _ = tree.CommitToEpisodicBuffer(150, []byte("fresh_blue"))

		Convey("When running REM consolidation", func() {
			tree.ExecuteREMSleepConsolidation(100, 200)

			freshState := tree.GetSensoryWeight([]byte("fresh_blue"))
			_, staleFound := tree.Get(sensoryStorageKey([]byte("stale")))

			Convey("Then replay should train fresh paths and decay stale clutter", func() {
				So(freshState.Count, ShouldBeGreaterThan, 0)
				So(staleFound, ShouldBeFalse)
			})
		})
	})
}

func BenchmarkComputeContrastiveEvidence(b *testing.B) {
	tree := NewTree("")
	winnerPath := sensoryStorageKey([]byte("Truck_blue_cab_big_wheel"))
	runnerPath := sensoryStorageKey([]byte("Car_blue_hood_spoiler"))

	_, _ = tree.Insert(winnerPath, MarshalCognitive(CognitiveState{
		Count:       10,
		Probability: 0.8,
	}))
	_, _ = tree.Insert(runnerPath, MarshalCognitive(CognitiveState{
		Count:       4,
		Probability: 0.2,
	}))

	for b.Loop() {
		_ = tree.ComputeContrastiveEvidence(winnerPath, runnerPath)
	}
}

func BenchmarkCalculateBranchEntropy(b *testing.B) {
	tree := NewTree("")

	_, _ = tree.InsertSensoryWeight([]byte("blue"), CognitiveState{Count: 5, Probability: 1.0})
	_, _ = tree.InsertSensoryWeight([]byte("blue_cab"), CognitiveState{Count: 3, Probability: 0.5})
	_, _ = tree.InsertSensoryWeight([]byte("blue_truck"), CognitiveState{Count: 3, Probability: 0.5})

	prefix := sensoryStorageKey([]byte("blue"))

	for b.Loop() {
		_ = tree.CalculateBranchEntropy(prefix)
	}
}

func BenchmarkExecuteDecayConsolidation(b *testing.B) {
	tree := NewTree("")
	for index := 0; index < 64; index++ {
		path := sensoryStorageKey([]byte(fmt.Sprintf("path_%d", index)))
		probability := 0.9

		if index%8 == 0 {
			probability = 0.01
		}

		_, _ = tree.Insert(path, MarshalCognitive(CognitiveState{
			Count:       uint64(index + 1),
			Probability: probability,
		}))
	}

	for b.Loop() {
		tree.ExecuteDecayConsolidation([]byte(sensoryNamespace), 0.5)
	}
}

func BenchmarkFindStructuralAnalog(b *testing.B) {
	tree := NewTree("")

	for index := 0; index < 128; index++ {
		key := []byte(fmt.Sprintf("blue_path_%d", index))
		_, _ = tree.Insert(key, []byte("value"))
	}

	unknownKey := []byte("blue_drone_rotor")

	for b.Loop() {
		_, _ = tree.FindStructuralAnalog(unknownKey)
	}
}

func TestComputeContrastiveEvidenceZeroAlloc(t *testing.T) {
	Convey("Given packed contrastive paths", t, func() {
		tree := NewTree("")

		winnerPath := sensoryStorageKey([]byte("winner"))
		runnerPath := sensoryStorageKey([]byte("runner"))

		_, _ = tree.Insert(winnerPath, MarshalCognitive(CognitiveState{Count: 2, Probability: 0.7}))
		_, _ = tree.Insert(runnerPath, MarshalCognitive(CognitiveState{Count: 1, Probability: 0.3}))

		Convey("When computing contrastive evidence repeatedly", func() {
			allocs := testing.AllocsPerRun(100, func() {
				evidence := tree.ComputeContrastiveEvidence(winnerPath, runnerPath)

				if math.IsNaN(evidence.Divergence) {
					t.Fatal("divergence is NaN")
				}
			})

			Convey("Then it should not allocate on the heap", func() {
				So(allocs, ShouldEqual, 0)
			})
		})
	})
}
