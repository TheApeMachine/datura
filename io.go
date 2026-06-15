package datura

import (
	"errors"
	"io"

	"github.com/theapemachine/errnie"
)

/*
Read implements the io.Reader interface for the Artifact.
It marshals the entire artifact into the provided byte slice.
*/
func (artifact *Artifact) Read(p []byte) (n int, err error) {
	buf, err := artifact.Message().Marshal()

	if err != nil {
		errnie.Error(err)
		return 0, err
	}

	// Copy as much as we can into the provided buffer
	n = copy(p, buf)

	// If we couldn't copy everything, return ErrShortBuffer
	if n < len(buf) {
		errnie.Error(io.ErrShortBuffer)
		return n, io.ErrShortBuffer
	}

	return n, io.EOF
}

/*
Write implements the io.Writer interface for the Artifact.
It unmarshals the provided bytes into the current artifact.
*/
func (artifact *Artifact) Write(p []byte) (n int, err error) {
	if len(p) == 0 {
		errnie.Error(errors.New("empty input"))
		return 0, errnie.Error(errors.New("empty input"))
	}

	// Use the existing Unmarshal method and check if it succeeded
	if result := artifact.Unmarshal(p); result == nil {
		errnie.Error(errors.New("failed to unmarshal event"))
		return 0, errors.New("failed to unmarshal event")
	}

	return len(p), nil
}

/*
Close implements the io.Closer interface for the Artifact.
*/
func (artifact *Artifact) Close() error {
	artifact = nil
	return nil
}
