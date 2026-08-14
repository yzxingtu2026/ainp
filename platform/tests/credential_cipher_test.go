package tests

import (
	"context"
	"encoding/base64"
	"errors"
	"strings"
	"testing"

	"github.com/yzxingtu2026/ainp/platform/framework/config"
	"github.com/yzxingtu2026/ainp/platform/framework/credential"
)

func TestAESGCMCipherUsesRandomNonceAndAAD(t *testing.T) {
	key := base64.StdEncoding.EncodeToString([]byte("0123456789abcdef0123456789abcdef"))
	cipher, err := credential.NewAESGCMCipher(key)
	if err != nil {
		t.Fatal(err)
	}
	first, err := cipher.Encrypt(context.Background(), "tenant-secret", []byte("tenant-a|setting-a|field-a"))
	if err != nil {
		t.Fatal(err)
	}
	second, err := cipher.Encrypt(context.Background(), "tenant-secret", []byte("tenant-a|setting-a|field-a"))
	if err != nil {
		t.Fatal(err)
	}
	if first == second || strings.Contains(first, "tenant-secret") {
		t.Fatal("ciphertext must use a random nonce and hide plaintext")
	}
	plain, err := cipher.Decrypt(context.Background(), first, []byte("tenant-a|setting-a|field-a"))
	if err != nil || plain != "tenant-secret" {
		t.Fatalf("decrypt failed: %q %v", plain, err)
	}
	if _, err := cipher.Decrypt(context.Background(), first, []byte("tenant-b|setting-a|field-a")); !errors.Is(err, credential.ErrUnavailable) {
		t.Fatalf("AAD replacement must fail closed: %v", err)
	}
}

func TestCredentialEncryptionKeyCompatibility(t *testing.T) {
	legacy := &config.Config{AI: config.AIConfig{CredentialEncryptionKey: "legacy"}}
	if legacy.EffectiveCredentialEncryptionKey() != "legacy" || !legacy.UsesDeprecatedCredentialEncryptionKey() {
		t.Fatal("legacy AI credential key must remain available for one deprecation cycle")
	}
	if warnings := legacy.DeprecationWarnings(); len(warnings) != 1 || strings.Contains(warnings[0], "legacy") {
		t.Fatalf("legacy warning must be stable and contain no key material: %#v", warnings)
	}
	current := &config.Config{CredentialEncryptionKey: "current", AI: config.AIConfig{CredentialEncryptionKey: "legacy"}}
	if current.EffectiveCredentialEncryptionKey() != "current" || current.UsesDeprecatedCredentialEncryptionKey() {
		t.Fatal("platform credential key must take precedence without a deprecation warning")
	}
}
