package datura

import (
	capnp "capnproto.org/go/capnp/v3"
	"github.com/bytedance/sonic"
	"github.com/theapemachine/errnie"
)

func As[T any](artifact *Artifact) T {
	var v T

	if err := artifact.To(&v); err != nil {
		return v
	}

	return v
}

/*
To is a convenience function to convert the artifact's payload into some
other type by unmarshalling it into the provided type.
*/
func (artifact *Artifact) To(v any) (err error) {
	var payload []byte

	if payload, err = artifact.DecryptPayload(); err != nil {
		return errnie.Error(err, "payload", payload)
	}

	if err = sonic.Unmarshal(payload, v); err != nil {
		return errnie.Error(err, "payload", payload)
	}

	return nil
}

/*
From is a convenience function to set the artifact's payload from some
other type by marshalling it into the artifact's payload.
*/
func (artifact *Artifact) From(v any) (err error) {
	var payload []byte

	if payload, err = sonic.Marshal(v); err != nil {
		return errnie.Error(err, "payload", string(payload))
	}

	artifact.WithPayload(payload)
	return nil
}

/*
Pack the artifact's payload into a byte slice.
*/
func (artifact *Artifact) Pack() (payload []byte, err error) {
	return artifact.Message().MarshalPacked()
}

/*
Unpack the artifact from a packed byte slice.
*/
func (artifact *Artifact) Unpack(payload []byte) (err error) {
	var (
		msg *capnp.Message
		buf Artifact
	)

	if msg, err = capnp.UnmarshalPacked(payload); err != nil {
		return errnie.Error(err)
	}

	if buf, err = ReadRootArtifact(msg); err != nil {
		return errnie.Error(err)
	}

	*artifact = buf
	return nil
}
