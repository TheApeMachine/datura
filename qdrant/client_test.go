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

func BenchmarkNewClient(b *testing.B) {
	b.ResetTimer()

	for b.Loop() {
		client, err := NewClient(Config{PoolSize: 1})
		if err != nil {
			b.Fatal(err)
		}

		_ = client.Close()
	}
}

func BenchmarkMergeURLOverrides(b *testing.B) {
	b.ResetTimer()

	for b.Loop() {
		host, port, useTLS := mergeURLOverrides("http://x:9999", "", 0, false)
		if host != "x" || port != 9999 || useTLS {
			b.Fatalf("unexpected merge result: %s %d %v", host, port, useTLS)
		}
	}
}
