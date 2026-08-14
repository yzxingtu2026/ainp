package aimodel

import "github.com/yzxingtu2026/ainp/platform/framework/credential"

type CredentialCipher = credential.Cipher
type AESGCMCredentialCipher = credential.AESGCMCipher

func NewAESGCMCredentialCipher(encodedKey string) (*AESGCMCredentialCipher, error) {
	return credential.NewAESGCMCipher(encodedKey)
}
