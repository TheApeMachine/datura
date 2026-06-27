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
*/
type Tree struct {
	state        *batch
	root         atomic.Pointer[iradix.Tree[[]byte]]
	persist      *PersistentStore
	persistMu    sync.Mutex
	term         atomic.Uint64
	logIndex     atomic.Uint64
	opCount      atomic.Uint64
	opTotalNanos atomic.Int64
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

func (tree *Tree) persistenceError() error {
	if tree == nil {
		return nil
	}

	if tree.state != nil && tree.state.Err() != nil {
		return tree.state.Err()
	}

	if tree.persist != nil {
		return tree.persist.fatalError()
	}

	return nil
}

func (tree *Tree) failPersistence(err error) error {
	if err == nil {
		return nil
	}

	if tree.state != nil && tree.state.Err() == nil {
		tree.state.err = errnie.Err(errnie.IO, "dmt/tree", err)
	}

	return tree.persistenceError()
}

const treeOpSampleMask = uint64(63)

func (tree *Tree) beginOp() (started time.Time, track bool) {
	if tree == nil {
		return time.Time{}, false
	}

	if tree.opCount.Load()&treeOpSampleMask != 0 {
		return time.Time{}, false
	}

	return time.Now(), true
}

/*
endOp increments the operation count and tracks the time taken.
*/
func (tree *Tree) endOp(started time.Time, track bool) {
	if tree == nil {
		return
	}

	tree.opCount.Add(1)

	if !track {
		return
	}

	tree.opTotalNanos.Add(time.Since(started).Nanoseconds())
}

/*
NewTree creates and returns a new empty Tree instance.
The underlying radix tree is initialized with no entries.
*/
func NewTree(persistDir string) *Tree {
	tree := &Tree{
		state: newBatch("dmt/tree"),
	}

	emptyRoot := iradix.New[[]byte]()
	tree.root.Store(emptyRoot)

	if persistDir != "" {
		tree.persist = guardValue(tree.state, func() (*PersistentStore, error) {
			return NewPersistentStore(persistDir)
		})

		entries := guardValue(tree.state, tree.persist.Replay)
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
	}

	return tree
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
The walk stops early if fn returns false. This is the history read: write
observations keyed by Artifact.Prefix, then walk the scope prefix to replay them.
*/
func (tree *Tree) WalkPrefix(prefix []byte, fn func(key, value []byte) bool) {
	started, track := tree.beginOp()
	root := tree.loadRoot()

	it := root.Root().Iterator()
	it.SeekPrefix(prefix)

	for key, value, ok := it.Next(); ok; key, value, ok = it.Next() {
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
		return tree.insertPersistent(started, track, key, value)
	}

	for {
		oldRoot := tree.loadRoot()
		newRoot, _, _ := oldRoot.Insert(key, value)

		if newRoot == oldRoot {
			tree.endOp(started, track)

			return tree, false, nil
		}

		if tree.root.CompareAndSwap(oldRoot, newRoot) {
			tree.endOp(started, track)

			return tree, true, nil
		}
	}
}

func (tree *Tree) insertPersistent(
	started time.Time,
	track bool,
	key []byte,
	value []byte,
) (*Tree, bool, error) {
	tree.persistMu.Lock()
	defer tree.persistMu.Unlock()

	if err := tree.persistenceError(); err != nil {
		tree.endOp(started, track)

		return tree, false, err
	}

	oldRoot := tree.loadRoot()
	newRoot, _, _ := oldRoot.Insert(key, value)

	if newRoot == oldRoot {
		tree.endOp(started, track)

		return tree, false, nil
	}

	index := tree.logIndex.Load() + 1
	if err := tree.persist.LogInsert(key, value, tree.term.Load(), index); err != nil {
		tree.endOp(started, track)

		return tree, false, tree.failPersistence(err)
	}

	tree.root.Store(newRoot)
	tree.logIndex.Store(index)

	if index%tree.persist.snapCount == 0 {
		if err := tree.SaveSnapshot(); err != nil {
			tree.endOp(started, track)

			return tree, true, tree.failPersistence(err)
		}
	}

	tree.endOp(started, track)

	return tree, true, nil
}

func (tree *Tree) SaveSnapshot() error {
	if tree == nil || tree.persist == nil {
		return nil
	}

	if err := tree.persistenceError(); err != nil {
		return err
	}

	if err := tree.persist.CreateSnapshot(func(yield func(key, value []byte) bool) {
		root := tree.loadRoot()
		it := root.Root().Iterator()

		for key, value, ok := it.Next(); ok; key, value, ok = it.Next() {
			if !yield(key, value) {
				return
			}
		}
	}); err != nil {
		return tree.failPersistence(err)
	}

	return nil
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
	if tree == nil || artifact == nil || len(prefix) == 0 {
		return tree, false, nil
	}

	wire := artifact.Pack()

	if len(wire) == 0 {
		return tree, false, nil
	}

	return tree.Insert(prefix, wire)
}

/*
Get retrieves the value associated with the given key.
Returns the value and true if the key exists, or nil and false if it doesn't.
*/
func (tree *Tree) Get(key []byte) ([]byte, bool) {
	started, track := tree.beginOp()
	value, ok := tree.loadRoot().Get(key)
	tree.endOp(started, track)

	return value, ok
}

/*
AVG returns the average performance of the tree in nanoseconds.
*/
func (tree *Tree) AVG() int64 {
	if tree == nil {
		return 0
	}

	count := tree.opCount.Load()

	if count == 0 {
		return 0
	}

	return tree.opTotalNanos.Load() * int64(treeOpSampleMask+1) / int64(count)
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

func (tree *Tree) UpdateTerm(term uint64) error {
	if tree == nil {
		return nil
	}

	if err := tree.persistenceError(); err != nil {
		return err
	}

	if tree.persist != nil {
		tree.persistMu.Lock()
		defer tree.persistMu.Unlock()

		if err := tree.persistenceError(); err != nil {
			return err
		}

		if err := tree.persist.LogTerm(term); err != nil {
			return tree.failPersistence(err)
		}
	}

	tree.term.Store(term)

	return nil
}

func (tree *Tree) GetLogState() (term, index uint64) {
	if tree == nil {
		return 0, 0
	}

	return tree.term.Load(), tree.logIndex.Load()
}
