package dmt

/*
merkleSnapshot is an immutable Merkle tree view published through atomic pointers.
*/
type merkleSnapshot struct {
	root     *MerkleNode
	leafMap  map[string]*MerkleNode
	nodeMap  map[string]*MerkleNode
	parent   map[*MerkleNode]*MerkleNode
	modified bool
}

func newMerkleSnapshot() *merkleSnapshot {
	return &merkleSnapshot{
		leafMap: make(map[string]*MerkleNode),
		nodeMap: make(map[string]*MerkleNode),
		parent:  make(map[*MerkleNode]*MerkleNode),
	}
}

func (snapshot *merkleSnapshot) load() *merkleSnapshot {
	if snapshot == nil {
		return newMerkleSnapshot()
	}

	return snapshot
}

func (snapshot *merkleSnapshot) withLeaf(keyHex string, leaf *MerkleNode) *merkleSnapshot {
	current := snapshot.load()
	nextLeafMap := make(map[string]*MerkleNode, len(current.leafMap)+1)

	for key, value := range current.leafMap {
		nextLeafMap[key] = value
	}

	nextLeafMap[keyHex] = leaf

	return &merkleSnapshot{
		root:     current.root,
		leafMap:  nextLeafMap,
		nodeMap:  current.nodeMap,
		parent:   current.parent,
		modified: true,
	}
}

func (snapshot *merkleSnapshot) rebuilt(
	root *MerkleNode,
	nodeMap map[string]*MerkleNode,
	parent map[*MerkleNode]*MerkleNode,
) *merkleSnapshot {
	current := snapshot.load()

	return &merkleSnapshot{
		root:     root,
		leafMap:  current.leafMap,
		nodeMap:  nodeMap,
		parent:   parent,
		modified: false,
	}
}
