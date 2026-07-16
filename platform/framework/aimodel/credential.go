package aimodel

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
	"strings"
)

const credentialCipherVersion = "v1"

type CredentialCipher interface {
	Encrypt(context.Context, string, []byte) (string, error)
	Decrypt(context.Context, string, []byte) (string, error)
}

type AESGCMCredentialCipher struct{ aead cipher.AEAD }

func NewAESGCMCredentialCipher(encodedKey string) (*AESGCMCredentialCipher, error) {
	key, err := base64.StdEncoding.DecodeString(strings.TrimSpace(encodedKey))
	if err != nil || len(key) != 32 {
		return nil, fmt.Errorf("AI credential encryption key must be base64-encoded 32 bytes")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("create credential cipher: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("create credential GCM: %w", err)
	}
	return &AESGCMCredentialCipher{aead: aead}, nil
}

func (c *AESGCMCredentialCipher) Encrypt(_ context.Context, plaintext string, aad []byte) (string, error) {
	if c == nil || c.aead == nil || plaintext == "" {
		return "", fmt.Errorf("credential is required")
	}
	nonce := make([]byte, c.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("generate credential nonce: %w", err)
	}
	sealed := c.aead.Seal(nonce, nonce, []byte(plaintext), aad)
	return credentialCipherVersion + ":" + base64.RawStdEncoding.EncodeToString(sealed), nil
}

func (c *AESGCMCredentialCipher) Decrypt(_ context.Context, ciphertext string, aad []byte) (string, error) {
	parts := strings.SplitN(ciphertext, ":", 2)
	if c == nil || c.aead == nil || len(parts) != 2 || parts[0] != credentialCipherVersion {
		return "", fmt.Errorf("unsupported credential ciphertext")
	}
	sealed, err := base64.RawStdEncoding.DecodeString(parts[1])
	if err != nil || len(sealed) <= c.aead.NonceSize() {
		return "", fmt.Errorf("invalid credential ciphertext")
	}
	nonce, encrypted := sealed[:c.aead.NonceSize()], sealed[c.aead.NonceSize():]
	plaintext, err := c.aead.Open(nil, nonce, encrypted, aad)
	if err != nil {
		return "", fmt.Errorf("decrypt credential failed")
	}
	return string(plaintext), nil
}
