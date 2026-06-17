package dmt

import (
	"math"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestMarshalWeight(t *testing.T) {
	Convey("Given a packed weight", t, func() {
		encoded := MarshalWeight(42, 0.25)
		decoded := UnmarshalWeight(encoded)

		Convey("Then it should round-trip without reflection", func() {
			So(decoded.Count, ShouldEqual, 42)
			So(decoded.Probability, ShouldEqual, 0.25)
		})
	})
}

func TestGetContextWeight(t *testing.T) {
	Convey("Given a tree with context weights", t, func() {
		tree := NewTree("")

		_, _ = tree.InsertContextWeight([]byte("blue_cab"), PackedWeight{
			Count:       8,
			Probability: 0.5,
		})

		Convey("When fetching a stored context", func() {
			weight := tree.GetContextWeight([]byte("blue_cab"))

			Convey("Then it should return the packed telemetry", func() {
				So(weight.Count, ShouldEqual, 8)
				So(weight.Probability, ShouldEqual, 0.5)
			})
		})

		Convey("When fetching a missing context", func() {
			weight := tree.GetContextWeight([]byte("missing"))

			Convey("Then it should return zero weight", func() {
				So(weight.Count, ShouldEqual, 0)
				So(weight.Probability, ShouldEqual, 0)
			})
		})
	})
}

func TestGetSurprisal(t *testing.T) {
	Convey("Given a trained context tree", t, func() {
		tree := NewTree("")

		_, _ = tree.InsertContextWeight([]byte("blue"), PackedWeight{Count: 10, Probability: 1.0})
		_, _ = tree.InsertContextWeight([]byte("blue_cab"), PackedWeight{Count: 4, Probability: 0.5})
		_, _ = tree.InsertContextWeight([]byte("blue_cab_big"), PackedWeight{Count: 2, Probability: 0.25})

		Convey("When scoring a known sequence", func() {
			items := tree.GetSurprisal([]byte("blue_cab_big"))

			Convey("Then it should emit surprisal per token boundary", func() {
				So(len(items), ShouldEqual, 3)
				So(string(items[0].Token), ShouldEqual, "blue")
				So(items[0].Surprisal, ShouldAlmostEqual, 0.0, 0.0001)
				So(string(items[1].Token), ShouldEqual, "cab")
				So(items[1].Surprisal, ShouldAlmostEqual, 1.0, 0.0001)
				So(string(items[2].Token), ShouldEqual, "big")
				So(items[2].Surprisal, ShouldAlmostEqual, 2.0, 0.0001)
			})
		})

		Convey("When scoring a novel suffix", func() {
			items := tree.GetSurprisal([]byte("blue_cab_small"))

			Convey("Then novel token surprisal should derive from parent count", func() {
				So(len(items), ShouldEqual, 3)
				expected := -math.Log2(1.0 / float64(4+1))
				So(items[2].Surprisal, ShouldAlmostEqual, expected, 0.0001)
			})
		})
	})
}

func TestPredictNextTokens(t *testing.T) {
	Convey("Given prefix branch weights", t, func() {
		tree := NewTree("")

		_, _ = tree.InsertContextWeight([]byte("blue_cab_big"), PackedWeight{Count: 3, Probability: 0.6})
		_, _ = tree.InsertContextWeight([]byte("blue_cab_small"), PackedWeight{Count: 2, Probability: 0.4})
		_, _ = tree.InsertContextWeight([]byte("blue_truck"), PackedWeight{Count: 1, Probability: 0.1})

		buffer := make([]LookaheadPrediction, 0, 4)

		Convey("When predicting from blue", func() {
			predictions := tree.PredictNextTokens([]byte("blue"), buffer)

			Convey("Then it should return immediate child tokens only", func() {
				So(len(predictions), ShouldEqual, 2)
				So(string(predictions[0].Token), ShouldEqual, "cab")
				So(predictions[0].Probability, ShouldEqual, 0.6)
				So(string(predictions[1].Token), ShouldEqual, "truck")
			})
		})

		Convey("When predicting from blue_cab", func() {
			predictions := tree.PredictNextTokens([]byte("blue_cab"), buffer[:0])

			Convey("Then it should surface the next token suffixes", func() {
				So(len(predictions), ShouldEqual, 2)
				So(string(predictions[0].Token), ShouldEqual, "big")
				So(string(predictions[1].Token), ShouldEqual, "small")
			})
		})
	})
}

func BenchmarkGetContextWeight(b *testing.B) {
	tree := NewTree("")
	_, _ = tree.InsertContextWeight([]byte("blue_cab_big"), PackedWeight{Count: 4, Probability: 0.5})
	lookupKey := []byte("blue_cab_big")

	for b.Loop() {
		_ = tree.GetContextWeight(lookupKey)
	}
}

func BenchmarkGetSurprisal(b *testing.B) {
	tree := NewTree("")
	_, _ = tree.InsertContextWeight([]byte("blue"), PackedWeight{Count: 10, Probability: 1.0})
	_, _ = tree.InsertContextWeight([]byte("blue_cab"), PackedWeight{Count: 4, Probability: 0.5})
	_, _ = tree.InsertContextWeight([]byte("blue_cab_big"), PackedWeight{Count: 2, Probability: 0.25})
	sequence := []byte("blue_cab_big")

	for b.Loop() {
		_ = tree.GetSurprisal(sequence)
	}
}

func BenchmarkPredictNextTokens(b *testing.B) {
	tree := NewTree("")

	_, _ = tree.InsertContextWeight([]byte("blue_cab_big"), PackedWeight{Count: 3, Probability: 0.6})
	_, _ = tree.InsertContextWeight([]byte("blue_cab_small"), PackedWeight{Count: 2, Probability: 0.4})
	prefix := []byte("blue_cab")
	buffer := make([]LookaheadPrediction, 0, 8)

	for b.Loop() {
		buffer = tree.PredictNextTokens(prefix, buffer[:0])
	}
}
