package dmt

import (
	"context"
	"errors"
	"iter"
	"sync/atomic"

	"github.com/theapemachine/datura"
)

/*
Forest manages a collection of Tree instances with one authoritative durable
commit path. Writes go to the primary tree first; replicas apply only after the
commit succeeds and track an applied index for read routing.
*/
type Forest struct {
	state       *batch
	snapshot    atomic.Pointer[Snapshot]
	closed      atomic.Bool
	commitIndex atomic.Uint64
	ctx         context.Context
	cancel      context.CancelFunc
	network     *NetworkNode
}

// ForestConfig holds configuration for creating a new Forest
type ForestConfig struct {
	PersistDir string
	Network    *NetworkConfig
}

/*
NewForest creates and returns a new empty Forest instance with background
synchronization enabled.
*/
func NewForest(config ForestConfig) (*Forest, error) {
	ctx, cancel := context.WithCancel(context.Background())
	forest := &Forest{
		state:  newBatch("dmt/forest"),
		ctx:    ctx,
		cancel: cancel,
	}

	forest.snapshot.Store(&Snapshot{})

	tree, err := NewTree(config.PersistDir)

	if err != nil {
		cancel()
		return nil, err
	}

	if err := forest.AddTree(tree); err != nil {
		cancel()
		return nil, err
	}

	if config.Network != nil {
		forest.network = guardValue(forest.state, func() (*NetworkNode, error) {
			return NewNetworkNode(*config.Network, forest)
		})
	}

	return forest, forest.state.Err()
}

/*
Close stops the background synchronization goroutine and cleans up resources.
*/
func (forest *Forest) Close() error {
	if !forest.closed.CompareAndSwap(false, true) {
		return forest.state.Err()
	}

	if forest.cancel != nil {
		forest.cancel()
	}

	trees := forest.snapshot.Load().Trees()

	if forest.network != nil {
		guardStep(forest.state, forest.network.Close)
	}

	for _, tree := range trees {
		guardStep(forest.state, tree.Close)
	}

	return forest.state.Err()
}

/*
AddTree incorporates a new Tree instance into the forest using lock-free snapshot CAS.
Late-joining replicas are caught up by replaying the primary's current entries.
*/
func (forest *Forest) AddTree(tree *Tree) error {
	if tree == nil {
		return errors.New("dmt: cannot add nil tree to forest")
	}

	for {
		current := forest.snapshot.Load()
		next := current.Append(tree)

		if !forest.snapshot.CompareAndSwap(current, next) {
			continue
		}

		trees := next.Trees()

		if len(trees) > 1 {
			if err := forest.catchUpReplica(trees[0], tree); err != nil {
				return err
			}
		}

		return nil
	}
}

/*
catchUpReplica copies primary entries into replica and marks the applied index.
*/
func (forest *Forest) catchUpReplica(primary, replica *Tree) error {
	if primary == nil || replica == nil || primary == replica {
		return nil
	}

	root := primary.loadRoot()
	iterator := root.Root().Iterator()

	for key, value, ok := iterator.Next(); ok; key, value, ok = iterator.Next() {
		_, _, err := replica.Insert(key, value)

		if err != nil {
			return err
		}
	}

	replica.SetAppliedIndex(forest.commitIndex.Load())

	return nil
}

/*
getFastestTree returns a readable replica whose applied index matches the forest
commit index and that has a completed latency sample. Prefer the lowest average
latency among eligible trees.
*/
func (forest *Forest) getFastestTree() *Tree {
	trees := forest.snapshot.Load().Trees()
	commit := forest.commitIndex.Load()

	var fastestTree *Tree
	var fastestAvg int64

	for _, tree := range trees {
		if tree.AppliedIndex() != commit {
			continue
		}

		if !tree.HasLatencySample() {
			continue
		}

		avg := tree.AVG()

		if fastestTree == nil || avg < fastestAvg {
			fastestTree = tree
			fastestAvg = avg
		}
	}

	if fastestTree != nil {
		return fastestTree
	}

	for _, tree := range trees {
		if tree.AppliedIndex() == commit {
			return tree
		}
	}

	if len(trees) == 0 {
		return nil
	}

	return trees[0]
}

/*
GetFastestTree returns the tree selected for reads by applied-index and latency.
*/
func (forest *Forest) GetFastestTree() *Tree {
	return forest.getFastestTree()
}

/*
Get retrieves a value from the forest using the most performant eligible tree.
*/
func (forest *Forest) Get(key []byte) ([]byte, bool) {
	fastestTree := forest.getFastestTree()

	if fastestTree == nil {
		return nil, false
	}

	return fastestTree.Get(key)
}

/*
Seek performs a prefix-based search using the most performant eligible tree.
*/
func (forest *Forest) Seek(key []byte) iter.Seq[*datura.Artifact] {
	fastestTree := forest.getFastestTree()

	if fastestTree == nil {
		return iter.Seq[*datura.Artifact](func(yield func(*datura.Artifact) bool) {})
	}

	return fastestTree.Seek(key)
}

/*
Insert adds or updates a key-value pair through the durable primary commit log,
then applies the same mutation to replicas. Returns the first durability or
apply error.
*/
func (forest *Forest) Insert(key []byte, value []byte) error {
	trees := forest.snapshot.Load().Trees()

	if len(trees) == 0 {
		return errors.New("dmt: forest has no trees")
	}

	primary := trees[0]
	_, _, err := primary.Insert(key, value)

	if err != nil {
		return err
	}

	index := primary.GetLogIndex()
	primary.SetAppliedIndex(index)
	forest.commitIndex.Store(index)

	for _, tree := range trees[1:] {
		_, _, err := tree.Insert(key, value)

		if err != nil {
			return err
		}

		tree.SetAppliedIndex(index)
	}

	if forest.network != nil {
		forest.network.stageInsert(key, value)
	}

	return nil
}

/*
Iterate walks all key-value pairs in the selected readable tree with copied
buffers at the public boundary.
*/
func (forest *Forest) Iterate(fn func(key []byte, value []byte) bool) {
	tree := forest.getFastestTree()

	if tree == nil {
		return
	}

	root := tree.loadRoot()
	iterator := root.Root().Iterator()

	for key, value, ok := iterator.Next(); ok; key, value, ok = iterator.Next() {
		if !fn(key, value) {
			return
		}
	}
}
