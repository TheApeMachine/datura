package datura

import (
	"sync"
	"time"

	capnp "capnproto.org/go/capnp/v3"
	"github.com/google/uuid"
	"github.com/theapemachine/errnie"
)

var artifactPool = sync.Pool{
	New: func() any {
		arena := capnp.SingleSegment(nil)

		_, seg, err := capnp.NewMessage(arena)

		if errnie.Error(err) != nil {
			return nil
		}

		artifact, err := NewRootArtifact(seg)

		if errnie.Error(err) != nil {
			return nil
		}

		artifact.SetUuid(uuid.NewString())
		artifact.SetTimestamp(time.Now().UnixNano())

		return artifact
	},
}

func Acquire(
	origin string,
	artifactType Artifact_Type,
) *Artifact {
	artifact := artifactPool.Get().(*Artifact)

	if errnie.Error(artifact.SetOrigin(origin)) != nil {
		return nil
	}

	artifact.SetType(artifactType)

	return artifact
}

func (artifact *Artifact) Release() {
	artifactPool.Put(artifact)
}
