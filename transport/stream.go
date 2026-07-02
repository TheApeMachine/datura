package transport

import (
	"bytes"
	"io"
)

/*
Stream buffers writes to an io.ReadWriteCloser until Flush.
*/
type Stream struct {
	writer bytes.Buffer
	closer io.ReadWriteCloser
}

/*
NewStream wraps rwc for use in Copy, FlipFlop, Pipeline, and Number.
*/
func NewStream(rwc io.ReadWriteCloser) *Stream {
	return &Stream{
		closer: rwc,
	}
}

func (stream *Stream) Read(p []byte) (n int, err error) {
	return stream.closer.Read(p)
}

func (stream *Stream) Write(p []byte) (n int, err error) {
	return stream.writer.Write(p)
}

/*
Flush delivers buffered writes to the underlying ReadWriteCloser.
Copy defers this so capnp frames arrive in one Write to destinations that need it.
*/
func (stream *Stream) Flush() error {
	if stream.writer.Len() > 0 {
		if _, err := stream.closer.Write(stream.writer.Bytes()); err != nil {
			return err
		}

		stream.writer.Reset()
	}

	if flusher, ok := stream.closer.(interface{ Flush() error }); ok {
		return flusher.Flush()
	}

	return nil
}

func (stream *Stream) Close() error {
	if err := stream.Flush(); err != nil {
		return err
	}

	return stream.closer.Close()
}
