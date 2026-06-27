package datura

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestMortonCoder(t *testing.T) {
	Convey("Given two 32-bit coordinates", t, func() {
		coder := NewMortonCoder()
		const maxUint32 = uint64(0xffffffff)

		Convey("Encode should interleave bits and Decode should recover them", func() {
			code := coder.Encode(0b1010, 0b0101)
			So(code, ShouldEqual, 0b01100110)

			x, y := coder.Decode(code)
			So(x, ShouldEqual, 0b1010)
			So(y, ShouldEqual, 0b0101)
		})

		Convey("Encode and Decode should round-trip the full supported width", func() {
			x, y := coder.Decode(coder.Encode(maxUint32, 0))
			So(x, ShouldEqual, maxUint32)
			So(y, ShouldEqual, 0)

			x, y = coder.Decode(coder.Encode(0, maxUint32))
			So(x, ShouldEqual, 0)
			So(y, ShouldEqual, maxUint32)
		})

		Convey("Distinct supported coordinates should not collide", func() {
			seen := make(map[uint64][2]uint64)
			values := []uint64{0, 1, 2, 3, 15, 16, 255, 256, 65535, maxUint32}

			for _, x := range values {
				for _, y := range values {
					code := coder.Encode(x, y)
					previous, exists := seen[code]

					So(exists, ShouldBeFalse)
					So(previous, ShouldResemble, [2]uint64{})

					seen[code] = [2]uint64{x, y}
				}
			}
		})
	})
}
