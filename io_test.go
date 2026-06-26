package datura

import (
	"context"
	"io"
	"net"
	"sync"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func testArtifact() *Artifact {
	return Acquire(
		"test", Artifact_Type_json,
	).WithPayload(
		[]byte("test payload"),
	)
}

func TestPackInto(t *testing.T) {
	Convey("Given an artifact", t, func() {
		artifact := testArtifact()
		expected := artifact.Pack()

		Convey("When the artifact is packed into a buffer", func() {
			buffer := make([]byte, len(expected))
			n, err := artifact.PackInto(buffer)

			So(err, ShouldEqual, io.EOF)
			So(n, ShouldEqual, len(expected))
			So(buffer, ShouldResemble, expected)
		})
	})
}

func TestPackIntoShortBuffer(t *testing.T) {
	Convey("Given an artifact and a short destination buffer", t, func() {
		artifact := testArtifact()
		buffer := make([]byte, 3)

		Convey("When the artifact is packed into the buffer", func() {
			n, err := artifact.PackInto(buffer)

			So(err, ShouldEqual, io.ErrShortBuffer)
			So(n, ShouldEqual, len(buffer))
		})
	})
}

func TestUnpack(t *testing.T) {
	Convey("Given an empty artifact", t, func() {
		empty := &Artifact{}

		Convey("When unpacking a packed artifact frame", func() {
			artifact := testArtifact()
			wire := artifact.Pack()
			n, err := empty.Unpack(wire)

			So(err, ShouldBeNil)
			So(n, ShouldEqual, len(wire))
			So(empty.DecryptPayload(), ShouldResemble, artifact.DecryptPayload())
		})
	})
}

func TestUnpackRejectsInvalidWire(testingTB *testing.T) {
	Convey("Given an artifact and invalid packed data", testingTB, func() {
		artifact := Acquire("unpack-invalid", Artifact_Type_json)

		Convey("When unpacking the invalid data", func() {
			written, err := artifact.Unpack(nil)

			So(written, ShouldEqual, 0)
			So(err, ShouldNotBeNil)
		})
	})
}

func TestArtifactStreamRPC(t *testing.T) {
	Convey("Given a Cap'n Proto artifact stream over a net.Pipe", t, func() {
		ctx := context.Background()
		serverSide, clientSide := net.Pipe()
		var mu sync.Mutex
		received := make([]string, 0, 2)
		doneCalled := false

		server := NewArtifactStream(
			func(_ context.Context, artifact *Artifact) error {
				mu.Lock()
				defer mu.Unlock()

				received = append(received, string(artifact.DecryptPayload()))

				return nil
			},
			func(context.Context) error {
				mu.Lock()
				defer mu.Unlock()

				doneCalled = true

				return nil
			},
		)

		serverConn := NewArtifactStreamConnection(serverSide, server)
		defer serverConn.Close()

		client, clientConn := NewArtifactStreamClient(ctx, clientSide)
		defer clientConn.Close()

		first := Acquire("stream-test", Artifact_Type_json).WithPayload([]byte("first"))
		second := Acquire("stream-test", Artifact_Type_json).WithPayload([]byte("second"))

		Convey("When two artifacts are sent and the stream is closed", func() {
			err := client.Send(ctx, first, second)

			mu.Lock()
			defer mu.Unlock()

			So(err, ShouldBeNil)
			So(doneCalled, ShouldBeTrue)
			So(received, ShouldResemble, []string{"first", "second"})
		})
	})
}

func BenchmarkPackUnpack(benchmark *testing.B) {
	source := testArtifact()
	wire := source.Pack()
	buffer := make([]byte, len(wire))

	benchmark.ReportAllocs()
	benchmark.ResetTimer()

	for benchmark.Loop() {
		target := &Artifact{}

		if _, err := target.Unpack(wire); err != nil {
			benchmark.Fatal(err)
		}

		if _, err := source.PackInto(buffer); err != io.EOF {
			benchmark.Fatal(err)
		}
	}
}
