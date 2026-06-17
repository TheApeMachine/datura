package datura

import "github.com/theapemachine/errnie"

/*
DecryptPayload decrypts the artifact encrypted payload into a newly allocated slice.
*/
func (artifact *Artifact) DecryptPayload() (payload []byte, err error) {
	buffer := make([]byte, 0)

	return artifact.DecryptPayloadInto(buffer)
}

/*
DecryptPayloadInto decrypts directly into dst when capacity allows.
*/
func (artifact *Artifact) DecryptPayloadInto(dst []byte) ([]byte, error) {
	encryptedKey, err := artifact.EncryptedKey()

	if err != nil {
		return nil, errnie.Error(err, "encryptedKey", encryptedKey)
	}

	ephemeralPubKey, err := artifact.EphemeralPublicKey()

	if err != nil {
		return nil, errnie.Error(err, "ephemeralPubKey", ephemeralPubKey)
	}

	encryptedPayload, err := artifact.EncryptedPayload()

	if err != nil {
		return nil, errnie.Error(err, "encryptedPayload", encryptedPayload)
	}

	cryptoSuite := NewCryptoSuite()
	payload, err := cryptoSuite.DecryptPayloadDirect(dst, encryptedPayload, encryptedKey)

	if err != nil {
		return nil, errnie.Error(
			err,
			"encryptedPayload", encryptedPayload,
			"encryptedKey", encryptedKey,
			"ephemeralPubKey", ephemeralPubKey,
		)
	}

	return payload, nil
}
