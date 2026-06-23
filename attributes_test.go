package datura

import (
	"math"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestPeek(t *testing.T) {
	Convey("Given attributes on an artifact", t, func() {
		artifact := Acquire("peek-test", Artifact_Type_json).
			WithAttributes(Map[any]{"output": "processed"})

		Convey("It should read a top-level key", func() {
			So(Peek[string](artifact, "output"), ShouldEqual, "processed")
		})

		Convey("It should return the type zero for missing keys", func() {
			So(Peek[string](artifact, "missing"), ShouldEqual, "")
		})
	})

	Convey("Given an artifact with only an encrypted payload", t, func() {
		envelope := Acquire("peek-payload", Artifact_Type_json).
			WithPayload([]byte(`{"method":"add_order","params":{"symbol":"BTC/USD"}}`))

		Convey("It should read nested payload paths", func() {
			So(Peek[string](envelope, "method"), ShouldEqual, "add_order")
			So(Peek[string](envelope, "params", "symbol"), ShouldEqual, "BTC/USD")
		})
	})

	Convey("Given a Kraken heartbeat frame without a type field", t, func() {
		envelope := Acquire("kraken:public", Artifact_Type_json).
			WithPayload([]byte(`{"channel":"heartbeat"}`))

		Convey("It should read channel and return zero for missing type", func() {
			So(Peek[string](envelope, "channel"), ShouldEqual, "heartbeat")
			So(Peek[string](envelope, "type"), ShouldEqual, "")
		})
	})

	Convey("Given a fresh artifact with no attributes or payload", t, func() {
		artifact := Acquire("peek-empty", Artifact_Type_json)

		Convey("It should return the type zero without touching crypto", func() {
			So(Peek[string](artifact, "destination"), ShouldEqual, "")
		})
	})
}

func TestPoke(t *testing.T) {
	Convey("Given an artifact without attributes", t, func() {
		artifact := Acquire("poke-test", Artifact_Type_json)

		Convey("It should set a top-level key", func() {
			artifact.Poke("frame", "role")
			So(Peek[string](artifact, "role"), ShouldEqual, "frame")
		})

		Convey("It should deep-set nested paths with auto-created containers", func() {
			artifact.Poke(1, "transforms", "cancelBid")
			So(Peek[float64](artifact, "transforms", "cancelBid"), ShouldEqual, 1)
		})
	})

	Convey("Given an artifact with existing attributes", t, func() {
		artifact := Acquire("poke-update", Artifact_Type_json).
			WithAttributes(Map[any]{"count": 1})

		Convey("It should replace an existing value in place", func() {
			artifact.Poke(42, "count")
			So(Peek[float64](artifact, "count"), ShouldEqual, 42)
		})
	})

	Convey("Given a float64 history slice", t, func() {
		artifact := Acquire("poke-history", Artifact_Type_json)
		history := make([]float64, 60)

		for index := range history {
			history[index] = float64(index + 1)
		}

		Convey("It should round-trip the history slice", func() {
			artifact.Poke(history, "history")
			So(len(Peek[[]float64](artifact, "history")), ShouldEqual, 60)
			So(Peek[[]float64](artifact, "history")[0], ShouldEqual, 1)
		})
	})

	Convey("Given non-finite float values", t, func() {
		artifact := Acquire("poke-nonfinite", Artifact_Type_json)

		Convey("It should store non-finite floats as zero", func() {
			artifact.Poke(math.NaN(), "output", "score")
			So(Peek[float64](artifact, "output", "score"), ShouldEqual, 0)
		})

		Convey("It should round-trip a Map with finite values", func() {
			artifact.Poke(Map[float64]{
				"alpha": 1.5,
				"value": 0.25,
			}, "output")
			So(Peek[float64](artifact, "output", "alpha"), ShouldEqual, 1.5)
		})

		Convey("It should sanitize non-finite float slices", func() {
			artifact.Poke([]float64{1, math.NaN(), 3, math.Inf(1)}, "history")
			history := Peek[[]float64](artifact, "history")
			So(len(history), ShouldEqual, 4)
			So(history[0], ShouldEqual, 1)
			So(history[1], ShouldEqual, 0)
			So(history[2], ShouldEqual, 3)
			So(history[3], ShouldEqual, 0)
		})
	})
}

func TestWithAttribute(t *testing.T) {
	Convey("Given a dotted attribute key", t, func() {
		artifact := Acquire("with-attribute", Artifact_Type_json).
			WithAttribute("transforms.cancelBid", "ema")

		Convey("It should store the nested value", func() {
			So(Peek[string](artifact, "transforms", "cancelBid"), ShouldEqual, "ema")
		})
	})
}

func TestWithAttributesAsPayload(t *testing.T) {
	Convey("Given an artifact with attributes staged for payload publish", t, func() {
		artifact := Acquire("payload-publish", Artifact_Type_json)
		artifact.Poke(0.71, "output", "confidence")
		artifact.WithAttributesAsPayload()

		Convey("It should copy attributes into the decrypted payload", func() {
			So(Peek[float64](artifact, "output", "confidence"), ShouldEqual, 0.71)
		})
	})
}

func BenchmarkPeek(b *testing.B) {
	artifact := Acquire("peek-bench", Artifact_Type_json).
		WithAttributes(Map[any]{"output": "processed"})

	b.ResetTimer()

	for b.Loop() {
		if Peek[string](artifact, "output") != "processed" {
			b.Fatal("unexpected peek value")
		}
	}
}

func BenchmarkPoke(b *testing.B) {
	b.ResetTimer()

	for b.Loop() {
		artifact := Acquire("poke-bench", Artifact_Type_json)
		artifact.Poke("processed", "output")

		if Peek[string](artifact, "output") != "processed" {
			b.Fatal("unexpected poke value")
		}
	}
}