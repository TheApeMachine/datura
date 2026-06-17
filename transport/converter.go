package transport

import (
	"bytes"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/stream"
)

/*
Converter is a workflow component that transforms data between different formats.
It uses a buffer to store incoming data and processes it through a transformation
function before making it available for reading.

Structure:
  - buffer: Stream buffer that processes incoming data through a transformation function
  - out: Output buffer that holds the transformed data ready for reading
*/
type Converter struct {
	buffer *stream.Buffer
	out    *bytes.Buffer
}

/*
NewConverter creates a new Converter instance with initialized buffers.
The converter is set up to extract and store the "output" metadata value
from incoming artifacts.

Returns:
  - *Converter: A new Converter instance ready to transform data
*/
func NewConverter() *Converter {
	out := bytes.NewBuffer([]byte{})

	conv := &Converter{
		buffer: stream.NewBuffer(func(artifact *datura.Artifact) (err error) {
			// If not Params, write the raw payload
			out.Write([]byte(datura.Peek[string](artifact, "output")))

			return nil
		}),
		out: out,
	}

	return conv
}

/*
Read implements io.Reader. It reads transformed data from the output buffer.

Parameters:
  - p: Byte slice to read data into

Returns:
  - n: Number of bytes read
  - err: Any error that occurred during reading
*/
func (c *Converter) Read(p []byte) (n int, err error) {
	return c.out.Read(p)
}

/*
Write implements io.Writer. It writes data to the input buffer for transformation.

Parameters:
  - p: Byte slice containing data to be transformed

Returns:
  - n: Number of bytes written
  - err: Any error that occurred during writing
*/
func (c *Converter) Write(p []byte) (n int, err error) {
	return c.buffer.Write(p)
}

/*
Close implements io.Closer. It's currently a no-op as the buffers don't require
explicit cleanup.

Returns:
  - error: Always nil as there's nothing to close
*/
func (c *Converter) Close() error {
	return nil
}
