package dmt

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestChildProbabilityIndex(t *testing.T) {
	Convey("Given many sibling branches under one prefix", t, func() {
		tree, err := NewTree("")
		So(err, ShouldBeNil)

		for index := 0; index < 40; index++ {
			path := sensoryStorageKey(append([]byte("blue_t"), byte('a'+index)))
			probability := float64(index+1) / 100.0

			_, _, _ = tree.Insert(path, MarshalCognitive(CognitiveState{
				Count:       1,
				Probability: probability,
			}))
		}

		Convey("When predicting next tokens from blue", func() {
			buffer := make([]LookaheadPrediction, 0, defaultChildIndexCapacity)
			predictions := tree.PredictNextSensoryTokens([]byte("blue"), buffer)

			Convey("Then it should return probability-ordered top branches", func() {
				So(len(predictions), ShouldEqual, defaultChildIndexCapacity)
				So(predictions[0].Probability, ShouldBeGreaterThan, predictions[len(predictions)-1].Probability)
			})
		})
	})
}

func TestEventTimeDecayMultiplier(t *testing.T) {
	Convey("Given elapsed event time", t, func() {
		stale := eventTimeDecayMultiplier(10, 110, 1, 100)
		recent := eventTimeDecayMultiplier(100, 110, 1, 100)

		Convey("Then stale mass should decay more than recent mass", func() {
			So(stale, ShouldBeLessThan, recent)
			So(stale, ShouldBeLessThan, 1)
		})
	})
}

func TestAnalogCandidateIndex(t *testing.T) {
	Convey("Given indexed structural siblings", t, func() {
		tree, err := NewTree("")
		So(err, ShouldBeNil)
		knownKey := []byte("blue_cab_big")
		unknownKey := []byte("blue_drone_rotor")

		_, _, _ = tree.Insert(knownKey, []byte("payload"))
		tree.registerAnalogCandidate(knownKey)

		Convey("When resolving a structural analog", func() {
			analog, found := tree.FindStructuralAnalog(unknownKey)

			Convey("Then it should use the indexed sibling", func() {
				So(found, ShouldBeTrue)
				So(string(analog.ClosestKey), ShouldEqual, string(knownKey))
			})
		})
	})
}

func BenchmarkReadChildProbabilityIndex(b *testing.B) {
	tree, err := NewTree("")
	if err != nil {
		b.Fatal(err)
	}

	_, _, _ = tree.InsertContextWeight([]byte("blue_cab_big"), PackedWeight{Count: 3, Probability: 0.6})
	_, _, _ = tree.InsertContextWeight([]byte("blue_cab_small"), PackedWeight{Count: 2, Probability: 0.4})
	tree.refreshChildProbabilityIndex([]byte("blue_cab"))
	prefix := []byte("blue_cab")
	buffer := make([]LookaheadPrediction, 0, defaultChildIndexCapacity)

	for b.Loop() {
		_, _ = tree.readChildProbabilityIndex(prefix, buffer[:0], defaultChildIndexCapacity)
	}
}
