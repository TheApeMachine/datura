package structure

import (
	"context"
	"fmt"
	"io"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/bytedance/sonic"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/datura"
)

type mpmcFrame [128]uint64

func TestNewMPMCRing(t *testing.T) {
	Convey("Given a valid context and power-of-two capacity", t, func() {
		ring, err := NewMPMCRing[*mpmcFrame](context.Background(), 8)

		Convey("NewMPMCRing should return a usable ring and no validation error", func() {
			So(err, ShouldBeNil)
			So(ring, ShouldNotBeNil)
			So(ring.ctx, ShouldNotBeNil)
			So(ring.Error(), ShouldBeNil)
		})
	})
}

func TestMPMCRingPush(t *testing.T) {
	Convey("Given a small MPMCRing", t, func() {
		ring, err := NewMPMCRing[*mpmcFrame](context.Background(), 8)
		So(err, ShouldBeNil)

		var a, b, c mpmcFrame

		Convey("Pop on empty returns zero before any Push", func() {
			So(ring.Pop(), ShouldBeNil)
		})

		Convey("Navigator Pop on an empty cell returns zero instead of panicking", func() {
			So(ring.Select(0).Pop(), ShouldBeNil)
		})

		Convey("FIFO order holds for sequential Push then Pop", func() {
			So(ring.Push(&a), ShouldBeTrue)
			So(ring.Push(&b), ShouldBeTrue)
			So(ring.Push(&c), ShouldBeTrue)
			So(ring.Pop(), ShouldEqual, &a)
			So(ring.Pop(), ShouldEqual, &b)
			So(ring.Pop(), ShouldEqual, &c)
			So(ring.Pop(), ShouldBeNil)
		})
	})

	Convey("Given a capacity-2 MPMCRing filled without popping", t, func() {
		ring, err := NewMPMCRing[*mpmcFrame](context.Background(), 2)
		So(err, ShouldBeNil)

		var x, y, z mpmcFrame

		Convey("Push should fail when every slot holds a frame", func() {
			So(ring.Push(&x), ShouldBeTrue)
			So(ring.Push(&y), ShouldBeTrue)
			So(ring.Push(&z), ShouldBeFalse)
		})

		Convey("after one Pop another Push succeeds", func() {
			So(ring.Push(&x), ShouldBeTrue)
			So(ring.Push(&y), ShouldBeTrue)
			So(ring.Push(&z), ShouldBeFalse)
			So(ring.Pop(), ShouldEqual, &x)
			So(ring.Push(&z), ShouldBeTrue)
		})
	})

	Convey("MPMCRing stress: two producers and main-thread drain count all pushes", t, func() {
		const capacity = 256
		const perProducer = 4000

		ring, err := NewMPMCRing[*mpmcFrame](context.Background(), capacity)
		So(err, ShouldBeNil)

		var producers sync.WaitGroup

		producers.Add(2)

		runProducer := func(base int) {
			defer producers.Done()

			for offset := 0; offset < perProducer; offset++ {
				word := &mpmcFrame{}
				word[0] = uint64(base + offset)

				for !ring.Push(word) {
					runtime.Gosched()
				}
			}
		}

		go runProducer(0)
		go runProducer(perProducer)

		popped := 0
		target := perProducer * 2

		drainTimeout := time.NewTimer(30 * time.Second)
		defer drainTimeout.Stop()

		for popped < target {
			select {
			case <-drainTimeout.C:
				t.Fatalf(
					"TestMPMCRingPush: drain timed out (popped=%d target=%d perProducer=%d)",
					popped,
					target,
					perProducer,
				)
			default:
				if ring.Pop() == nil {
					runtime.Gosched()
					continue
				}

				popped++
			}
		}

		producers.Wait()
		So(popped, ShouldEqual, target)
	})
}

func TestMPMCRingConcurrentNoLossOrDuplicate(t *testing.T) {
	const capacity = 512
	const producers = 4
	const consumers = 4
	const perProducer = 2500
	const total = producers * perProducer

	ring, err := NewMPMCRing[uint64](context.Background(), capacity)
	if err != nil {
		t.Fatal(err)
	}

	seen := make([]atomic.Bool, total+1)
	errs := make(chan string, consumers)

	var pushed atomic.Int64
	var consumed atomic.Int64
	var producerWG sync.WaitGroup
	var consumerWG sync.WaitGroup

	recordError := func(format string, args ...any) {
		select {
		case errs <- fmt.Sprintf(format, args...):
		default:
		}
	}

	producerWG.Add(producers)
	for producer := 0; producer < producers; producer++ {
		base := producer * perProducer

		go func() {
			defer producerWG.Done()

			for offset := 0; offset < perProducer; offset++ {
				value := uint64(base + offset + 1)

				for !ring.Push(value) {
					runtime.Gosched()
				}

				pushed.Add(1)
			}
		}()
	}

	consumerWG.Add(consumers)
	for range consumers {
		go func() {
			defer consumerWG.Done()

			for consumed.Load() < total {
				value := ring.Pop()
				if value == 0 {
					runtime.Gosched()
					continue
				}

				if value > total {
					recordError("value outside produced range: %d", value)
					continue
				}

				if !seen[value].CompareAndSwap(false, true) {
					recordError("duplicate value: %d", value)
				}

				consumed.Add(1)
			}
		}()
	}

	done := make(chan struct{})
	go func() {
		producerWG.Wait()
		consumerWG.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(30 * time.Second):
		t.Fatalf(
			"MPMC concurrent drain timed out (pushed=%d consumed=%d target=%d)",
			pushed.Load(),
			consumed.Load(),
			total,
		)
	}

	close(errs)
	for err := range errs {
		t.Error(err)
	}

	for value := 1; value <= total; value++ {
		if !seen[value].Load() {
			t.Errorf("missing value: %d", value)
		}
	}
}

func TestMPMCRingReadWrite(t *testing.T) {
	Convey("Given an MPMCRing with a bound artifact", t, func() {
		ring, err := NewMPMCRing[int](context.Background(), 4)
		So(err, ShouldBeNil)

		source := datura.Acquire("mpmc", datura.Artifact_Type_json)
		So(source, ShouldNotBeNil)

		payload, marshalErr := sonic.Marshal(7)
		So(marshalErr, ShouldBeNil)
		source.WithPayload(payload)

		ring.WithArtifact(datura.Acquire("mpmc", datura.Artifact_Type_json))
		wire := source.Pack()
		written, err := ring.Write(wire)

		Convey("Write should enqueue the decoded value", func() {
			So(err, ShouldBeNil)
			So(written, ShouldEqual, len(wire))
			So(ring.Len(), ShouldEqual, 1)
		})

		buffer := make([]byte, 4096)
		readCount, readErr := ring.Read(buffer)

		Convey("Read should dequeue and marshal through the artifact", func() {
			So(readErr, ShouldEqual, io.EOF)
			So(readCount, ShouldBeGreaterThan, 0)
			So(ring.Len(), ShouldEqual, 0)

			decoded := datura.Acquire("mpmc", datura.Artifact_Type_json)
			_, writeErr := decoded.Unpack(buffer[:readCount])
			So(writeErr, ShouldBeNil)

			out := decoded.DecryptPayload()
			So(string(out), ShouldEqual, "7")
		})
	})
}

func TestMPMCRingSelectMergeSlice(t *testing.T) {
	Convey("Given an MPMCRing with queued values", t, func() {
		ring, err := NewMPMCRing[*mpmcFrame](context.Background(), 4)
		So(err, ShouldBeNil)

		var first, second mpmcFrame

		So(ring.Push(&first), ShouldBeTrue)
		So(ring.Push(&second), ShouldBeTrue)

		Convey("Select should return a navigator at the requested offset", func() {
			selected := ring.Select(1)

			So(selected, ShouldNotBeNil)
			So(selected.Pop(), ShouldEqual, &second)
		})

		Convey("Slice should detach queued values into a new ring", func() {
			sliced := ring.Slice(2).(*MPMCRing[*mpmcFrame])

			So(sliced, ShouldNotBeNil)
			So(sliced.Len(), ShouldEqual, 2)
			So(ring.Len(), ShouldEqual, 0)
		})

		Convey("Merge should grow and absorb another ring", func() {
			other, err := NewMPMCRing[*mpmcFrame](context.Background(), 2)
			So(err, ShouldBeNil)

			var third mpmcFrame

			So(other.Push(&third), ShouldBeTrue)
			So(ring.Merge(other), ShouldBeTrue)
			So(ring.Len(), ShouldEqual, 3)
		})
	})
}

func TestMPMCRingClose(t *testing.T) {
	Convey("Given an MPMCRing from NewMPMCRing", t, func() {
		ring, err := NewMPMCRing[*mpmcFrame](context.Background(), 4)
		So(err, ShouldBeNil)

		Convey("Close should cancel the derived context", func() {
			closeErr := ring.Close()

			So(closeErr, ShouldBeNil)
			So(ring.ctx.Err(), ShouldNotBeNil)
		})
	})
}

func TestMPMCRingError(t *testing.T) {
	Convey("Given an MPMCRing from NewMPMCRing", t, func() {
		ring, err := NewMPMCRing[*mpmcFrame](context.Background(), 4)
		So(err, ShouldBeNil)

		Convey("Error should return the stored failure until one is set", func() {
			So(ring.Error(), ShouldBeNil)
		})
	})
}

func BenchmarkMPMCRingReadWrite(b *testing.B) {
	ring, err := NewMPMCRing[int](context.Background(), 1024)

	if err != nil {
		b.Fatal(err)
	}

	source := datura.Acquire("mpmc", datura.Artifact_Type_json)

	if source == nil {
		b.Fatal("Acquire returned nil")
	}

	payload, err := sonic.Marshal(7)

	if err != nil {
		b.Fatal(err)
	}

	source.WithPayload(payload)
	ring.WithArtifact(datura.Acquire("mpmc", datura.Artifact_Type_json))
	wire := source.Pack()
	buffer := make([]byte, 4096)

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if _, err := ring.Write(wire); err != nil {
			b.Fatal(err)
		}

		if _, err := ring.Read(buffer); err != io.EOF && err != io.ErrShortBuffer {
			b.Fatal(err)
		}
	}
}

func BenchmarkMPMCRingPush(b *testing.B) {
	ring, err := NewMPMCRing[*mpmcFrame](context.Background(), 1024)

	if err != nil {
		b.Fatal(err)
	}

	var blob mpmcFrame

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		for !ring.Push(&blob) {
		}

		for ring.Pop() == nil {
		}
	}
}
