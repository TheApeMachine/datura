package datura

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestAcquire_invalidPoolEntry(t *testing.T) {
	Convey("Acquire rejects non-artifact pool entries", t, func() {
		artifactPool.Put("invalid")

		artifact := Acquire("origin", Artifact_Type_json)
		So(artifact, ShouldBeNil)
	})
}

func TestAcquire(t *testing.T) {
	Convey("Acquire", t, func() {
		artifact := Acquire("origin", Artifact_Type_json)
		So(artifact, ShouldNotBeNil)

		origin, err := artifact.Origin()
		So(err, ShouldBeNil)
		So(origin, ShouldEqual, "origin")
		So(artifact.Type(), ShouldEqual, Artifact_Type_json)

		uuid, err := artifact.Uuid()
		So(err, ShouldBeNil)
		So(uuid, ShouldNotBeEmpty)

		artifact.Release()
	})
}

func TestArtifactRelease(t *testing.T) {
	Convey("Release allows subsequent Acquire", t, func() {
		first := Acquire("first", Artifact_Type_json)
		So(first, ShouldNotBeNil)

		first.Release()

		second := Acquire("second", Artifact_Type_json)
		So(second, ShouldNotBeNil)

		secondOrigin, err := second.Origin()
		So(err, ShouldBeNil)
		So(secondOrigin, ShouldEqual, "second")

		second.Release()
	})
}

func TestArtifactRelease_nil(t *testing.T) {
	Convey("Release ignores nil artifact", t, func() {
		var artifact *Artifact
		artifact.Release()
	})
}

func BenchmarkAcquire(b *testing.B) {
	b.ResetTimer()

	for b.Loop() {
		artifact := Acquire("origin", Artifact_Type_json)
		if artifact == nil {
			b.Fatal("Acquire returned nil")
		}

		artifact.Release()
	}
}

func BenchmarkRelease_nil(b *testing.B) {
	b.ResetTimer()

	for b.Loop() {
		var artifact *Artifact
		artifact.Release()
	}
}

func BenchmarkPopulateArtifactFields(b *testing.B) {
	artifact := Acquire("origin", Artifact_Type_json)
	if artifact == nil {
		b.Fatal("Acquire returned nil")
	}

	defer artifact.Release()

	if err := readArtifactFields(artifact); err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()

	for b.Loop() {
		if err := mutateArtifactFields(artifact); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkArtifactLifecycle(b *testing.B) {
	b.ResetTimer()

	for b.Loop() {
		artifact := Acquire("origin", Artifact_Type_json)
		if artifact == nil {
			b.Fatal("Acquire returned nil")
		}

		if err := exerciseConversionRoundTrip(artifact); err != nil {
			artifact.Release()
			b.Fatal(err)
		}

		if err := exerciseIORoundTrip(artifact); err != nil {
			artifact.Release()
			b.Fatal(err)
		}

		artifact.Release()
	}
}
