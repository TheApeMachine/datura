package neo4j

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestNewClient(test *testing.T) {
	Convey("NewClient", test, func() {
		Convey("rejects empty URI", func() {
			_, err := NewClient(Config{URI: ""})
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "URI")
		})

		Convey("accepts URI string", func() {
			_, err := NewClient(Config{URI: "neo4j://localhost:7687", Username: "u", Password: "p"})
			So(err, ShouldBeNil)
		})
	})
}
