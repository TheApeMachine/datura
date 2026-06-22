package datura

import (
	"github.com/theapemachine/errnie"
)

const capnpTraverseLimitBytes = 64 << 20

/*
ensureReadBudget restores the capnp read traversal budget on reused artifact messages.
Repeated getter calls on the same message accumulate against a fixed limit and eventually
fail with "read pointer: read traversal limit reached".
*/
func (artifact *Artifact) ensureReadBudget() {
	if artifact == nil {
		return
	}

	message := capnpArtifact(artifact).Message()

	if message == nil {
		return
	}

	limit := message.TraverseLimit

	if limit == 0 {
		limit = capnpTraverseLimitBytes
	}

	message.ResetReadLimit(limit)
}

/*
decryptPayload returns decrypted payload bytes when the artifact holds valid ciphertext.
Absence of encrypted material is an ordinary outcome and returns an error without logging.
*/
func (artifact *Artifact) decryptPayload() ([]byte, error) {
	artifact.ensureReadBudget()

	if artifact == nil || !artifact.HasEncryptedPayload() {
		return nil, errnie.Err(
			errnie.Validation,
			"artifact is nil or has no encrypted payload",
			nil,
		)
	}

	encryptedKey, err := artifact.EncryptedKey()

	if err != nil {
		return nil, errnie.Err(errnie.Validation, "encrypted key unavailable", err)
	}

	if len(encryptedKey) < aesKeyBytes {
		return nil, errnie.Err(errnie.Validation, "encrypted key too short", nil)
	}

	encryptedPayload, err := artifact.EncryptedPayload()

	if err != nil {
		return nil, errnie.Err(errnie.Validation, "encrypted payload unavailable", err)
	}

	cryptoSuite := NewCryptoSuite()

	return cryptoSuite.DecryptPayloadDirect(nil, encryptedPayload, encryptedKey)
}

/*
DecryptPayloadError decrypts the artifact encrypted payload and returns an error
when ciphertext is absent or invalid. It does not log.
*/
func (artifact *Artifact) DecryptPayloadError() ([]byte, error) {
	return artifact.decryptPayload()
}

/*
DecryptPayload decrypts the artifact encrypted payload.
When the artifact has no ciphertext, it returns nil without logging.
*/
func (artifact *Artifact) DecryptPayload() []byte {
	payload, _ := artifact.decryptPayload()

	return payload
}
