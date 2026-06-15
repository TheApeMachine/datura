package elasticsearch

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/datura/types"
)

func TestStoreAdapterValidation(t *testing.T) {
	Convey("Store adapter validation", t, func() {
		client, err := NewClient(Config{Addresses: []string{"http://127.0.0.1:9200"}})
		So(err, ShouldBeNil)

		store := NewStore(client)
		ctx := context.Background()

		Convey("Get requires query id", func() {
			_, err := store.Get(ctx, types.Query{})
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "query id is required")
		})

		Convey("Put requires mutation id", func() {
			err := store.Put(ctx, types.Mutation{})
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "mutation id is required")
		})

		Convey("Delete requires mutation id", func() {
			err := store.Delete(ctx, types.Mutation{})
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "mutation id is required")
		})
	})
}

func TestMemoryFromResponse(t *testing.T) {
	Convey("memoryFromResponse", t, func() {
		Convey("returns empty memory for nil response", func() {
			memory := memoryFromResponse(nil)
			So(memory.Documents, ShouldBeEmpty)
		})

		Convey("returns empty memory for invalid JSON", func() {
			memory := memoryFromResponse(&HTTPResponse{Body: []byte("{")})
			So(memory.Documents, ShouldBeEmpty)
		})

		Convey("maps search hits to documents", func() {
			body := []byte(`{"hits":{"hits":[{"_id":"doc-1","_source":{"text":"hello","embedding":[0.1,0.2]}}]}}`)
			memory := memoryFromResponse(&HTTPResponse{Body: body})

			So(len(memory.Documents), ShouldEqual, 1)
			So(memory.Documents[0].ID, ShouldEqual, "doc-1")
			So(memory.Documents[0].Text, ShouldEqual, "hello")
			So(memory.Documents[0].Embedding, ShouldResemble, []float32{0.1, 0.2})
		})
	})
}

func BenchmarkMemoryFromResponse(b *testing.B) {
	body := []byte(`{"hits":{"hits":[{"_id":"doc-1","_source":{"text":"hello","embedding":[0.1,0.2,0.3]}}]}}`)
	response := &HTTPResponse{Body: body}

	b.ResetTimer()

	for b.Loop() {
		memory := memoryFromResponse(response)
		if len(memory.Documents) != 1 {
			b.Fatalf("expected 1 document, got %d", len(memory.Documents))
		}
	}
}
