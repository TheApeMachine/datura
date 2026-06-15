package datura

import (
	"io"
	"time"

	capnp "capnproto.org/go/capnp/v3"
	"capnproto.org/go/capnp/v3/schemas"
)

func mutateArtifactFields(artifact *Artifact) error {
	if err := artifact.SetDestination("destination"); err != nil {
		return err
	}

	if err := artifact.SetRole("role"); err != nil {
		return err
	}

	if err := artifact.SetScope("scope"); err != nil {
		return err
	}

	if err := artifact.SetPayload([]byte("payload")); err != nil {
		return err
	}

	nestedError, err := artifact.Error()
	if err != nil || !artifact.HasError() {
		nestedError, err = artifact.NewError()
		if err != nil {
			return err
		}
	}

	nestedError.SetType(Artifact_Error_Type_validation)
	nestedError.SetTimestamp(time.Now().UnixNano())

	return nestedError.SetMessage_("validation failed")
}

func readArtifactFields(artifact *Artifact) error {
	_, _ = artifact.Origin()
	_, _ = artifact.OriginBytes()
	_, _ = artifact.Destination()
	_, _ = artifact.DestinationBytes()
	_, _ = artifact.Role()
	_, _ = artifact.RoleBytes()
	_, _ = artifact.Scope()
	_, _ = artifact.ScopeBytes()
	_, _ = artifact.Payload()
	_, _ = artifact.Uuid()
	_, _ = artifact.UuidBytes()

	_ = artifact.HasOrigin()
	_ = artifact.HasDestination()
	_ = artifact.HasRole()
	_ = artifact.HasScope()
	_ = artifact.HasPayload()
	_ = artifact.HasUuid()
	_ = artifact.HasError()
	_ = artifact.IsValid()
	_ = artifact.Timestamp()
	_ = artifact.Type()
	_ = artifact.String()
	_ = artifact.Segment()
	_ = artifact.Message()

	restored := Artifact{}.DecodeFromPtr(artifact.ToPtr())
	if !restored.IsValid() {
		return io.ErrUnexpectedEOF
	}

	readError, err := artifact.Error()
	if err != nil {
		return err
	}

	_, _ = readError.Message_()
	_, _ = readError.Message_Bytes()
	_ = readError.HasMessage_()
	_ = readError.Type()
	_ = readError.Timestamp()
	_ = readError.IsValid()
	_ = readError.String()
	_ = readError.Segment()
	_ = readError.Message()
	_ = Artifact_Error{}.DecodeFromPtr(readError.ToPtr()).IsValid()

	return nil
}

func populateArtifactFields(artifact *Artifact) error {
	if err := mutateArtifactFields(artifact); err != nil {
		return err
	}

	return readArtifactFields(artifact)
}

func exerciseCapnpConstructors() error {
	arena := capnp.SingleSegment(nil)
	_, segment, err := capnp.NewMessage(arena)
	if err != nil {
		return err
	}

	child, err := NewArtifact(segment)
	if err != nil {
		return err
	}

	artifactList, err := NewArtifact_List(segment, 2)
	if err != nil {
		return err
	}

	if artifactList.Len() != 2 {
		return io.ErrUnexpectedEOF
	}

	rootError, err := NewRootArtifact_Error(segment)
	if err != nil {
		return err
	}

	nestedError, err := NewArtifact_Error(segment)
	if err != nil {
		return err
	}

	errorList, err := NewArtifact_Error_List(segment, 2)
	if err != nil {
		return err
	}

	if errorList.Len() != 2 {
		return io.ErrUnexpectedEOF
	}

	typeList, err := NewArtifact_Type_List(segment, 2)
	if err != nil {
		return err
	}

	if typeList.Len() != 2 {
		return io.ErrUnexpectedEOF
	}

	RegisterSchema(&schemas.Registry{})

	_ = child.IsValid()
	_ = rootError.IsValid()
	_ = nestedError.IsValid()
	_ = Artifact_Type_json.String()
	_ = Artifact_TypeFromString("json")
	_ = Artifact_TypeFromString("unknown")
	_ = Artifact_Error_Type_unknown.String()
	_ = Artifact_Error_Type_validation.String()
	_ = Artifact_Error_TypeFromString("unknown")
	_ = Artifact_Error_TypeFromString("validation")
	_ = Artifact_Error_TypeFromString("missing")

	return nil
}

func exerciseConversionRoundTrip(artifact *Artifact) error {
	if err := populateArtifactFields(artifact); err != nil {
		return err
	}

	marshaled := artifact.Marshal()
	if len(marshaled) == 0 {
		return io.ErrUnexpectedEOF
	}

	encodedBuffer := make([]byte, 0, len(marshaled)+512)
	artifact.Encode(encodedBuffer)

	target := Acquire("", Artifact_Type_json)
	if target == nil {
		return io.ErrUnexpectedEOF
	}

	defer target.Release()

	if target.Unmarshal(marshaled) == nil {
		return io.ErrUnexpectedEOF
	}

	if target.Decode(marshaled) == nil {
		return io.ErrUnexpectedEOF
	}

	packed := artifact.Pack()
	if len(packed) == 0 {
		return io.ErrUnexpectedEOF
	}

	if target.Unpack(packed) == nil {
		return io.ErrUnexpectedEOF
	}

	raw, err := artifact.Message().Marshal()
	if err != nil {
		return err
	}

	message, err := capnp.Unmarshal(raw)
	if err != nil {
		return err
	}

	read, err := ReadRootArtifact(message)
	if err != nil {
		return err
	}

	if !read.IsValid() {
		return io.ErrUnexpectedEOF
	}

	return nil
}

func exerciseIORoundTrip(artifact *Artifact) error {
	marshaled := artifact.Marshal()
	buffer := make([]byte, len(marshaled)+64)

	if _, err := artifact.Read(buffer); err != io.EOF {
		return err
	}

	shortBuffer := make([]byte, 1)
	if _, err := artifact.Read(shortBuffer); err != io.ErrShortBuffer {
		return err
	}

	target := Acquire("", Artifact_Type_json)
	if target == nil {
		return io.ErrUnexpectedEOF
	}

	defer target.Release()

	if _, err := target.Write(marshaled); err != nil {
		return err
	}

	if err := target.Close(); err != nil {
		return err
	}

	rejectTarget := Acquire("", Artifact_Type_json)
	if rejectTarget == nil {
		return io.ErrUnexpectedEOF
	}

	defer rejectTarget.Release()

	if _, err := rejectTarget.Write(nil); err == nil {
		return io.ErrUnexpectedEOF
	}

	if _, err := rejectTarget.Write([]byte{0xff, 0xff, 0xff}); err == nil {
		return io.ErrUnexpectedEOF
	}

	return nil
}
