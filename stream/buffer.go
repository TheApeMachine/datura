package stream

import (
	"errors"
	"io"

	capnp "capnproto.org/go/capnp/v3"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/errnie"
)

/*
Buffer provides a bidirectional streaming mechanism with encoding/decoding capabilities.
It connects a sender and receiver through pipes, handling data transformations using a pluggable codec system.
Buffer implements io.Reader, io.Writer, and io.Closer interfaces to support standard streaming operations.
*/
type Buffer struct {
	artifact     *datura.Artifact
	pendingWrite []byte
	fn           func(*datura.Artifact) error
}

/*
NewBuffer creates a new Buffer with the specified receiver, sender, and handler function.
It sets up the necessary pipe connections and defaults to Gob encoding.

Parameters:
  - fn: A function that processes the event

Returns a configured Buffer instance that's ready to use.
*/
func NewBuffer(fn func(*datura.Artifact) error) *Buffer {
	return &Buffer{
		artifact: datura.Acquire("buffer", datura.Artifact_Type_json),
		fn:       fn,
	}
}

/*
Read implements the io.Reader interface.
It reads data from the pipe reader, which contains data encoded by the Write method.

Parameters:
  - p: Byte slice where read data will be stored

Returns:
  - n: Number of bytes read
  - err: Any error encountered during reading
*/
func (buffer *Buffer) Read(p []byte) (n int, err error) {
	if buffer.artifact == nil {
		return 0, io.EOF
	}

	n, err = buffer.artifact.Read(p)

	if n > 0 && err == io.EOF {
		return n, nil
	}

	if err != nil {
		if err == io.EOF {
			return n, err
		}

		return n, errnie.Error(err, "p", string(p))
	}

	if n == 0 {
		return 0, io.EOF
	}

	return n, nil
}

/*
Write implements the io.Writer interface.
It decodes incoming data into the receiver, applies the handler function,
then asynchronously encodes the sender's data back into the pipe.

Parameters:
  - p: Byte slice containing data to be written

Returns:
  - n: Number of bytes written
  - err: Any error encountered during writing
*/
func (buffer *Buffer) Write(p []byte) (n int, err error) {
	if len(p) == 0 {
		return 0, errnie.Error(errors.New("empty input"))
	}

	if buffer.artifact == nil {
		return 0, errnie.Error(errors.New("buffer artifact is nil"))
	}

	buffer.pendingWrite = append(buffer.pendingWrite, p...)

	if _, err = capnp.Unmarshal(buffer.pendingWrite); err != nil {
		return len(p), nil
	}

	if _, err = buffer.artifact.Write(buffer.pendingWrite); err != nil {
		return 0, errnie.Error(err, "p", string(p))
	}

	buffer.pendingWrite = nil

	if err = buffer.fn(buffer.artifact); err != nil {
		return 0, errnie.Error(err)
	}

	return len(p), nil
}

/*
Close implements the io.Closer interface.
It properly closes both the pipe reader and writer to prevent resource leaks.

Returns any error encountered during the closing process.
*/
func (buffer *Buffer) Close() error {
	buffer.pendingWrite = nil

	return buffer.artifact.Close()
}
