package types

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

func TestMetadataMap(t *testing.T) {
	Convey("Map converts Metadata to backend JSON fields", t, func() {
		stamp := time.Unix(1700000000, 0)
		metadata := Metadata{ID: "id-1", Source: "test", Timestamp: stamp}

		mapped := metadata.Map()
		So(mapped["id"], ShouldEqual, "id-1")
		So(mapped["source"], ShouldEqual, "test")
		So(mapped["timestamp"], ShouldEqual, stamp)
	})
}

func BenchmarkMetadataMap(b *testing.B) {
	metadata := Metadata{ID: "id-1", Source: "bench", Timestamp: time.Now()}

	b.ResetTimer()

	for b.Loop() {
		mapped := metadata.Map()
		if mapped["id"] != "id-1" {
			b.Fatal("unexpected map id")
		}
	}
}
