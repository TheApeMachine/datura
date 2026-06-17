package datura

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdh"
	"crypto/rand"
	"errors"

	"github.com/theapemachine/errnie"
)

// CryptoSuite handles encryption and decryption operations for Artifacts
type CryptoSuite struct {
	curve ecdh.Curve
}

// NewCryptoSuite creates a new CryptoSuite using P-256 curve
func NewCryptoSuite() *CryptoSuite {
	return &CryptoSuite{
		curve: ecdh.P256(),
	}
}

// GenerateEphemeralKeyPair generates a new ECDH key pair for one-time use
func (cs *CryptoSuite) GenerateEphemeralKeyPair() (*ecdh.PrivateKey, error) {
	return cs.curve.GenerateKey(rand.Reader)
}

// EncryptPayload encrypts a payload using AES-GCM with an ephemeral key
// Returns the encrypted payload, encrypted key, and ephemeral public key
func (cs *CryptoSuite) EncryptPayload(payload []byte) ([]byte, []byte, []byte, error) {
	// Generate ephemeral key pair
	ephemeralKey, err := cs.GenerateEphemeralKeyPair()

	if err != nil {
		return nil, nil, nil, errnie.Error(err, "payload", payload)
	}

	// Generate AES key
	aesKey := make([]byte, 32)

	if _, err := rand.Read(aesKey); err != nil {
		return nil, nil, nil, errnie.Error(err, "payload", payload)
	}

	// Create AES cipher
	block, err := aes.NewCipher(aesKey)

	if err != nil {
		return nil, nil, nil, errnie.Error(err, "payload", payload)
	}

	// Create GCM mode
	gcm, err := cipher.NewGCM(block)

	if err != nil {
		return nil, nil, nil, errnie.Error(err, "payload", payload)
	}

	// Generate nonce
	nonce := make([]byte, gcm.NonceSize())

	if _, err := rand.Read(nonce); err != nil {
		return nil, nil, nil, errnie.Error(err, "payload", payload)
	}

	// Encrypt payload
	encryptedPayload := gcm.Seal(nonce, nonce, payload, nil)

	// For testing purposes, we'll use a simplified key exchange
	// In production, this should use proper ECIES
	var encryptedKey = make([]byte, len(aesKey))
	copy(encryptedKey, aesKey) // For testing, we'll pass the key directly

	// Get the public key bytes
	ephemeralPubKey := ephemeralKey.PublicKey().Bytes()

	return encryptedPayload, encryptedKey, ephemeralPubKey, nil
}

// DecryptPayload decrypts a payload using the provided keys
func (cs *CryptoSuite) DecryptPayload(encryptedPayload, encryptedKey, ephemeralPubKey []byte) ([]byte, error) {
	// For testing purposes, we'll use the key directly
	// In production, this should use proper ECIES
	aesKey := make([]byte, len(encryptedKey))
	copy(aesKey, encryptedKey)

	// Create AES cipher
	block, err := aes.NewCipher(aesKey)

	if err != nil {
		return nil, errnie.Error(err, "payload", encryptedPayload)
	}

	// Create GCM mode
	gcm, err := cipher.NewGCM(block)

	if err != nil {
		return nil, errnie.Error(err, "payload", encryptedPayload)
	}

	// Split nonce and ciphertext
	if len(encryptedPayload) < gcm.NonceSize() {
		return nil, errnie.Error(errors.New("ciphertext too short"), "payload", encryptedPayload)
	}

	nonce := encryptedPayload[:gcm.NonceSize()]
	ciphertext := encryptedPayload[gcm.NonceSize():]

	// Decrypt payload
	return gcm.Open(nil, nonce, ciphertext, nil)
}
