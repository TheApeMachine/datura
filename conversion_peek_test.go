package datura

import (
	"strconv"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

func TestMapPeek(testingTB *testing.T) {
	Convey("Given a typed map", testingTB, func() {
		eventAt := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
		data := Map{
			"price":    101.5,
			"count":    3,
			"active":   true,
			"observed": eventAt.Format(time.RFC3339Nano),
			"label":    "risk_on",
		}

		Convey("It should peek numeric and temporal values", func() {
			So(MapPeek[float64](data, "price"), ShouldEqual, 101.5)
			So(MapPeek[int](data, "count"), ShouldEqual, 3)
			So(MapPeek[bool](data, "active"), ShouldBeTrue)
			So(MapPeek[time.Time](data, "observed"), ShouldEqual, eventAt)
			So(MapPeek[string](data, "label"), ShouldEqual, "risk_on")
		})

		Convey("It should return zero values for missing keys", func() {
			So(MapPeek[float64](data, "missing"), ShouldEqual, 0)
			So(MapPeek[int](data, "missing"), ShouldEqual, 0)
			So(MapPeek[string](data, "missing"), ShouldBeEmpty)
		})
	})
}

func TestArtifactPeek(testingTB *testing.T) {
	Convey("Given an artifact with typed attributes", testingTB, func() {
		artifact := Acquire("origin", Artifact_Type_json)
		defer artifact.Release()

		artifact.Poke("strength", strconv.FormatFloat(0.75, 'g', -1, 64))
		artifact.Poke("category", "2")
		artifact.Poke("probabilities", "0.1,0.7,0.2")
		artifact.Poke("role", "measurement")

		Convey("It should peek typed values from attributes", func() {
			So(Peek[float64](artifact, "strength"), ShouldEqual, 0.75)
			So(Peek[int](artifact, "category"), ShouldEqual, 2)
			So(Peek[[]float64](artifact, "probabilities"), ShouldResemble, []float64{0.1, 0.7, 0.2})
			So(Peek[string](artifact, "role"), ShouldEqual, "measurement")
		})

		Convey("It should report parse failures with PeekOK", func() {
			artifact.Poke("broken", "not-a-number")

			value, ok := PeekOK[float64](artifact, "broken")

			So(ok, ShouldBeFalse)
			So(value, ShouldEqual, 0)
		})
	})
}

func BenchmarkArtifactPeekFloat(testingTB *testing.B) {
	testingTB.ResetTimer()

	for testingTB.Loop() {
		artifact := Acquire("origin", Artifact_Type_json)
		artifact.Poke("strength", "0.75")

		if Peek[float64](artifact, "strength") == 0 {
			artifact.Release()
			testingTB.Fatal("Peek returned zero")
		}

		artifact.Release()
	}
}
