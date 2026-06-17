package dmt

import (
	"bytes"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestNewMerkleTree(t *testing.T) {
	Convey("Given a new Merkle tree", t, func() {
		merkleTree := NewMerkleTree()

		Convey("Then it should be properly initialized", func() {
			So(merkleTree, ShouldNotBeNil)
			So(merkleTree.LeafMap(), ShouldNotBeNil)
			So(merkleTree.Root(), ShouldBeNil)
			So(merkleTree.Modified(), ShouldBeFalse)
		})
	})
}

func TestMerkleTreeInsertAndRebuild(t *testing.T) {
	Convey("Given a Merkle tree", t, func() {
		merkleTree := NewMerkleTree()

		Convey("When inserting key-value pairs", func() {
			merkleTree.Insert([]byte("key1"), []byte("value1"))
			merkleTree.Insert([]byte("key2"), []byte("value2"))
			merkleTree.Insert([]byte("key3"), []byte("value3"))

			Convey("Then the tree should be marked as modified", func() {
				So(merkleTree.Modified(), ShouldBeTrue)
			})

			Convey("When rebuilding the tree", func() {
				merkleTree.Rebuild()

				Convey("Then the tree should have a valid root", func() {
					root := merkleTree.Root()
					So(root, ShouldNotBeNil)
					So(root.Hash, ShouldNotBeNil)
					So(len(root.Hash), ShouldEqual, 32)
				})

				Convey("And the modified flag should be reset", func() {
					So(merkleTree.Modified(), ShouldBeFalse)
				})
			})
		})
	})
}

func TestMerkleTreeDiff(t *testing.T) {
	Convey("Given two Merkle trees", t, func() {
		merkleTreeOne := NewMerkleTree()
		merkleTreeTwo := NewMerkleTree()

		merkleTreeOne.Insert([]byte("key1"), []byte("value1"))
		merkleTreeOne.Insert([]byte("key2"), []byte("value2"))
		merkleTreeOne.Insert([]byte("key3"), []byte("value3"))
		merkleTreeOne.Rebuild()

		merkleTreeTwo.Insert([]byte("key1"), []byte("value1"))
		merkleTreeTwo.Insert([]byte("key2"), []byte("different"))
		merkleTreeTwo.Insert([]byte("key4"), []byte("value4"))
		merkleTreeTwo.Rebuild()

		Convey("When getting differences", func() {
			diffs := merkleTreeOne.GetDiff(merkleTreeTwo)

			Convey("Then it should identify all differences", func() {
				So(len(diffs), ShouldEqual, 2)

				foundModified := false
				foundNew := false

				for _, diff := range diffs {
					if bytes.Equal(diff.Key, []byte("key2")) {
						foundModified = true
						So(diff.Modified, ShouldBeTrue)
					}

					if bytes.Equal(diff.Key, []byte("key3")) {
						foundNew = true
						So(diff.Modified, ShouldBeFalse)
					}
				}

				So(foundModified, ShouldBeTrue)
				So(foundNew, ShouldBeTrue)
			})
		})
	})
}

func TestMerkleTreeVerify(t *testing.T) {
	Convey("Given a Merkle tree with data", t, func() {
		merkleTree := NewMerkleTree()
		merkleTree.Insert([]byte("key1"), []byte("value1"))
		merkleTree.Rebuild()

		Convey("When verifying existing data", func() {
			exists := merkleTree.Verify([]byte("key1"), []byte("value1"))
			So(exists, ShouldBeTrue)
		})

		Convey("When verifying non-existent data", func() {
			exists := merkleTree.Verify([]byte("nonexistent"), []byte("value"))
			So(exists, ShouldBeFalse)
		})

		Convey("When verifying with wrong value", func() {
			exists := merkleTree.Verify([]byte("key1"), []byte("wrong"))
			So(exists, ShouldBeFalse)
		})
	})
}

func TestMerkleProof(t *testing.T) {
	Convey("Given a Merkle tree with multiple entries", t, func() {
		merkleTree := NewMerkleTree()
		merkleTree.Insert([]byte("key1"), []byte("value1"))
		merkleTree.Insert([]byte("key2"), []byte("value2"))
		merkleTree.Insert([]byte("key3"), []byte("value3"))
		merkleTree.Insert([]byte("key4"), []byte("value4"))
		merkleTree.Rebuild()

		Convey("When generating a proof", func() {
			proof, err := merkleTree.GetProof([]byte("key2"))

			Convey("Then it should succeed", func() {
				So(err, ShouldBeNil)
				So(proof, ShouldNotBeNil)
				So(len(proof), ShouldBeGreaterThan, 0)
			})

			Convey("And the proof should be verifiable", func() {
				valid := merkleTree.VerifyProof([]byte("key2"), []byte("value2"), proof)
				So(valid, ShouldBeTrue)
			})

			Convey("And the proof should fail for wrong values", func() {
				valid := merkleTree.VerifyProof([]byte("key2"), []byte("wrong"), proof)
				So(valid, ShouldBeFalse)
			})
		})

		Convey("When generating a proof for non-existent key", func() {
			proof, err := merkleTree.GetProof([]byte("nonexistent"))

			Convey("Then it should fail", func() {
				So(err, ShouldNotBeNil)
				So(proof, ShouldBeNil)
			})
		})
	})
}

func TestMerkleTreeDeterministic(t *testing.T) {
	Convey("Given two identical sets of data", t, func() {
		merkleTreeOne := NewMerkleTree()
		merkleTreeTwo := NewMerkleTree()

		merkleTreeOne.Insert([]byte("key1"), []byte("value1"))
		merkleTreeOne.Insert([]byte("key2"), []byte("value2"))
		merkleTreeOne.Insert([]byte("key3"), []byte("value3"))

		merkleTreeTwo.Insert([]byte("key3"), []byte("value3"))
		merkleTreeTwo.Insert([]byte("key1"), []byte("value1"))
		merkleTreeTwo.Insert([]byte("key2"), []byte("value2"))

		Convey("When rebuilding both trees", func() {
			merkleTreeOne.Rebuild()
			merkleTreeTwo.Rebuild()

			Convey("Then they should have identical root hashes", func() {
				So(bytes.Equal(merkleTreeOne.Root().Hash, merkleTreeTwo.Root().Hash), ShouldBeTrue)
			})
		})
	})
}
