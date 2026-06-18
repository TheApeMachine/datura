package transport

import (
	"io"
	"sync"

	"github.com/smallnest/ringbuffer"
)

var copierPool = sync.Pool{
	New: func() any {
		rb := ringbuffer.New(32 * 1024)
		rb.Reset()
		return rb
	},
}

func Copy(destination io.Writer, origin io.Reader) (int64, error) {
	rb := copierPool.Get().(*ringbuffer.RingBuffer)
	defer copierPool.Put(rb)

	return rb.Copy(destination, origin)
}
