package datura

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdh"
	"crypto/rand"
	"errors"

	"github.com/theapemachine/errnie"
)

const (
	aesKeyBytes     = 32
	p256PubKeyBytes = 65
)

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

/*
EncryptedPayloadSize returns the Cap'n Proto data field size for AES-GCM ciphertext.
*/
func (cryptoSuite *CryptoSuite) EncryptedPayloadSize(plaintextLen int) int {
	block, err := aes.NewCipher(make([]byte, aesKeyBytes))

	if err != nil {
		return 12 + plaintextLen + 16
	}

	gcm, err := cipher.NewGCM(block)

	if err != nil {
		return 12 + plaintextLen + 16
	}

	return gcm.NonceSize() + plaintextLen + gcm.Overhead()
}

/*
EncryptPayloadDirect encrypts a payload into pre-allocated destination slices.
*/
func (cryptoSuite *CryptoSuite) EncryptPayloadDirect(
	dstPayload, dstKey, dstPubKey, payload []byte,
) error {
	ephemeralKey, err := cryptoSuite.GenerateEphemeralKeyPair()

	if err != nil {
		return errnie.Error(err, "ephemeral_key")
	}

	var aesKey [aesKeyBytes]byte

	if _, err = rand.Read(aesKey[:]); err != nil {
		return errnie.Error(err, "rand_aes")
	}

	block, err := aes.NewCipher(aesKey[:])

	if err != nil {
		return errnie.Error(err, "cipher")
	}

	gcm, err := cipher.NewGCM(block)

	if err != nil {
		return errnie.Error(err, "gcm")
	}

	nonceSize := gcm.NonceSize()
	needed := nonceSize + len(payload) + gcm.Overhead()

	if len(dstPayload) < needed {
		return errnie.Error(errors.New("dstPayload too short"))
	}

	if len(dstKey) < aesKeyBytes {
		return errnie.Error(errors.New("dstKey too short"))
	}

	pubKeyBytes := ephemeralKey.PublicKey().Bytes()

	if len(dstPubKey) < len(pubKeyBytes) {
		return errnie.Error(errors.New("dstPubKey too short"))
	}

	nonceBuf := dstPayload[:nonceSize]

	if _, err = rand.Read(nonceBuf); err != nil {
		return errnie.Error(err, "rand_nonce")
	}

	gcm.Seal(dstPayload[:nonceSize], nonceBuf, payload, nil)
	copy(dstKey, aesKey[:])
	copy(dstPubKey, pubKeyBytes)

	return nil
}

/*
EncryptPayload encrypts a payload using AES-GCM with an ephemeral key.
*/
func (cryptoSuite *CryptoSuite) EncryptPayload(payload []byte) ([]byte, []byte, []byte, error) {
	cipherLen := cryptoSuite.EncryptedPayloadSize(len(payload))
	encryptedPayload := make([]byte, cipherLen)
	encryptedKey := make([]byte, aesKeyBytes)
	ephemeralPubKey := make([]byte, p256PubKeyBytes)

	err := cryptoSuite.EncryptPayloadDirect(
		encryptedPayload,
		encryptedKey,
		ephemeralPubKey,
		payload,
	)

	if err != nil {
		return nil, nil, nil, err
	}

	return encryptedPayload, encryptedKey, ephemeralPubKey, nil
}

/*
DecryptPayloadDirect decrypts into dst when capacity allows, otherwise allocates.
*/
func (cryptoSuite *CryptoSuite) DecryptPayloadDirect(
	dst, encryptedPayload, encryptedKey []byte,
) ([]byte, error) {
	payload, err := cryptoSuite.decryptPayloadDirect(dst, encryptedPayload, encryptedKey)

	if err != nil {
		return nil, errnie.Error(err)
	}

	return payload, nil
}

func (cryptoSuite *CryptoSuite) decryptPayloadDirect(
	dst, encryptedPayload, encryptedKey []byte,
) ([]byte, error) {
	if len(encryptedKey) < aesKeyBytes {
		return nil, errors.New("encrypted key too short")
	}

	block, err := aes.NewCipher(encryptedKey[:aesKeyBytes])

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

	nonce := encryptedPayload[:nonceSize]
	ciphertext := encryptedPayload[nonceSize:]

	return gcm.Open(dst[:0], nonce, ciphertext, nil)
}

/*
DecryptPayload decrypts a payload using the provided keys.
*/
func (cryptoSuite *CryptoSuite) DecryptPayload(
	encryptedPayload, encryptedKey, ephemeralPubKey []byte,
) ([]byte, error) {
	_ = ephemeralPubKey

	return cryptoSuite.DecryptPayloadDirect(nil, encryptedPayload, encryptedKey)
}
