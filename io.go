package datura

import (
	"bytes"
	"io"
	"sync"

	capnp "capnproto.org/go/capnp/v3"
	"github.com/theapemachine/errnie"
)

var (
	readBuffers  sync.Map // map[*Artifact][]byte
	writeBuffers sync.Map // map[*Artifact]*bytes.Buffer
)

/*
Read implements the io.Reader interface for the Artifact.
*/
func (artifact *Artifact) Read(p []byte) (n int, err error) {
	var buf []byte

	if val, ok := readBuffers.Load(artifact); ok {
		buf = val.([]byte)
	} else {
		buf, err = artifact.Message().MarshalPacked()
		if err != nil {
			return 0, errnie.Error(err)
		}
	}

	n = copy(p, buf)
	remaining := buf[n:]

	if len(remaining) > 0 {
		readBuffers.Store(artifact, remaining)
		return n, nil
	}

	readBuffers.Delete(artifact)
	return n, io.EOF
}

/*
Write implements the io.Writer interface for the Artifact.
*/
func (artifact *Artifact) Write(p []byte) (n int, err error) {
	var buf *bytes.Buffer
	if val, ok := writeBuffers.Load(artifact); ok {
		buf = val.(*bytes.Buffer)
	} else {
		buf = bytes.NewBuffer(nil)
		writeBuffers.Store(artifact, buf)
	}

	buf.Write(p)

	msg, err := capnp.UnmarshalPacked(buf.Bytes())
	if err != nil {
		// If unmarshalling fails, it's typically because we have not received
		// the complete packed stream yet. We return success for this chunk.
		return len(p), nil
	}

	readOnly, err := ReadRootArtifact(msg)
	if err != nil {
		writeBuffers.Delete(artifact)
		return 0, errnie.Error(err)
	}

	_, seg, err := capnp.NewMessage(capnp.MultiSegment(nil))
	if err != nil {
		writeBuffers.Delete(artifact)
		return 0, errnie.Error(err)
	}

	writable, err := NewRootArtifact(seg)
	if err != nil {
		writeBuffers.Delete(artifact)
		return 0, errnie.Error(err)
	}

	if err = capnp.Struct(writable).CopyFrom(capnp.Struct(readOnly)); err != nil {
		writeBuffers.Delete(artifact)
		return 0, errnie.Error(err)
	}

	*artifact = writable
	writeBuffers.Delete(artifact)
	return len(p), nil
}

/*
Close implements the io.Closer interface for the Artifact.
*/
func (artifact *Artifact) Close() error {
	errnie.Debug("artifact.Close")
	readBuffers.Delete(artifact)
	writeBuffers.Delete(artifact)
	artifact = nil
	return nil
}
