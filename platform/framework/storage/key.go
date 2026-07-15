package storage

import (
	"fmt"
	"path"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
)

var (
	segmentPat  = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]{0,127}$`)
	objectIDPat = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]{0,127}$`)
)

// KeyParts builds a storage key under a controlled namespace.
// Clients must never submit a full key; upper layers pass validated identifiers.
type KeyParts struct {
	TenantID  string
	Namespace string // e.g. attachments
	ObjectID  string // empty => generate UUID
	At        time.Time
}

// BuildKey returns tenant/{tenant_id}/{namespace}/{yyyy}/{mm}/{object_id}.
func BuildKey(parts KeyParts) (string, error) {
	if err := validateSegment("tenant_id", parts.TenantID); err != nil {
		return "", err
	}
	if err := validateSegment("namespace", parts.Namespace); err != nil {
		return "", err
	}
	objectID := parts.ObjectID
	if objectID == "" {
		objectID = uuid.NewString()
	}
	if !objectIDPat.MatchString(objectID) {
		return "", fmt.Errorf("%w: invalid object_id", ErrInvalidKey)
	}
	at := parts.At
	if at.IsZero() {
		at = time.Now().UTC()
	}
	key := path.Join(
		"tenant",
		parts.TenantID,
		parts.Namespace,
		at.Format("2006"),
		at.Format("01"),
		objectID,
	)
	if err := ValidateKey(key); err != nil {
		return "", err
	}
	return key, nil
}

func validateSegment(name, value string) error {
	if value == "" || !segmentPat.MatchString(value) {
		return fmt.Errorf("%w: invalid %s", ErrInvalidKey, name)
	}
	if strings.Contains(value, "..") {
		return fmt.Errorf("%w: invalid %s", ErrInvalidKey, name)
	}
	return nil
}

// ValidateKey rejects absolute paths, traversal, empty segments, and backslashes.
func ValidateKey(key string) error {
	if key == "" {
		return fmt.Errorf("%w: empty", ErrInvalidKey)
	}
	if strings.Contains(key, `\`) {
		return fmt.Errorf("%w: backslash not allowed", ErrInvalidKey)
	}
	if strings.HasPrefix(key, "/") || strings.HasPrefix(key, ".") {
		return fmt.Errorf("%w: absolute or dot-prefixed key", ErrInvalidKey)
	}
	if path.IsAbs(key) {
		return fmt.Errorf("%w: absolute key", ErrInvalidKey)
	}
	cleaned := path.Clean(key)
	if cleaned != key || cleaned == "." || strings.HasPrefix(cleaned, "..") {
		return fmt.Errorf("%w: unclean or traversal key", ErrInvalidKey)
	}
	parts := strings.Split(key, "/")
	if len(parts) == 0 {
		return fmt.Errorf("%w: empty", ErrInvalidKey)
	}
	for _, part := range parts {
		if part == "" || part == "." || part == ".." {
			return fmt.Errorf("%w: empty or traversal segment", ErrInvalidKey)
		}
		if strings.ContainsAny(part, `\:`) {
			return fmt.Errorf("%w: illegal character in segment", ErrInvalidKey)
		}
		if !segmentPat.MatchString(part) && !objectIDPat.MatchString(part) {
			// Allow pure numeric month/year segments already matching segmentPat.
			return fmt.Errorf("%w: illegal segment %q", ErrInvalidKey, part)
		}
	}
	return nil
}

// KeyDigest returns a short non-reversible-ish log-safe summary of a key.
func KeyDigest(key string) string {
	if key == "" {
		return ""
	}
	parts := strings.Split(key, "/")
	last := parts[len(parts)-1]
	if len(last) > 12 {
		last = last[:8] + "…"
	}
	return fmt.Sprintf("segs=%d/tail=%s", len(parts), last)
}
