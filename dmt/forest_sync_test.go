package dmt

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestForestReplicaApplyAfterCommit(t *testing.T) {
	Convey("Given a forest with seeded data", t, func() {
		forest, err := NewForest(ForestConfig{})
		So(err, ShouldBeNil)
		defer forest.Close()

		So(forest.Insert([]byte("sync-key"), []byte("sync-value")), ShouldBeNil)

		replica, err := NewTree("")
		So(err, ShouldBeNil)
		So(forest.AddTree(replica), ShouldBeNil)

		Convey("When inserting after the replica joins", func() {
			So(forest.Insert([]byte("later-key"), []byte("later-value")), ShouldBeNil)

			Convey("Then the replica should apply the committed entry", func() {
				value, exists := replica.Get([]byte("later-key"))
				So(exists, ShouldBeTrue)
				So(value, ShouldResemble, []byte("later-value"))
				So(replica.AppliedIndex(), ShouldEqual, forest.commitIndex.Load())
			})
		})
	})
}

func BenchmarkForestReplicaApply(b *testing.B) {
	forest, err := NewForest(ForestConfig{})
	if err != nil {
		b.Fatal(err)
	}
	defer forest.Close()

	replica, err := NewTree("")
	if err != nil {
		b.Fatal(err)
	}

	if err := forest.AddTree(replica); err != nil {
		b.Fatal(err)
	}

	for b.Loop() {
		if err := forest.Insert([]byte("bench-key"), []byte("value")); err != nil {
			b.Fatal(err)
		}
	}
}
