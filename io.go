package datura

import (
	"io"

	capnp "capnproto.org/go/capnp/v3"
	"github.com/theapemachine/errnie"
)

/*
Read implements the io.Reader interface for the Artifact.
*/
func (artifact *Artifact) Read(p []byte) (n int, err error) {
	buf, err := artifact.Message().MarshalPacked()

	if err != nil {
		return 0, errnie.Error(err)
	}

	n = copy(p, buf)

	if n < len(buf) {
		return n, errnie.Error(io.ErrShortBuffer)
	}

	return n, io.EOF
}

/*
Write implements the io.Writer interface for the Artifact.
*/
func (artifact *Artifact) Write(p []byte) (n int, err error) {
	msg, err := capnp.UnmarshalPacked(p)

	if err != nil {
		return 0, errnie.Error(err, "p", string(p))
	}

	readOnly, err := ReadRootArtifact(msg)

	if err != nil {
		return 0, errnie.Error(err)
	}

	_, seg, err := capnp.NewMessage(capnp.MultiSegment(nil))

	if err != nil {
		return 0, errnie.Error(err)
	}

	writable, err := NewRootArtifact(seg)

	if err != nil {
		return 0, errnie.Error(err)
	}

	if err = capnp.Struct(writable).CopyFrom(capnp.Struct(readOnly)); err != nil {
		return 0, errnie.Error(err)
	}

	*artifact = writable
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
