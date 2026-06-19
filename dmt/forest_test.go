package dmt

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/datura"
)

func TestNewForest(t *testing.T) {
	Convey("Given a forest configuration", t, func() {
		tmpDir := filepath.Join(os.TempDir(), "radix-test-"+time.Now().Format("20060102150405"))
		defer os.RemoveAll(tmpDir)

		config := ForestConfig{
			PersistDir: tmpDir,
		}

		Convey("When creating a new forest", func() {
			forest, err := NewForest(config)
			So(err, ShouldBeNil)
			defer forest.Close()

			Convey("Then it should be properly initialized", func() {
				So(forest, ShouldNotBeNil)
				So(forest.snapshot.Load().Trees(), ShouldNotBeNil)
				So(forest.ctx, ShouldNotBeNil)
				So(forest.cancel, ShouldNotBeNil)
			})

			Convey("And it should have one initial tree", func() {
				So(len(forest.snapshot.Load().Trees()), ShouldEqual, 1)
				So(forest.snapshot.Load().Trees()[0], ShouldNotBeNil)
			})
		})
	})
}

func TestForestUsesProvidedPool(t *testing.T) {
	Convey("Given a forest configuration with a worker pool", t, func() {
		workerPool := newWorkerPool(context.Background())
		defer workerPool.Close()

		forest, err := NewForest(ForestConfig{
			Pool: workerPool,
		})
		So(err, ShouldBeNil)
		defer forest.Close()

		Convey("Then the forest should reuse the provided pool", func() {
			So(forest.pool, ShouldEqual, workerPool)
			So(forest.owned, ShouldBeFalse)
		})
	})
}

func TestForestOperations(t *testing.T) {
	Convey("Given a new forest", t, func() {
		tmpDir := filepath.Join(os.TempDir(), "radix-test-"+time.Now().Format("20060102150405"))
		defer os.RemoveAll(tmpDir)

		forest, err := NewForest(ForestConfig{PersistDir: tmpDir})
		So(err, ShouldBeNil)
		defer forest.Close()

		Convey("When performing insert operations", func() {
			forest.Insert([]byte("key1"), []byte("value1"))
			forest.Insert([]byte("key2"), []byte("value2"))

			Convey("Then the data should be retrievable", func() {
				value1, exists := forest.Get([]byte("key1"))
				So(exists, ShouldBeTrue)
				So(value1, ShouldResemble, []byte("value1"))

				value2, exists := forest.Get([]byte("key2"))
				So(exists, ShouldBeTrue)
				So(value2, ShouldResemble, []byte("value2"))
			})

			Convey("And seek operations should work", func() {
				artifact := datura.Acquire("forest", datura.Artifact_Type_json)
				So(artifact, ShouldNotBeNil)
				defer artifact.Release()

				artifact.WithPayload([]byte("value1"))
				forest.Insert(artifact.Prefix(), artifact.Segment().Data())

				var found bool

				for result := range forest.Seek([]byte("key")) {
					found = true

					payload := result.DecryptPayload()
					So(payload, ShouldResemble, []byte("value1"))
				}

				So(found, ShouldBeTrue)
			})
		})
	})
}

func TestForestSynchronization(t *testing.T) {
	Convey("Given a forest with multiple trees", t, func() {
		tmpDir := filepath.Join(os.TempDir(), "radix-test-"+time.Now().Format("20060102150405"))
		defer os.RemoveAll(tmpDir)

		forest, err := NewForest(ForestConfig{PersistDir: tmpDir})
		So(err, ShouldBeNil)
		defer forest.Close()

		// Add a second tree
		tree2 := NewTree("")
		So(err, ShouldBeNil)
		forest.AddTree(tree2)

		Convey("When inserting data", func() {
			forest.Insert([]byte("sync-key"), []byte("sync-value"))

			Convey("Then all trees should have the data", func() {
				for _, tree := range forest.snapshot.Load().Trees() {
					value, exists := tree.Get([]byte("sync-key"))
					So(exists, ShouldBeTrue)
					So(value, ShouldResemble, []byte("sync-value"))
				}
			})
		})
	})
}

func TestForestAddTreeSynchronization(t *testing.T) {
	Convey("Given a forest with existing data", t, func() {
		forest, err := NewForest(ForestConfig{})
		So(err, ShouldBeNil)
		defer forest.Close()

		forest.Insert([]byte("seed-key"), []byte("seed-value"))

		tree2 := NewTree("")
		So(err, ShouldBeNil)

		Convey("When a new tree is added", func() {
			forest.AddTree(tree2)

			Convey("Then the new tree should receive the existing data", func() {
				value, exists := tree2.Get([]byte("seed-key"))
				So(exists, ShouldBeTrue)
				So(value, ShouldResemble, []byte("seed-value"))
			})
		})
	})
}

func TestForestPerformance(t *testing.T) {
	Convey("Given a forest with multiple trees", t, func() {
		forest, err := NewForest(ForestConfig{})
		So(err, ShouldBeNil)
		defer forest.Close()

		// Add trees with different simulated performance characteristics
		tree1 := NewTree("")
		tree2 := NewTree("")

		forest.AddTree(tree1)
		forest.AddTree(tree2)

		Convey("When getting the fastest tree", func() {
			fastestTree := forest.getFastestTree()

			Convey("Then it should return a valid tree", func() {
				So(fastestTree, ShouldNotBeNil)
				So(fastestTree.AVG(), ShouldBeGreaterThanOrEqualTo, 0)
			})
		})
	})
}

func TestForestNetworking(t *testing.T) {
	Convey("Given a forest with network configuration", t, func() {
		tmpDir := filepath.Join(os.TempDir(), "radix-test-"+time.Now().Format("20060102150405"))
		defer os.RemoveAll(tmpDir)

		config := ForestConfig{
			PersistDir: tmpDir,
			Network: &NetworkConfig{
				ListenAddr: ":0", // Use random port
				NodeID:     "test-node",
			},
		}

		forest, err := NewForest(config)
		So(err, ShouldBeNil)
		defer forest.Close()

		Convey("Then the network node should be initialized", func() {
			So(forest.network, ShouldNotBeNil)
			So(forest.network.config.NodeID, ShouldEqual, "test-node")
		})

		Convey("When inserting data with networking enabled", func() {
			forest.Insert([]byte("network-key"), []byte("network-value"))

			Convey("Then the network node should broadcast the insert", func() {
				// Verify local insertion worked
				value, exists := forest.Get([]byte("network-key"))
				So(exists, ShouldBeTrue)
				So(value, ShouldResemble, []byte("network-value"))

				// Network metrics should be updated
				metrics := forest.network.GetMetrics()
				networkStats := metrics["network"].(map[string]interface{})
				So(networkStats["bytes_tx"], ShouldBeGreaterThan, 0)
			})
		})
	})
}

func TestForestClose(t *testing.T) {
	Convey("Given a forest with multiple components", t, func() {
		tmpDir := filepath.Join(os.TempDir(), "radix-test-"+time.Now().Format("20060102150405"))
		defer os.RemoveAll(tmpDir)

		config := ForestConfig{
			PersistDir: tmpDir,
			Network: &NetworkConfig{
				ListenAddr: ":0",
				NodeID:     "test-node",
			},
		}

		forest, err := NewForest(config)
		So(err, ShouldBeNil)

		// Add additional tree
		tree2 := NewTree("")
		forest.AddTree(tree2)

		Convey("When closing the forest", func() {
			forest.Close()

			Convey("Then the context should be cancelled", func() {
				select {
				case <-forest.ctx.Done():
					// Context was cancelled as expected
					So(true, ShouldBeTrue)
				default:
					So(false, ShouldBeTrue, "Context was not cancelled")
				}
			})
		})
	})
}

func TestForestConcurrentInsert(test *testing.T) {
	Convey("Given concurrent writers on one forest", test, func() {
		forest, err := NewForest(ForestConfig{})
		So(err, ShouldBeNil)
		defer forest.Close()

		var waitGroup sync.WaitGroup

		for workerIndex := range 32 {
			waitGroup.Add(1)

			go func(index int) {
				defer waitGroup.Done()

				key := []byte("toxicity/BTC-USD/book/" + strconv.Itoa(index) + ".")
				forest.Insert(key, []byte("book"))
			}(workerIndex)
		}

		waitGroup.Wait()

		Convey("It should retain inserted keys", func() {
			value, ok := forest.Get([]byte("toxicity/BTC-USD/book/0."))
			So(ok, ShouldBeTrue)
			So(string(value), ShouldEqual, "book")
		})
	})
}

func TestForestConcurrentAddTree(test *testing.T) {
	Convey("Given concurrent tree registration", test, func() {
		forest, err := NewForest(ForestConfig{})
		So(err, ShouldBeNil)
		defer forest.Close()

		forest.Insert([]byte("seed"), []byte("value"))

		var waitGroup sync.WaitGroup

		for range 8 {
			waitGroup.Go(func() {
				tree := NewTree("")
				defer tree.Close()
				forest.AddTree(tree)
			})
		}

		waitGroup.Wait()

		Convey("It should keep every tree consistent with the seed key", func() {
			for _, tree := range forest.snapshot.Load().Trees() {
				value, ok := tree.Get([]byte("seed"))
				So(ok, ShouldBeTrue)
				So(string(value), ShouldEqual, "value")
			}
		})
	})
}
