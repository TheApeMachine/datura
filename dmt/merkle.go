package dmt

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
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
	keyCopy := append([]byte(nil), key...)
	valueCopy := append([]byte(nil), value...)

	leaf := &MerkleNode{
		Key:   keyCopy,
		Value: valueCopy,
		Hash:  tree.hashKV(keyCopy, valueCopy),
	}

	keyHex := hex.EncodeToString(keyCopy)

	for {
		current := tree.loadSnapshot()
		next := current.withLeaf(keyHex, leaf)

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

		leaves := make([]*MerkleNode, 0, len(current.leafMap))
		keys := make([]string, 0, len(current.leafMap))

		nodeMap := make(map[string]*MerkleNode, len(current.leafMap))
		parent := make(map[*MerkleNode]*MerkleNode, len(current.leafMap)*2)

		for key := range current.leafMap {
			keys = append(keys, key)
		}

		sort.Strings(keys)

		for _, key := range keys {
			leaves = append(leaves, current.leafMap[key])
		}

		root := tree.buildLevel(leaves, nodeMap, parent)
		next := current.rebuilt(root, nodeMap, parent)

		if tree.snapshot.CompareAndSwap(current, next) {
			return
		}
	}
}

func (tree *MerkleTree) buildLevel(
	nodes []*MerkleNode,
	nodeMap map[string]*MerkleNode,
	parent map[*MerkleNode]*MerkleNode,
) *MerkleNode {
	if len(nodes) == 0 {
		return nil
	}

	if len(nodes) == 1 {
		return nodes[0]
	}

	parents := make([]*MerkleNode, 0, (len(nodes)+1)/2)

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

		hashHex := hex.EncodeToString(parentNode.Hash)
		nodeMap[hashHex] = parentNode
		parent[left] = parentNode

		if right != nil {
			parent[right] = parentNode
		}

		parents = append(parents, parentNode)
	}

	return tree.buildLevel(parents, nodeMap, parent)
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
		keyHex := hex.EncodeToString(leftNode.Key)

		if otherLeaf, exists := other.leafMap[keyHex]; exists {
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
	diffs := make([]DiffEntry, 0, len(left.leafMap))

	for _, leaf := range left.leafMap {
		keyHex := hex.EncodeToString(leaf.Key)

		if otherLeaf, exists := right.leafMap[keyHex]; exists {
			if !bytes.Equal(leaf.Value, otherLeaf.Value) {
				diffs = append(diffs, DiffEntry{
					Key:      leaf.Key,
					Value:    leaf.Value,
					Modified: true,
				})
			}

			continue
		}

		diffs = append(diffs, DiffEntry{
			Key:      leaf.Key,
			Value:    leaf.Value,
			Modified: false,
		})
	}

	return diffs
}

/*
Verify returns true if the key exists and its stored value matches.
*/
func (tree *MerkleTree) Verify(key, value []byte) bool {
	snapshot := tree.loadSnapshot()
	keyHex := hex.EncodeToString(key)
	leaf, exists := snapshot.leafMap[keyHex]

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
	keyHex := hex.EncodeToString(key)
	leaf, exists := snapshot.leafMap[keyHex]

	if !exists {
		guardStep(tree.state, func() error {
			return fmt.Errorf("key not found")
		})

		return nil, tree.state.Err()
	}

	proof := make([][]byte, 0)
	current := leaf

	for current != snapshot.root {
		parent := snapshot.parent[current]

		if parent == nil {
			guardStep(tree.state, func() error {
				return fmt.Errorf("invalid tree structure")
			})

			return nil, tree.state.Err()
		}

		if parent.Left == current {
			if parent.Right != nil {
				entry := append([]byte{0x00}, parent.Right.Hash...)
				proof = append(proof, entry)
			}
		}

		if parent.Left != current {
			entry := append([]byte{0x01}, parent.Left.Hash...)
			proof = append(proof, entry)
		}

		current = parent
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

	for _, entry := range proof {
		if len(entry) <= 1 {
			return false
		}

		position := entry[0]
		siblingHash := entry[1:]

		if position == 0x00 {
			hash = tree.hashChildren(&MerkleNode{Hash: hash}, &MerkleNode{Hash: siblingHash})
		}

		if position == 0x01 {
			hash = tree.hashChildren(&MerkleNode{Hash: siblingHash}, &MerkleNode{Hash: hash})
		}
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

// LeafMap exposes the leaf map for tests.
func (tree *MerkleTree) LeafMap() map[string]*MerkleNode {
	return tree.loadSnapshot().leafMap
}
