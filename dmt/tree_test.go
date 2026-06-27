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

func TestSeekStopsAtPrefixBoundary(t *testing.T) {
	Convey("Given adjacent timestamp prefixes", t, func() {
		tree := NewTree("")

		matching := datura.Acquire("match", datura.Artifact_Type_json)
		defer matching.Release()
		matching.WithPayload([]byte(`{"ok":true}`))

		adjacent := datura.Acquire("adjacent", datura.Artifact_Type_json)
		defer adjacent.Release()
		adjacent.WithPayload([]byte(`{"ok":false}`))

		tree.Insert([]byte("book/2026/06/26/08/25.json"), matching.Pack())
		tree.Insert([]byte("book/2026/06/26/08/26.json"), adjacent.Pack())

		Convey("When seeking one minute", func() {
			count := 0

			for range tree.Seek([]byte("book/2026/06/26/08/25")) {
				count++
			}

			So(count, ShouldEqual, 1)
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

func TestWalkLowerBoundScansRange(testingTB *testing.T) {
	Convey("Given lexicographically ordered tree keys", testingTB, func() {
		tree := NewTree("")
		tree.Insert([]byte("001"), []byte("1"))
		tree.Insert([]byte("002"), []byte("2"))
		tree.Insert([]byte("005"), []byte("5"))
		tree.Insert([]byte("010"), []byte("10"))
		tree.Insert([]byte("100"), []byte("100"))

		Convey("When walking from a lower bound until an upper bound", func() {
			keys := make([]string, 0)

			tree.WalkLowerBound([]byte("003"), func(key, value []byte) bool {
				if string(key) >= "050" {
					return false
				}

				keys = append(keys, string(key))

				return true
			})

			So(keys, ShouldResemble, []string{"005", "010"})
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
			_, ok := tree.Insert([]byte(key), []byte(value))
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

func TestInsertArtifact(testingTB *testing.T) {
	Convey("Given a tree and artifact", testingTB, func() {
		tree := NewTree("")
		artifact := datura.Acquire("test", datura.APPJSON)
		So(artifact, ShouldNotBeNil)

		defer artifact.Release()

		artifact.WithRole("ticker")
		artifact.WithScope("update")
		artifact.WithOrigin("kraken:public")
		artifact.WithPayload([]byte(`{"channel":"ticker"}`))

		Convey("When inserting with an explicit prefix", func() {
			_, ok := tree.InsertArtifact(
				[]byte("ticker/update/kraken:public"),
				tree.WithCognition(artifact),
			)

			So(ok, ShouldBeTrue)

			var found bool

			for inbound := range tree.Seek([]byte("ticker/update/kraken:public")) {
				found = true
				So(datura.Peek[string](inbound, "cognition", "sequence", "value"), ShouldEqual, "update_kraken:public_ticker")
			}

			So(found, ShouldBeTrue)
		})
	})
}

func TestWithCognition(testingTB *testing.T) {
	Convey("Given a trained context path", testingTB, func() {
		tree := NewTree("")

		_, _ = tree.InsertContextWeight([]byte("update"), PackedWeight{
			Count:       10,
			Probability: 1.0,
		})
		_, _ = tree.InsertContextWeight([]byte("update_kraken:public"), PackedWeight{
			Count:       4,
			Probability: 0.5,
		})

		artifact := datura.Acquire("test", datura.APPJSON)
		So(artifact, ShouldNotBeNil)

		defer artifact.Release()

		artifact.WithRole("ticker")
		artifact.WithScope("update")
		artifact.WithOrigin("kraken:public")
		artifact.WithPayload([]byte(`{}`))

		Convey("When the cognitive engine stamps a known suffix", func() {
			stamped := NewCognitiveEngine(tree).Stamp(artifact)

			So(datura.Peek[float64](stamped, "cognition", "surprise", "value"), ShouldBeGreaterThan, 0)
			So(datura.Peek[string](stamped, "cognition", "sequence", "value"), ShouldEqual, "update_kraken:public_ticker")
		})
	})
}

func BenchmarkCognitiveEngineStamp(benchmark *testing.B) {
	tree := NewTree("")
	engine := NewCognitiveEngine(tree)

	_, _ = tree.InsertContextWeight([]byte("update"), PackedWeight{
		Count:       10,
		Probability: 1.0,
	})
	_, _ = tree.InsertContextWeight([]byte("update_kraken:public"), PackedWeight{
		Count:       4,
		Probability: 0.5,
	})

	benchmark.ReportAllocs()

	for benchmark.Loop() {
		artifact := datura.Acquire("test", datura.APPJSON)
		artifact.WithRole("ticker")
		artifact.WithScope("update")
		artifact.WithOrigin("kraken:public")
		artifact.WithPayload([]byte(`{}`))

		engine.Stamp(artifact)
		artifact.Release()
	}
}
