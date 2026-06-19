package datura

import "github.com/theapemachine/errnie"

func (artifact *Artifact) DecryptPayload() []byte {
	encryptedKey := errnie.Does(func() ([]byte, error) {
		return artifact.EncryptedKey()
	}).Or(func(err error) {
		errnie.Error(errnie.Err(errnie.Validation, "encryptedKey unavailable", err))
	}).Value()

	ephemeralPubKey := errnie.Does(func() ([]byte, error) {
		return artifact.EphemeralPublicKey()
	}).Or(func(err error) {
		errnie.Error(errnie.Err(errnie.Validation, "ephemeralPubKey unavailable", err))
	}).Value()

	encryptedPayload := errnie.Does(func() ([]byte, error) {
		return artifact.EncryptedPayload()
	}).Or(func(err error) {
		errnie.Error(errnie.Err(errnie.Validation, "encryptedPayload unavailable", err))
	}).Value()

	crypto := NewCryptoSuite()
	
	return errnie.Does(func() ([]byte, error) {
		return crypto.DecryptPayload(encryptedPayload, encryptedKey, ephemeralPubKey)
	}).Or(func(err error) {
		errnie.Error(errnie.Err(errnie.Validation, "payload decryption failed", err))
	}).Value()
}
