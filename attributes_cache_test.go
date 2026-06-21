package datura

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestAttributesCacheEntryFor(t *testing.T) {
	Convey("Given a fresh artifact", t, func() {
		artifact := Acquire("cache-entry", Artifact_Type_json)

		Convey("It should reuse the same resident root across peeks", func() {
			first := attributesCacheEntryFor(artifact)
			second := attributesCacheEntryFor(artifact)

			So(first, ShouldEqual, second)
		})
	})

	Convey("Given an invalidated cache", t, func() {
		artifact := Acquire("cache-invalidate", Artifact_Type_json)
		artifact.Poke("seed", "input")

		packed, err := artifact.MarshalPacked()
		So(err, ShouldBeNil)

		invalidateAttributesCache(artifact)
		_, err = artifact.Unpack(packed)
		So(err, ShouldBeNil)

		Convey("It should reload attributes from unpacked bytes", func() {
			So(Peek[string](artifact, "input"), ShouldEqual, "seed")
		})
	})
}

func BenchmarkAttributesCachePeek(b *testing.B) {
	artifact := Acquire("cache-peek-bench", Artifact_Type_json).
		WithAttributes(Map[any]{"output": "processed"})

	b.ResetTimer()

	for b.Loop() {
		if Peek[string](artifact, "output") != "processed" {
			b.Fatal("unexpected peek value")
		}
	}
}

func BenchmarkAttributesCachePoke(b *testing.B) {
	artifact := Acquire("cache-poke-bench", Artifact_Type_json)

	b.ResetTimer()

	for b.Loop() {
		artifact.Poke("processed", "output")

		if Peek[string](artifact, "output") != "processed" {
			b.Fatal("unexpected poke value")
		}
	}
}
