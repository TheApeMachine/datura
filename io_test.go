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
	state.Write(t.artifact.DecryptPayload())

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
	state.Write(t.artifact.DecryptPayload())
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
			// First get the expected marshaled data
			expected, err := artifact.MarshalPacked()
			So(err, ShouldBeNil)

			// Create a buffer of the right size
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
			p, err := artifact.MarshalPacked()
			So(err, ShouldBeNil)

			// Write the marshaled data to the empty artifact
			n, err := empty.Write(p)

			So(err, ShouldBeNil)
			So(n, ShouldEqual, len(p))

			origin, err := empty.Origin()
			So(err, ShouldBeNil)
			So(origin, ShouldEqual, "test")
			So(string(empty.DecryptPayload()), ShouldEqual, "test payload")
		})

		Convey("When poking attributes after write", func() {
			source := Acquire("write-poke", APPJSON).
				WithAttributes(Map[any]{"count": 1})

			packed, err := source.MarshalPacked()
			So(err, ShouldBeNil)

			restored := &Artifact{}
			_, err = restored.Write(packed)
			So(err, ShouldBeNil)

			restored.Poke(42, "count")
			So(Peek[float64](restored, "count"), ShouldEqual, 42)
		})
	})
}

func TestArtifactWithFlipFlop(t *testing.T) {
	Convey("Given a io.ReadWriteCloser with a FlipFlop instance", t, func() {
		atOne := TestTypeOne{artifact: Acquire("test-one", APPJSON)}
		input := Acquire(
			"test-artifact", APPJSON,
		).WithRole(
			"test-role",
		).WithScope(
			"test-scope",
		).WithPayload(
			[]byte("test"),
		).WithAttributes(
			Map[any]{"test": "test"},
		)

		Convey("And a FlipFlop instance", func() {
			transport.NewFlipFlop(input, atOne)

			origin, err := input.Origin()
			So(err, ShouldBeNil)
			So(origin, ShouldEqual, "test-artifact")

			role, err := input.Role()
			So(err, ShouldBeNil)
			So(role, ShouldEqual, "test-role")

			scope, err := input.Scope()
			So(err, ShouldBeNil)
			So(scope, ShouldEqual, "test-scope")

			attributes, err := AttributesBytes(input)
			So(err, ShouldBeNil)
			So(string(attributes), ShouldEqual, `{"test":"test"}`)

			So(string(input.DecryptPayload()), ShouldEqual, "toast")
		})
	})
}
