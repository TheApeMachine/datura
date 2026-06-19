package datura

import (
	"io"
	"math"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestPeek(t *testing.T) {
	Convey("Given an artifact with attributes", t, func() {
		artifact := Acquire("test", Artifact_Type_json).
			WithAttributes(Map[any]{"test": "test"})
		So(artifact, ShouldNotBeNil)

		Convey("It should peek the attribute", func() {
			So(Peek[string](artifact, "test"), ShouldEqual, "test")
		})
	})
}

func TestPoke(t *testing.T) {
	Convey("Given an artifact with attributes", t, func() {
		artifact := Acquire("test", Artifact_Type_json).
			WithAttributes(Map[any]{"test": "test"})

		Convey("It should poke the attribute", func() {
			So(artifact.Poke("test", "test"), ShouldEqual, artifact)
			So(Peek[string](artifact, "test"), ShouldEqual, "test")

			Convey("When poking the attribute again", func() {
				So(artifact.Poke("toast", "test"), ShouldEqual, artifact)
				So(Peek[string](artifact, "test"), ShouldEqual, "toast")
			})
		})
	})
}

func TestPeekNestedMap(t *testing.T) {
	Convey("Given an artifact with a nested output map", t, func() {
		artifact := Acquire("test", Artifact_Type_json)
		artifact.Poke(Map[float64]{
			"min":   1,
			"max":   3,
			"prev":  1,
			"value": 1,
		}, "output")

		Convey("It should peek the whole map with stable typing", func() {
			output := Peek[Map[float64]](artifact, "output")

			So(output, ShouldNotBeNil)
			So(output["value"], ShouldEqual, 1)
		})

		Convey("It should peek nested scalar paths", func() {
			So(Peek[float64](artifact, "output", "value"), ShouldEqual, 1)
		})
	})
}

func TestPeekNonFiniteFloat(t *testing.T) {
	Convey("Given a non-finite float encoded as a string token", t, func() {
		artifact := Acquire("test", Artifact_Type_json)
		artifact.Poke("NaN", "output", "value")

		Convey("It should peek as float64 NaN", func() {
			got := Peek[float64](artifact, "output", "value")

			So(math.IsNaN(got), ShouldBeTrue)
		})
	})
}

func TestPokeWriteMerge(t *testing.T) {
	Convey("Given a stage artifact with retained output", t, func() {
		stage := Acquire("stage", Artifact_Type_json).RetainStageAttributes()
		stage.Poke(Map[float64]{
			"min":   1,
			"max":   1,
			"prev":  1,
			"value": 0,
		}, "output")

		inbound := Acquire("inbound", Artifact_Type_json).Poke(3, "sample")
		wire := make([]byte, 4096)

		var wireLen int

		for {
			n, readErr := inbound.Read(wire[wireLen:])

			wireLen += n

			if readErr == io.EOF {
				break
			}

			So(readErr, ShouldBeNil)
		}

		So(wireLen, ShouldBeGreaterThan, 0)

		_, err := stage.Write(wire[:wireLen])

		So(err, ShouldBeNil)
		So(Peek[float64](stage, "sample"), ShouldEqual, 3)

		output := Peek[Map[float64]](stage, "output")

		So(output, ShouldNotBeNil)
		So(output["value"], ShouldEqual, 0)
		So(output["min"], ShouldEqual, 1)
	})

	Convey("When upstream peers arrive on a retained stage", t, func() {
		stage := Acquire("stage", Artifact_Type_json).RetainStageAttributes()
		stage.Poke(map[string]float64{"2": 0.04, "3": 0.06}, "peers")

		inbound := Acquire("inbound", Artifact_Type_json).
			Poke(1, "member").
			Poke(0.02, "sample")
		wire := make([]byte, 4096)

		var wireLen int

		for {
			n, readErr := inbound.Read(wire[wireLen:])

			wireLen += n

			if readErr == io.EOF {
				break
			}

			So(readErr, ShouldBeNil)
		}

		_, err := stage.Write(wire[:wireLen])

		So(err, ShouldBeNil)
		So(Peek[float64](stage, "sample"), ShouldEqual, 0.02)
		So(Peek[float64](stage, "member"), ShouldEqual, 1)

		peers := Peek[map[string]float64](stage, "peers")

		So(peers, ShouldNotBeNil)
		So(len(peers), ShouldEqual, 2)
		So(peers["2"], ShouldEqual, 0.04)
		So(peers["3"], ShouldEqual, 0.06)
	})
}
