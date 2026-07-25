/*
package dmt implements a wrapper around an immutable radix tree data structure.
A radix tree (also known as a radix trie or compact prefix tree) is a space-optimized
tree structure that is particularly efficient for string or byte slice keys. It compresses
common prefixes to save space and enables fast lookups, insertions, and prefix-based searches.
*/
package dmt

import (
	"bytes"
	"iter"
	"sync"
	"sync/atomic"
	"time"

	iradix "github.com/hashicorp/go-immutable-radix/v2"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/errnie"
)

/*
Tree wraps an immutable radix tree implementation from hashicorp/go-immutable-radix.
It stores byte slices as both keys and values, providing efficient prefix-based operations.
Readers load the root pointer atomically; writers publish new roots with compare-and-swap.
Public read APIs copy into caller-owned buffers; Borrow exposes a scoped view that must
not be retained past the callback.
*/
type Tree struct {
	state           *batch
	root            atomic.Pointer[iradix.Tree[[]byte]]
	persist         *PersistentStore
	persistMu       sync.Mutex
	term            atomic.Uint64
	logIndex        atomic.Uint64
	opCount         atomic.Uint64
	opSampleCount   atomic.Uint64
	opTotalNanos    atomic.Int64
	appliedIndex    atomic.Uint64
	basinClassNames atomic.Pointer[[]string]
}

func (tree *Tree) loadRoot() *iradix.Tree[[]byte] {
	if tree == nil {
		return iradix.New[[]byte]()
	}

	root := tree.root.Load()

	if root != nil {
		return root
	}

	return iradix.New[[]byte]()
}

const treeOpSampleMask = uint64(63)

func (tree *Tree) beginOp() (started time.Time, track bool) {
	if tree == nil {
		return time.Time{}, false
	}

	count := tree.opCount.Add(1)

	if count&treeOpSampleMask != 0 {
		return time.Time{}, false
	}

	return time.Now(), true
}

/*
endOp records a completed latency sample when track is true. Operation count is
advanced in beginOp so the sampling decision and count stay consistent.
*/
func (tree *Tree) endOp(started time.Time, track bool) {
	if tree == nil || !track {
		return
	}

	tree.opSampleCount.Add(1)
	tree.opTotalNanos.Add(time.Since(started).Nanoseconds())
}

/*
NewTree creates and returns a new empty Tree instance.
When persistDir is non-empty, the persistent store is opened and WAL replayed
before the tree is returned. Construction failures are returned as errors.
*/
func NewTree(persistDir string) (*Tree, error) {
	tree := &Tree{
		state: newBatch("dmt/tree"),
	}

	emptyRoot := iradix.New[[]byte]()
	tree.root.Store(emptyRoot)

	if persistDir == "" {
		return tree, nil
	}

	store, err := NewPersistentStore(persistDir)

	if err != nil {
		return nil, err
	}

	tree.persist = store

	entries, err := tree.persist.Replay()

	if err != nil {
		_ = tree.persist.Close()
		return nil, err
	}

	root := tree.loadRoot()

	for _, entry := range entries {
		if entry.Op == opDelete {
			root, _, _ = root.Delete(entry.Key)
			continue
		}

		root, _, _ = root.Insert(entry.Key, entry.Value)
	}

	tree.root.Store(root)

	term, index := tree.persist.GetLastState()
	tree.term.Store(term)
	tree.logIndex.Store(index)

	return tree, nil
}

/*
Seek performs a prefix-based search in the tree, and returns anything
matching the longest common prefix.
*/
func (tree *Tree) Seek(key []byte) iter.Seq[*datura.Artifact] {
	started, track := tree.beginOp()
	root := tree.loadRoot()

	it := root.Root().Iterator()
	it.SeekPrefix(key)

	return iter.Seq[*datura.Artifact](func(yield func(*datura.Artifact) bool) {
		for foundKey, value, ok := it.Next(); ok; foundKey, value, ok = it.Next() {
			if !bytes.HasPrefix(foundKey, key) {
				break
			}

			if len(value) == 0 {
				continue
			}

			inbound := &datura.Artifact{}

			if _, err := inbound.Unpack(value); err != nil {
				errnie.Error(errnie.Err(
					errnie.Validation, "failed to unpack artifact", err,
				))
				continue
			}

			if !yield(inbound) {
				tree.endOp(started, track)
				return
			}
		}

		tree.endOp(started, track)
	})
}

/*
WalkPrefix visits every key/value whose key begins with prefix, in
lexicographical (and therefore chronological, given Artifact.Prefix keys) order.
The walk stops at the first key that does not share the prefix. The walk stops
early if fn returns false. Key and value are copied into caller-owned buffers.
*/
func (tree *Tree) WalkPrefix(prefix []byte, fn func(key, value []byte) bool) {
	started, track := tree.beginOp()
	root := tree.loadRoot()

	it := root.Root().Iterator()
	it.SeekPrefix(prefix)

	for key, value, ok := it.Next(); ok; key, value, ok = it.Next() {
		if !bytes.HasPrefix(key, prefix) {
			break
		}

		if !fn(key, value) {
			tree.endOp(started, track)

			return
		}
	}

	tree.endOp(started, track)
}

/*
WalkLowerBound visits every key/value pair at or after lowerBound in
lexicographical order. The caller owns the stopping condition, which lets
role/timestamp readers scan [role/timestamp, next-role) without manufacturing
every intermediate second prefix.
*/
func (tree *Tree) WalkLowerBound(lowerBound []byte, fn func(key, value []byte) bool) {
	started, track := tree.beginOp()
	root := tree.loadRoot()

	it := root.Root().Iterator()
	it.SeekLowerBound(lowerBound)

	for key, value, ok := it.Next(); ok; key, value, ok = it.Next() {
		if !fn(key, value) {
			tree.endOp(started, track)

			return
		}
	}

	tree.endOp(started, track)
}

/*
Insert adds or updates a key-value pair in the tree.
Due to the immutable nature of the tree, this operation creates a new version
of the tree rather than modifying the existing one.
Returns the updated tree, a boolean indicating if the tree was modified, and a
persistence error when a durable tree cannot write its WAL.
*/
func (tree *Tree) Insert(key []byte, value []byte) (*Tree, bool, error) {
	started, track := tree.beginOp()

	if tree == nil {
		return tree, false, nil
	}

	if err := tree.persistenceError(); err != nil {
		tree.endOp(started, track)

		return tree, false, err
	}

	if tree.persist != nil {
		updated, changed, err := tree.insertPersistent(started, track, key, value)

		if changed {
			tree.noteBasinClass(key)
		}

		return updated, changed, err
	}

	for {
		oldRoot := tree.loadRoot()
		newRoot, _, _ := oldRoot.Insert(key, value)

		if newRoot == oldRoot {
			tree.endOp(started, track)

			return tree, false, nil
		}

		if tree.root.CompareAndSwap(oldRoot, newRoot) {
			tree.noteBasinClass(key)
			tree.endOp(started, track)

			return tree, true, nil
		}
	}
}

/*
Delete removes a key-value pair from the tree.
Due to the immutable nature of the tree, this operation creates a new version
of the tree rather than modifying the existing one.
Returns the updated tree, a boolean indicating if the tree was modified (i.e. if the key existed),
and a persistence error when a durable tree cannot write its WAL.
*/
func (tree *Tree) Delete(key []byte) (*Tree, bool, error) {
	started, track := tree.beginOp()

	if tree == nil {
		return tree, false, nil
	}

	if err := tree.persistenceError(); err != nil {
		tree.endOp(started, track)

		return tree, false, err
	}

	if tree.persist != nil {
		return tree.deletePersistent(started, track, key)
	}

	for {
		oldRoot := tree.loadRoot()
		newRoot, _, ok := oldRoot.Delete(key)

		if !ok {
			tree.endOp(started, track)

			return tree, false, nil
		}

		if tree.root.CompareAndSwap(oldRoot, newRoot) {
			tree.endOp(started, track)

			return tree, true, nil
		}
	}
}

/*
InsertArtifact adds or updates a datura.Artifact in the tree.
Due to the immutable nature of the tree, this operation creates a new version
of the tree rather than modifying the existing one.
Returns the updated tree, a boolean indicating if the tree was modified, and any
persistence error.
*/
func (tree *Tree) InsertArtifact(
	prefix []byte,
	artifact *datura.Artifact,
) (*Tree, bool, error) {
	if tree == nil {
		return tree, false, errnie.Err(errnie.Validation, "dmt: nil tree", nil)
	}

	if artifact == nil {
		return tree, false, errnie.Err(errnie.Validation, "dmt: nil artifact", nil)
	}

	if len(prefix) == 0 {
		return tree, false, errnie.Err(errnie.Validation, "dmt: empty artifact prefix", nil)
	}

	wire := artifact.Pack()

	if len(wire) == 0 {
		return tree, false, errnie.Err(errnie.Validation, "dmt: empty packed artifact", nil)
	}

	return tree.Insert(prefix, wire)
}

/*
Get retrieves a caller-owned copy of the value associated with the given key.
Returns the value and true if the key exists, or nil and false if it doesn't.
*/
func (tree *Tree) Get(key []byte) ([]byte, bool) {
	started, track := tree.beginOp()
	value, ok := tree.loadRoot().Get(key)
	tree.endOp(started, track)

	if !ok {
		return nil, false
	}

	return value, true
}

/*
getRaw returns the live radix value without copying. Callers must not retain or
mutate the slice beyond the immediate unmarshal.
*/
func (tree *Tree) getRaw(key []byte) ([]byte, bool) {
	return tree.loadRoot().Get(key)
}

/*
Borrow invokes fn with the live radix value for key. The bytes are valid only
for the duration of fn; callers must not retain or mutate them after return.
Returns false when the key is absent.
*/
func (tree *Tree) Borrow(key []byte, fn func(value []byte)) bool {
	started, track := tree.beginOp()
	value, ok := tree.loadRoot().Get(key)
	tree.endOp(started, track)

	if !ok {
		return false
	}

	fn(value)
	return true
}

/*
AVG returns the average sampled operation latency in nanoseconds. Trees with no
completed latency sample return 0 and must be excluded from latency-based
selection via HasLatencySample.
*/
func (tree *Tree) AVG() int64 {
	if tree == nil {
		return 0
	}

	samples := tree.opSampleCount.Load()

	if samples == 0 {
		return 0
	}

	return tree.opTotalNanos.Load() / int64(samples)
}

/*
HasLatencySample reports whether at least one operation latency sample exists.
*/
func (tree *Tree) HasLatencySample() bool {
	if tree == nil {
		return false
	}

	return tree.opSampleCount.Load() > 0
}

/*
Close closes the tree and persists any remaining data.
*/
func (tree *Tree) Close() error {
	if tree == nil {
		return nil
	}

	if tree.persist != nil {
		if err := tree.persist.Close(); err != nil {
			tree.failPersistence(err)
		}
	}

	return tree.state.Err()
}
