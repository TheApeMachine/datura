package datura

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestLegacyWireRoundTrip(t *testing.T) {
	Convey("Given a legacy payload round trip", t, func() {
		source := Acquire("legacy", Artifact_Type_json)
		So(source, ShouldNotBeNil)

		So(source.SetPayload([]byte("legacy-payload")), ShouldBeNil)

		wire := source.Marshal()
		So(len(wire), ShouldBeGreaterThan, 0)

		target := Acquire("legacy", Artifact_Type_json)
		So(target, ShouldNotBeNil)

		So(target.Unmarshal(wire), ShouldEqual, target)

		payload, err := target.Payload()
		So(err, ShouldBeNil)
		So(string(payload), ShouldEqual, "legacy-payload")
	})
}

func TestLegacyPeek(t *testing.T) {
	Convey("Given metadata on an artifact", t, func() {
		artifact := Acquire("legacy", Artifact_Type_json).Poke("role", "trade")

		So(artifact.Peek("role"), ShouldEqual, "trade")
		So(Peek[string](artifact, "role"), ShouldEqual, "trade")
	})
}
