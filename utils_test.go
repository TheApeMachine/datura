package datura

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestDecryptPayloadError(t *testing.T) {
	Convey("Given a freshly acquired artifact", t, func() {
		artifact := Acquire("decrypt-test", Artifact_Type_json)

		Convey("It should not decrypt without ciphertext", func() {
			payload, err := artifact.DecryptPayloadError()
			So(err, ShouldNotBeNil)
			So(payload, ShouldBeNil)
			So(artifact.DecryptPayload(), ShouldBeNil)
		})
	})
}

func TestDecryptPayload(t *testing.T) {
	Convey("Given a freshly acquired artifact", t, func() {
		artifact := Acquire("decrypt-test", Artifact_Type_json)

		Convey("It should not decrypt without ciphertext", func() {
			payload, err := artifact.decryptPayload()
			So(err, ShouldNotBeNil)
			So(payload, ShouldBeNil)
			So(artifact.DecryptPayload(), ShouldBeNil)
		})
	})

	Convey("Given an artifact with an encrypted payload", t, func() {
		artifact := Acquire("decrypt-test", Artifact_Type_json).
			WithPayload([]byte(`{"method":"add_order"}`))

		Convey("It should decrypt the payload", func() {
			payload, err := artifact.decryptPayload()
			So(err, ShouldBeNil)
			So(string(payload), ShouldEqual, `{"method":"add_order"}`)
		})
	})
}

func TestRelease(t *testing.T) {
	Convey("Given a used artifact returned to the pool", t, func() {
		artifact := Acquire("release-test", Artifact_Type_json).
			WithPayload([]byte(`{"count":1}`))

		artifact.Release()

		reused := Acquire("release-test", Artifact_Type_json)

		Convey("It should not retain encrypted payload slots", func() {
			So(reused.HasEncryptedPayload(), ShouldBeFalse)
			So(reused.DecryptPayload(), ShouldBeNil)
		})
	})
}

func BenchmarkDecryptPayload(b *testing.B) {
	artifact := Acquire("decrypt-bench", Artifact_Type_json).
		WithPayload([]byte(`{"method":"add_order","params":{"symbol":"BTC/USD"}}`))

	b.ResetTimer()

	for b.Loop() {
		if len(artifact.DecryptPayload()) == 0 {
			b.Fatal("expected decrypted payload")
		}
	}
}
