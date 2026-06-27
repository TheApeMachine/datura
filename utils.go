package datura

import (
	"crypto/ecdh"
	"strings"
	"time"

	"github.com/theapemachine/errnie"
)

/*
decryptPayload returns decrypted payload bytes when the artifact holds valid
ciphertext. Artifacts with payload bytes but no encrypted key are plaintext local
frames and return their payload directly.
*/
func (artifact *Artifact) decryptPayload() ([]byte, error) {
	if artifact == nil || !artifact.HasPayload() {
		return nil, errnie.Err(
			errnie.Validation,
			"artifact is nil or has no encrypted payload",
			nil,
		)
	}

	publicKey, publicKeyErr := artifact.PublicKey()
	encryptedKey, err := artifact.EncryptedKey()

	if publicKeyErr == nil && len(publicKey) > 0 && (err != nil || len(encryptedKey) == 0) {
		return nil, errnie.Err(
			errnie.Validation,
			"sealed payload requires DecryptPayloadWithKey",
			nil,
		)
	}

	if err != nil {
		payload, payloadErr := artifact.Payload()
		if payloadErr != nil {
			return nil, errnie.Err(errnie.Validation, "payload unavailable", payloadErr)
		}

		return payload, nil
	}

	if len(encryptedKey) == 0 {
		payload, payloadErr := artifact.Payload()
		if payloadErr != nil {
			return nil, errnie.Err(errnie.Validation, "payload unavailable", payloadErr)
		}

		return payload, nil
	}

	if len(encryptedKey) < aesKeyBytes {
		return nil, errnie.Err(errnie.Validation, "encrypted key too short", nil)
	}

	encryptedPayload, err := artifact.Payload()

	if err != nil {
		return nil, errnie.Err(errnie.Validation, "encrypted payload unavailable", err)
	}

	cryptoSuite := NewCryptoSuite()

	return cryptoSuite.DecryptPayloadDirect(nil, encryptedPayload, encryptedKey)
}

func (artifact *Artifact) DecryptPayloadWithKey(privateKey *ecdh.PrivateKey) ([]byte, error) {
	if artifact == nil || !artifact.HasPayload() {
		return nil, errnie.Err(
			errnie.Validation,
			"artifact is nil or has no payload",
			nil,
		)
	}

	publicKey, err := artifact.PublicKey()
	if err != nil || len(publicKey) == 0 {
		return nil, errnie.Err(
			errnie.Validation,
			"artifact has no sealed payload public key",
			err,
		)
	}

	encryptedKey, keyErr := artifact.EncryptedKey()
	if keyErr == nil && len(encryptedKey) > 0 {
		return nil, errnie.Err(
			errnie.Validation,
			"legacy encrypted-key payload is not sealed payload",
			nil,
		)
	}

	encryptedPayload, err := artifact.Payload()
	if err != nil {
		return nil, errnie.Err(errnie.Validation, "sealed payload unavailable", err)
	}

	cryptoSuite := NewCryptoSuite()

	return cryptoSuite.OpenSealedPayload(
		encryptedPayload,
		publicKey,
		privateKey,
		artifact.payloadAAD(),
	)
}

/*
DecryptPayload decrypts the artifact encrypted payload.
When the artifact has no ciphertext, it returns nil without logging.
*/
func (artifact *Artifact) DecryptPayload() []byte {
	payload, err := artifact.decryptPayload()

	if err != nil {
		return nil
	}

	return payload
}

func FormatTimestamp(timestamp int64) string {
	observed := time.Unix(0, timestamp).UTC()
	return strings.ReplaceAll(strings.ReplaceAll(
		observed.Format("2006/01/02 15:04:05"),
		" ",
		"/",
	),
		":",
		"/",
	)
}
