package dmt

import (
	"bufio"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

var errForcedWALWrite = errors.New("forced wal write failure")

type failingWALWriter struct{}

func (failingWALWriter) Write(_ []byte) (int, error) {
	return 0, errForcedWALWrite
}

func TestTreeWithPersistence(t *testing.T) {
	Convey("Given a temporary directory", t, func() {
		tmpDir := filepath.Join(os.TempDir(), "radix-test-"+time.Now().Format("20060102150405"))
		defer os.RemoveAll(tmpDir)

		Convey("When creating a tree with persistence", func() {
			tree := NewTree(tmpDir)
			defer tree.Close()

			Convey("Then the persistence store should be initialized", func() {
				So(tree.persist, ShouldNotBeNil)
			})

			Convey("And when inserting data", func() {
				newTree, ok, err := tree.Insert([]byte("test-key"), []byte("test-value"))
				So(err, ShouldBeNil)
				So(ok, ShouldBeTrue)
				So(newTree, ShouldNotBeNil)

				Convey("The data should be persisted", func() {
					// Create new tree instance with same persistence
					tree2 := NewTree(tmpDir)
					defer tree2.Close()

					// Verify term and index were loaded
					term, index := tree2.GetLogState()
					termBefore, indexBefore := tree.GetLogState()
					So(term, ShouldEqual, termBefore)
					So(index, ShouldEqual, indexBefore)

					// Verify data was recovered
					value, exists := tree2.Get([]byte("test-key"))
					So(exists, ShouldBeTrue)
					So(value, ShouldResemble, []byte("test-value"))
				})
			})
		})
	})
}

func TestTreePersistentInsertFailsClosedOnWALWriteFailure(t *testing.T) {
	Convey("Given a persistent tree with a failing WAL writer", t, func() {
		tree := NewTree(t.TempDir())
		So(tree.persist, ShouldNotBeNil)

		originalWriter := tree.persist.walWriter
		originalPool := tree.persist.pool
		tree.persist.pool = nil
		tree.persist.walWriter = bufio.NewWriter(failingWALWriter{})
		defer func() {
			tree.persist.walWriter = originalWriter
			tree.persist.pool = originalPool
			_ = tree.Close()
		}()

		Convey("When inserting a key", func() {
			_, ok, err := tree.Insert([]byte("unsafe"), []byte("value"))

			Convey("Then the insert should fail without publishing the root", func() {
				So(err, ShouldNotBeNil)
				So(ok, ShouldBeFalse)

				_, exists := tree.Get([]byte("unsafe"))
				So(exists, ShouldBeFalse)
			})

			Convey("And future writes should fail closed", func() {
				_, ok, err := tree.Insert([]byte("future"), []byte("value"))
				So(err, ShouldNotBeNil)
				So(ok, ShouldBeFalse)

				_, exists := tree.Get([]byte("future"))
				So(exists, ShouldBeFalse)
			})
		})
	})
}

func TestTreeStateRecovery(t *testing.T) {
	Convey("Given a tree with existing WAL", t, func() {
		tmpDir := filepath.Join(os.TempDir(), "radix-test-"+time.Now().Format("20060102150405"))
		defer os.RemoveAll(tmpDir)

		// Create and populate first tree
		tree1 := NewTree(tmpDir)

		entries := []struct {
			key   string
			value string
			term  uint64
		}{
			{"key1", "value1", 1},
			{"key2", "value2", 1},
			{"key3", "value3", 2},
		}

		for _, e := range entries {
			tree1.UpdateTerm(e.term)
			tree1, _, _ = tree1.Insert([]byte(e.key), []byte(e.value))
		}
		tree1.Close()

		Convey("When creating a new tree instance", func() {
			tree2 := NewTree(tmpDir)
			defer tree2.Close()

			Convey("Then it should recover the correct state", func() {
				term, index := tree2.GetLogState()
				So(term, ShouldEqual, entries[len(entries)-1].term)
				So(index, ShouldEqual, uint64(len(entries)))

				Convey("And all data should be accessible", func() {
					for _, e := range entries {
						value, exists := tree2.Get([]byte(e.key))
						So(exists, ShouldBeTrue)
						So(value, ShouldResemble, []byte(e.value))
					}
				})
			})
		})
	})
}

func TestTreeSnapshotPreservesActiveEntries(t *testing.T) {
	Convey("Given a persistent tree with a low snapshot interval", t, func() {
		tmpDir := t.TempDir()

		tree := NewTree(tmpDir)
		tree.persist.snapCount = 3

		entries := map[string]string{
			"snapshot/key/one":   "value-one",
			"snapshot/key/two":   "value-two",
			"snapshot/key/three": "value-three",
		}

		for key, value := range entries {
			_, ok, err := tree.Insert([]byte(key), []byte(value))
			So(err, ShouldBeNil)
			So(ok, ShouldBeTrue)
		}

		closeErr := tree.Close()
		So(closeErr, ShouldBeNil)

		Convey("When the tree is reopened after the snapshot", func() {
			reopened := NewTree(tmpDir)
			defer reopened.Close()

			Convey("Then every active entry should be recovered", func() {
				for key, value := range entries {
					recovered, ok := reopened.Get([]byte(key))
					So(ok, ShouldBeTrue)
					So(string(recovered), ShouldEqual, value)
				}
			})

			Convey("And the log index should match the inserted entries", func() {
				_, index := reopened.GetLogState()
				So(index, ShouldEqual, uint64(len(entries)))
			})
		})
	})
}

func TestTreeTermUpdate(t *testing.T) {
	Convey("Given a persistent tree", t, func() {
		tmpDir := filepath.Join(os.TempDir(), "radix-test-"+time.Now().Format("20060102150405"))
		defer os.RemoveAll(tmpDir)

		tree := NewTree(tmpDir)
		defer tree.Close()

		Convey("When updating the term", func() {
			tree.UpdateTerm(5)

			Convey("Then the term should be persisted", func() {
				term, _ := tree.GetLogState()
				So(term, ShouldEqual, uint64(5))

				// Verify term survives restart
				tree.Close()
				newTree := NewTree(tmpDir)
				defer newTree.Close()

				term, _ = newTree.GetLogState()
				So(term, ShouldEqual, uint64(5))
			})
		})
	})
}
