package transport

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/datura"
)

func TestNewOutputArtifact(t *testing.T) {
	Convey("Given a pipeline that emits an artifact frame", t, func() {
		artifact := datura.Acquire("output-test", datura.Artifact_Type_json).
			Poke("output", "test data")

		wire, err := artifact.Message().Marshal()
		So(err, ShouldBeNil)

		pipeline := newTestBuffer(wire)
		out := NewOutput[*datura.Artifact](pipeline)

		Convey("Then output should materialize the artifact", func() {
			So(out, ShouldNotBeNil)
			So(datura.Peek[string](out, "output"), ShouldEqual, "test data")
		})
	})
}

func TestNewOutputJSON(t *testing.T) {
	Convey("Given a pipeline that emits JSON", t, func() {
		pipeline := newTestBuffer([]byte(`{"label":"value"}`))

		out := NewOutput[map[string]string](pipeline)

		Convey("Then output should decode JSON", func() {
			So(out["label"], ShouldEqual, "value")
		})
	})
}

func BenchmarkNewOutputArtifact(b *testing.B) {
	artifact := datura.Acquire("output-bench", datura.Artifact_Type_json).
		Poke("output", "bench")

	wire, err := artifact.Message().Marshal()

	if err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()

	for b.Loop() {
		pipeline := newTestBuffer(wire)
		out := NewOutput[*datura.Artifact](pipeline)

		if datura.Peek[string](out, "output") != "bench" {
			b.Fatal("output metadata missing")
		}
	}
}

func BenchmarkNewOutputJSON(b *testing.B) {
	payload := []byte(`{"label":"value"}`)

	b.ResetTimer()

	for b.Loop() {
		pipeline := newTestBuffer(payload)
		out := NewOutput[map[string]string](pipeline)

		if out["label"] != "value" {
			b.Fatal("json output missing")
		}
	}
}
