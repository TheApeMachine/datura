package datura

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestCryptoSuiteEncryptPayloadDirect(t *testing.T) {
	Convey("Given pre-allocated destination buffers", t, func() {
		cryptoSuite := NewCryptoSuite()
		plaintext := []byte("direct crypto path")
		cipherLen := cryptoSuite.EncryptedPayloadSize(len(plaintext))

		encPayload := make([]byte, cipherLen)
		encKey := make([]byte, aesKeyBytes)
		encPubKey := make([]byte, p256PubKeyBytes)

		err := cryptoSuite.EncryptPayloadDirect(encPayload, encKey, encPubKey, plaintext)
		So(err, ShouldBeNil)

		Convey("It should decrypt back into a reusable destination buffer", func() {
			dst := make([]byte, 0, len(plaintext)+16)
			decrypted, err := cryptoSuite.DecryptPayloadDirect(dst[:0], encPayload, encKey)
			So(err, ShouldBeNil)
			So(decrypted, ShouldResemble, plaintext)
		})
	})
}

func TestArtifactWithPayloadDirect(t *testing.T) {
	Convey("Given a pooled artifact encrypting in place", t, func() {
		artifact := Acquire("crypto-direct", Artifact_Type_json)
		So(artifact, ShouldNotBeNil)

		payload := []byte("pool cycle payload")
		result := artifact.WithPayload(payload)
		So(result, ShouldNotBeNil)

		Convey("It should round-trip through DecryptPayloadInto", func() {
			buffer := make([]byte, 0, len(payload)+16)
			decrypted, err := artifact.DecryptPayloadInto(buffer[:0])
			So(err, ShouldBeNil)
			So(decrypted, ShouldResemble, payload)
		})

		Reset(func() {
			artifact.Release()
		})
	})
}

func BenchmarkEncryptPayloadDirect(b *testing.B) {
	cryptoSuite := NewCryptoSuite()
	payload := []byte("benchmark payload for direct crypto path")
	cipherLen := cryptoSuite.EncryptedPayloadSize(len(payload))
	encPayload := make([]byte, cipherLen)
	encKey := make([]byte, aesKeyBytes)
	encPubKey := make([]byte, p256PubKeyBytes)

	b.ResetTimer()

	for b.Loop() {
		_ = cryptoSuite.EncryptPayloadDirect(encPayload, encKey, encPubKey, payload)
	}
}

func BenchmarkArtifactPrefix(b *testing.B) {
	artifact := Acquire("prefix-bench", Artifact_Type_json).
		WithRole("measurement").
		WithScope("BTC/USD").
		WithDestination("ui")
	defer artifact.Release()

	b.ResetTimer()

	for b.Loop() {
		if len(artifact.Prefix()) == 0 {
			b.Fatal("prefix empty")
		}
	}
}
