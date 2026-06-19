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
	errnie.Debug("datura.To")

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
	errnie.Debug("datura.From")

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
		return artifact.Message().MarshalPacked()
	}).Or(func(err error) {
		errnie.Error(errnie.Err(errnie.Validation, "payload marshalling failed", err))
	}).Value()
}

func (artifact *Artifact) Unpack(data []byte) error {
	return artifact.UnpackWire(data, true)
}

/*
UnpackWire decodes either packed or unpacked capnp artifact wire into artifact.
When logErrors is false, decode failures are returned without logging.
*/
func (artifact *Artifact) UnpackWire(data []byte, logErrors bool) error {
	if len(data) == 0 {
		if logErrors {
			return errnie.Error(errnie.Err(
				errnie.Validation,
				"payload unmarshalling failed",
				errnie.Err(errnie.Validation, "artifact wire is empty", nil),
			))
		}

		return errnie.Err(errnie.Validation, "artifact wire is empty", nil)
	}

	if msg, err := capnp.UnmarshalPacked(data); err == nil {
		if buf, readErr := ReadRootArtifact(msg); readErr == nil {
			*artifact = buf

			return nil
		}
	}

	if _, err := artifact.Write(data); err == nil {
		return nil
	}

	if logErrors {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"payload unmarshalling failed",
			errnie.Err(errnie.Validation, "artifact wire is not capnp", nil),
		))
	}

	return errnie.Err(errnie.Validation, "artifact wire is not capnp", nil)
}
