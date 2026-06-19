package datura

import (
	"io"

	capnp "capnproto.org/go/capnp/v3"
	"github.com/theapemachine/errnie"
)

/*
Read implements the io.Reader interface for the Artifact.
It marshals the entire artifact into the provided byte slice.
*/
func (artifact *Artifact) Read(p []byte) (n int, err error) {
	buf := errnie.Does(func() ([]byte, error) {
		return artifact.Message().Marshal()
	}).Or(func(err error) {
		errnie.Error(errnie.Err(errnie.Validation, "artifact marshal failed", err))
	}).Value()

	n = copy(p, buf)

	if n < len(buf) {
		return n, errnie.Error(io.ErrShortBuffer)
	}

	return n, io.EOF
}

/*
Write implements the io.Writer interface for the Artifact.
It unmarshals the provided bytes into the current artifact.
*/
func (artifact *Artifact) Write(p []byte) (n int, err error) {
	msg := errnie.Does(func() (*capnp.Message, error) {
		return capnp.Unmarshal(p)
	}).Or(func(err error) {
		errnie.Error(errnie.Err(
			errnie.Validation,
			"artifact unmarshal failed",
			err,
		))
	}).Value()

	inbound := errnie.Does(func() (Artifact, error) {
		return ReadRootArtifact(msg)
	}).Or(func(err error) {
		errnie.Error(errnie.Err(
			errnie.Validation,
			"artifact read root failed",
			err,
		))
	}).Value()

	segment := errnie.Does(func() (*capnp.Segment, error) {
		return artifact.Message().Reset(
			capnp.SingleSegment(nil),
		)
	}).Or(func(err error) {
		errnie.Error(errnie.Err(
			errnie.Validation,
			"artifact reset failed",
			err,
		))
	}).Value()

	writable := errnie.Does(func() (Artifact, error) {
		return NewRootArtifact(segment)
	}).Or(func(err error) {
		errnie.Error(errnie.Err(
			errnie.Validation,
			"artifact new root failed",
			err,
		))
	}).Value()

	errnie.Error(
		capnp.Struct(writable).CopyFrom(
			capnp.Struct(inbound),
		),
	)

	*artifact = writable

	return len(p), nil
}

/*
Close implements the io.Closer interface for the Artifact.
*/
func (artifact *Artifact) Close() error {
	errnie.Debug("artifact.Close")
	artifact = nil
	return nil
}
