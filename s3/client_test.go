package s3

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestNewClient(t *testing.T) {
	Convey("NewClient rejects bucket_url together with bucket or region", t, func() {
		_, err := NewClient(context.Background(), Config{
			BucketURL: "s3://x?region=us-east-1",
			Bucket:    "y",
		})

		So(err, ShouldNotBeNil)
	})

	Convey("NewClient rejects missing bucket without bucket_url", t, func() {
		_, err := NewClient(context.Background(), Config{Region: "us-east-1"})

		So(err, ShouldNotBeNil)
	})

	Convey("NewClient rejects missing region without bucket_url", t, func() {
		_, err := NewClient(context.Background(), Config{Bucket: "my-bucket"})

		So(err, ShouldNotBeNil)
	})

	Convey("NewClient with bucket and region yields a usable client shell", t, func() {
		client, err := NewClient(context.Background(), Config{
			Bucket: "my-bucket",
			Region: "us-east-1",
		})

		So(err, ShouldBeNil)
		So(client, ShouldNotBeNil)
		So(client.Bucket(), ShouldNotBeNil)

		So(client.Close(), ShouldBeNil)
	})
}

func BenchmarkNewClient(b *testing.B) {
	ctx := context.Background()
	cfg := Config{
		Bucket: "my-bucket",
		Region: "us-east-1",
	}

	b.ResetTimer()

	for range b.N {
		client, err := NewClient(ctx, cfg)

		if err != nil {
			b.Fatal(err)
		}

		_ = client.Close()
	}
}
