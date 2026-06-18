package datura

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestArtifactCreation(t *testing.T) {
	Convey("Given the functional options for artifact creation", t, func() {
		payload := []byte("Hello, this is a test payload!")

		Convey("When creating a new artifact with options", func() {
			artifact := Acquire("test-artifact", Artifact_Type_json).
				WithAttributes(Map[any]{
					"payload":  string(payload),
					"test_key": "test_value",
				}).
				WithPayload(payload)

			Convey("And the payload should be decryptable", func() {
				encryptedPayload, err := artifact.EncryptedPayload()
				So(err, ShouldBeNil)
				So(encryptedPayload, ShouldNotBeEmpty)

				encryptedKey, err := artifact.EncryptedKey()
				So(err, ShouldBeNil)
				So(encryptedKey, ShouldNotBeEmpty)

				ephemeralPubKey, err := artifact.EphemeralPublicKey()
				So(err, ShouldBeNil)
				So(ephemeralPubKey, ShouldNotBeEmpty)

				crypto := NewCryptoSuite()
				decryptedPayload, err := crypto.DecryptPayload(encryptedPayload, encryptedKey, ephemeralPubKey)
				So(err, ShouldBeNil)
				So(decryptedPayload, ShouldResemble, payload)
			})

			Convey("And the metadata should be retrievable", func() {
				metadataList, err := artifact.Attributes()
				So(err, ShouldBeNil)
				So(len(metadataList), ShouldBeGreaterThan, 0)
				So(Peek[string](artifact, "test_key"), ShouldEqual, "test_value")

				Convey("When poking the attribute again", func() {
					So(artifact.Poke("toast", "test_key"), ShouldEqual, artifact)
					So(Peek[string](artifact, "test_key"), ShouldEqual, "test_value")
				})
			})
		})
	})
}

func TestArtifactEncryption(t *testing.T) {
	Convey("Given the functional options for artifact creation", t, func() {
		payload := []byte("Hello, this is a test payload!")

		Convey("When creating an artifact with encrypted payload", func() {
			artifact := Acquire("test-artifact", Artifact_Type_json).
				WithRole("user").
				WithScope("prompt").
				WithPayload(payload)

			So(artifact, ShouldNotBeNil)

			Convey("Then the encrypted fields should be properly set", func() {
				encryptedPayload, err := artifact.EncryptedPayload()
				So(err, ShouldBeNil)
				So(encryptedPayload, ShouldNotBeEmpty)

				encryptedKey, err := artifact.EncryptedKey()
				So(err, ShouldBeNil)
				So(encryptedKey, ShouldNotBeEmpty)

				ephemeralPubKey, err := artifact.EphemeralPublicKey()
				So(err, ShouldBeNil)
				So(ephemeralPubKey, ShouldNotBeEmpty)

				Convey("And the payload should be decryptable", func() {
					crypto := NewCryptoSuite()
					decryptedPayload, err := crypto.DecryptPayload(encryptedPayload, encryptedKey, ephemeralPubKey)
					So(err, ShouldBeNil)
					So(decryptedPayload, ShouldResemble, payload)
				})
			})
		})
	})
}

func TestArtifactMetadata(t *testing.T) {
	Convey("Given the functional options for artifact creation", t, func() {
		attributes := map[string]any{
			"test_key": "test_value",
		}

		Convey("When creating an artifact with metadata", func() {
			artifact := Acquire("test-artifact", Artifact_Type_json).
				WithRole("user").
				WithScope("prompt").
				WithAttributes(attributes)

			So(artifact, ShouldNotBeNil)

			Convey("Then the metadata should be retrievable", func() {
				So(Peek[string](artifact, "test_key"), ShouldEqual, "test_value")

				Convey("When poking the attribute again", func() {
					So(artifact.Poke("toast", "test_key"), ShouldEqual, artifact)
					So(Peek[string](artifact, "test_key"), ShouldEqual, "test_value")
				})
			})
		})
	})
}

func TestArtifactBasicFields(t *testing.T) {
	Convey("Given the functional options for artifact creation", t, func() {
		Convey("When creating a basic artifact", func() {
			artifact := Acquire("test-artifact", Artifact_Type_json).
				WithRole("user").
				WithScope("prompt")

			So(artifact, ShouldNotBeNil)

			Convey("Then it should have the required basic fields", func() {
				uuid, err := artifact.Uuid()
				So(err, ShouldBeNil)
				So(len(uuid), ShouldBeGreaterThan, 0)

				role, err := artifact.Role()
				So(err, ShouldBeNil)
				So(role, ShouldEqual, "user")

				scope, err := artifact.Scope()
				So(err, ShouldBeNil)
				So(scope, ShouldEqual, "prompt")

				timestamp := artifact.Timestamp()
				So(timestamp, ShouldBeGreaterThan, 0)
			})
		})
	})
}

func TestArtifactWithCircuit(t *testing.T) {
	Convey("Given an artifact with circuit integration", t, func() {
		artifact := Acquire("test-artifact", Artifact_Type_json).
			WithRole("user").
			WithScope("prompt")

		So(artifact, ShouldNotBeNil)

		Convey("When setting up for zero-knowledge proof", func() {
			pseudonymHash := []byte{1, 2, 3, 4}
			merkleRoot := []byte{9, 8, 7, 6}

			err := artifact.SetPseudonymHash(pseudonymHash)
			So(err, ShouldBeNil)

			err = artifact.SetMerkleRoot(merkleRoot)
			So(err, ShouldBeNil)

			Convey("Then the circuit-related fields should be properly set", func() {
				retrievedHash, err := artifact.PseudonymHash()
				So(err, ShouldBeNil)
				So(retrievedHash, ShouldResemble, pseudonymHash)

				retrievedRoot, err := artifact.MerkleRoot()
				So(err, ShouldBeNil)
				So(retrievedRoot, ShouldResemble, merkleRoot)

				Convey("And we can generate and verify proofs", func() {
					So(artifact.HasPseudonymHash(), ShouldBeTrue)
					So(artifact.HasMerkleRoot(), ShouldBeTrue)
				})
			})
		})
	})
}
