package neo4j

import (
	"context"
	"testing"

	ndriver "github.com/neo4j/neo4j-go-driver/v5/neo4j"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/datura/types"
)

func TestStoreAdapterValidation(t *testing.T) {
	Convey("Store adapter validation", t, func() {
		driver, err := ndriver.NewDriverWithContext("neo4j://localhost:7687", ndriver.BasicAuth("neo4j", "pw", ""))
		So(err, ShouldBeNil)
		defer driver.Close(context.Background())

		store := NewStore(driver, "")
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

		Convey("Put with relationship requires mutation id", func() {
			err := store.Put(ctx, types.Mutation{Relationship: "LINKS", RelatedID: "b"})
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "mutation id is required for relationships")
		})

		Convey("Delete requires mutation id", func() {
			err := store.Delete(ctx, types.Mutation{})
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "mutation id is required")
		})

		Convey("Search requires text query", func() {
			_, err := store.Search(ctx, types.Query{})
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "text query is required")
		})
	})
}

func TestStringValue(t *testing.T) {
	Convey("stringValue", t, func() {
		So(stringValue(nil), ShouldEqual, "")
		So(stringValue("hello"), ShouldEqual, "hello")
		So(stringValue(42), ShouldEqual, "42")
	})
}

func TestFloatsFromAny(t *testing.T) {
	Convey("floatsFromAny", t, func() {
		So(floatsFromAny(nil), ShouldBeEmpty)

		values := []any{float64(1.5), float32(2.5), int64(3)}
		So(floatsFromAny(values), ShouldResemble, []float32{1.5, 2.5, 3})
	})
}

func BenchmarkFloatsFromAny(b *testing.B) {
	values := make([]any, 128)
	for index := range values {
		values[index] = float64(index)
	}

	b.ResetTimer()

	for b.Loop() {
		out := floatsFromAny(values)
		if len(out) != 128 {
			b.Fatalf("expected 128 floats, got %d", len(out))
		}
	}
}
