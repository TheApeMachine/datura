package deeplake

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/datura/types"
)

func TestStoreAdapterUnsupported(t *testing.T) {
	Convey("Store adapter unsupported operations", t, func() {
		client, err := NewClient(Config{APIKey: "k", OrgID: "o", Workspace: "ws"})
		So(err, ShouldBeNil)

		store := NewStore(client)
		ctx := context.Background()

		Convey("Get is not supported", func() {
			_, err := store.Get(ctx, types.Query{ID: "doc-1"})
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "get by id is not supported")
		})

		Convey("Delete is not supported", func() {
			err := store.Delete(ctx, types.Mutation{ID: "doc-1"})
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "delete is not supported")
		})
	})
}

func TestMemoryFromResponse(t *testing.T) {
	Convey("memoryFromResponse", t, func() {
		Convey("returns empty memory for nil response", func() {
			memory := memoryFromResponse(nil)
			So(memory.Documents, ShouldBeEmpty)
		})

		Convey("maps row array JSON to documents", func() {
			response := &QueryResponse{Body: []byte(`[{"text":" alpha "},{"text":""},{"text":"beta"}]`)}
			memory := memoryFromResponse(response)

			So(len(memory.Documents), ShouldEqual, 2)
			So(memory.Documents[0].Text, ShouldEqual, " alpha ")
			So(memory.Documents[1].Text, ShouldEqual, "beta")
		})

		Convey("maps wrapped data JSON to documents", func() {
			response := &QueryResponse{Body: []byte(`{"data":[{"text":"one"}],"rows":[{"text":"ignored"}]}`)}
			memory := memoryFromResponse(response)

			So(len(memory.Documents), ShouldEqual, 1)
			So(memory.Documents[0].Text, ShouldEqual, "one")
		})
	})
}

func BenchmarkMemoryFromResponse(b *testing.B) {
	response := &QueryResponse{Body: []byte(`[{"text":"sample"},{"text":"row"}]`)}

	b.ResetTimer()

	for b.Loop() {
		memory := memoryFromResponse(response)
		if len(memory.Documents) != 2 {
			b.Fatalf("expected 2 documents, got %d", len(memory.Documents))
		}
	}
}
