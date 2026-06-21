package datura

import (
	"capnproto.org/go/capnp/v3"
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
	payload, err := artifact.decryptPayload()

	if err != nil {
		errnie.Error(errnie.Err(errnie.Validation, "payload unmarshalling failed", err))

		return err
	}

	if errnie.Error(sonic.Unmarshal(payload, v)) != nil {
		errnie.Error(errnie.Err(errnie.Validation, "payload unmarshalling failed", err))
	}

	return nil
}

/*
From is a convenience function to set the artifact's payload from some
other type by marshalling it into the artifact's payload.
*/
func (artifact *Artifact) From(v any) (err error) {
	payload := errnie.Does(func() ([]byte, error) {
		return sonic.Marshal(v)
	}).Or(func(err error) {
		errnie.Error(errnie.Err(errnie.Validation, "payload marshalling failed", err))
	}).Value()

	artifact.WithPayload(payload)

	return nil
}

func (artifact *Artifact) Pack() []byte {
	return errnie.Does(func() ([]byte, error) {
		return artifact.MarshalPacked()
	}).Or(func(err error) {
		errnie.Error(errnie.Err(errnie.Validation, "payload marshalling failed", err))
	}).Value()
}

func (artifact *Artifact) Unpack(p []byte) (n int, err error) {
	msg := errnie.Does(func() (*capnp.Message, error) {
		return capnp.UnmarshalPacked(p)
	}).Or(func(err error) {
		errnie.Error(errnie.Err(errnie.Validation, "payload unmarshalling failed", err))
	}).Value()

	readOnly := errnie.Does(func() (Artifact, error) {
		return ReadRootArtifact(msg)
	}).Or(func(err error) {
		errnie.Error(errnie.Err(errnie.Validation, "payload unmarshalling failed", err))
	}).Value()

	writable, err := restoreWritable(readOnly)

	if err != nil {
		errnie.Error(errnie.Err(errnie.Validation, "payload unmarshalling failed", err))

		return 0, err
	}

	*artifact = writable
	invalidateAttributesCache(artifact)

	return len(p), nil
}

/*
restoreWritable copies a deserialized artifact into a writable message arena.
Wire deserialization binds segments into a read-only arena; attribute mutation
requires an allocator-backed arena.
*/
func restoreWritable(readOnly Artifact) (Artifact, error) {
	_, seg, err := capnp.NewMessage(capnp.MultiSegment(nil))

	if err != nil {
		return Artifact{}, err
	}

	writable, err := NewRootArtifact(seg)

	if err != nil {
		return Artifact{}, err
	}

	if err = capnp.Struct(writable).CopyFrom(capnp.Struct(readOnly)); err != nil {
		return Artifact{}, err
	}

	return writable, nil
}
