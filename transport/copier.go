package transport

import (
	"io"
	"sync"

	"github.com/smallnest/ringbuffer"
)

var copierPool = sync.Pool{
	New: func() any {
		return ringbuffer.New(32 * 1024)
	},
}

func Copy(destination io.Writer, origin io.Reader) (n int64, err error) {
	rb := copierPool.Get().(*ringbuffer.RingBuffer)
	rb.Reset()

	defer func() {
		copierPool.Put(rb)

		if stream, ok := destination.(*Stream); ok {
			if flushErr := stream.Flush(); flushErr != nil && err == nil {
				err = flushErr
			}
		}
	}()

	if n, err = rb.Copy(destination, origin); n == 0 && err == nil {
		return 0, io.EOF
	}

	return n, err
}
