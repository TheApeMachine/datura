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
		if err := tree.SaveSnapshot(); err != nil {
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
