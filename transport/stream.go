package transport

import (
	"bufio"
	"io"
)

/*
Stream buffers an io.ReadWriteCloser with bufio for chunked streaming.
*/
type Stream struct {
	buffer *bufio.ReadWriter
	closer io.Closer
}

/*
NewStream wraps rwc with bufio for use in Copy, FlipFlop, Pipeline, and Number.
*/
func NewStream(rwc io.ReadWriteCloser) *Stream {
	return &Stream{
		buffer: bufio.NewReadWriter(
			bufio.NewReader(rwc),
			bufio.NewWriter(rwc),
		),
		closer: rwc,
	}
}

func (stream *Stream) Read(p []byte) (n int, err error) {
	return stream.buffer.Read(p)
}

func (stream *Stream) Write(p []byte) (n int, err error) {
	return stream.buffer.Write(p)
}

/*
Flush delivers buffered writes to the underlying ReadWriteCloser.
Copy defers this so capnp frames arrive in one Write to destinations that need it.
*/
func (stream *Stream) Flush() error {
	return stream.buffer.Flush()
}

func (stream *Stream) Close() error {
	if err := stream.buffer.Flush(); err != nil {
		return err
	}

	return stream.closer.Close()
}
