package datura

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"

	"capnproto.org/go/capnp/v3"
)

func TestUnpackArtifact(t *testing.T) {
	Convey("Given a packed artifact wire frame", t, func() {
		source := Acquire("unpack-source", Artifact_Type_json).
			WithRole("measurement").
			WithScope("BTC/USD")
		packed, err := source.Pack()
		So(err, ShouldBeNil)
		source.Release()

		Convey("It should inflate into a pooled artifact without heap escapes", func() {
			artifact, err := UnpackArtifact(packed)
			So(err, ShouldBeNil)
			So(Peek[string](artifact, "role"), ShouldEqual, "measurement")
			So(Peek[string](artifact, "scope"), ShouldEqual, "BTC/USD")

			Reset(func() {
				artifact.Release()
			})
		})
	})
}

func TestArtifactPrefix(t *testing.T) {
	Convey("Given routing fields on an artifact", t, func() {
		_, seg, err := capnp.NewMessage(capnp.SingleSegment(nil))
		So(err, ShouldBeNil)

		root, err := NewRootArtifact(seg)
		So(err, ShouldBeNil)

		artifact := root

		So(artifact.SetOrigin("origin"), ShouldBeNil)
		So(artifact.SetDestination("destination"), ShouldBeNil)
		So(artifact.SetRole("role"), ShouldBeNil)
		So(artifact.SetScope("scope"), ShouldBeNil)
		So(artifact.SetUuid([]byte("uuid-bytes")), ShouldBeNil)
		artifact.SetTimestamp(4096)
		artifact.SetType(Artifact_Type_json)

		Convey("It should build the trie address without slice churn", func() {
			prefix := artifact.Prefix()
			So(prefix, ShouldContainSubstring, "origin/destination/role/scope/")
			So(prefix, ShouldContainSubstring, "uuid-bytes")
			So(prefix, ShouldEndWith, ".")
		})
	})
}

func BenchmarkUnpackArtifact(b *testing.B) {
	source := Acquire("unpack-bench", Artifact_Type_json).WithRole("measurement")
	packed, err := source.Pack()

	if err != nil {
		b.Fatal(err)
	}

	source.Release()
	b.ResetTimer()

	for b.Loop() {
		artifact, unpackErr := UnpackArtifact(packed)

		if unpackErr != nil {
			b.Fatal(unpackErr)
		}

		if Peek[string](artifact, "role") != "measurement" {
			b.Fatal("role missing after unpack")
		}

		artifact.Release()
	}
}
