package transport

import (
	"bytes"
	"context"
	"io"

	"github.com/bytedance/sonic"
	"github.com/smallnest/ringbuffer"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/errnie"
)

/*
Output is a termination boundary for transport primitives.
*/
type Output struct {
	ctx    context.Context
	cancel context.CancelFunc
	err    error
	rb     *ringbuffer.RingBuffer
}

func NewOutput[T any](pipeline io.ReadWriteCloser) T {
	rb := ringbuffer.New(32 * 1024)
	var typed T

	switch any(typed).(type) {
	case *datura.Artifact:
		out := datura.Acquire("transport", datura.Artifact_Type_json)

		errnie.Does(func() (int64, error) {
			return rb.Copy(out, pipeline)
		}).Or(func(err error) {
			errnie.Error(errnie.Err(
				errnie.IO, "failed to copy pipeline to output", err,
			))
		}).Value()

		return any(out).(T)
	default:
		out := bytes.NewBuffer(make([]byte, 0, 32*1024))

		errnie.Does(func() (int64, error) {
			return rb.Copy(out, pipeline)
		}).Or(func(err error) {
			errnie.Error(errnie.Err(
				errnie.IO, "failed to copy pipeline to output", err,
			))
		}).Value()

		errnie.Error(sonic.Unmarshal(out.Bytes(), &typed))
	}

	return typed
}
