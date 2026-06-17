package dmt

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestForestSynchronizeTrees(t *testing.T) {
	Convey("Given a forest with seeded data", t, func() {
		forest, err := NewForest(ForestConfig{})
		So(err, ShouldBeNil)
		defer forest.Close()

		forest.Insert([]byte("sync-key"), []byte("sync-value"))

		emptyTree := NewTree("")

		Convey("When synchronizing a trailing tree", func() {
			forest.synchronizeTrees(append(forest.snapshot.Load().Trees(), emptyTree))

			Convey("Then it should share the reference root without copying entries", func() {
				reference := forest.getFastestTree()
				So(emptyTree.loadRoot(), ShouldEqual, reference.loadRoot())

				value, exists := emptyTree.Get([]byte("sync-key"))
				So(exists, ShouldBeTrue)
				So(value, ShouldResemble, []byte("sync-value"))
			})
		})
	})
}

func BenchmarkForestSynchronizeTrees(b *testing.B) {
	forest, err := NewForest(ForestConfig{})
	if err != nil {
		b.Fatal(err)
	}
	defer forest.Close()

	for index := range 10000 {
		key := []byte("bench-key-" + string(rune('a'+index%26)))
		forest.Insert(key, []byte("value"))
	}

	trailingTree := NewTree("")
	trees := forest.snapshot.Load().Trees()

	for b.Loop() {
		forest.synchronizeTrees(append(trees, trailingTree))
	}
}
