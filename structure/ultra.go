//go:build hft

package structure

import (
	"sync/atomic"
	"unsafe"
)

// 1. Force tight memory alignment and layout.
// Standard Go structs can have internal padding gaps. We tightly pack types
// and cache-pad the entire Event array layout manually if needed.
type OrderEvent struct {
	OrderID   uint64  // 8 bytes
	Price     uint64  // 8 bytes
	Timestamp int64   // 8 bytes
	Quantity  uint32  // 4 bytes
	Side      byte    // 1 byte
	_         [3]byte // Explicit padding to round up to 32 bytes (perfect sub-cache line fraction)
}

// 2. Pure 64-byte Cache-Line Isolation to eliminate False Sharing completely.
type PaddedSequence struct {
	_p1, _p2, _p3, _p4, _p5, _p6, _p7      int64
	Value                                  int64
	_p8, _p9, _p10, _p11, _p12, _p13, _p14 int64 // Rear padding for dual-core isolation
}

type UltraRingBuffer struct {
	// We use an unsafe pointer to the underlying array memory block
	// to bypass Go's slice boundary checks runtime overhead.
	arrayPtr  unsafe.Pointer
	mask      int64
	_padding1 [8]uint64

	ProducerSequence PaddedSequence
	_padding2        [8]uint64

	ConsumerSequence PaddedSequence
}

func NewUltraRingBuffer(size int64) *UltraRingBuffer {
	// Size must be power of 2
	buffer := make([]OrderEvent, size)

	return &UltraRingBuffer{
		// Grab the raw memory address of the first element
		arrayPtr: unsafe.Pointer(&buffer[0]),
		mask:     size - 1,
	}
}

//go:nosplit
//go:nowritebarrier
func (rb *UltraRingBuffer) Publish(orderID, price uint64, qty uint32, side byte, ts int64) {
	// 1. Direct atomic read without locks
	nextWrite := rb.ProducerSequence.Value + 1
	wrapPoint := nextWrite - (rb.mask + 1)

	// 2. Aggressive CPU Spinning (No Gosched() or context switching)
	// For HFT, yielding the CPU to the Go scheduler is a 2-5 microsecond penalty.
	// We spin purely in assembly or a tight loop.
	for atomic.LoadInt64(&rb.ConsumerSequence.Value) < wrapPoint {
		// Pure hardware spin. Don't yield execution.
	}

	// 3. Unsafe Pointer Arithmetic (Bypasses bounds checking entirely)
	slot := nextWrite & rb.mask
	elementOffset := uintptr(slot) * unsafe.Sizeof(OrderEvent{})

	// Directly mutate the raw hardware memory address
	event := (*OrderEvent)(unsafe.Pointer(uintptr(rb.arrayPtr) + elementOffset))

	event.OrderID = orderID
	event.Price = price
	event.Quantity = qty
	event.Side = side
	event.Timestamp = ts

	// 4. Release Barrier: Atomic Store releases the visibility to the consumer core
	atomic.StoreInt64(&rb.ProducerSequence.Value, nextWrite)
}

//go:nosplit
func (rb *UltraRingBuffer) ConsumeBatch(limit int64, handler func(*OrderEvent)) {
	nextRead := rb.ConsumerSequence.Value + 1

	for {
		// Read the maximum available published sequence at this moment
		currentAvailable := atomic.LoadInt64(
			&rb.ProducerSequence.Value,
		)

		if nextRead > currentAvailable {
			// Zero-latency wait: Spin on hardware cache invalidation lines
			continue
		}

		// Cap the batch if it exceeds your max pipeline processing limits
		if currentAvailable-nextRead > limit {
			currentAvailable = nextRead + limit
		}

		// Linear cache pre-fetching loop
		for nextRead <= currentAvailable {
			slot := nextRead & rb.mask
			elementOffset := uintptr(slot) * unsafe.Sizeof(OrderEvent{})

			// Zero-allocation, zero-check direct pointer reference
			event := (*OrderEvent)(unsafe.Pointer(
				uintptr(rb.arrayPtr) + elementOffset,
			))

			handler(event)
			nextRead++
		}

		// Commit processing progress in one single instruction for the whole batch
		atomic.StoreInt64(
			&rb.ConsumerSequence.Value, nextRead-1,
		)
	}
}
