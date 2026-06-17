package datura

import "errors"

/*
PayloadQuiet decrypts when crypto metadata is present.
It returns false without logging for missing or invalid crypto fields.
*/
func (artifact *Artifact) PayloadQuiet() ([]byte, bool) {
	if artifact == nil {
		return nil, false
	}

	encryptedKey, err := artifact.EncryptedKey()

	if err != nil || len(encryptedKey) < aesKeyBytes {
		return nil, false
	}

	encryptedPayload, err := artifact.EncryptedPayload()

	if err != nil || len(encryptedPayload) == 0 {
		return nil, false
	}

	cryptoSuite := NewCryptoSuite()
	payload, err := cryptoSuite.decryptPayloadDirect(nil, encryptedPayload, encryptedKey)

	if err != nil || len(payload) == 0 {
		return nil, false
	}

	return payload, true
}

/*
DecryptPayload decrypts the artifact encrypted payload into a newly allocated slice.
*/
func (artifact *Artifact) DecryptPayload() (payload []byte, err error) {
	payload, payloadOK := artifact.PayloadQuiet()

	if !payloadOK {
		return nil, errors.New("datura: payload unavailable")
	}

	return payload, nil
}

/*
DecryptPayloadInto decrypts directly into dst when capacity allows.
*/
func (artifact *Artifact) DecryptPayloadInto(dst []byte) ([]byte, error) {
	payload, payloadOK := artifact.PayloadQuiet()

	if !payloadOK {
		return nil, errors.New("datura: payload unavailable")
	}

	if cap(dst) >= len(payload) {
		return append(dst[:0], payload...), nil
	}

	return append([]byte(nil), payload...), nil
}
