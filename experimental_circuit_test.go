package datura

import (
	"errors"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestExperimentalProofsFailClosed(t *testing.T) {
	Convey("Given production proof support is disabled", t, func() {
		artifact := Acquire("proof-test", Artifact_Type_json)
		So(artifact.SetPseudonym([]byte{1, 2, 3, 4}), ShouldBeNil)
		So(artifact.SetMerkleRoot([]byte{1, 2, 3, 4}), ShouldBeNil)

		Convey("It should not initialize a production verifier", func() {
			err := InitZKSNARKEngine()
			So(errors.Is(err, ErrExperimentalProofsDisabled), ShouldBeTrue)
		})

		Convey("It should not generate a proof from an empty circuit", func() {
			proof, err := GenerateProof(artifact)
			So(proof, ShouldBeNil)
			So(errors.Is(err, ErrExperimentalProofsDisabled), ShouldBeTrue)
		})

		Convey("It should reject any proof bytes", func() {
			So(VerifyProof(artifact, []byte("proof")), ShouldBeFalse)
		})

		Convey("It should fail batch verification", func() {
			approvals, err := artifact.NewApprovals(1)
			So(err, ShouldBeNil)
			So(approvals.At(0).SetZkProof([]byte("proof")), ShouldBeNil)

			err = BatchVerifyProofs([]*Artifact{artifact})
			So(errors.Is(err, ErrExperimentalProofsDisabled), ShouldBeTrue)
		})
	})
}
