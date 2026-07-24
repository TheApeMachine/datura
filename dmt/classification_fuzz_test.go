package dmt

import (
	"math"
	"testing"
)

/*
FuzzClassificationMassPreservation proves softmax posteriors sum to one.
*/
func FuzzClassificationMassPreservation(f *testing.F) {
	f.Add([]byte("the_blue"), uint64(12), 0.737, uint64(4), 0.12)
	f.Add([]byte("alpha_beta"), uint64(3), 0.5, uint64(2), 0.25)

	f.Fuzz(func(
		t *testing.T,
		sequence []byte,
		leftCount uint64,
		leftProbability float64,
		rightCount uint64,
		rightProbability float64,
	) {
		if len(sequence) == 0 || leftCount == 0 || rightCount == 0 {
			t.Skip()
		}

		if math.IsNaN(leftProbability) || math.IsInf(leftProbability, 0) ||
			math.IsNaN(rightProbability) || math.IsInf(rightProbability, 0) {
			t.Skip()
		}

		if leftProbability < 0 || leftProbability > 1 || rightProbability < 0 || rightProbability > 1 {
			t.Skip()
		}

		tree, err := NewTree("")
		if err != nil {
			t.Fatalf("NewTree: %v", err)
		}

		_, _, _ = tree.InsertAttractorBasin(
			[]byte("Left"),
			sequence,
			CognitiveState{Count: leftCount, Probability: leftProbability},
		)
		_, _, _ = tree.InsertAttractorBasin(
			[]byte("Right"),
			sequence,
			CognitiveState{Count: rightCount, Probability: rightProbability},
		)

		var scratch ClassificationScratch
		result, classifyErr := tree.Classify(sequence, &scratch)

		if classifyErr != nil {
			t.Fatalf("Classify: %v", classifyErr)
		}

		if len(result.Scores) == 0 {
			t.Fatal("expected posterior scores")
		}

		mass := 0.0

		for _, score := range result.Scores {
			if score.Value < 0 || math.IsNaN(score.Value) {
				t.Fatalf("invalid posterior mass: %v", score.Value)
			}

			mass += score.Value
		}

		if math.Abs(mass-1.0) > 1e-9 {
			t.Fatalf("posterior mass = %v, want 1", mass)
		}
	})
}
