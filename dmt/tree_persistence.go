package dmt

import (
	"time"

	"github.com/theapemachine/errnie"
)

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
		// Already holding persistMu; call the locked variant to avoid a
		// self-deadlock on the non-reentrant mutex.
		if err := tree.saveSnapshotLocked(false); err != nil {
			tree.endOp(started, track)

			return tree, true, tree.failPersistence(err)
		}
	}

	tree.endOp(started, track)

	return tree, true, nil
}

func (tree *Tree) deletePersistent(
	started time.Time,
	track bool,
	key []byte,
) (*Tree, bool, error) {
	tree.persistMu.Lock()
	defer tree.persistMu.Unlock()

	if err := tree.persistenceError(); err != nil {
		tree.endOp(started, track)

		return tree, false, err
	}

	oldRoot := tree.loadRoot()
	newRoot, _, ok := oldRoot.Delete(key)

	if !ok {
		tree.endOp(started, track)

		return tree, false, nil
	}

	index := tree.logIndex.Load() + 1
	if err := tree.persist.LogDelete(key, tree.term.Load(), index); err != nil {
		tree.endOp(started, track)

		return tree, false, tree.failPersistence(err)
	}

	tree.root.Store(newRoot)
	tree.logIndex.Store(index)

	if index%tree.persist.snapCount == 0 {
		// Already holding persistMu; call the locked variant to avoid a
		// self-deadlock on the non-reentrant mutex.
		if err := tree.saveSnapshotLocked(false); err != nil {
			tree.endOp(started, track)

			return tree, true, tree.failPersistence(err)
		}
	}

	tree.endOp(started, track)

	return tree, true, nil
}

func (tree *Tree) SaveSnapshot() error {
	return tree.saveSnapshot(false)
}

/*
SaveSnapshotForced compacts the WAL after bulk namespace rewrites such as REM
decay, bypassing the ordinary snapshot rate limit so durability matches memory.
*/
func (tree *Tree) SaveSnapshotForced() error {
	return tree.saveSnapshot(true)
}

func (tree *Tree) saveSnapshot(force bool) error {
	if tree == nil || tree.persist == nil {
		return nil
	}

	if err := tree.persistenceError(); err != nil {
		return err
	}

	// External snapshot callers do not hold persistMu; acquire it so rotation is
	// exclusive with inserts/deletes (same lock order they use: persistMu →
	// walMu). Inline triggers from insertPersistent/deletePersistent already
	// hold persistMu and must call saveSnapshotLocked directly to avoid
	// self-deadlock on this non-reentrant mutex.
	tree.persistMu.Lock()
	defer tree.persistMu.Unlock()

	return tree.saveSnapshotLocked(force)
}

/*
saveSnapshotLocked performs WAL snapshot rotation. The caller MUST hold
persistMu: rotation iterates the live tree and renames/reopens the WAL file,
which must not overlap a concurrent insert that mutates tree.root and appends to
the same WAL. A prior version iterated without persistMu, letting an insert race
the rename and latch a fatal "dmt/tree" error (leaving a stale wal.log.new).
*/
func (tree *Tree) saveSnapshotLocked(force bool) error {
	if tree == nil || tree.persist == nil {
		return nil
	}

	if err := tree.persistenceError(); err != nil {
		return err
	}

	if err := tree.persist.createSnapshot(force, func(yield func(key, value []byte) bool) {
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

/*
GetLogIndex returns the durable log index published on this tree.
*/
func (tree *Tree) GetLogIndex() uint64 {
	if tree == nil {
		return 0
	}

	return tree.logIndex.Load()
}

/*
AppliedIndex returns the forest commit index this tree has applied.
*/
func (tree *Tree) AppliedIndex() uint64 {
	if tree == nil {
		return 0
	}

	return tree.appliedIndex.Load()
}

/*
SetAppliedIndex records that this tree has applied the given commit index.
*/
func (tree *Tree) SetAppliedIndex(index uint64) {
	if tree == nil {
		return
	}

	tree.appliedIndex.Store(index)
}
