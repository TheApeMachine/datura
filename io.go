package datura

import (
	"io"

	capnp "capnproto.org/go/capnp/v3"
	"github.com/theapemachine/errnie"
)

/*
Read implements the io.Reader interface for the Artifact.
It marshals the entire artifact into the provided byte slice.
*/
func (artifact *Artifact) Read(p []byte) (n int, err error) {
	buf, err := artifact.Message().MarshalPacked()

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
	var (
		msg *capnp.Message
		buf Artifact
	)

	if msg, err = capnp.UnmarshalPacked(p); err != nil {
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
	artifact.Release()
	return nil
}
