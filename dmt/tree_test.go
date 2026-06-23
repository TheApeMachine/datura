package dmt

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/datura"
)

func TestNewTree(t *testing.T) {
	Convey("Given a new tree", t, func() {
		tree := NewTree("")

		Convey("When a new tree is created", func() {
			So(tree, ShouldNotBeNil)
		})
	})
}

func TestSeek(t *testing.T) {
	Convey("Given a new tree", t, func() {
		tree := NewTree("")

		Convey("When a seek is performed", func() {
			artifact := datura.Acquire("test", datura.Artifact_Type_json)
			So(artifact, ShouldNotBeNil)
			defer artifact.Release()

			artifact.WithPayload([]byte("test"))
			wire, err := artifact.Message().MarshalPacked()
			So(err, ShouldBeNil)
			tree.Insert([]byte("test"), wire)

			var found bool

			for inbound := range tree.Seek([]byte("test")) {
				found = true

				payload := inbound.DecryptPayload()
				So(payload, ShouldResemble, []byte("test"))
			}

			So(found, ShouldBeTrue)
		})
	})
}

func TestSeekReturnsMutableArtifacts(testingTB *testing.T) {
	Convey("Given an artifact stored in a tree", testingTB, func() {
		tree := NewTree("")
		artifact := datura.Acquire("test", datura.Artifact_Type_json)
		So(artifact, ShouldNotBeNil)
		defer artifact.Release()

		artifact.WithRole("book")
		artifact.WithScope("update")
		artifact.WithPayload([]byte(`{"channel":"book"}`))
		tree.Insert(artifact.Prefix(), artifact.Pack())

		Convey("When seeking and mutating the returned artifact", func() {
			var found bool

			for inbound := range tree.Seek([]byte("book/update")) {
				found = true
				inbound.WithRole("measurement")

				role, err := inbound.Role()
				So(err, ShouldBeNil)
				So(role, ShouldEqual, "measurement")
			}

			So(found, ShouldBeTrue)
		})
	})
}

func TestSeekSkipsInvalidArtifactValue(testingTB *testing.T) {
	Convey("Given a tree with an invalid artifact value", testingTB, func() {
		tree := NewTree("")

		Convey("When seeking that prefix", func() {
			tree.Insert([]byte("instrument/snapshot"), nil)

			var found bool

			for range tree.Seek([]byte("instrument/snapshot")) {
				found = true
			}

			So(found, ShouldBeFalse)
		})
	})
}

func TestInsert(t *testing.T) {
	Convey("Given a new tree", t, func() {
		tree := NewTree("")

		Convey("When an insert is performed", func() {
			newTree, ok := tree.Insert([]byte("test"), []byte("test"))
			So(ok, ShouldBeTrue)
			So(newTree, ShouldNotBeNil)

			// Verify the insert worked
			value, exists := newTree.Get([]byte("test"))
			So(exists, ShouldBeTrue)
			So(value, ShouldResemble, []byte("test"))
		})
	})
}

func TestGet(t *testing.T) {
	Convey("Given a new tree", t, func() {
		tree := NewTree("")

		Convey("When a get is performed", func() {
			tree.Insert([]byte("test"), []byte("test"))
			value, ok := tree.Get([]byte("test"))
			So(ok, ShouldBeTrue)
			So(value, ShouldResemble, []byte("test"))
		})
	})
}

func TestAVG(t *testing.T) {
	Convey("Given a new tree", t, func() {
		tree := NewTree("")

		Convey("When a avg is performed", func() {
			for range 128 {
				tree.Insert([]byte("test"), []byte("test"))
				tree.Get([]byte("test"))
			}

			avg := tree.AVG()
			So(avg, ShouldBeGreaterThan, 0)
		})
	})
}

func TestClose(t *testing.T) {
	Convey("Given a new tree", t, func() {
		tree := NewTree("")

		Convey("When a close is performed", func() {
			err := tree.Close()
			So(err, ShouldBeNil)
		})
	})
}

func TestUpdateTerm(t *testing.T) {
	Convey("Given a new tree", t, func() {
		tree := NewTree("")

		Convey("When a update term is performed", func() {
			tree.UpdateTerm(1)
			term, _ := tree.GetLogState()
			So(term, ShouldEqual, 1)
		})
	})
}

func TestGetLogState(t *testing.T) {
	Convey("Given a new tree", t, func() {
		tree := NewTree("")

		Convey("When a get log state is performed", func() {
			term, index := tree.GetLogState()
			So(term, ShouldEqual, 0)
			So(index, ShouldEqual, 0)
		})
	})
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
				newTree, ok := tree.Insert([]byte("test-key"), []byte("test-value"))
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
			tree1, _ = tree1.Insert([]byte(e.key), []byte(e.value))
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

func TestTreeConcurrentInsert(test *testing.T) {
	Convey("Given concurrent writers on one tree", test, func() {
		tree := NewTree("")
		defer tree.Close()

		var waitGroup sync.WaitGroup

		for workerIndex := range 32 {
			waitGroup.Add(1)

			go func(index int) {
				defer waitGroup.Done()

				key := []byte("toxicity/BTC-USD/book/" + strconv.Itoa(index) + ".")
				tree.Insert(key, []byte("book"))
			}(workerIndex)
		}

		waitGroup.Wait()

		Convey("It should retain inserted keys", func() {
			value, ok := tree.Get([]byte("toxicity/BTC-USD/book/0."))
			So(ok, ShouldBeTrue)
			So(string(value), ShouldEqual, "book")
		})
	})
}

func BenchmarkTreeInsert(b *testing.B) {
	tree := NewTree("")
	defer tree.Close()

	b.ReportAllocs()

	index := 0
	for b.Loop() {
		key := []byte(fmt.Sprintf("bench-key-%d", index))
		value := []byte(fmt.Sprintf("bench-value-%d", index))
		tree.Insert(key, value)
		index++
	}
}

func BenchmarkTreeSeek(b *testing.B) {
	tree := NewTree("")
	defer tree.Close()

	artifact := datura.Acquire("bench", datura.Artifact_Type_json)
	if artifact == nil {
		b.Fatal("Acquire returned nil")
	}
	defer artifact.Release()

	artifact.WithPayload([]byte("bench-value"))
	wire, err := artifact.Message().MarshalPacked()

	if err != nil {
		b.Fatal(err)
	}

	tree.Insert([]byte("bench-key"), wire)

	b.ReportAllocs()

	for b.Loop() {
		for range tree.Seek([]byte("bench-key")) {
		}
	}
}

func BenchmarkTreeGet(b *testing.B) {
	tree := NewTree("")
	defer tree.Close()

	const seedCount = 4096
	for i := 0; i < seedCount; i++ {
		key := []byte(fmt.Sprintf("seed-key-%d", i))
		value := []byte(fmt.Sprintf("seed-value-%d", i))
		tree.Insert(key, value)
	}

	index := 0
	b.ReportAllocs()

	for b.Loop() {
		key := []byte(fmt.Sprintf("seed-key-%d", index%seedCount))
		value, ok := tree.Get(key)
		if !ok || len(value) == 0 {
			b.Fatalf("missing key: %s", key)
		}
		index++
	}
}
