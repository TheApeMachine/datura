package dmt

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestMerkleSnapshotLookupLeaf(t *testing.T) {
	Convey("Given a sorted merkle snapshot", t, func() {
		snapshot := &merkleSnapshot{
			leaves: []*MerkleNode{
				{Key: []byte("a"), Value: []byte("1")},
				{Key: []byte("b"), Value: []byte("2")},
				{Key: []byte("c"), Value: []byte("3")},
			},
		}

		Convey("When looking up keys with binary search", func() {
			leafA, foundA := snapshot.LookupLeaf([]byte("a"))
			leafB, foundB := snapshot.LookupLeaf([]byte("b"))
			leafC, foundC := snapshot.LookupLeaf([]byte("c"))
			_, foundMissing := snapshot.LookupLeaf([]byte("z"))

			Convey("Then it should resolve without string conversions", func() {
				So(foundA, ShouldBeTrue)
				So(foundB, ShouldBeTrue)
				So(foundC, ShouldBeTrue)
				So(foundMissing, ShouldBeFalse)
				So(string(leafA.Value), ShouldEqual, "1")
				So(string(leafB.Value), ShouldEqual, "2")
				So(string(leafC.Value), ShouldEqual, "3")
			})
		})
	})
}

func TestSnapshotAppend(t *testing.T) {
	Convey("Given a forest snapshot", t, func() {
		firstTree := &Tree{}
		secondTree := &Tree{}
		base := &Snapshot{trees: []*Tree{firstTree}}

		Convey("When appending a tree", func() {
			next := base.Append(secondTree)

			Convey("Then it should copy prior trees and append the new one", func() {
				So(len(next.trees), ShouldEqual, 2)
				So(next.trees[0], ShouldEqual, firstTree)
				So(next.trees[1], ShouldEqual, secondTree)
				So(len(base.trees), ShouldEqual, 1)
			})
		})
	})
}

func BenchmarkMerkleTreeInsert(b *testing.B) {
	merkleTree := NewMerkleTree()
	key := []byte("key")
	value := []byte("value")

	for b.Loop() {
		merkleTree.Insert(key, value)
	}
}

func BenchmarkMerkleSnapshotLookupLeaf(b *testing.B) {
	snapshot := &merkleSnapshot{
		leaves: []*MerkleNode{
			{Key: []byte("alpha"), Value: []byte("1")},
			{Key: []byte("beta"), Value: []byte("2")},
			{Key: []byte("gamma"), Value: []byte("3")},
		},
	}
	lookupKey := []byte("beta")

	for b.Loop() {
		_, _ = snapshot.LookupLeaf(lookupKey)
	}
}

func BenchmarkSnapshotAppend(b *testing.B) {
	snapshot := &Snapshot{trees: make([]*Tree, 0, 1)}

	for b.Loop() {
		snapshot = snapshot.Append(&Tree{})
	}
}
