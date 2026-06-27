package datura

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestMergeOutputs(t *testing.T) {
	Convey("Given an artifact with existing payload data and output", t, func() {
		artifact := Acquire("merge-outputs", Artifact_Type_json).
			WithPayload([]byte(`{"symbol":"BTC/USD","output":{"old":0.25}}`))

		artifact.MergeOutputs(map[string]any{
			"confidence": 0.75,
			"category":   3,
		})

		Convey("It should preserve top-level fields and sibling output keys", func() {
			So(Peek[string](artifact, "symbol"), ShouldEqual, "BTC/USD")
			So(Peek[float64](artifact, "output", "old"), ShouldEqual, 0.25)
			So(Peek[float64](artifact, "output", "confidence"), ShouldEqual, 0.75)
			So(Peek[int](artifact, "output", "category"), ShouldEqual, 3)
		})
	})
}

func TestMergeFields(t *testing.T) {
	Convey("Given an artifact with existing payload fields", t, func() {
		artifact := Acquire("merge-fields", Artifact_Type_json).
			WithPayload([]byte(`{"symbol":"BTC/USD","output":{"confidence":0.25}}`))

		artifact.MergeFields(map[string]any{
			"depth":     12.5,
			"timestamp": 42,
		})

		Convey("It should preserve sibling payload and output fields", func() {
			So(Peek[string](artifact, "symbol"), ShouldEqual, "BTC/USD")
			So(Peek[float64](artifact, "depth"), ShouldEqual, 12.5)
			So(Peek[int](artifact, "timestamp"), ShouldEqual, 42)
			So(Peek[float64](artifact, "output", "confidence"), ShouldEqual, 0.25)
		})
	})
}
