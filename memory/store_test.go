package memory

import (
	"context"
	"fmt"
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

		Convey("Search ranks documents by embedding cosine similarity", func() {
			_ = store.Put(ctx, types.Mutation{ID: "x", Text: "east", Embedding: []float32{1, 0}})
			_ = store.Put(ctx, types.Mutation{ID: "y", Text: "north", Embedding: []float32{0, 1}})
			_ = store.Put(ctx, types.Mutation{ID: "z", Text: "northeast", Embedding: []float32{0.8, 0.2}})

			mem, err := store.Search(ctx, types.Query{Embedding: []float32{1, 0}, Limit: 2})

			So(err, ShouldBeNil)
			So(len(mem.Documents), ShouldEqual, 2)
			So(mem.Documents[0].ID, ShouldEqual, "x")
			So(mem.Documents[1].ID, ShouldEqual, "z")
		})

		Convey("Search applies score threshold to embedding results", func() {
			threshold := float32(0.99)

			_ = store.Put(ctx, types.Mutation{ID: "x", Text: "east", Embedding: []float32{1, 0}})
			_ = store.Put(ctx, types.Mutation{ID: "z", Text: "northeast", Embedding: []float32{0.8, 0.2}})

			mem, err := store.Search(ctx, types.Query{
				Embedding:      []float32{1, 0},
				ScoreThreshold: &threshold,
			})

			So(err, ShouldBeNil)
			So(len(mem.Documents), ShouldEqual, 1)
			So(mem.Documents[0].ID, ShouldEqual, "x")
		})

		Convey("Search combines text and embedding scores deterministically", func() {
			_ = store.Put(ctx, types.Mutation{ID: "text-vector", Text: "alpha topic", Embedding: []float32{0.8, 0.2}})
			_ = store.Put(ctx, types.Mutation{ID: "vector-only", Text: "other", Embedding: []float32{1, 0}})

			mem, err := store.Search(ctx, types.Query{Text: "alpha", Embedding: []float32{1, 0}, Limit: 2})

			So(err, ShouldBeNil)
			So(len(mem.Documents), ShouldEqual, 2)
			So(mem.Documents[0].ID, ShouldEqual, "text-vector")
			So(mem.Documents[1].ID, ShouldEqual, "vector-only")
		})

		Convey("Search returns no matches for unmatched text", func() {
			_ = store.Put(ctx, types.Mutation{ID: "a", Text: "beta gamma"})

			mem, err := store.Search(ctx, types.Query{Text: "missing", Limit: 10})

			So(err, ShouldBeNil)
			So(mem.Documents, ShouldBeEmpty)
		})

		Convey("Get rejects empty query id", func() {
			_, err := store.Get(ctx, types.Query{})
			So(err, ShouldNotBeNil)
		})

		Convey("Search rejects empty text and embedding", func() {
			_, err := store.Search(ctx, types.Query{})
			So(err, ShouldNotBeNil)
		})
	})
}

func BenchmarkStorePutGet(b *testing.B) {
	store := NewStore()
	ctx := context.Background()
	mutation := types.Mutation{ID: "bench", Text: "benchmark payload", Embedding: []float32{0.1, 0.2}}

	b.ResetTimer()

	for b.Loop() {
		if err := store.Put(ctx, mutation); err != nil {
			b.Fatal(err)
		}

		memory, err := store.Get(ctx, types.Query{ID: "bench"})
		if err != nil {
			b.Fatal(err)
		}

		if len(memory.Documents) != 1 {
			b.Fatalf("expected 1 document, got %d", len(memory.Documents))
		}
	}
}

func BenchmarkStoreSearch(b *testing.B) {
	store := NewStore()
	ctx := context.Background()

	for index := range 128 {
		_ = store.Put(ctx, types.Mutation{
			ID:   fmt.Sprintf("doc-%d", index),
			Text: fmt.Sprintf("document number %d with searchable token", index),
		})
	}

	query := types.Query{Text: "searchable", Limit: 10}

	b.ResetTimer()

	for b.Loop() {
		memory, err := store.Search(ctx, query)
		if err != nil {
			b.Fatal(err)
		}

		if len(memory.Documents) == 0 {
			b.Fatal("expected search hits")
		}
	}
}
