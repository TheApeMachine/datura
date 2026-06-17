package transport

import (
	"bytes"
	"io"
	"sync"

	"github.com/theapemachine/errnie"
)

/*
Pump is a workflow component that creates a continuous feedback loop in a pipeline.
It reads from the pipeline's output and feeds it back into its input, creating
an infinite processing cycle that can be stopped via the done channel.
*/
type Pump struct {
	pipeline io.ReadWriteCloser
	done     chan struct{}
	wg       *sync.WaitGroup
}

/*
NewPump creates a new Pump instance that wraps the provided pipeline.
It initializes a done channel for graceful shutdown and sets up a buffer
that continuously processes data through the pipeline using a FlipFlop pattern.

Parameters:
  - pipeline: The io.ReadWriteCloser that will be pumped in a loop

Returns:
  - *Pump: A new Pump instance ready to create a feedback loop
*/
func NewPump(pipeline io.ReadWriteCloser) {
	done := make(chan struct{})
	passthrough := bytes.NewBuffer([]byte{})
	wg := &sync.WaitGroup{}

	var err error

	for {
		select {
		case <-done:
			wg.Done()
			return
		default:
			// FlipFlop creates the feedback loop:
			// 1. Reads from artifact and writes to pipeline
			// 2. Reads the pipeline output and writes back to artifact
			// This creates a continuous cycle of data flow
			if err = NewFlipFlop(passthrough, pipeline); err != nil {
				errnie.Error(err)
				return
			}
		}
	}
}

/*
Read implements the io.Reader interface.
It delegates the read operation to the underlying pipeline.

Parameters:
  - p: Byte slice to read data into

Returns:
  - n: Number of bytes read
  - err: Any error that occurred during reading
*/
func (pump *Pump) Read(p []byte) (n int, err error) {
	return pump.pipeline.Read(p)
}

/*
Write implements the io.Writer interface.
It delegates the write operation to the underlying pipeline.

Parameters:
  - p: Byte slice containing data to write

Returns:
  - n: Number of bytes written
  - err: Any error that occurred during writing
*/
func (pump *Pump) Write(p []byte) (n int, err error) {
	return pump.pipeline.Write(p)
}

/*
Close implements the io.Closer interface.
It signals shutdown via the done channel and closes the underlying pipeline.

Returns:
  - error: Any error that occurred during closure
*/
func (pump *Pump) Close() error {
	close(pump.done)
	pump.wg.Done()
	return pump.pipeline.Close()
}
