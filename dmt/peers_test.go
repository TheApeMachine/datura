package dmt

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestPeersWith(t *testing.T) {
	Convey("Given an empty peer snapshot", t, func() {
		peers := &Peers{entries: make([]peerEntry, 0)}

		Convey("When upserting peers out of order", func() {
			peers = peers.With("peer-c", &peer{addr: "peer-c"})
			peers = peers.With("peer-a", &peer{addr: "peer-a"})
			peers = peers.With("peer-b", &peer{addr: "peer-b"})

			Convey("Then entries should remain sorted by address", func() {
				So(peers.entries[0].addr, ShouldEqual, "peer-a")
				So(peers.entries[1].addr, ShouldEqual, "peer-b")
				So(peers.entries[2].addr, ShouldEqual, "peer-c")
			})

			Convey("And Has should use binary search", func() {
				So(peers.Has("peer-b"), ShouldBeTrue)
				So(peers.Has("peer-z"), ShouldBeFalse)
			})
		})

		Convey("When replacing an existing peer", func() {
			first := &peer{addr: "peer-a"}
			second := &peer{addr: "peer-a"}
			peers = peers.With("peer-a", first)
			peers = peers.With("peer-a", second)

			Convey("Then it should update in place without growing", func() {
				So(peers.Len(), ShouldEqual, 1)
				So(peers.entries[0].node, ShouldEqual, second)
			})
		})

		Convey("When removing a peer", func() {
			peers = peers.With("peer-a", &peer{addr: "peer-a"})
			peers = peers.With("peer-b", &peer{addr: "peer-b"})
			peers = peers.Without("peer-a")

			Convey("Then only the remaining peer should be present", func() {
				So(peers.Len(), ShouldEqual, 1)
				So(peers.entries[0].addr, ShouldEqual, "peer-b")
			})
		})
	})
}

func BenchmarkPeersHas(b *testing.B) {
	peers := &Peers{entries: make([]peerEntry, 0)}

	for index := range 128 {
		address := "peer-" + string(rune('a'+index%26)) + string(rune('0'+index/26))
		peers = peers.With(address, &peer{addr: address})
	}

	lookupAddr := peers.entries[64].addr

	for b.Loop() {
		peers.Has(lookupAddr)
	}
}
