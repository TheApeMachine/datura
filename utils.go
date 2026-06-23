package datura

import (
	"github.com/theapemachine/errnie"
)

/*
decryptPayload returns decrypted payload bytes when the artifact holds valid ciphertext.
Absence of encrypted material is an ordinary outcome and returns an error without logging.
*/
func (artifact *Artifact) decryptPayload() ([]byte, error) {
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
DecryptPayload decrypts the artifact encrypted payload.
When the artifact has no ciphertext, it returns nil without logging.
*/
func (artifact *Artifact) DecryptPayload() []byte {
	return errnie.Does(func() ([]byte, error) {
		return artifact.decryptPayload()
	}).Or(func(err error) {
		errnie.Error(errnie.Err(
			errnie.Validation,
			"artifact is nil or has no encrypted payload",
			err,
		))
	}).Value()
}
