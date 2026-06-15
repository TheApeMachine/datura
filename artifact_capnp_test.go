package datura

import (
	"testing"
	"time"

	capnp "capnproto.org/go/capnp/v3"
	"capnproto.org/go/capnp/v3/schemas"
	. "github.com/smartystreets/goconvey/convey"
)

func TestArtifactCapnpFields(t *testing.T) {
	Convey("Artifact capnp accessors", t, func() {
		artifact := Acquire("origin", Artifact_Type_json)
		So(artifact, ShouldNotBeNil)
		defer artifact.Release()

		So(artifact.SetDestination("destination"), ShouldBeNil)
		So(artifact.SetRole("role"), ShouldBeNil)
		So(artifact.SetScope("scope"), ShouldBeNil)
		So(artifact.SetPayload([]byte("payload")), ShouldBeNil)

		So(artifact.HasOrigin(), ShouldBeTrue)
		So(artifact.HasDestination(), ShouldBeTrue)
		So(artifact.HasRole(), ShouldBeTrue)
		So(artifact.HasScope(), ShouldBeTrue)
		So(artifact.HasPayload(), ShouldBeTrue)
		So(artifact.IsValid(), ShouldBeTrue)
		So(artifact.Timestamp(), ShouldBeGreaterThan, 0)
		_ = artifact.String()

		origin, err := artifact.Origin()
		So(err, ShouldBeNil)
		So(origin, ShouldEqual, "origin")

		originBytes, err := artifact.OriginBytes()
		So(err, ShouldBeNil)
		So(string(originBytes), ShouldEqual, "origin")

		destination, err := artifact.Destination()
		So(err, ShouldBeNil)
		So(destination, ShouldEqual, "destination")

		destinationBytes, err := artifact.DestinationBytes()
		So(err, ShouldBeNil)
		So(string(destinationBytes), ShouldEqual, "destination")

		role, err := artifact.Role()
		So(err, ShouldBeNil)
		So(role, ShouldEqual, "role")

		roleBytes, err := artifact.RoleBytes()
		So(err, ShouldBeNil)
		So(string(roleBytes), ShouldEqual, "role")

		scope, err := artifact.Scope()
		So(err, ShouldBeNil)
		So(scope, ShouldEqual, "scope")

		scopeBytes, err := artifact.ScopeBytes()
		So(err, ShouldBeNil)
		So(string(scopeBytes), ShouldEqual, "scope")

		payload, err := artifact.Payload()
		So(err, ShouldBeNil)
		So(string(payload), ShouldEqual, "payload")

		uuidBytes, err := artifact.UuidBytes()
		So(err, ShouldBeNil)
		So(len(uuidBytes), ShouldBeGreaterThan, 0)
		So(artifact.HasUuid(), ShouldBeTrue)

		So(artifact.Segment(), ShouldNotBeNil)
		So(artifact.Message(), ShouldNotBeNil)

		restored := Artifact{}.DecodeFromPtr(artifact.ToPtr())
		So(restored.IsValid(), ShouldBeTrue)

		nestedError, err := artifact.NewError()
		So(err, ShouldBeNil)

		nestedError.SetType(Artifact_Error_Type_validation)
		nestedError.SetTimestamp(time.Now().UnixNano())
		So(nestedError.SetMessage_("validation failed"), ShouldBeNil)
		So(artifact.SetError(nestedError), ShouldBeNil)
		So(artifact.HasError(), ShouldBeTrue)

		readError, err := artifact.Error()
		So(err, ShouldBeNil)
		So(readError.Type(), ShouldEqual, Artifact_Error_Type_validation)

		errorMessage, err := readError.Message_()
		So(err, ShouldBeNil)
		So(errorMessage, ShouldEqual, "validation failed")

		errorMessageBytes, err := readError.Message_Bytes()
		So(err, ShouldBeNil)
		So(string(errorMessageBytes), ShouldEqual, "validation failed")

		So(readError.IsValid(), ShouldBeTrue)
		_ = readError.String()
		So(readError.Segment(), ShouldNotBeNil)
		So(readError.Message(), ShouldNotBeNil)

		errorRestored := Artifact_Error{}.DecodeFromPtr(readError.ToPtr())
		So(errorRestored.IsValid(), ShouldBeTrue)
	})
}

func TestArtifactCapnpConstructors(t *testing.T) {
	Convey("Artifact capnp constructors and schema", t, func() {
		arena := capnp.SingleSegment(nil)
		_, segment, err := capnp.NewMessage(arena)
		So(err, ShouldBeNil)

		child, err := NewArtifact(segment)
		So(err, ShouldBeNil)
		So(child.IsValid(), ShouldBeTrue)

		artifactList, err := NewArtifact_List(segment, 2)
		So(err, ShouldBeNil)
		So(artifactList.Len(), ShouldEqual, 2)

		rootError, err := NewRootArtifact_Error(segment)
		So(err, ShouldBeNil)
		So(rootError.IsValid(), ShouldBeTrue)

		nestedError, err := NewArtifact_Error(segment)
		So(err, ShouldBeNil)
		So(nestedError.IsValid(), ShouldBeTrue)

		errorList, err := NewArtifact_Error_List(segment, 2)
		So(err, ShouldBeNil)
		So(errorList.Len(), ShouldEqual, 2)

		typeList, err := NewArtifact_Type_List(segment, 2)
		So(err, ShouldBeNil)
		So(typeList.Len(), ShouldEqual, 2)

		RegisterSchema(&schemas.Registry{})
	})
}

func TestArtifactCapnpEnums(t *testing.T) {
	Convey("Artifact enum string conversions", t, func() {
		So(Artifact_Type_json.String(), ShouldEqual, "json")
		So(Artifact_TypeFromString("json"), ShouldEqual, Artifact_Type_json)
		So(Artifact_TypeFromString("unknown"), ShouldEqual, Artifact_Type_json)

		So(Artifact_Error_Type_unknown.String(), ShouldEqual, "unknown")
		So(Artifact_Error_Type_validation.String(), ShouldEqual, "validation")
		So(Artifact_Error_TypeFromString("unknown"), ShouldEqual, Artifact_Error_Type_unknown)
		So(Artifact_Error_TypeFromString("validation"), ShouldEqual, Artifact_Error_Type_validation)
		So(Artifact_Error_TypeFromString("missing"), ShouldEqual, Artifact_Error_Type_unknown)
	})
}

func BenchmarkArtifactCapnpConstructors(b *testing.B) {
	b.ResetTimer()

	for b.Loop() {
		if err := exerciseCapnpConstructors(); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkArtifactCapnpFields(b *testing.B) {
	artifact := Acquire("origin", Artifact_Type_json)
	if artifact == nil {
		b.Fatal("Acquire returned nil")
	}

	defer artifact.Release()

	if err := readArtifactFields(artifact); err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()

	for b.Loop() {
		if err := mutateArtifactFields(artifact); err != nil {
			b.Fatal(err)
		}
	}
}

func TestReadRootArtifactFromMessage(t *testing.T) {
	Convey("ReadRootArtifact round-trips through capnp message", t, func() {
		source := sampleArtifact(t)
		defer source.Release()

		raw, err := source.Message().Marshal()
		So(err, ShouldBeNil)

		message, err := capnp.Unmarshal(raw)
		So(err, ShouldBeNil)

		read, err := ReadRootArtifact(message)
		So(err, ShouldBeNil)
		So(read.IsValid(), ShouldBeTrue)

		payload, err := read.Payload()
		So(err, ShouldBeNil)
		So(string(payload), ShouldEqual, "payload")
	})
}
