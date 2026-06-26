package datura

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"

	"capnproto.org/go/capnp/v3"
)

func TestZkSnarkProof(t *testing.T) {
	Convey("Given an artifact, generate and verify a zk-SNARK proof", t, func() {
		_, seg, err := capnp.NewMessage(capnp.SingleSegment(nil))
		So(err, ShouldBeNil)

		mockArtifact, err := NewArtifact(seg)
		So(err, ShouldBeNil)

		pseudonymHash := []byte{1, 2, 3, 4}
		merkleRoot := []byte{9, 8, 7, 6}

		err = mockArtifact.SetPseudonym(pseudonymHash)
		So(err, ShouldBeNil)
		err = mockArtifact.SetMerkleRoot(merkleRoot)
		So(err, ShouldBeNil)

		err = InitZKSNARKEngine()
		So(err, ShouldBeNil)

		proof, err := GenerateProof(&mockArtifact)
		So(err, ShouldBeNil)
		So(proof, ShouldNotBeNil)

		valid := VerifyProof(&mockArtifact, proof)
		So(valid, ShouldBeTrue)
	})
}

func TestBatchVerifyProofs(t *testing.T) {
	Convey("Given two artifacts with valid proofs", t, func() {
		err := InitZKSNARKEngine()
		So(err, ShouldBeNil)

		first := buildProofArtifact(t, []byte{1, 1, 1, 1}, []byte{2, 2, 2, 2})
		second := buildProofArtifact(t, []byte{3, 3, 3, 3}, []byte{4, 4, 4, 4})

		Convey("It should verify the batch sequentially", func() {
			err = BatchVerifyProofs([]*Artifact{first, second})
			So(err, ShouldBeNil)
		})
	})
}

func buildProofArtifact(t *testing.T, pseudonymHash, merkleRoot []byte) *Artifact {
	t.Helper()

	_, seg, err := capnp.NewMessage(capnp.SingleSegment(nil))

	if err != nil {
		t.Fatal(err)
	}

	artifact, err := NewRootArtifact(seg)

	if err != nil {
		t.Fatal(err)
	}

	if err = artifact.SetPseudonym(pseudonymHash); err != nil {
		t.Fatal(err)
	}

	if err = artifact.SetMerkleRoot(merkleRoot); err != nil {
		t.Fatal(err)
	}

	proof, err := GenerateProof(&artifact)

	if err != nil {
		t.Fatal(err)
	}

	approvals, err := artifact.NewApprovals(1)

	if err != nil {
		t.Fatal(err)
	}

	approval := approvals.At(0)

	if err = approval.SetZkProof(proof); err != nil {
		t.Fatal(err)
	}

	return &artifact
}

func BenchmarkGenerateProof(b *testing.B) {
	if err := InitZKSNARKEngine(); err != nil {
		b.Fatal(err)
	}

	_, seg, err := capnp.NewMessage(capnp.SingleSegment(nil))

	if err != nil {
		b.Fatal(err)
	}

	artifact, err := NewRootArtifact(seg)

	if err != nil {
		b.Fatal(err)
	}

	_ = artifact.SetPseudonym([]byte{1, 2, 3, 4})
	_ = artifact.SetMerkleRoot([]byte{9, 8, 7, 6})

	b.ResetTimer()

	for b.Loop() {
		proof, proveErr := GenerateProof(&artifact)

		if proveErr != nil || len(proof) == 0 {
			b.Fatal(proveErr)
		}
	}
}
