package aitool

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

type HandleClaims struct {
	TenantID    string `json:"tid"`
	UserID      string `json:"uid"`
	Realm       string `json:"realm"`
	SessionID   string `json:"sid"`
	AuthVersion uint64 `json:"auth_version"`
	ScopeID     string `json:"scope_id"`
	ToolID      string `json:"tool_id"`
	RuntimeRef  string `json:"runtime_ref"`
	SchemaHash  string `json:"schema_hash"`
	ExpiresAt   int64  `json:"exp"`
}
type HandleSigner struct{ key []byte }

func NewHandleSigner(secret string) (*HandleSigner, error) {
	if secret == "" {
		return nil, fmt.Errorf("tool handle secret is required")
	}
	key := sha256.Sum256([]byte("ainp-tool-handle:" + secret))
	return &HandleSigner{key: key[:]}, nil
}
func (s *HandleSigner) Sign(c HandleClaims) (string, error) {
	b, err := json.Marshal(c)
	if err != nil {
		return "", err
	}
	payload := base64.RawURLEncoding.EncodeToString(b)
	mac := hmac.New(sha256.New, s.key)
	mac.Write([]byte(payload))
	return payload + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil)), nil
}
func (s *HandleSigner) Verify(token string, now time.Time) (HandleClaims, error) {
	var c HandleClaims
	parts := strings.Split(token, ".")
	if len(parts) != 2 {
		return c, fmt.Errorf("invalid tool handle")
	}
	mac := hmac.New(sha256.New, s.key)
	mac.Write([]byte(parts[0]))
	sig, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil || !hmac.Equal(sig, mac.Sum(nil)) {
		return c, fmt.Errorf("invalid tool handle")
	}
	b, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil || json.Unmarshal(b, &c) != nil {
		return c, fmt.Errorf("invalid tool handle")
	}
	if now.Unix() >= c.ExpiresAt {
		return c, fmt.Errorf("tool handle expired")
	}
	return c, nil
}
