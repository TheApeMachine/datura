package transport

import (
	"context"
	"errors"
	"io"

	"github.com/theapemachine/errnie"
)

/*
Pipeline manages a chain of io.ReadWriter components that
are being streamed into a single sink on Write, while Read
will perform a "fold" operation on the data. A fold is essentially
an io.Copy over two objects (Values) that are designed to interact
when one is written to the other. It uses an io.MultiReader, which
is fully consumed by the Read operation, preventing any duplicate
reads from the same source. To add more readers (when new Values are
emitted for example), use the Update method.
*/
type Pipeline struct {
	ctx     context.Context
	cancel  context.CancelFunc
	err     error
	sink    io.Writer
	readers io.Reader
}

/*
NewPipeline allocates a pipeline that feeds data into the given sink
on Write, while Read will perform a "fold" operation on the data.
*/
func NewPipeline(
	ctx context.Context,
	sink io.Writer,
	rwcs ...io.ReadWriter,
) *Pipeline {
	ctx, cancel := context.WithCancel(ctx)

	readers := make([]io.Reader, 0, len(rwcs))

	for _, rwc := range rwcs {
		readers = append(readers, rwc)
	}

	return &Pipeline{
		ctx:     ctx,
		cancel:  cancel,
		sink:    sink,
		readers: io.MultiReader(readers...),
	}
}

/*
Read reads data from the pipeline.
*/
func (pipeline *Pipeline) Read(p []byte) (n int, err error) {
	errnie.Trace("transport.Pipeline.Read")

	select {
	case <-pipeline.ctx.Done():
		return 0, pipeline.ctx.Err()
	default:
		return pipeline.readers.Read(p)
	}
}

/*
Write feeds the data into the sink.
*/
func (pipeline *Pipeline) Write(p []byte) (n int, err error) {
	errnie.Trace("transport.Pipeline.Write")

	select {
	case <-pipeline.ctx.Done():
		return 0, pipeline.ctx.Err()
	default:
		return pipeline.sink.Write(p)
	}
}

/*
Close closes the pipeline.
*/
func (pipeline *Pipeline) Close() (err error) {
	if closer, ok := pipeline.readers.(io.Closer); ok {
		if err = closer.Close(); err != nil {
			err = errors.Join(err, errnie.Error(err))
		}
	}

	return err
}
