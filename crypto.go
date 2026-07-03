package datura

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdh"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"errors"

	"github.com/theapemachine/errnie"
)

const aesKeyBytes = 32

var sealedPayloadSalt = []byte("datura sealed payload v1")

/*
CryptoSuite handles encryption and decryption operations for Artifacts.
*/
type CryptoSuite struct {
	curve ecdh.Curve
}

/*
NewCryptoSuite creates a new CryptoSuite using P-256 curve.
*/
func NewCryptoSuite() *CryptoSuite {
	return &CryptoSuite{
		curve: ecdh.P256(),
	}
}

/*
GenerateEphemeralKeyPair generates a new ECDH key pair for one-time use.
*/
func (cryptoSuite *CryptoSuite) GenerateEphemeralKeyPair() (*ecdh.PrivateKey, error) {
	return cryptoSuite.curve.GenerateKey(rand.Reader)
}

func (cryptoSuite *CryptoSuite) SealPayload(
	payload, recipientPublicKey, aad []byte,
) ([]byte, []byte, error) {
	ephemeralKey, err := cryptoSuite.GenerateEphemeralKeyPair()
	if err != nil {
		return nil, nil, errnie.Error(err, "ephemeral_key")
	}

	recipientKey, err := cryptoSuite.curve.NewPublicKey(recipientPublicKey)
	if err != nil {
		return nil, nil, errnie.Error(err, "recipient_public_key")
	}

	sharedSecret, err := ephemeralKey.ECDH(recipientKey)
	if err != nil {
		return nil, nil, errnie.Error(err, "ecdh")
	}

	aesKey := derivePayloadKey(sharedSecret, aad)
	block, err := aes.NewCipher(aesKey[:])
	if err != nil {
		return nil, nil, errnie.Error(err, "cipher")
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, nil, errnie.Error(err, "gcm")
	}

	encryptedPayload := make([]byte, gcm.NonceSize(), gcm.NonceSize()+len(payload)+gcm.Overhead())
	if _, err = rand.Read(encryptedPayload); err != nil {
		return nil, nil, errnie.Error(err, "rand_nonce")
	}

	encryptedPayload = gcm.Seal(encryptedPayload, encryptedPayload, payload, aad)

	return encryptedPayload, ephemeralKey.PublicKey().Bytes(), nil
}

func (cryptoSuite *CryptoSuite) OpenSealedPayload(
	encryptedPayload, ephemeralPubKey []byte,
	privateKey *ecdh.PrivateKey,
	aad []byte,
) ([]byte, error) {
	if privateKey == nil {
		return nil, errors.New("private key is required")
	}

	publicKey, err := cryptoSuite.curve.NewPublicKey(ephemeralPubKey)
	if err != nil {
		return nil, err
	}

	sharedSecret, err := privateKey.ECDH(publicKey)
	if err != nil {
		return nil, err
	}

	aesKey := derivePayloadKey(sharedSecret, aad)
	block, err := aes.NewCipher(aesKey[:])
	if err != nil {
		return nil, err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	nonceSize := gcm.NonceSize()
	if len(encryptedPayload) < nonceSize {
		return nil, errors.New("ciphertext too short")
	}

	return gcm.Open(nil, encryptedPayload[:nonceSize], encryptedPayload[nonceSize:], aad)
}

func derivePayloadKey(sharedSecret, aad []byte) [aesKeyBytes]byte {
	mac := hmac.New(sha256.New, sealedPayloadSalt)
	mac.Write(sharedSecret)
	prk := mac.Sum(nil)

	mac = hmac.New(sha256.New, prk)
	mac.Write([]byte("artifact payload"))
	mac.Write(aad)
	mac.Write([]byte{1})

	var out [aesKeyBytes]byte
	copy(out[:], mac.Sum(nil))
	return out
}
