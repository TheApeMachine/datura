package datura

import (
	"io"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/datura/transport"
)

type TestTypeOne struct {
	artifact *Artifact
}

func (t TestTypeOne) Read(p []byte) (int, error) {
	state := Acquire("test-state1", APPJSON)
	state.Write(p)
	state.Inspect("test-state1", "Read()", "p")

	state.WithPayload([]byte("test"))

	ttTwo := TestTypeTwo{artifact: t.artifact}

	transport.NewFlipFlop(state, ttTwo)

	return state.Read(p)
}

func (t TestTypeOne) Write(p []byte) (int, error) {
	t.artifact.WithPayload(p)
	return len(p), nil
}

func (t TestTypeOne) Close() error {
	return t.artifact.Close()
}

type TestTypeTwo struct {
	artifact *Artifact
}

func (t TestTypeTwo) Read(p []byte) (int, error) {
	state := Acquire("test-state2", APPJSON)
	state.Write(p)

	state.WithPayload([]byte("toast"))

	return state.Read(p)
}

func (t TestTypeTwo) Write(p []byte) (int, error) {
	t.artifact.WithPayload(p)
	return len(p), nil
}

func (t TestTypeTwo) Close() error {
	return t.artifact.Close()
}

func testArtifact() *Artifact {
	return Acquire(
		"test", Artifact_Type_json,
	).WithPayload(
		[]byte("test payload"),
	)
}

func TestRead(t *testing.T) {
	Convey("Given an artifact", t, func() {
		artifact := testArtifact()

		Convey("When the artifact is read", func() {
			expected, err := artifact.Message().MarshalPacked()
			So(err, ShouldBeNil)

			p := make([]byte, len(expected))
			n, err := artifact.Read(p)

			So(err, ShouldEqual, io.EOF)
			So(n, ShouldEqual, len(expected))
			So(p, ShouldResemble, expected)
		})
	})
}

func TestWrite(t *testing.T) {
	Convey("Given an empty artifact", t, func() {
		empty := &Artifact{}

		Convey("When writing a marshaled artifact", func() {
			artifact := testArtifact()

			// Get the marshaled data to write
			p, err := artifact.Message().MarshalPacked()
			So(err, ShouldBeNil)

			// Write the marshaled data to the empty artifact
			n, err := empty.Write(p)

			So(err, ShouldBeNil)
			So(n, ShouldEqual, len(p))

			// Verify the empty artifact now restores the original payload.
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

func TestArtifactWithFlipFlop(t *testing.T) {
	Convey("Given a io.ReadWriteCloser with a FlipFlop instance", t, func() {
		atOne := TestTypeOne{artifact: Acquire("test-one", APPJSON)}
		input := Acquire("test-input", APPJSON).WithPayload([]byte("test"))

		Convey("And a FlipFlop instance", func() {
			transport.NewFlipFlop(input, atOne)
			So(string(input.DecryptPayload()), ShouldEqual, "toast")
		})
	})
}
