package datura

import (
	"bytes"
	"testing"

	. "github.com/smartystreets/goconvey/convey"

	"capnproto.org/go/capnp/v3"
	"github.com/consensys/gnark-crypto/ecc/bn254"
	"github.com/consensys/gnark-crypto/ecc/bn254/fr"
	"github.com/consensys/gnark/backend/groth16"
	"github.com/consensys/gnark/frontend"
	"github.com/consensys/gnark/frontend/cs/r1cs"
)

func TestZkSnarkProof(t *testing.T) {
	Convey("Given an artifact, generate and verify a zk-SNARK proof", t, func() {
		// Create a new Cap'n Proto message and root artifact
		_, seg, err := capnp.NewMessage(capnp.SingleSegment(nil))
		So(err, ShouldBeNil)
		mockArtifact, err := NewArtifact(seg)
		So(err, ShouldBeNil)

		// Mock pseudonym hash and Merkle root (fake test values)
		pseudonymHash := []byte{1, 2, 3, 4}
		merkleRoot := []byte{9, 8, 7, 6}

		// Ensure artifact has SetPseudonymHash and SetMerkleRoot methods
		err = mockArtifact.SetPseudonymHash(pseudonymHash)
		So(err, ShouldBeNil)
		err = mockArtifact.SetMerkleRoot(merkleRoot)
		So(err, ShouldBeNil)

		// Generate proving and verification keys
		var circuit AuthCircuit
		modulus := fr.Modulus()
		r1cs, err := frontend.Compile(modulus, r1cs.NewBuilder, &circuit)
		So(err, ShouldBeNil)

		pk, vk, err := groth16.Setup(r1cs)
		So(err, ShouldBeNil)

		// Generate zk-SNARK proof
		proof, err := GenerateProof(mockArtifact, pk)
		So(err, ShouldBeNil)
		So(proof, ShouldNotBeNil)

		// Deserialize proof before verifying
		proofStruct := groth16.NewProof(bn254.ID)
		buf := bytes.NewBuffer(proof)
		_, err = proofStruct.ReadFrom(buf)
		So(err, ShouldBeNil)

		// Verify proof
		valid := VerifyProof(mockArtifact, proof, vk)
		So(valid, ShouldBeTrue)
	})
}
