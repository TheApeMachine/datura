package datura

import (
	"io"

	capnp "capnproto.org/go/capnp/v3"
	"github.com/theapemachine/errnie"
)

/*
Append the provided bytes to the artifact Payload.
*/
func (artifact *Artifact) AppendPayload(p []byte) error {
	payload := errnie.Does(func() ([]byte, error) {
		return artifact.DecryptPayload()
	}).Or(func(err error) {
		errnie.Error(err)
	}).Value()

	payload = append(payload, p...)
	artifact.WithPayload(payload)

	return nil
}

/*
Read implements the io.Reader interface for the Artifact.
It marshals the entire artifact into the provided byte slice.
*/
func (artifact *Artifact) Read(p []byte) (n int, err error) {
	errnie.Debug("artifact.Read")

	buf, err := artifact.Message().Marshal()

	if err != nil {
		return n, errnie.Error(err, "p", string(p))
	}

	n = copy(p, buf)

	if n < len(buf) {
		return n, errnie.Error(io.ErrShortBuffer)
	}

	return n, io.EOF
}

/*
Write implements the io.Writer interface for the Artifact.
It unmarshals the provided bytes into the current artifact.
*/
func (artifact *Artifact) Write(p []byte) (n int, err error) {
	errnie.Debug("artifact.Write")

	var (
		msg *capnp.Message
		buf Artifact
	)

	if msg, err = capnp.Unmarshal(p); err != nil {
		return 0, errnie.Error(err, "p", string(p))
	}

	if buf, err = ReadRootArtifact(msg); err != nil {
		return 0, errnie.Error(err)
	}

	*artifact = buf
	return len(p), nil
}

/*
Close implements the io.Closer interface for the Artifact.
*/
func (artifact *Artifact) Close() error {
	errnie.Debug("artifact.Close")
	artifact = nil
	return nil
}
