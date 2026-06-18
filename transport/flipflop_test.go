package transport

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/datura"
)

func TestNewFlipFlop(t *testing.T) {
	Convey("Given an artifact and a stream buffer stage", t, func() {
		artifact := datura.Acquire("flipflop-test", datura.Artifact_Type_json).
			WithAttributes(datura.Map[any]{
				"input":  "seed",
				"output": "processed",
			})

		Convey("When running FlipFlop", func() {
			err := NewFlipFlop(artifact, artifact)

			Convey("Then the stage output should land back on the artifact", func() {
				So(err, ShouldBeNil)
				So(datura.Peek[string](artifact, "output"), ShouldEqual, "processed")
			})
		})
	})
}

func TestCopyFlipFlopStages(t *testing.T) {
	Convey("Given copy stages in isolation", t, func() {
		artifact := datura.Acquire("flipflop-copy", datura.Artifact_Type_json).
			WithAttributes(datura.Map[any]{
				"input":  "seed",
				"output": "processed",
			})

		err := NewFlipFlop(artifact, artifact)
		So(err, ShouldBeNil)

		So(datura.Peek[string](artifact, "output"), ShouldEqual, "processed")
	})
}

func BenchmarkNewFlipFlop(b *testing.B) {
	artifact := datura.Acquire("flipflop-bench", datura.Artifact_Type_json).
		WithAttributes(datura.Map[any]{
			"input":  "seed",
			"output": "processed",
		})

	err := NewFlipFlop(artifact, artifact)
	So(err, ShouldBeNil)

	b.ResetTimer()

	for b.Loop() {
		artifact := datura.Acquire("flipflop-bench", datura.Artifact_Type_json).
			WithAttributes(datura.Map[any]{
				"input":  "seed",
				"output": "processed",
			})

		if err := NewFlipFlop(artifact, artifact); err != nil {
			b.Fatal(err)
		}
	}
}
