package transport

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/stream"
)

func flipflopStage() *stream.Buffer {
	return stream.NewBuffer(func(processed *datura.Artifact) error {
		return processed.SetMetaValue("output", "processed")
	})
}

func TestNewFlipFlop(t *testing.T) {
	Convey("Given an artifact and a stream buffer stage", t, func() {
		artifact := datura.Acquire("flipflop-test", datura.Artifact_Type_json).
			Poke("input", "seed")

		stage := flipflopStage()

		Convey("When running FlipFlop", func() {
			err := NewFlipFlop(artifact, stage)

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
			Poke("input", "seed")

		stage := flipflopStage()

		_, err := Copy(stage, artifact)
		So(err, ShouldBeNil)

		_, err = Copy(artifact, stage)
		So(err, ShouldBeNil)

		So(datura.Peek[string](artifact, "output"), ShouldEqual, "processed")
	})
}

func BenchmarkNewFlipFlop(b *testing.B) {
	stage := flipflopStage()

	b.ResetTimer()

	for b.Loop() {
		artifact := datura.Acquire("flipflop-bench", datura.Artifact_Type_json).
			Poke("input", "seed")

		if err := NewFlipFlop(artifact, stage); err != nil {
			b.Fatal(err)
		}
	}
}
