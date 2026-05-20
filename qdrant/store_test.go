package qdrant

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestStore_CreateCollection(t *testing.T) {
	Convey("CreateCollection validation", t, func() {
		client, err := NewClient(Config{PoolSize: 1})
		So(err, ShouldBeNil)
		defer client.Close()

		store := NewStoreWithCollection(client, "kb")

		err = store.CreateCollection(context.Background(), 0, "Cosine")
		So(err, ShouldNotBeNil)
		So(err.Error(), ShouldContainSubstring, "vectorSize")
	})
}

func TestStore_Search(t *testing.T) {
	Convey("Search validation", t, func() {
		client, err := NewClient(Config{PoolSize: 1})
		So(err, ShouldBeNil)
		defer client.Close()

		store := NewStore(client)

		Convey("rejects invalid limit", func() {
			_, err := store.Search(context.Background(), []float32{1}, 0, nil)
			So(err, ShouldNotBeNil)
		})

		Convey("rejects empty vector", func() {
			_, err := store.Search(context.Background(), nil, 3, nil)
			So(err, ShouldNotBeNil)
		})
	})
}

func TestStore_UpsertPoints_empty(t *testing.T) {
	Convey("UpsertPoints no-op on empty slice", t, func() {
		client, err := NewClient(Config{PoolSize: 1})
		So(err, ShouldBeNil)
		defer client.Close()

		store := NewStore(client)
		err = store.UpsertPoints(context.Background(), nil)
		So(err, ShouldBeNil)
	})
}
