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

func TestWithPayload(testingTB *testing.T) {
	Convey("Given an artifact with ingest metadata", testingTB, func() {
		artifact := Acquire("kraken:public", Artifact_Type_json)
		artifact.WithRole("trade")
		artifact.WithScope("update")

		Convey("It should reject an empty payload", func() {
			result := artifact.WithPayload(nil)

			So(result, ShouldBeNil)
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

func TestWithPayloadReadBudgetReset(t *testing.T) {
	Convey("Given a reused artifact with a small capnp read traversal budget", t, func() {
		artifact := Acquire("read-budget", Artifact_Type_json).
			WithPayload([]byte(`{"seed":1}`))

		message := capnpArtifact(artifact).Message()
		message.TraverseLimit = 512
		message.ResetReadLimit(message.TraverseLimit)

		for range 500 {
			if len(artifact.DecryptPayload()) == 0 {
				t.Fatal("decrypt failed before read budget fix could be validated")
			}

			if artifact.WithPayload([]byte(`{"tick":1}`)) == nil {
				t.Fatal("WithPayload failed after repeated decrypt/write cycles")
			}
		}

		Convey("It should still accept a final payload write", func() {
			result := artifact.WithPayload([]byte(`{"final":1}`))

			So(result, ShouldNotBeNil)
			So(string(result.DecryptPayload()), ShouldEqual, `{"final":1}`)
		})
	})
}

func BenchmarkWithPayloadReuse(b *testing.B) {
	payload := []byte(`{"method":"add_order","params":{"symbol":"BTC/USD"}}`)
	artifact := Acquire("withpayload-bench", Artifact_Type_json).
		WithPayload(payload)

	b.ResetTimer()

	for b.Loop() {
		if artifact.WithPayload(payload) == nil {
			b.Fatal("WithPayload failed on reused artifact")
		}
	}
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
