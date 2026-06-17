package datura

import "github.com/theapemachine/errnie"

func (artifact *Artifact) DecryptPayload() (payload []byte, err error) {
	errnie.Debug("datura.Artifact.DecryptPayload")

	encryptedKey, err := artifact.EncryptedKey()

	if err != nil {
		return nil, errnie.Error(err, "payload", payload, "encryptedKey", encryptedKey)
	}

	ephemeralPubKey, err := artifact.EphemeralPublicKey()

	if err != nil {
		return nil, errnie.Error(err, "payload", payload, "ephemeralPubKey", ephemeralPubKey)
	}

	encryptedPayload, err := artifact.EncryptedPayload()

	if err != nil {
		return nil, errnie.Error(err, "payload", payload, "encryptedPayload", encryptedPayload)
	}

	crypto := NewCryptoSuite()
	payload, err = crypto.DecryptPayload(encryptedPayload, encryptedKey, ephemeralPubKey)

	if err != nil {
		return nil, errnie.Error(err, "payload", payload, "encryptedPayload", encryptedPayload, "encryptedKey", encryptedKey, "ephemeralPubKey", ephemeralPubKey)
	}

	return
}