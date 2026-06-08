package memory

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/datura/types"
)

func TestStore(t *testing.T) {
	Convey("Store", t, func() {
		store := NewStore()
		ctx := context.Background()

		Convey("Put and Get round-trip", func() {
			err := store.Put(ctx, types.Mutation{ID: "k", Text: "v"})
			So(err, ShouldBeNil)

			mem, err := store.Get(ctx, types.Query{ID: "k"})
			So(err, ShouldBeNil)
			So(mem.Documents[0].Text, ShouldEqual, "v")
		})

		Convey("Delete removes a document", func() {
			_ = store.Put(ctx, types.Mutation{ID: "k", Text: "v"})
			err := store.Delete(ctx, types.Mutation{ID: "k"})
			So(err, ShouldBeNil)

			_, err = store.Get(ctx, types.Query{ID: "k"})
			So(err, ShouldNotBeNil)
		})

		Convey("Search matches document text", func() {
			_ = store.Put(ctx, types.Mutation{ID: "a", Text: "beta gamma"})
			mem, err := store.Search(ctx, types.Query{Text: "gamma", Limit: 10})
			So(err, ShouldBeNil)
			So(mem.Documents[0].Text, ShouldEqual, "beta gamma")
		})
	})
}
