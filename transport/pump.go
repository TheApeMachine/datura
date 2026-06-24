package transport

import (
	"context"
	"io"
)

/*
Pump is a workflow component that creates a continuous feedback loop in a pipeline.
It reads from the pipeline's output and feeds it back into its input, creating
an infinite processing cycle that can be stopped via the done channel.
*/
type Pump struct {
	ctx      context.Context
	cancel   context.CancelFunc
	pipeline io.ReadWriteCloser
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
func NewPump(ctx context.Context, pipeline io.ReadWriteCloser) *Pump {
	ctx, cancel := context.WithCancel(ctx)

	pump := &Pump{
		ctx:      ctx,
		cancel:   cancel,
		pipeline: pipeline,
	}

	go func() {
		defer cancel()

		for {
			select {
			case <-ctx.Done():
				return
			default:
				NewFlipFlop(pump.pipeline, pump.pipeline)
			}
		}
	}()

	return pump
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
	if pump.cancel != nil {
		pump.cancel()
	}

	if pump.pipeline != nil {
		return pump.pipeline.Close()
	}

	return nil
}
