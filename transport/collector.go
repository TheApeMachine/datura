package transport

import (
	"context"

	"github.com/smallnest/ringbuffer"
	"github.com/theapemachine/datura"
)

/*
Collector is a growable byte buffer used as an io.Writer destination for
io.Copy when *bytes.Buffer must be avoided: *bytes.Buffer implements
io.ReaderFrom, so io.Copy delegates to ReadFrom and the source may see
small first reads (bytes.MinRead), which breaks framed readers that require
each Read(p) to use at least one full wire frame (e.g. gossip.Conn).

Collector only implements Write (not ReaderFrom), so io.Copy uses the
stdlib copy loop with a large internal buffer. Len and Next mirror
bytes.Buffer semantics for pending byte count and consuming the next n
bytes from the front.
*/
type Collector struct {
	ctx    context.Context
	cancel context.CancelFunc
	err    error
	rb     *ringbuffer.RingBuffer
	pr     *ringbuffer.PipeReader
	pw     *ringbuffer.PipeWriter
	buf    chan *datura.Artifact
}

// NewCollector returns an empty collector; initialCap hints preallocation
// (e.g. core.Cfg.Value.Bytes).
func NewCollector() *Collector {
	ctx, cancel := context.WithCancel(context.Background())
	rb := ringbuffer.New(1024)
	pr, pw := rb.Pipe()

	return &Collector{
		ctx:    ctx,
		cancel: cancel,
		rb:     rb,
		pr:     pr,
		pw:     pw,
		buf:    make(chan *datura.Artifact, 64),
	}
}

// Len returns the number of pending bytes (like bytes.Buffer.Len).
func (collector *Collector) Len() int {
	if collector == nil {
		return 0
	}

	return len(collector.buf)
}

// Next returns the next n bytes and removes them from the front. If fewer
// than n bytes are buffered, it returns nil.
func (collector *Collector) Next(n int) chan *datura.Artifact {
	select {
	case <-collector.ctx.Done():
		return nil
	default:
		return collector.buf
	}
}

// Read consumes up to len(p) bytes from the front of the buffer.
func (collector *Collector) Read(p []byte) (n int, err error) {
	select {
	case <-collector.ctx.Done():
		return 0, collector.ctx.Err()
	default:
		return collector.pr.Read(p)
	}
}

/*
Write the given frame to the collector, if it has a RESOLVED status,
while also acting as a simple throughput pipe, so it doesn't block
anything.
*/
func (collector *Collector) Write(p []byte) (n int, err error) {
	select {
	case <-collector.ctx.Done():
		return 0, collector.ctx.Err()
	default:
		value := datura.Acquire("collector", datura.Artifact_Type_json)

		collector.buf <- value

		return value.Write(p)
	}
}

/*
Close closes the collector.
*/
func (collector *Collector) Close() error {
	if collector == nil {
		return nil
	}

	if collector.cancel != nil {
		collector.cancel()
	}

	if collector.pw != nil {
		_ = collector.pw.Close()
	}

	return collector.err
}

/*
Error implements the error interface.
*/
func (collector *Collector) Error() string {
	if collector == nil || collector.err == nil {
		return ""
	}

	return collector.err.Error()
}
