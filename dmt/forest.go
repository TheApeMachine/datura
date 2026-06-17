package dmt

import (
	"context"
	"sync/atomic"

	"github.com/theapemachine/qpool"
)

/*
Forest manages a collection of Tree instances, providing intelligent routing of operations
to the most performant tree based on running performance metrics. It maintains data
consistency across all trees while optimizing read operations by selecting the fastest
responding tree.
*/
type Forest struct {
	state    *batch
	snapshot atomic.Pointer[Snapshot]
	closed   atomic.Bool
	// Context for controlling background workers
	ctx    context.Context
	cancel context.CancelFunc
	pool   *qpool.Q[any]
	owned  bool
	// Network node for distributed operation
	network *NetworkNode
}

// ForestConfig holds configuration for creating a new Forest
type ForestConfig struct {
	// Directory for persistence
	PersistDir string
	// Worker pool for background tasks
	Pool *qpool.Q[any]
	// Network configuration
	Network *NetworkConfig
}

/*
NewForest creates and returns a new empty Forest instance with background
synchronization enabled. The forest starts with no trees and trees can be
added using the AddTree method. A background goroutine is started to handle
tree synchronization.
*/
func NewForest(config ForestConfig) (*Forest, error) {
	ctx, cancel := context.WithCancel(context.Background())
	forest := &Forest{
		state:  newBatch("dmt/forest"),
		ctx:    ctx,
		cancel: cancel,
		pool:   config.Pool,
	}

	forest.snapshot.Store(&Snapshot{})

	if forest.pool == nil {
		forest.pool = newWorkerPool(forest.ctx)
		forest.owned = true
	}

	// Create initial tree (with persistence if directory is provided)
	tree := guardValue(forest.state, func() (*Tree, error) {
		return NewTree(config.PersistDir)
	})

	forest.AddTree(tree)

	// Initialize network node if network config provided
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

	if forest.owned && forest.pool != nil {
		forest.pool.Close()
	}

	return forest.state.Err()
}

/*
synchronizeTrees ensures all trees have consistent data by comparing and
updating them based on the most up-to-date tree.
*/
func (forest *Forest) synchronizeTrees(trees []*Tree) {
	if len(trees) <= 1 {
		return
	}

	// Use the first tree as reference
	reference := trees[0]

	// Build Merkle tree for reference
	refMerkle := NewMerkleTree()
	it := reference.loadRoot().Root().Iterator()
	for key, value, ok := it.Next(); ok; key, value, ok = it.Next() {
		refMerkle.Insert(key, value)
	}
	refMerkle.Rebuild()

	// Sync other trees using Merkle diffs
	for _, tree := range trees[1:] {
		// Build Merkle tree for target
		targetMerkle := NewMerkleTree()
		it := tree.loadRoot().Root().Iterator()
		for key, value, ok := it.Next(); ok; key, value, ok = it.Next() {
			targetMerkle.Insert(key, value)
		}
		targetMerkle.Rebuild()

		// Get diff and apply changes
		diffs := refMerkle.GetDiff(targetMerkle)
		for _, diff := range diffs {
			tree.Insert(diff.Key, diff.Value)
		}
	}
}

/*
AddTree incorporates a new Tree instance into the forest.
Each added tree will be maintained with identical data but may have different
performance characteristics based on its specific implementation or state.
*/
func (forest *Forest) AddTree(tree *Tree) {
	forest.snapshot.Store(forest.snapshot.Load().Append(tree))
	forest.synchronizeTrees(forest.snapshot.Load().Trees())
}

/*
getFastestTree returns the tree with the lowest average performance time.
It analyzes the running performance metrics of each tree to determine which
one is currently responding most quickly to operations. Returns nil if the
forest contains no trees.
*/
func (forest *Forest) getFastestTree() *Tree {
	trees := forest.snapshot.Load().Trees()

	if len(trees) == 0 {
		return nil
	}

	fastestTree := trees[0]
	fastestAvg := fastestTree.AVG()

	for _, tree := range trees[1:] {
		if avg := tree.AVG(); avg < fastestAvg {
			fastestTree = tree
			fastestAvg = avg
		}
	}

	return fastestTree
}

/*
Get retrieves a value from the forest using the most performant tree.
It automatically selects the tree with the best average response time to
handle the request. Returns the value and true if the key exists, or nil
and false if it doesn't or if the forest is empty.
*/
func (forest *Forest) Get(key []byte) ([]byte, bool) {
	fastestTree := forest.getFastestTree()
	if fastestTree == nil {
		return nil, false
	}
	return fastestTree.Get(key)
}

/*
Seek performs a prefix-based search using the most performant tree in the forest.
It finds the first value whose key is greater than or equal to the provided key
in lexicographical order. Returns the value and true if found, or nil and false
if no such key exists or if the forest is empty.
*/
func (forest *Forest) Seek(key []byte) ([]byte, bool) {
	fastestTree := forest.getFastestTree()
	if fastestTree == nil {
		return nil, false
	}
	return fastestTree.Seek(key)
}

/*
Insert adds or updates a key-value pair across all trees in the forest.
To maintain data consistency, the operation is performed on every tree,
ensuring that subsequent read operations will find the same data regardless
of which tree they query. This method prioritizes consistency over performance.
*/
func (forest *Forest) Insert(key []byte, value []byte) {
	trees := forest.snapshot.Load().Trees()

	// Update all local trees immediately
	for _, tree := range trees {
		tree.Insert(key, value)
	}

	// Broadcast to other nodes if networked
	if forest.network != nil {
		forest.network.BroadcastInsert(key, value)
	}
}

/*
Iterate walks all key-value pairs in the fastest tree, calling fn for each.
Stops early if fn returns false.
*/
func (forest *Forest) Iterate(fn func(key []byte, value []byte) bool) {
	tree := forest.getFastestTree()
	if tree == nil {
		return
	}

	root := tree.loadRoot()
	it := root.Root().Iterator()

	for key, value, ok := it.Next(); ok; key, value, ok = it.Next() {
		if !fn(key, value) {
			return
		}
	}
}
