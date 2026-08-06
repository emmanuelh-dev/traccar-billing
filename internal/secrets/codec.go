package secrets

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"strings"
)

const ciphertextPrefix = "v1:"

// Codec encrypts tenant-owned API credentials before they reach storage.
// SESSION_SECRET is deployment-owned key material; hashing it produces the
// fixed-size AES-256 key required by GCM.
type Codec struct {
	aead cipher.AEAD
}

func NewCodec(keyMaterial string) (*Codec, error) {
	key := sha256.Sum256([]byte(keyMaterial))
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, fmt.Errorf("secrets: create cipher: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("secrets: create gcm: %w", err)
	}
	return &Codec{aead: aead}, nil
}

func (c *Codec) Encrypt(plaintext string) (string, error) {
	nonce := make([]byte, c.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", fmt.Errorf("secrets: create nonce: %w", err)
	}
	sealed := c.aead.Seal(nil, nonce, []byte(plaintext), nil)
	payload := append(nonce, sealed...)
	return ciphertextPrefix + base64.RawURLEncoding.EncodeToString(payload), nil
}

func (c *Codec) Decrypt(ciphertext string) (string, error) {
	if !strings.HasPrefix(ciphertext, ciphertextPrefix) {
		return "", fmt.Errorf("secrets: unsupported ciphertext")
	}
	payload, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(ciphertext, ciphertextPrefix))
	if err != nil {
		return "", fmt.Errorf("secrets: decode ciphertext: %w", err)
	}
	if len(payload) < c.aead.NonceSize() {
		return "", fmt.Errorf("secrets: ciphertext is too short")
	}
	plaintext, err := c.aead.Open(nil, payload[:c.aead.NonceSize()], payload[c.aead.NonceSize():], nil)
	if err != nil {
		return "", fmt.Errorf("secrets: decrypt ciphertext: %w", err)
	}
	return string(plaintext), nil
}
