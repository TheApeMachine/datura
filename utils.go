package datura

import "errors"

/*
decryptPayload returns decrypted payload bytes when the artifact holds valid ciphertext.
Absence of encrypted material is an ordinary outcome and returns an error without logging.
*/
func (artifact *Artifact) decryptPayload() ([]byte, error) {
	if artifact == nil || !artifact.HasEncryptedPayload() {
		return nil, errors.New("no encrypted payload")
	}

	encryptedKey, err := artifact.EncryptedKey()

	if err != nil {
		return nil, err
	}

	if len(encryptedKey) < aesKeyBytes {
		return nil, errors.New("encrypted key too short")
	}

	encryptedPayload, err := artifact.EncryptedPayload()

	if err != nil {
		return nil, err
	}

	cryptoSuite := NewCryptoSuite()

	return cryptoSuite.DecryptPayloadDirect(nil, encryptedPayload, encryptedKey)
}

/*
DecryptPayload decrypts the artifact encrypted payload.
When the artifact has no ciphertext, it returns nil.
*/
func (artifact *Artifact) DecryptPayload() []byte {
	payload, err := artifact.decryptPayload()

	if err != nil {
		return nil
	}

	return payload
}
