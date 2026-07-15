package storage

import (
	"context"
	"errors"
	"io"
	"time"
)

// Stable storage errors. Callers should use errors.Is.
var (
	ErrNotFound            = errors.New("storage: object not found")
	ErrInvalidKey          = errors.New("storage: invalid key")
	ErrSizeMismatch        = errors.New("storage: size mismatch")
	ErrRangeInvalid        = errors.New("storage: invalid range")
	ErrProviderUnavailable = errors.New("storage: provider unavailable")
	ErrIO                  = errors.New("storage: io failure")
	ErrUnknownProvider     = errors.New("storage: unknown provider")
)

// ObjectInfo describes a stored object without exposing provider-specific paths.
type ObjectInfo struct {
	Key         string    `json:"key"`
	Size        int64     `json:"size"`
	ContentType string    `json:"content_type,omitempty"`
	ETag        string    `json:"etag,omitempty"`
	ModifiedAt  time.Time `json:"modified_at"`
	Provider    string    `json:"provider"`
}

// PutRequest streams object bytes. Size may be -1 when unknown; when >= 0 the
// provider must reject a stream whose length differs from Size.
type PutRequest struct {
	Key         string
	Body        io.Reader
	Size        int64
	ContentType string
}

// ObjectStore is the provider-agnostic binary object API.
// Business modules must not open local paths or branch on provider type.
type ObjectStore interface {
	Put(ctx context.Context, req PutRequest) (ObjectInfo, error)
	Open(ctx context.Context, key string) (io.ReadCloser, ObjectInfo, error)
	OpenRange(ctx context.Context, key string, offset, length int64) (io.ReadCloser, ObjectInfo, error)
	Stat(ctx context.Context, key string) (ObjectInfo, error)
	Delete(ctx context.Context, key string) error
	Exists(ctx context.Context, key string) (bool, error)
}

// ReadyChecker is an optional deep readiness probe (not used by lightweight healthz).
type ReadyChecker interface {
	CheckReady(ctx context.Context) error
}

// Presigner is optional; cloud adapters may implement URL signing later.
// Core ObjectStore must not require it.
type Presigner interface {
	PresignPut(ctx context.Context, key string, contentType string, ttl time.Duration) (url string, err error)
	PresignGet(ctx context.Context, key string, ttl time.Duration) (url string, err error)
}

// MetricsRecorder is an optional extension point; a no-op is fine in Phase 1.
type MetricsRecorder interface {
	ObserveBytes(op string, n int64)
	ObserveDuration(op string, d time.Duration)
	ObserveFailure(op string)
}
