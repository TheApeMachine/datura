package dmt

/*
Snapshot is an immutable view of the trees managed by a Forest.
Writers publish a new snapshot pointer; readers load it without locking.
*/
type Snapshot struct {
	trees []*Tree
}

func (snapshot *Snapshot) load() *Snapshot {
	if snapshot == nil {
		return &Snapshot{}
	}

	return snapshot
}

func (snapshot *Snapshot) Trees() []*Tree {
	return snapshot.load().trees
}

func (snapshot *Snapshot) Append(tree *Tree) *Snapshot {
	current := snapshot.load()
	nextTrees := make([]*Tree, len(current.trees)+1)

	copy(nextTrees, current.trees)
	nextTrees[len(current.trees)] = tree

	return &Snapshot{trees: nextTrees}
}
