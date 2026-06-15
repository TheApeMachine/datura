package structure

import (
	"context"
	"runtime"
	"sync"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
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
			So(isNilValue(ring.Pop()), ShouldBeTrue)
		})

		Convey("FIFO order holds for sequential Push then Pop", func() {
			So(ring.Push(&a), ShouldBeTrue)
			So(ring.Push(&b), ShouldBeTrue)
			So(ring.Push(&c), ShouldBeTrue)
			So(ring.Pop(), ShouldEqual, &a)
			So(ring.Pop(), ShouldEqual, &b)
			So(ring.Pop(), ShouldEqual, &c)
			So(isNilValue(ring.Pop()), ShouldBeTrue)
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
				if isNilValue(ring.Pop()) {
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

		for isNilValue(ring.Pop()) {
		}
	}
}
