package structure

import (
	"io"
	"testing"

	"github.com/bytedance/sonic"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/datura"
)

type spscFrame [128]uint64

func TestNewSPSCRing(t *testing.T) {
	Convey("Given a power-of-two capacity", t, func() {
		ring, err := NewSPSCRing[*spscFrame](8, false, nil)

		Convey("NewSPSCRing should return a usable ring and no validation error", func() {
			So(err, ShouldBeNil)
			So(ring, ShouldNotBeNil)
			So(ring.Len(), ShouldEqual, 0)
			So(ring.Empty(), ShouldBeTrue)
		})
	})
}

func TestSPSCRingPush(t *testing.T) {
	Convey("Given a small SPSCRing", t, func() {
		ring, err := NewSPSCRing[*spscFrame](8, false, nil)
		So(err, ShouldBeNil)

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
		ring, err := NewSPSCRing[*spscFrame](1, true, nil)
		So(err, ShouldBeNil)

		var first, second spscFrame

		Convey("Push should evict the oldest value when full", func() {
			So(ring.Push(&first), ShouldBeTrue)
			So(ring.Push(&second), ShouldBeTrue)
			So(ring.Pop(), ShouldEqual, &second)
		})
	})

	Convey("Given a capacity-2 SPSCRing filled without popping", t, func() {
		ring, err := NewSPSCRing[*spscFrame](2, false, nil)
		So(err, ShouldBeNil)

		var first, second, third spscFrame

		Convey("Push should fail when every slot holds a frame", func() {
			So(ring.Push(&first), ShouldBeTrue)
			So(ring.Push(&second), ShouldBeTrue)
			So(ring.Push(&third), ShouldBeFalse)
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

func TestSPSCRingReadWrite(t *testing.T) {
	Convey("Given an SPSCRing with a bound artifact", t, func() {
		ring, err := NewSPSCRing[int](4, false, nil)
		So(err, ShouldBeNil)

		source := datura.Acquire("spsc", datura.Artifact_Type_json)
		So(source, ShouldNotBeNil)

		payload, marshalErr := sonic.Marshal(9)
		So(marshalErr, ShouldBeNil)
		source.WithPayload(payload)

		wire := source.Marshal()

		written, writeErr := ring.Write(wire)

		Convey("Write should enqueue the decoded value", func() {
			So(writeErr, ShouldBeNil)
			So(written, ShouldEqual, len(wire))
			So(ring.Len(), ShouldEqual, 1)
		})

		buffer := make([]byte, 4096)
		readCount, readErr := ring.Read(buffer)

		Convey("Read should dequeue and marshal through the artifact", func() {
			So(readErr, ShouldEqual, io.EOF)
			So(readCount, ShouldBeGreaterThan, 0)
			So(ring.Len(), ShouldEqual, 0)

			decoded := datura.Acquire("spsc", datura.Artifact_Type_json)
			So(decoded.Unmarshal(buffer[:readCount]), ShouldNotBeNil)

			out, payloadErr := decoded.Payload()
			So(payloadErr, ShouldBeNil)
			So(string(out), ShouldEqual, "9")
		})
	})
}

func TestSPSCRingClose(t *testing.T) {
	Convey("Given an SPSCRing with queued values", t, func() {
		ring, err := NewSPSCRing[*spscFrame](4, false, nil)
		So(err, ShouldBeNil)

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
		ring, err := NewSPSCRing[*spscFrame](4, false, nil)
		So(err, ShouldBeNil)

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
			other, err := NewSPSCRing[*spscFrame](2, false, nil)
			So(err, ShouldBeNil)

			var third spscFrame

			So(other.Push(&third), ShouldBeTrue)
			So(ring.Merge(other), ShouldBeTrue)
			So(ring.Len(), ShouldEqual, 3)
		})
	})
}

func TestSPSCRingImplementsRing(t *testing.T) {
	Convey("Given an SPSCRing assigned to Ring", t, func() {
		ring, err := NewSPSCRing[*spscFrame](4, false, nil)
		So(err, ShouldBeNil)

		var asRing Ring[*spscFrame] = ring

		Convey("Ring methods should be callable", func() {
			So(asRing.Len(), ShouldEqual, 0)
			So(asRing.Error(), ShouldBeNil)
			So(asRing.Close(), ShouldBeNil)
			So(asRing.Select(0), ShouldNotBeNil)
		})
	})
}

func BenchmarkSPSCRingReadWrite(b *testing.B) {
	ring, err := NewSPSCRing[int](1024, false, nil)

	if err != nil {
		b.Fatal(err)
	}

	source := datura.Acquire("spsc", datura.Artifact_Type_json)

	if source == nil {
		b.Fatal("Acquire returned nil")
	}

	payload, err := sonic.Marshal(9)

	if err != nil {
		b.Fatal(err)
	}

	source.WithPayload(payload)
	wire := source.Marshal()
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

func BenchmarkSPSCRingPush(b *testing.B) {
	ring, err := NewSPSCRing[*spscFrame](1024, false, nil)

	if err != nil {
		b.Fatal(err)
	}

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
