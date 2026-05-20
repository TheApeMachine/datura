package elasticsearch

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestNewClient(test *testing.T) {
	Convey("NewClient", test, func() {
		Convey("rejects no addresses", func() {
			_, err := NewClient(Config{Addresses: nil})
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "address")
		})

		Convey("rejects all blank addresses", func() {
			_, err := NewClient(Config{Addresses: []string{" ", ""}})
			So(err, ShouldNotBeNil)
		})

		Convey("accepts trimmed addresses", func() {
			_, err := NewClient(Config{Addresses: []string{" http://localhost:9200 "}})
			So(err, ShouldBeNil)
		})
	})
}
