package datura

import (
	"errors"

	capnp "capnproto.org/go/capnp/v3"
	"github.com/theapemachine/errnie"
)

/*
UnpackArtifact inflates a pooled artifact from packed wire bytes.
*/
func UnpackArtifact(packedBytes []byte) (*Artifact, error) {
	msg, err := capnp.UnmarshalPacked(packedBytes)

	if err != nil {
		return nil, errnie.Error(err)
	}

	inbound, err := ReadRootArtifact(msg)

	if err != nil {
		return nil, errnie.Error(err)
	}

	pooled := artifactPool.Get()

	if pooled == nil {
		return nil, errnie.Error(errors.New("artifact pool exhausted"))
	}

	pa, ok := pooled.(*pooledArtifact)

	if !ok {
		return nil, errnie.Error(errors.New("artifact pool type mismatch"))
	}

	resetArtifactStreamState(&pa.Artifact)

	segment, err := pa.Artifact.Message().Reset(capnp.SingleSegment(nil))

	if err != nil {
		return nil, errnie.Error(err)
	}

	writable, err := NewRootArtifact(segment)

	if err != nil {
		return nil, errnie.Error(err)
	}

	if err = capnp.Struct(writable).CopyFrom(capnp.Struct(inbound)); err != nil {
		return nil, errnie.Error(err)
	}

	pa.Artifact = writable
	artifactPoolIndex.Store(&pa.Artifact, pa)

	return &pa.Artifact, nil
}
