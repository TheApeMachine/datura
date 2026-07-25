package dmt

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"sort"
	"sync/atomic"
)

/*
MerkleNode is one node in the tree. Leaves store Key/Value; internal nodes
store only Hash. Left/Right are nil for leaves.
*/
type MerkleNode struct {
	Hash  []byte
	Left  *MerkleNode
	Right *MerkleNode
	Key   []byte
	Value []byte
}

/*
MerkleTree maintains a hash tree over key-value pairs for O(log n) diff
detection. Rebuild must be called after Insert before GetDiff or VerifyProof.
*/
type MerkleTree struct {
	state    *batch
	snapshot atomic.Pointer[merkleSnapshot]
}

/*
NewMerkleTree allocates an empty tree. Call Insert then Rebuild before use.
*/
func NewMerkleTree() *MerkleTree {
	tree := &MerkleTree{
		state: newBatch("dmt/merkle"),
	}

	tree.snapshot.Store(newMerkleSnapshot())

	return tree
}

func (tree *MerkleTree) loadSnapshot() *merkleSnapshot {
	return tree.snapshot.Load().load()
}

/*
Insert stores a key-value pair as a leaf. Copies key/value to avoid caller
aliasing. Rebuild required before GetDiff or VerifyProof.
*/
func (tree *MerkleTree) Insert(key, value []byte) {
	leaf := &MerkleNode{
		Key:   append([]byte(nil), key...),
		Value: append([]byte(nil), value...),
	}
	leaf.Hash = tree.hashKV(leaf.Key, leaf.Value)

	for {
		current := tree.loadSnapshot()
		currentLeaves := current.leaves
		leafCount := len(currentLeaves)

		insertIndex := sort.Search(leafCount, func(index int) bool {
			return bytes.Compare(currentLeaves[index].Key, leaf.Key) >= 0
		})

		var nextLeaves []*MerkleNode

		if insertIndex < leafCount && bytes.Equal(currentLeaves[insertIndex].Key, leaf.Key) {
			nextLeaves = make([]*MerkleNode, leafCount)
			copy(nextLeaves, currentLeaves)
			nextLeaves[insertIndex] = leaf
		}

		if insertIndex >= leafCount || !bytes.Equal(currentLeaves[insertIndex].Key, leaf.Key) {
			nextLeaves = make([]*MerkleNode, leafCount+1)
			copy(nextLeaves[:insertIndex], currentLeaves[:insertIndex])
			nextLeaves[insertIndex] = leaf
			copy(nextLeaves[insertIndex+1:], currentLeaves[insertIndex:])
		}

		next := &merkleSnapshot{
			root:     current.root,
			leaves:   nextLeaves,
			parent:   current.parent,
			modified: true,
		}

		if tree.snapshot.CompareAndSwap(current, next) {
			return
		}
	}
}

/*
Rebuild reconstructs the tree from the current leaf set. No-op if unmodified.
*/
func (tree *MerkleTree) Rebuild() {
	for {
		current := tree.loadSnapshot()

		if !current.modified {
			return
		}

		parent := make(map[*MerkleNode]*MerkleNode, len(current.leaves)*2)
		scratchBuffer := make([]*MerkleNode, 0, len(current.leaves))

		root := tree.buildLevelZeroAlloc(current.leaves, scratchBuffer, parent)

		next := &merkleSnapshot{
			root:     root,
			leaves:   current.leaves,
			parent:   parent,
			modified: false,
		}

		if tree.snapshot.CompareAndSwap(current, next) {
			return
		}
	}
}

func (tree *MerkleTree) buildLevelZeroAlloc(
	nodes []*MerkleNode,
	scratchBuffer []*MerkleNode,
	parent map[*MerkleNode]*MerkleNode,
) *MerkleNode {
	if len(nodes) == 0 {
		return nil
	}

	if len(nodes) == 1 {
		return nodes[0]
	}

	levelOffset := len(scratchBuffer)

	for index := 0; index < len(nodes); index += 2 {
		var right *MerkleNode
		left := nodes[index]

		if index+1 < len(nodes) {
			right = nodes[index+1]
		}

		parentNode := &MerkleNode{
			Left:  left,
			Right: right,
			Hash:  tree.hashChildren(left, right),
		}

		parent[left] = parentNode

		if right != nil {
			parent[right] = parentNode
		}

		scratchBuffer = append(scratchBuffer, parentNode)
	}

	nextLevelNodes := scratchBuffer[levelOffset:]

	return tree.buildLevelZeroAlloc(nextLevelNodes, scratchBuffer, parent)
}

/*
GetDiff returns keys that differ between this tree and other.
*/
func (tree *MerkleTree) GetDiff(other *MerkleTree) []DiffEntry {
	left := tree.loadSnapshot()
	right := other.loadSnapshot()

	if left.root == nil || right.root == nil {
		return tree.fullDiff(left, right)
	}

	diffs := make([]DiffEntry, 0)
	tree.diffNode(left.root, right.root, right, &diffs)

	return diffs
}

/*
DiffEntry records one key-value pair that differs.
*/
type DiffEntry struct {
	Key      []byte
	Value    []byte
	Modified bool
}

func (tree *MerkleTree) diffNode(
	leftNode, rightNode *MerkleNode,
	other *merkleSnapshot,
	diffs *[]DiffEntry,
) {
	if bytes.Equal(leftNode.Hash, rightNode.Hash) {
		return
	}

	if leftNode.Key != nil {
		if otherLeaf, exists := other.LookupLeaf(leftNode.Key); exists {
			if !bytes.Equal(leftNode.Value, otherLeaf.Value) {
				*diffs = append(*diffs, DiffEntry{
					Key:      leftNode.Key,
					Value:    leftNode.Value,
					Modified: true,
				})
			}

			return
		}

		*diffs = append(*diffs, DiffEntry{
			Key:      leftNode.Key,
			Value:    leftNode.Value,
			Modified: false,
		})

		return
	}

	if leftNode.Left != nil && rightNode.Left != nil {
		tree.diffNode(leftNode.Left, rightNode.Left, other, diffs)
	}

	if leftNode.Right != nil && rightNode.Right != nil {
		tree.diffNode(leftNode.Right, rightNode.Right, other, diffs)
	}
}

func (tree *MerkleTree) fullDiff(left, right *merkleSnapshot) []DiffEntry {
	leftLeaves := left.leaves
	rightLeaves := right.leaves
	diffs := make([]DiffEntry, 0, len(leftLeaves))

	leftIndex, rightIndex := 0, 0

	for leftIndex < len(leftLeaves) && rightIndex < len(rightLeaves) {
		leftLeaf := leftLeaves[leftIndex]
		rightLeaf := rightLeaves[rightIndex]
		compareKeys := bytes.Compare(leftLeaf.Key, rightLeaf.Key)

		if compareKeys == 0 {
			if !bytes.Equal(leftLeaf.Value, rightLeaf.Value) {
				diffs = append(diffs, DiffEntry{
					Key:      leftLeaf.Key,
					Value:    leftLeaf.Value,
					Modified: true,
				})
			}

			leftIndex++
			rightIndex++

			continue
		}

		if compareKeys < 0 {
			diffs = append(diffs, DiffEntry{
				Key:      leftLeaf.Key,
				Value:    leftLeaf.Value,
				Modified: false,
			})
			leftIndex++

			continue
		}

		rightIndex++
	}

	for leftIndex < len(leftLeaves) {
		leftLeaf := leftLeaves[leftIndex]
		diffs = append(diffs, DiffEntry{
			Key:      leftLeaf.Key,
			Value:    leftLeaf.Value,
			Modified: false,
		})
		leftIndex++
	}

	return diffs
}

/*
Verify returns true if the key exists and its stored value matches.
*/
func (tree *MerkleTree) Verify(key, value []byte) bool {
	snapshot := tree.loadSnapshot()
	leaf, exists := snapshot.LookupLeaf(key)

	if !exists {
		return false
	}

	return bytes.Equal(leaf.Value, value)
}

/*
GetProof returns sibling hashes from leaf to root.
*/
func (tree *MerkleTree) GetProof(key []byte) ([][]byte, error) {
	snapshot := tree.loadSnapshot()
	leaf, exists := snapshot.LookupLeaf(key)

	if !exists {
		guardStep(tree.state, func() error {
			return fmt.Errorf("key not found")
		})

		return nil, tree.state.Err()
	}

	proof := make([][]byte, 0)
	current := leaf

	for current != snapshot.root {
		parentNode := snapshot.parent[current]

		if parentNode == nil {
			guardStep(tree.state, func() error {
				return fmt.Errorf("invalid tree structure")
			})

			return nil, tree.state.Err()
		}

		if parentNode.Left == current {
			if parentNode.Right != nil {
				entry := append([]byte{0x00}, parentNode.Right.Hash...)
				proof = append(proof, entry)
			}
		}

		if parentNode.Left != current {
			entry := append([]byte{0x01}, parentNode.Left.Hash...)
			proof = append(proof, entry)
		}

		current = parentNode
	}

	return proof, nil
}

/*
VerifyProof recomputes the root from key/value and proof hashes.
*/
func (tree *MerkleTree) VerifyProof(key, value []byte, proof [][]byte) bool {
	snapshot := tree.loadSnapshot()

	if snapshot.root == nil {
		return false
	}

	hash := tree.hashKV(key, value)
	hasher := sha256.New()

	for _, entry := range proof {
		if len(entry) <= 1 {
			return false
		}

		position := entry[0]
		siblingHash := entry[1:]

		hasher.Reset()

		if position == 0x00 {
			hasher.Write(hash)
			hasher.Write(siblingHash)
		}

		if position == 0x01 {
			hasher.Write(siblingHash)
			hasher.Write(hash)
		}

		hash = hasher.Sum(hash[:0])
	}

	return bytes.Equal(hash, snapshot.root.Hash)
}

func (tree *MerkleTree) hashKV(key, value []byte) []byte {
	hasher := sha256.New()
	hasher.Write(key)
	hasher.Write(value)

	return hasher.Sum(nil)
}

func (tree *MerkleTree) hashChildren(left, right *MerkleNode) []byte {
	hasher := sha256.New()
	hasher.Write(left.Hash)

	if right != nil {
		hasher.Write(right.Hash)
	}

	return hasher.Sum(nil)
}

// Root exposes the current root for RPC handlers.
func (tree *MerkleTree) Root() *MerkleNode {
	return tree.loadSnapshot().root
}

// Modified reports whether a rebuild is pending.
func (tree *MerkleTree) Modified() bool {
	return tree.loadSnapshot().modified
}

// Leaves exposes the sorted leaf slice for tests.
func (tree *MerkleTree) Leaves() []*MerkleNode {
	return tree.loadSnapshot().leaves
}
