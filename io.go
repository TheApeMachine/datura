package datura

import (
	"bufio"
	"bytes"
	"io"

	"github.com/theapemachine/errnie"
)

/*
RWCStream buffers an io.ReadWriteCloser with bufio for chunked streaming.
*/
type RWCStream struct {
	artifact    *Artifact
	readBuffer  *bytes.Buffer
	read        *bufio.Reader
	writeBuffer *bytes.Buffer
	write       *bufio.Writer
}

/*
NewRWCStream wraps artifact with bufio for use in Copy, FlipFlop, Pipeline, and Number.
*/
func NewRWCStream(artifact *Artifact) *RWCStream {
	readBuffer := bytes.NewBuffer(nil)
	writeBuffer := bytes.NewBuffer(nil)

	if artifact != nil && artifact.IsValid() {
		readBuffer.Write(artifact.Pack())
	}

	return &RWCStream{
		artifact:    artifact,
		readBuffer:  readBuffer,
		read:        bufio.NewReader(readBuffer),
		writeBuffer: writeBuffer,
		write:       bufio.NewWriter(writeBuffer),
	}
}

func (stream *RWCStream) Read(p []byte) (n int, err error) {
	if stream == nil || stream.artifact == nil || !stream.artifact.IsValid() {
		return 0, io.EOF
	}

	return stream.read.Read(p)
}

func (stream *RWCStream) Write(p []byte) (n int, err error) {
	if stream == nil || stream.artifact == nil {
		return 0, io.ErrClosedPipe
	}

	n, err = stream.write.Write(p)

	if err != nil {
		return n, errnie.Error(errnie.Err(errnie.IO, err.Error(), err))
	}

	if err = stream.write.Flush(); err != nil {
		return n, errnie.Error(errnie.Err(errnie.IO, err.Error(), err))
	}

	stream.commit()

	return n, nil
}

func (stream *RWCStream) Close() error {
	if stream == nil {
		return nil
	}

	if err := stream.write.Flush(); err != nil {
		return errnie.Error(errnie.Err(errnie.IO, err.Error(), err))
	}

	if stream.writeBuffer.Len() == 0 {
		return nil
	}

	if !stream.commit() {
		err := io.ErrUnexpectedEOF

		return errnie.Error(errnie.Err(errnie.IO, err.Error(), err))
	}

	return nil
}

func (stream *RWCStream) commit() bool {
	if stream.writeBuffer.Len() == 0 {
		return true
	}

	if _, err := stream.artifact.Unpack(stream.writeBuffer.Bytes()); err != nil {
		return false
	}

	stream.writeBuffer.Reset()

	return true
}
