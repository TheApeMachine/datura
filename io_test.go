package datura

import (
	"bytes"
	"io"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/datura/transport"
	"github.com/theapemachine/errnie"
)

func TestRWCStreamRead(t *testing.T) {
	Convey("Setup", t, func() {
		artifact := Acquire(
			"test-origin", APPJSON,
		).WithRole(
			"test-role",
		).WithScope(
			"test-scope",
		).WithPayload(Map[any]{
			"testkey": "testvalue",
		}.Marshal())

		Convey("Given an Artifact wrapped in an RWCStream", func() {
			stream := NewRWCStream(artifact)

			Convey("When using io.Copy", func() {
				result := bytes.NewBuffer([]byte{})
				n, err := io.Copy(result, stream)

				So(err, ShouldBeNil)
				So(n, ShouldNotEqual, 0)

				Convey("Result should receive the full artifact", func() {
					out := Acquire(
						"test-result", APPJSON,
					)

					out.Unpack(result.Bytes())
					payload := out.DecryptPayload()

					So(payload, ShouldResemble, artifact.DecryptPayload())
				})
			})
		})
	})
}

func TestRWCStreamWrite(t *testing.T) {
	Convey("Setup", t, func() {
		source := Acquire(
			"test-source", APPJSON,
		).WithRole(
			"source-role",
		).WithScope(
			"source-scope",
		).WithPayload(Map[any]{
			"answer": 42,
		}.Marshal())

		target := Acquire(
			"test-target", APPJSON,
		).WithPayload(Map[any]{
			"answer": 0,
		}.Marshal())

		Convey("Given an Artifact wrapped in an RWCStream", func() {
			stream := NewRWCStream(target)
			wire := source.Pack()
			split := len(wire) / 2

			Convey("When writing one packed artifact as chunks", func() {
				first, err := stream.Write(wire[:split])
				So(err, ShouldBeNil)
				So(first, ShouldEqual, split)
				So(Peek[int](target, "answer"), ShouldEqual, 0)

				second, err := stream.Write(wire[split:])
				So(err, ShouldBeNil)
				So(second, ShouldEqual, len(wire)-split)

				Convey("Then the complete artifact should commit once", func() {
					So(Peek[int](target, "answer"), ShouldEqual, 42)
					So(Peek[string](target, "role"), ShouldEqual, "source-role")
					So(Peek[string](target, "scope"), ShouldEqual, "source-scope")
				})
			})
		})
	})
}

type Compute struct {
	artifact *Artifact
	writes   []byte
}

func (modb *Compute) Read(p []byte) (n int, err error) {
	state := Acquire("feature-extractor", APPJSON)

	if _, err := state.Unpack(modb.artifact.DecryptPayload()); err != nil {
		return 0, errnie.Error(errnie.Err(
			errnie.Validation,
			"feature-extractor: state write failed",
			err,
		))
	}

	modb.writes = nil
	state.Poke("test", "output")
	return state.PackInto(p)
}

func (modb *Compute) Write(p []byte) (n int, err error) {
	modb.writes = append(modb.writes, p...)
	modb.artifact.WithPayload(modb.writes)

	return len(p), nil
}

func (modb *Compute) Close() (err error) {
	return nil
}

func TestRWCIntergrtion(t *testing.T) {
	Convey("Setup", t, func() {
		source := Acquire(
			"test-source", APPJSON,
		).WithRole(
			"source-role",
		).WithScope(
			"source-scope",
		).WithPayload(Map[any]{
			"answer": 42,
		}.Marshal())

		Convey("Given an Artifact wrapped in an RWCStream", func() {
			stream := NewRWCStream(source)
			modb := &Compute{
				artifact: Acquire("test", APPJSON),
			}

			Convey("When FlipFlopping", func() {
				err := transport.NewFlipFlop(stream, modb)
				So(err, ShouldBeNil)

				Convey("It should contain the new data", func() {
					So(Peek[string](source, "output"), ShouldEqual, "test")
				})
			})
		})
	})
}
