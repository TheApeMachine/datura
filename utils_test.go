package datura

import (
	"crypto/ecdh"
	"crypto/rand"
	"fmt"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestDecryptPayloadError(t *testing.T) {
	Convey("Given a freshly acquired artifact", t, func() {
		artifact := Acquire("decrypt-test", Artifact_Type_json)

		Convey("It should not decrypt without ciphertext", func() {
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

	Convey("Given an artifact with a default payload", t, func() {
		artifact := Acquire("decrypt-test", Artifact_Type_json).
			WithPayload([]byte(`{"method":"add_order"}`))

		Convey("It should expose the plaintext payload", func() {
			payload, err := artifact.decryptPayload()
			So(err, ShouldBeNil)
			So(string(payload), ShouldEqual, `{"method":"add_order"}`)
		})
	})

	Convey("Given an artifact with a plaintext payload", t, func() {
		artifact := Acquire("decrypt-test", Artifact_Type_json).
			WithPlaintextPayload([]byte(`{"method":"local_stage"}`))

		Convey("It should expose the plaintext payload", func() {
			payload, err := artifact.decryptPayload()
			So(err, ShouldBeNil)
			So(string(payload), ShouldEqual, `{"method":"local_stage"}`)
			So(string(artifact.DecryptPayload()), ShouldEqual, `{"method":"local_stage"}`)
		})
	})
}

func TestSealedPayload(t *testing.T) {
	Convey("Given a recipient key pair and a sealed payload", t, func() {
		recipientKey, keyErr := ecdh.P256().GenerateKey(rand.Reader)
		So(keyErr, ShouldBeNil)

		artifact := Acquire("seal-test", Artifact_Type_json).
			WithRole("secret").
			WithScope("BTC/USD")
		artifact.SetTimestamp(123)
		So(artifact.WithSealedPayload(
			[]byte(`{"secret":true}`),
			recipientKey.PublicKey().Bytes(),
		), ShouldNotBeNil)

		Convey("It should not store the raw AES key in the artifact", func() {
			encryptedKey, encryptedKeyErr := artifact.EncryptedKey()
			So(encryptedKeyErr != nil || len(encryptedKey) == 0, ShouldBeTrue)
		})

		Convey("It should not silently decode without the private key", func() {
			payload, err := artifact.decryptPayload()
			So(payload, ShouldBeNil)
			So(err, ShouldNotBeNil)
			So(artifact.DecryptPayload(), ShouldBeNil)
		})

		Convey("It should decrypt with the recipient private key", func() {
			payload, err := artifact.DecryptPayloadWithKey(recipientKey)
			So(err, ShouldBeNil)
			So(string(payload), ShouldEqual, `{"secret":true}`)
		})

		Convey("It should authenticate artifact metadata", func() {
			artifact.WithScope("ETH/USD")

			payload, err := artifact.DecryptPayloadWithKey(recipientKey)
			So(payload, ShouldBeNil)
			So(err, ShouldNotBeNil)
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

func TestWithPayloadOverwriteDoesNotGrowTraversal(testingTB *testing.T) {
	Convey("Given a long-lived artifact used as a stage payload buffer", testingTB, func() {
		artifact := Acquire("payload-overwrite", Artifact_Type_json).
			WithRole("measurement").
			WithScope("update").
			Poke([]string{"last"}, "inputs")

		for index := range 5000 {
			payload := []byte(fmt.Sprintf(`{"last":%d,"symbol":"BTC/USD"}`, index))
			So(artifact.WithPayload(payload), ShouldNotBeNil)
			So(string(artifact.DecryptPayload()), ShouldEqual, string(payload))
		}

		Convey("It should preserve metadata and remain traversable", func() {
			So(Peek[[]string](artifact, "inputs"), ShouldResemble, []string{"last"})

			role, err := artifact.Role()
			So(err, ShouldBeNil)
			So(role, ShouldEqual, "measurement")

			_, err = artifact.Payload()
			So(err, ShouldBeNil)
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
			So(reused.HasPayload(), ShouldBeFalse)
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

func BenchmarkWithPayloadOverwrite(b *testing.B) {
	artifact := Acquire("payload-overwrite-bench", Artifact_Type_json).
		Poke([]string{"last"}, "inputs")

	b.ReportAllocs()

	for b.Loop() {
		artifact.WithPayload([]byte(`{"last":100,"symbol":"BTC/USD"}`))
	}
}
