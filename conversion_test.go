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

	if err := artifact.SetPayload([]byte("payload")); err != nil {
		tb.Fatalf("SetPayload: %v", err)
	}

	return artifact
}

func TestArtifactMarshalUnmarshal(t *testing.T) {
	Convey("Marshal and Unmarshal round-trip", t, func() {
		source := sampleArtifact(t)
		defer source.Release()

		buf := source.Marshal()
		So(len(buf), ShouldBeGreaterThan, 0)

		target := Acquire("", Artifact_Type_json)
		defer target.Release()

		result := target.Unmarshal(buf)
		So(result, ShouldNotBeNil)

		payload, err := result.Payload()
		So(err, ShouldBeNil)
		So(string(payload), ShouldEqual, "payload")
	})
}

func TestArtifactUnmarshal_emptyBuffer(t *testing.T) {
	Convey("Unmarshal rejects empty buffer", t, func() {
		artifact := Acquire("", Artifact_Type_json)
		defer artifact.Release()

		So(artifact.Unmarshal(nil), ShouldBeNil)
		So(artifact.Unmarshal([]byte{}), ShouldBeNil)
	})
}

func TestArtifactPackUnpack(t *testing.T) {
	Convey("Pack and Unpack round-trip", t, func() {
		source := sampleArtifact(t)
		defer source.Release()

		buf := source.Pack()
		So(len(buf), ShouldBeGreaterThan, 0)

		target := Acquire("", Artifact_Type_json)
		defer target.Release()

		result := target.Unpack(buf)
		So(result, ShouldNotBeNil)

		payload, err := result.Payload()
		So(err, ShouldBeNil)
		So(string(payload), ShouldEqual, "payload")
	})
}

func TestArtifactUnpack_emptyBuffer(t *testing.T) {
	Convey("Unpack rejects empty buffer", t, func() {
		artifact := Acquire("", Artifact_Type_json)
		defer artifact.Release()

		So(artifact.Unpack(nil), ShouldBeNil)
		So(artifact.Unpack([]byte{}), ShouldBeNil)
	})
}

func TestArtifactEncode(t *testing.T) {
	Convey("Encode writes artifact message to buffer", t, func() {
		source := sampleArtifact(t)
		defer source.Release()

		buffer := make([]byte, 0, 512)
		source.Encode(buffer)

		target := Acquire("", Artifact_Type_json)
		defer target.Release()

		result := target.Decode(source.Marshal())
		So(result, ShouldNotBeNil)

		payload, err := result.Payload()
		So(err, ShouldBeNil)
		So(string(payload), ShouldEqual, "payload")
	})
}

func TestArtifactDecode_invalidBuffer(t *testing.T) {
	Convey("Decode returns nil for invalid buffer", t, func() {
		artifact := Acquire("", Artifact_Type_json)
		defer artifact.Release()

		So(artifact.Decode([]byte{0xff, 0xff, 0xff}), ShouldBeNil)
	})
}

func TestArtifactUnmarshal_invalidBuffer(t *testing.T) {
	Convey("Unmarshal returns nil for invalid buffer", t, func() {
		artifact := Acquire("", Artifact_Type_json)
		defer artifact.Release()

		So(artifact.Unmarshal([]byte{0xff, 0xff, 0xff}), ShouldBeNil)
	})
}

func TestArtifactUnpack_invalidBuffer(t *testing.T) {
	Convey("Unpack returns nil for invalid packed buffer", t, func() {
		artifact := Acquire("", Artifact_Type_json)
		defer artifact.Release()

		So(artifact.Unpack([]byte{0xff, 0xff, 0xff}), ShouldBeNil)
	})
}

func TestArtifactDecode(t *testing.T) {
	Convey("Decode restores marshaled artifact", t, func() {
		source := sampleArtifact(t)
		defer source.Release()

		buf := source.Marshal()

		target := Acquire("", Artifact_Type_json)
		defer target.Release()

		result := target.Decode(buf)
		So(result, ShouldNotBeNil)

		payload, err := result.Payload()
		So(err, ShouldBeNil)
		So(string(payload), ShouldEqual, "payload")
	})
}

func BenchmarkArtifactMarshal(b *testing.B) {
	artifact := sampleArtifact(b)
	defer artifact.Release()

	b.ResetTimer()

	for b.Loop() {
		buf := artifact.Marshal()
		if len(buf) == 0 {
			b.Fatal("Marshal returned empty buffer")
		}
	}
}

func BenchmarkArtifactUnmarshal(b *testing.B) {
	artifact := sampleArtifact(b)
	defer artifact.Release()

	buf := artifact.Marshal()

	b.ResetTimer()

	for b.Loop() {
		target := Acquire("", Artifact_Type_json)
		if target == nil {
			b.Fatal("Acquire returned nil")
		}

		if target.Unmarshal(buf) == nil {
			target.Release()
			b.Fatal("Unmarshal returned nil")
		}

		target.Release()
	}
}

func BenchmarkArtifactPack(b *testing.B) {
	artifact := sampleArtifact(b)
	defer artifact.Release()

	b.ResetTimer()

	for b.Loop() {
		buf := artifact.Pack()
		if len(buf) == 0 {
			b.Fatal("Pack returned empty buffer")
		}
	}
}

func BenchmarkArtifactEncode(b *testing.B) {
	artifact := sampleArtifact(b)
	defer artifact.Release()

	buffer := make([]byte, 0, 512)

	b.ResetTimer()

	for b.Loop() {
		artifact.Encode(buffer)
	}
}

func BenchmarkArtifactDecode(b *testing.B) {
	artifact := sampleArtifact(b)
	defer artifact.Release()

	buf := artifact.Marshal()

	b.ResetTimer()

	for b.Loop() {
		target := Acquire("", Artifact_Type_json)
		if target == nil {
			b.Fatal("Acquire returned nil")
		}

		if target.Decode(buf) == nil {
			target.Release()
			b.Fatal("Decode returned nil")
		}

		target.Release()
	}
}

func BenchmarkArtifactUnpack(b *testing.B) {
	artifact := sampleArtifact(b)
	defer artifact.Release()

	buf := artifact.Pack()

	b.ResetTimer()

	for b.Loop() {
		target := Acquire("", Artifact_Type_json)
		if target == nil {
			b.Fatal("Acquire returned nil")
		}

		if target.Unpack(buf) == nil {
			target.Release()
			b.Fatal("Unpack returned nil")
		}

		target.Release()
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
