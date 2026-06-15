package datura

import (
	"io"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestArtifactRead(t *testing.T) {
	Convey("Read copies marshaled bytes", t, func() {
		artifact := sampleArtifact(t)
		defer artifact.Release()

		expected := artifact.Marshal()
		buffer := make([]byte, len(expected)+64)

		readCount, err := artifact.Read(buffer)
		So(err, ShouldEqual, io.EOF)
		So(readCount, ShouldEqual, len(expected))
		So(buffer[:readCount], ShouldResemble, expected)
	})
}

func TestArtifactRead_shortBuffer(t *testing.T) {
	Convey("Read returns ErrShortBuffer when p is too small", t, func() {
		artifact := sampleArtifact(t)
		defer artifact.Release()

		buffer := make([]byte, 1)
		readCount, err := artifact.Read(buffer)

		So(err, ShouldEqual, io.ErrShortBuffer)
		So(readCount, ShouldEqual, 1)
	})
}

func TestArtifactWrite(t *testing.T) {
	Convey("Write unmarshals bytes into artifact", t, func() {
		source := sampleArtifact(t)
		defer source.Release()

		buf := source.Marshal()

		target := Acquire("", Artifact_Type_json)
		defer target.Release()

		written, err := target.Write(buf)
		So(err, ShouldBeNil)
		So(written, ShouldEqual, len(buf))

		payload, payloadErr := target.Payload()
		So(payloadErr, ShouldBeNil)
		So(string(payload), ShouldEqual, "payload")
	})
}

func TestArtifactWrite_emptyInput(t *testing.T) {
	Convey("Write rejects empty input", t, func() {
		artifact := Acquire("", Artifact_Type_json)
		defer artifact.Release()

		written, err := artifact.Write(nil)
		So(err, ShouldNotBeNil)
		So(written, ShouldEqual, 0)
	})
}

func TestArtifactWrite_invalidBytes(t *testing.T) {
	Convey("Write rejects invalid marshaled bytes", t, func() {
		artifact := Acquire("", Artifact_Type_json)
		defer artifact.Release()

		written, err := artifact.Write([]byte{0xff, 0xff, 0xff})
		So(err, ShouldNotBeNil)
		So(written, ShouldEqual, 0)
	})
}

func TestArtifactClose(t *testing.T) {
	Convey("Close returns nil", t, func() {
		artifact := Acquire("", Artifact_Type_json)
		defer artifact.Release()

		So(artifact.Close(), ShouldBeNil)
	})
}

func BenchmarkArtifactRead(b *testing.B) {
	artifact := sampleArtifact(b)
	defer artifact.Release()

	buffer := make([]byte, 4096)

	b.ResetTimer()

	for b.Loop() {
		if _, err := artifact.Read(buffer); err != io.EOF && err != io.ErrShortBuffer {
			b.Fatalf("Read: %v", err)
		}
	}
}

func BenchmarkArtifactWrite(b *testing.B) {
	source := sampleArtifact(b)
	defer source.Release()

	buf := source.Marshal()

	b.ResetTimer()

	for b.Loop() {
		target := Acquire("", Artifact_Type_json)
		if target == nil {
			b.Fatal("Acquire returned nil")
		}

		if _, err := target.Write(buf); err != nil {
			target.Release()
			b.Fatalf("Write: %v", err)
		}

		target.Release()
	}
}

func BenchmarkArtifactClose(b *testing.B) {
	artifact := sampleArtifact(b)
	defer artifact.Release()

	b.ResetTimer()

	for b.Loop() {
		if err := artifact.Close(); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkArtifactIORoundTrip(b *testing.B) {
	artifact := sampleArtifact(b)
	defer artifact.Release()

	b.ResetTimer()

	for b.Loop() {
		if err := exerciseIORoundTrip(artifact); err != nil {
			b.Fatal(err)
		}
	}
}
