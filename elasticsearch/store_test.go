package elasticsearch

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestStore_EnsureIndex(t *testing.T) {
	Convey("EnsureIndex", t, func() {
		client, err := NewClient(Config{Addresses: []string{"http://127.0.0.1:9200"}})
		So(err, ShouldBeNil)
		store := NewStore(client)

		Convey("rejects non-positive vector dims", func() {
			err := store.EnsureIndex(context.Background(), 0)
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "vectorDims")
		})
	})
}

func TestStore_HybridSearch(t *testing.T) {
	Convey("HybridSearch validation", t, func() {
		client, err := NewClient(Config{Addresses: []string{"http://127.0.0.1:9200"}})
		So(err, ShouldBeNil)
		store := NewStore(client)

		Convey("rejects non-positive limit", func() {
			_, err := store.HybridSearch(context.Background(), []float32{1}, "q", 0, 0.5, 0.5)
			So(err, ShouldNotBeNil)
		})

		Convey("rejects empty vector and empty query", func() {
			_, err := store.HybridSearch(context.Background(), nil, "   ", 5, 0.5, 0.5)
			So(err, ShouldNotBeNil)
		})
	})
}

func BenchmarkStoreHybridSearchValidation(b *testing.B) {
	client, err := NewClient(Config{Addresses: []string{"http://127.0.0.1:9200"}})
	if err != nil {
		b.Fatal(err)
	}

	store := NewStore(client)

	b.ResetTimer()

	for b.Loop() {
		_, err := store.HybridSearch(context.Background(), nil, "   ", 5, 0.5, 0.5)
		if err == nil {
			b.Fatal("expected validation error")
		}
	}
}
