package qdrant

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestNewClient(test *testing.T) {
	Convey("NewClient uses defaults for host and port", test, func() {
		client, err := NewClient(Config{})
		So(err, ShouldBeNil)
		So(client, ShouldNotBeNil)
		So(client.Native(), ShouldNotBeNil)
		So(client.Close(), ShouldBeNil)
	})
}

func TestMergeURLOverrides_preservesExplicitGRPCPortFromURL(test *testing.T) {
	Convey("non-6333 URL port is kept", test, func() {
		host, port, useTLS := mergeURLOverrides("http://x:9999", "", 0, false)
		So(host, ShouldEqual, "x")
		So(port, ShouldEqual, 9999)
		So(useTLS, ShouldBeFalse)
	})
}
