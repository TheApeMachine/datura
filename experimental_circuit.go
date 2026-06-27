package datura

import (
	"errors"
	"fmt"

	"github.com/theapemachine/errnie"
)

var ErrExperimentalProofsDisabled = errors.New(
	"datura: zk proof support is experimental and disabled in production",
)

func InitZKSNARKEngine() error {
	return ErrExperimentalProofsDisabled
}

func GenerateProof(artifact *Artifact) ([]byte, error) {
	if artifact == nil {
		return nil, errnie.Error(errors.New("nil artifact"))
	}

	return nil, errnie.Error(ErrExperimentalProofsDisabled)
}

func VerifyProof(artifact *Artifact, proofBytes []byte) bool {
	_ = artifact
	_ = proofBytes

	return false
}

func BatchVerifyProofs(artifacts []*Artifact) error {
	for _, artifact := range artifacts {
		if artifact == nil {
			return errnie.Error(errors.New("nil artifact in batch"))
		}
	}

	return errnie.Error(fmt.Errorf("%w", ErrExperimentalProofsDisabled))
}
