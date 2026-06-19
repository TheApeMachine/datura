package datura

import (
	"io"
	"time"
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

	if result := artifact.WithPayload([]byte("payload")); result == nil {
		return io.ErrUnexpectedEOF
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
	_, _ = artifact.EncryptedPayload()
	_, _ = artifact.Uuid()

	_ = artifact.HasOrigin()
	_ = artifact.HasDestination()
	_ = artifact.HasRole()
	_ = artifact.HasScope()
	_ = artifact.HasEncryptedPayload()
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

func exerciseConversionRoundTrip(artifact *Artifact) error {
	if err := populateArtifactFields(artifact); err != nil {
		return err
	}

	marshaled, err := artifact.Message().Marshal()

	if err != nil || len(marshaled) == 0 {
		return io.ErrUnexpectedEOF
	}

	target := Acquire("", Artifact_Type_json)

	if target == nil {
		return io.ErrUnexpectedEOF
	}

	defer target.Release()

	if _, err := target.Write(marshaled); err != nil {
		return err
	}

	packed, err := artifact.Pack()

	if err != nil || len(packed) == 0 {
		return io.ErrUnexpectedEOF
	}

	unpackTarget := Acquire("", Artifact_Type_json)

	if unpackTarget == nil {
		return io.ErrUnexpectedEOF
	}

	defer unpackTarget.Release()

	if err := unpackTarget.Unpack(packed); err != nil {
		return err
	}

	return readArtifactFields(target)
}
