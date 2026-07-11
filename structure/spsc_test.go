package structure

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

type spscFrame [128]uint64

func TestNewSPSCRing(t *testing.T) {
	Convey("Given a power-of-two capacity", t, func() {
		ring := NewSPSCRing[*spscFrame](8, false)

		Convey("NewSPSCRing should return a usable ring and no validation error", func() {
			So(ring, ShouldNotBeNil)
			So(ring.Len(), ShouldEqual, 0)
			So(ring.Empty(), ShouldBeTrue)
		})
	})

	Convey("Given capacity one", t, func() {
		ring := NewSPSCRing[*spscFrame](1, false)

		Convey("NewSPSCRing should accept it as the smallest valid ring", func() {
			So(ring, ShouldNotBeNil)
		})
	})

	Convey("Given zero or non-power-of-two capacity", t, func() {
		zero := NewSPSCRing[*spscFrame](0, false)
		three := NewSPSCRing[*spscFrame](3, false)

		Convey("NewSPSCRing should reject invalid capacities", func() {
			So(zero, ShouldBeNil)
			So(three, ShouldBeNil)
		})
	})
}

func TestSPSCRingPush(t *testing.T) {
	Convey("Given a small SPSCRing", t, func() {
		ring := NewSPSCRing[*spscFrame](8, false)

		var first, second, third spscFrame

		Convey("Pop on empty returns zero before any Push", func() {
			So(ring.Pop(), ShouldBeNil)
		})

		Convey("FIFO order holds for sequential Push then Pop", func() {
			So(ring.Push(&first), ShouldBeTrue)
			So(ring.Push(&second), ShouldBeTrue)
			So(ring.Push(&third), ShouldBeTrue)
			So(ring.Pop(), ShouldEqual, &first)
			So(ring.Pop(), ShouldEqual, &second)
			So(ring.Pop(), ShouldEqual, &third)
			So(ring.Pop(), ShouldBeNil)
		})
	})

	Convey("Given a capacity-1 SPSCRing that drops oldest on full", t, func() {
		ring := NewSPSCRing[*spscFrame](1, true)

		var first, second spscFrame

		Convey("Push should evict the oldest value when full", func() {
			So(ring.Push(&first), ShouldBeTrue)
			So(ring.Push(&second), ShouldBeTrue)
			So(ring.Pop(), ShouldEqual, &second)
			So(ring.Dropped(), ShouldEqual, 1)
			So(ring.Rejected(), ShouldEqual, 0)
		})
	})

	Convey("Given a capacity-2 SPSCRing filled without popping", t, func() {
		ring := NewSPSCRing[*spscFrame](2, false)

		var first, second, third spscFrame

		Convey("Push should fail when every slot holds a frame", func() {
			So(ring.Push(&first), ShouldBeTrue)
			So(ring.Push(&second), ShouldBeTrue)
			So(ring.Push(&third), ShouldBeFalse)
			So(ring.Rejected(), ShouldEqual, 1)
		})

		Convey("after one Pop another Push succeeds", func() {
			So(ring.Push(&first), ShouldBeTrue)
			So(ring.Push(&second), ShouldBeTrue)
			So(ring.Push(&third), ShouldBeFalse)
			So(ring.Pop(), ShouldEqual, &first)
			So(ring.Push(&third), ShouldBeTrue)
		})
	})
}

func TestSPSCRingClose(t *testing.T) {
	Convey("Given an SPSCRing with queued values", t, func() {
		ring := NewSPSCRing[*spscFrame](4, false)

		var first, second spscFrame

		So(ring.Push(&first), ShouldBeTrue)
		So(ring.Push(&second), ShouldBeTrue)

		Convey("Close should drain all queued values", func() {
			So(ring.Close(), ShouldBeNil)
			So(ring.Pop(), ShouldBeNil)
			So(ring.Len(), ShouldEqual, 0)
		})
	})
}

func TestSPSCRingSelectMergeSlice(t *testing.T) {
	Convey("Given an SPSCRing with queued values", t, func() {
		ring := NewSPSCRing[*spscFrame](4, false)

		var first, second spscFrame

		So(ring.Push(&first), ShouldBeTrue)
		So(ring.Push(&second), ShouldBeTrue)

		Convey("Select should return a navigator at the requested offset", func() {
			selected := ring.Select(1)

			So(selected, ShouldNotBeNil)
			So(selected.Pop(), ShouldEqual, &second)
		})

		Convey("Slice should detach queued values into a new ring", func() {
			sliced := ring.Slice(2).(*SPSCRing[*spscFrame])

			So(sliced, ShouldNotBeNil)
			So(sliced.Len(), ShouldEqual, 2)
			So(ring.Len(), ShouldEqual, 0)
		})

		Convey("Merge should grow and absorb another ring", func() {
			other := NewSPSCRing[*spscFrame](2, false)

			var third spscFrame

			So(other.Push(&third), ShouldBeTrue)
			So(ring.Merge(other), ShouldBeTrue)
			So(ring.Len(), ShouldEqual, 3)
		})
	})
}

func TestSPSCRingImplementsRing(t *testing.T) {
	Convey("Given an SPSCRing assigned to Ring", t, func() {
		ring := NewSPSCRing[*spscFrame](4, false)

		var asRing Ring[*spscFrame] = ring

		Convey("Ring methods should be callable", func() {
			So(asRing.Len(), ShouldEqual, 0)
			So(asRing.Error(), ShouldBeNil)
			So(asRing.Close(), ShouldBeNil)
			So(asRing.Select(0), ShouldNotBeNil)
		})
	})
}

func BenchmarkSPSCRingPush(b *testing.B) {
	ring := NewSPSCRing[*spscFrame](1024, false)

	var blob spscFrame

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		for !ring.Push(&blob) {
		}

		for ring.Pop() == nil {
		}
	}
}
