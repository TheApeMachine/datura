package datura

import "errors"

/*
Payload returns decrypted payload bytes.

Legacy callers used this name before DecryptPayload existed.
*/
func (artifact *Artifact) Payload() ([]byte, error) {
	return artifact.DecryptPayload()
}

/*
SetPayload encrypts and stores payload bytes.

Legacy callers used this name before WithPayload existed.
*/
func (artifact *Artifact) SetPayload(payload []byte) error {
	if artifact == nil {
		return errors.New("datura: nil artifact")
	}

	if len(payload) == 0 {
		return errors.New("datura: payload is empty")
	}

	if artifact.WithPayload(payload) == nil {
		return errors.New("datura: set payload failed")
	}

	return nil
}

/*
Peek returns a metadata string for key.

Legacy callers used this method before the package-level Peek helper.
*/
func (artifact *Artifact) Peek(key string) string {
	return Peek[string](artifact, key)
}

/*
Marshal serializes the artifact capnp wire frame.

Legacy callers used this name before Message().Marshal().
*/
func (artifact *Artifact) Marshal() []byte {
	wire, err := artifact.Message().Marshal()

	if err != nil {
		return nil
	}

	return wire
}

/*
Unmarshal loads artifact state from capnp wire bytes.

Legacy callers expected a non-nil receiver pointer on success.
*/
func (artifact *Artifact) Unmarshal(wire []byte) *Artifact {
	if artifact == nil {
		return nil
	}

	if _, err := artifact.Write(wire); err != nil {
		return nil
	}

	return artifact
}
