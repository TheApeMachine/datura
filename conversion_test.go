package datura

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestRestoreWritable(t *testing.T) {
	Convey("Given a packed artifact", t, func() {
		source := Acquire("restore-writable", Artifact_Type_json).
			WithPayload([]byte("payload-bytes"))

		packed, err := source.MarshalPacked()
		So(err, ShouldBeNil)

		restored := &Artifact{}
		_, err = restored.Write(packed)
		So(err, ShouldBeNil)

		Convey("It should preserve payload bytes", func() {
			So(string(restored.DecryptPayload()), ShouldEqual, "payload-bytes")
		})

		Convey("It should preserve uuid", func() {
			uuidBytes, err := restored.Uuid()
			So(err, ShouldBeNil)
			So(len(uuidBytes), ShouldBeGreaterThan, 0)

			sourceUUID, err := source.Uuid()
			So(err, ShouldBeNil)
			So(string(uuidBytes), ShouldEqual, string(sourceUUID))
		})

		Convey("It should allow attribute mutation", func() {
			restored.Poke(7, "count")
			So(Peek[float64](restored, "count"), ShouldEqual, 7)
		})
	})
}
