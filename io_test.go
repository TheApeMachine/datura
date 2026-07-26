package datura

import (
	"bytes"
	"io"
	"strings"
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
		).WithPayload(mustMapMarshal(Map[any]{
			"testkey": "testvalue",
		}))

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
		).WithPayload(mustMapMarshal(Map[any]{
			"answer": 42,
		}))

		target := Acquire(
			"test-target", APPJSON,
		).WithPayload(mustMapMarshal(Map[any]{
			"answer": 0,
		}))

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
		).WithPayload(mustMapMarshal(Map[any]{
			"answer": 42,
		}))

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

func TestRWCStreamLargeArtifactPipelineIntegration(t *testing.T) {
	Convey("Setup", t, func() {
		source := Acquire(
			"test-source", APPJSON,
		).WithRole(
			"source-role",
		).WithScope(
			"source-scope",
		).WithPayload(mustMapMarshal(Map[any]{
			"answer": 42,
			"large":  strings.Repeat("x", 128*1024),
		}))

		Convey("Given a large Artifact wrapped in an RWCStream", func() {
			stream := NewRWCStream(source)
			modb := &Compute{
				artifact: Acquire("test", APPJSON),
			}
			pipeline := transport.NewPipeline(modb)

			Convey("When FlipFlopping through a pipeline stage that unpacks inbound state", func() {
				err := transport.NewFlipFlop(stream, pipeline)
				So(err, ShouldBeNil)

				Convey("It should preserve the complete artifact frame", func() {
					So(Peek[string](source, "output"), ShouldEqual, "test")
					So(Peek[string](source, "large"), ShouldEqual, strings.Repeat("x", 128*1024))
				})
			})
		})
	})
}

func TestRWCStreamLargeArtifactTwoStagePipelineIntegration(t *testing.T) {
	Convey("Setup", t, func() {
		large := strings.Repeat("x", 128*1024)
		source := Acquire(
			"test-source", APPJSON,
		).WithRole(
			"source-role",
		).WithScope(
			"source-scope",
		).WithPayload(mustMapMarshal(Map[any]{
			"answer": 42,
			"large":  large,
		}))

		Convey("Given a large Artifact and a two-stage pipeline", func() {
			stream := NewRWCStream(source)
			first := &Compute{
				artifact: Acquire("first", APPJSON),
			}
			second := &Compute{
				artifact: Acquire("second", APPJSON),
			}
			pipeline := transport.NewPipeline(first, second)

			Convey("When FlipFlopping through both unpacking stages", func() {
				err := transport.NewFlipFlop(stream, pipeline)
				So(err, ShouldBeNil)

				Convey("It should preserve the complete artifact frame through every stage", func() {
					So(Peek[string](source, "output"), ShouldEqual, "test")
					So(Peek[string](source, "large"), ShouldEqual, large)
				})
			})
		})
	})
}

func mustMapMarshal(body Map[any]) []byte {
	payload := body.Marshal()
	return payload
}
