package structure

import (
	"io"
	"testing"

	"github.com/bytedance/sonic"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/datura"
)

func TestNewListRing(t *testing.T) {
	Convey("Given a non-positive element count", t, func() {
		Convey("NewListRing should return nil", func() {
			So(NewListRing[int](0), ShouldBeNil)
			So(NewListRing[int](-1), ShouldBeNil)
		})
	})

	Convey("Given a positive element count", t, func() {
		ring := NewListRing[int](3)

		Convey("NewListRing should build a ring of that length", func() {
			So(ring, ShouldNotBeNil)
			So(ring.Len(), ShouldEqual, 3)
		})
	})
}

func TestListRingPush(t *testing.T) {
	Convey("Given a one-element ListRing", t, func() {
		ring := NewListRing[int](1)

		Convey("Push should store at the cursor and advance", func() {
			So(ring.Push(42), ShouldBeTrue)
			So(ring.Pop(), ShouldEqual, 42)
		})
	})

	Convey("Given a three-element ListRing", t, func() {
		ring := NewListRing[int](3)

		Convey("Push should fill slots in order and wrap", func() {
			So(ring.Push(1), ShouldBeTrue)
			So(ring.Push(2), ShouldBeTrue)
			So(ring.Push(3), ShouldBeTrue)
			So(ring.Push(4), ShouldBeTrue)

			seen := make([]int, 0, 3)

			ring.Do(func(value int) {
				seen = append(seen, value)
			})

			So(seen, ShouldResemble, []int{2, 3, 4})
		})
	})
}

func TestListRingSelect(t *testing.T) {
	Convey("Given a three-element ListRing", t, func() {
		ring := NewListRing[int](3)
		ring.cursor.Value = 1
		ring.cursor.next.Value = 2
		ring.cursor.next.next.Value = 3

		Convey("Select should return a new cursor without mutating the receiver", func() {
			selected := ring.Select(1).(*ListRing[int])

			So(selected.cursor.Value, ShouldEqual, 2)
			So(ring.cursor.Value, ShouldEqual, 1)
		})

		Convey("Select with negative step should walk backward", func() {
			selected := ring.Select(-1).(*ListRing[int])

			So(selected.cursor.Value, ShouldEqual, 3)
		})
	})
}

func TestListRingMerge(t *testing.T) {
	Convey("Given two one-element ListRings", t, func() {
		left := NewListRing[int](1)
		right := NewListRing[int](1)
		left.cursor.Value = 10
		right.cursor.Value = 20

		Convey("Merge should splice right after left", func() {
			So(left.Merge(right), ShouldBeTrue)
			So(left.Len(), ShouldEqual, 2)
			So(left.cursor.next.Value, ShouldEqual, 20)
		})
	})
}

func TestListRingSlice(t *testing.T) {
	Convey("Given a three-element ListRing", t, func() {
		ring := NewListRing[int](3)
		ring.cursor.Value = 1
		ring.cursor.next.Value = 2
		ring.cursor.next.next.Value = 3

		Convey("Slice should remove the requested segment", func() {
			removed := ring.Slice(1).(*ListRing[int])

			So(removed, ShouldNotBeNil)
			So(removed.cursor.Value, ShouldEqual, 2)
			So(ring.Len(), ShouldEqual, 2)
		})
	})
}

func TestListRingDo(t *testing.T) {
	Convey("Given a three-element ListRing", t, func() {
		ring := NewListRing[int](3)
		ring.cursor.Value = 1
		ring.cursor.next.Value = 2
		ring.cursor.next.next.Value = 3

		seen := make([]int, 0, 3)

		ring.Do(func(value int) {
			seen = append(seen, value)
		})

		Convey("Do should visit every element starting at the cursor", func() {
			So(seen, ShouldResemble, []int{1, 2, 3})
		})
	})
}

func TestListRingReadWrite(t *testing.T) {
	Convey("Given a nil ListRing", t, func() {
		var ring *ListRing[int]
		buffer := make([]byte, 4096)
		readCount, err := ring.Read(buffer)

		Convey("Read should fail closed", func() {
			So(readCount, ShouldEqual, 0)
			So(err, ShouldEqual, io.EOF)
		})
	})

	Convey("Given a ListRing with no cursor", t, func() {
		ring := &ListRing[int]{}
		buffer := make([]byte, 4096)
		readCount, err := ring.Read(buffer)

		Convey("Read should fail closed", func() {
			So(readCount, ShouldEqual, 0)
			So(err, ShouldEqual, io.EOF)
		})
	})

	Convey("Given a ListRing with a bound artifact", t, func() {
		ring := NewListRing[int](1)
		source := datura.Acquire("list", datura.Artifact_Type_json)

		So(source, ShouldNotBeNil)

		payload, err := sonic.Marshal(42)

		So(err, ShouldBeNil)
		source.WithPayload(payload)

		wire := source.Pack()
		written, err := ring.Write(wire)

		Convey("Write should unmarshal into the ring", func() {
			So(err, ShouldBeNil)
			So(written, ShouldEqual, len(wire))
		})

		buffer := make([]byte, 4096)
		readCount, readErr := ring.Read(buffer)

		Convey("Read should marshal the cursor value through the artifact", func() {
			So(readErr, ShouldEqual, io.EOF)
			So(readCount, ShouldBeGreaterThan, 0)

			decoded := datura.Acquire("list", datura.Artifact_Type_json)
			_, writeErr := decoded.Unpack(buffer[:readCount])
			So(writeErr, ShouldBeNil)

			out := decoded.DecryptPayload()
			So(string(out), ShouldEqual, "42")
		})
	})
}

func BenchmarkListRingReadWrite(b *testing.B) {
	ring := NewListRing[int](1)
	source := datura.Acquire("list", datura.Artifact_Type_json)

	if source == nil {
		b.Fatal("Acquire returned nil")
	}

	payload, err := sonic.Marshal(42)

	if err != nil {
		b.Fatal(err)
	}

	source.WithPayload(payload)
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

func TestListRingImplementsRing(t *testing.T) {
	Convey("Given a ListRing assigned to Ring", t, func() {
		var ring Ring[int] = NewListRing[int](2)

		Convey("Ring methods should be callable", func() {
			So(ring.Len(), ShouldEqual, 2)
			So(ring.Error(), ShouldBeNil)
			So(ring.Close(), ShouldBeNil)
		})
	})
}
