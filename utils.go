package datura

import (
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

	encryptedKey, err := artifact.EncryptedKey()

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
