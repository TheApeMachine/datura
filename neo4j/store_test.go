package neo4j

import (
	"context"
	"testing"

	ndriver "github.com/neo4j/neo4j-go-driver/v5/neo4j"

	. "github.com/smartystreets/goconvey/convey"
)

func TestStore_Link(t *testing.T) {
	Convey("Store.Link", t, func() {
		driver, err := ndriver.NewDriverWithContext("neo4j://localhost:7687", ndriver.BasicAuth("neo4j", "pw", ""))
		So(err, ShouldBeNil)
		defer driver.Close(context.Background())

		store := NewStore(driver, "")
		ctx := context.Background()

		Convey("rejects invalid relationship type", func() {
			err := store.Link(ctx, "a", "b", "BAD-TYPE", nil)
			So(err, ShouldNotBeNil)
		})

		Convey("rejects empty relationship type", func() {
			err := store.Link(ctx, "a", "b", "", nil)
			So(err, ShouldNotBeNil)
		})
	})
}
