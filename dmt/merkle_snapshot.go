package dmt

import (
	"bytes"
	"sort"
)

/*
merkleSnapshot is an immutable Merkle tree view published through atomic pointers.
Leaves are kept in sorted key order for allocation-free binary search lookups.
*/
type merkleSnapshot struct {
	root     *MerkleNode
	leaves   []*MerkleNode
	parent   map[*MerkleNode]*MerkleNode
	modified bool
}

func newMerkleSnapshot() *merkleSnapshot {
	return &merkleSnapshot{
		leaves: make([]*MerkleNode, 0),
		parent: make(map[*MerkleNode]*MerkleNode),
	}
}

func (snapshot *merkleSnapshot) load() *merkleSnapshot {
	if snapshot == nil {
		return newMerkleSnapshot()
	}

	return snapshot
}

/*
LookupLeaf performs an allocation-free binary search across the sorted leaf slice.
*/
func (snapshot *merkleSnapshot) LookupLeaf(key []byte) (*MerkleNode, bool) {
	current := snapshot.load()
	leafCount := len(current.leaves)

	leafIndex := sort.Search(leafCount, func(index int) bool {
		return bytes.Compare(current.leaves[index].Key, key) >= 0
	})

	if leafIndex < leafCount && bytes.Equal(current.leaves[leafIndex].Key, key) {
		return current.leaves[leafIndex], true
	}

	return nil, false
}
