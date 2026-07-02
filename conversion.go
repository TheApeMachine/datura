package datura

import (
	"io"

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
		return err
	}

	if err := sonic.Unmarshal(payload, v); err != nil {
		return err
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
		return artifact.Message().MarshalPacked()
	}).Or(func(err error) {
		errnie.Error(errnie.Err(errnie.Validation, "payload marshalling failed", err))
	}).Value()
}

/*
PackInto copies the packed Cap'n Proto frame into p.
It is a complete-frame helper, not an io.Reader implementation.
*/
func (artifact *Artifact) PackInto(p []byte) (n int, err error) {
	wire := artifact.Pack()
	n = copy(p, wire)

	if n < len(wire) {
		return n, io.ErrShortBuffer
	}

	return n, io.EOF
}

func (artifact *Artifact) Unpack(p []byte) (n int, err error) {
	msg, err := capnp.UnmarshalPacked(p)

	if err != nil {
		return 0, err
	}

	decoded, err := ReadRootArtifact(msg)

	if err != nil {
		return 0, err
	}

	writable, err := cloneDecodedArtifact(decoded)

	if err != nil {
		return 0, err
	}

	*artifact = *writable

	return len(p), nil
}

/*
Clone copies an artifact into a new Cap'n Proto message.
*/
func (artifact *Artifact) Clone() (*Artifact, error) {
	if artifact == nil || !artifact.IsValid() {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation, "artifact clone failed", nil,
		))
	}

	cloned, err := cloneDecodedArtifact(*artifact)

	if err == nil {
		return cloned, nil
	}

	wire, err := artifact.Message().MarshalPacked()

	if err != nil {
		return nil, err
	}

	cloned = &Artifact{}

	if _, err = cloned.Unpack(wire); err != nil {
		return nil, err
	}

	return cloned, nil
}

func cloneDecodedArtifact(source Artifact) (*Artifact, error) {
	msg, _, err := capnp.NewMessage(capnp.SingleSegment(nil))

	if err != nil {
		return nil, err
	}

	if err = msg.SetRoot(source.ToPtr()); err != nil {
		return nil, err
	}

	cloned, err := ReadRootArtifact(msg)

	if err != nil {
		return nil, err
	}

	return &cloned, nil
}
