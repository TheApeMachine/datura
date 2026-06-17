package datura

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func sampleArtifact(tb testing.TB) *Artifact {
	tb.Helper()

	artifact := Acquire("origin", Artifact_Type_json)

	if artifact == nil {
		tb.Fatal("Acquire returned nil")
	}

	result := artifact.WithPayload([]byte("payload"))

	if result == nil {
		tb.Fatal("WithPayload returned nil")
	}

	return result
}

func TestArtifactWireRoundTrip(t *testing.T) {
	Convey("Message().Marshal and Write round-trip", t, func() {
		source := sampleArtifact(t)
		defer source.Release()

		buf, err := source.Message().Marshal()

		So(err, ShouldBeNil)
		So(len(buf), ShouldBeGreaterThan, 0)

		target := Acquire("", Artifact_Type_json)
		defer target.Release()

		written, writeErr := target.Write(buf)

		So(writeErr, ShouldBeNil)
		So(written, ShouldEqual, len(buf))

		payload, decryptErr := target.DecryptPayload()

		So(decryptErr, ShouldBeNil)
		So(string(payload), ShouldEqual, "payload")
	})
}

func TestArtifactWrite_emptyBuffer(t *testing.T) {
	Convey("Write rejects empty buffer", t, func() {
		artifact := Acquire("", Artifact_Type_json)
		defer artifact.Release()

		_, err := artifact.Write(nil)
		So(err, ShouldNotBeNil)

		_, err = artifact.Write([]byte{})
		So(err, ShouldNotBeNil)
	})
}

func TestArtifactWrite_invalidBuffer(t *testing.T) {
	Convey("Write buffers incomplete frame bytes", t, func() {
		artifact := Acquire("", Artifact_Type_json)
		defer artifact.Release()

		_, err := artifact.Write([]byte{0xff, 0xff, 0xff})
		So(err, ShouldBeNil)
	})
}

func TestArtifactPackUnpack(t *testing.T) {
	Convey("Pack and Unpack round-trip", t, func() {
		source := sampleArtifact(t)
		defer source.Release()

		buf, err := source.Pack()

		So(err, ShouldBeNil)
		So(len(buf), ShouldBeGreaterThan, 0)

		target := Acquire("", Artifact_Type_json)
		defer target.Release()

		So(target.Unpack(buf), ShouldBeNil)

		payload, decryptErr := target.DecryptPayload()

		So(decryptErr, ShouldBeNil)
		So(string(payload), ShouldEqual, "payload")
	})
}

func TestArtifactUnpack_invalidBuffer(t *testing.T) {
	Convey("Unpack rejects invalid packed buffer", t, func() {
		artifact := Acquire("", Artifact_Type_json)
		defer artifact.Release()

		So(artifact.Unpack([]byte{0xff, 0xff, 0xff}), ShouldNotBeNil)
	})
}

func BenchmarkArtifactMessageMarshal(b *testing.B) {
	artifact := sampleArtifact(b)
	defer artifact.Release()

	b.ResetTimer()

	for b.Loop() {
		buf, err := artifact.Message().Marshal()

		if err != nil || len(buf) == 0 {
			b.Fatal("Marshal returned empty buffer")
		}
	}
}

func BenchmarkArtifactWrite(b *testing.B) {
	artifact := sampleArtifact(b)
	defer artifact.Release()

	buf, err := artifact.Message().Marshal()

	if err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()

	for b.Loop() {
		target := Acquire("", Artifact_Type_json)

		if target == nil {
			b.Fatal("Acquire returned nil")
		}

		if _, writeErr := target.Write(buf); writeErr != nil {
			target.Release()
			b.Fatal(writeErr)
		}

		target.Release()
	}
}

func BenchmarkArtifactPack(b *testing.B) {
	artifact := sampleArtifact(b)
	defer artifact.Release()

	b.ResetTimer()

	for b.Loop() {
		buf, err := artifact.Pack()

		if err != nil || len(buf) == 0 {
			b.Fatal("Pack returned empty buffer")
		}
	}
}

func BenchmarkArtifactConversionRoundTrip(b *testing.B) {
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

		artifact.Release()
	}
}
