package job

import (
	"errors"
	"fmt"

	"github.com/yzxingtu2026/ainp/platform/framework/aimodelruntime"
)

func ClassifyEmbeddingError(stage string, err error) error {
	var runtimeErr *aimodelruntime.RuntimeError
	if !errors.As(err, &runtimeErr) {
		return ClassifiedRetryable("persistence", stage, "embedding index persistence failed", err)
	}
	category := string(runtimeErr.Category)
	summary := "embedding runtime failed"
	if runtimeErr.Category == aimodelruntime.ErrorInvalidVector || runtimeErr.Category == aimodelruntime.ErrorResponseMismatch || runtimeErr.Category == aimodelruntime.ErrorDimensionsUnsupported {
		return ClassifiedPermanent(category, stage, "embedding response is invalid", err)
	}
	if runtimeErr.Category == aimodelruntime.ErrorCapabilityNotSupported || runtimeErr.Category == aimodelruntime.ErrorInvalidRequest || runtimeErr.Category == aimodelruntime.ErrorModelNotFound {
		return ClassifiedPermanent(category, stage, "embedding configuration is invalid", err)
	}
	if runtimeErr.Category == aimodelruntime.ErrorAuthenticationFailed || runtimeErr.Category == aimodelruntime.ErrorPermissionDenied {
		return ClassifiedPermanent(category, stage, "embedding credential is unavailable", err)
	}
	if runtimeErr.Retryable || runtimeErr.Category == aimodelruntime.ErrorTimeout || runtimeErr.Category == aimodelruntime.ErrorRateLimited || runtimeErr.Category == aimodelruntime.ErrorProviderUnavailable {
		return ClassifiedRetryable(category, stage, summary, err)
	}
	return ClassifiedPermanent(category, stage, summary, err)
}

// Typed errors for transport mapping. Causes are for logs only (must be sanitized before storage).
var (
	ErrUnavailable = errors.New("job: unavailable")
	ErrNotFound    = errors.New("job: not found")
	ErrConflict    = errors.New("job: conflict")
)

type classError struct {
	class    string
	err      error
	category string
	stage    string
	summary  string
}

func ClassifiedRetryable(category, stage, summary string, err error) error {
	return classified("retryable", category, stage, summary, err)
}

func ClassifiedPermanent(category, stage, summary string, err error) error {
	return classified("permanent", category, stage, summary, err)
}

func classified(class, category, stage, summary string, err error) error {
	if err == nil {
		err = errors.New(summary)
	}
	return &classError{class: class, err: err, category: category, stage: stage, summary: summary}
}

func ErrorFacts(err error) (category, stage, summary string) {
	var classified *classError
	if errors.As(err, &classified) {
		return classified.category, classified.stage, classified.summary
	}
	return "", "", ""
}

func (e *classError) Error() string {
	if e.summary != "" {
		return fmt.Sprintf("job:%s: %s", e.class, e.summary)
	}
	if e.err == nil {
		return "job:" + e.class
	}
	return fmt.Sprintf("job:%s: %v", e.class, e.err)
}

func (e *classError) Unwrap() error { return e.err }

func Retryable(err error) error {
	if err == nil {
		return nil
	}
	return &classError{class: "retryable", err: err}
}

func Permanent(err error) error {
	if err == nil {
		return nil
	}
	return &classError{class: "permanent", err: err}
}

func Cancelled(err error) error {
	if err == nil {
		return &classError{class: "cancelled", err: errors.New("cancelled")}
	}
	return &classError{class: "cancelled", err: err}
}

func IsRetryable(err error) bool {
	var ce *classError
	return errors.As(err, &ce) && ce.class == "retryable"
}

func IsPermanent(err error) bool {
	var ce *classError
	return errors.As(err, &ce) && ce.class == "permanent"
}

func IsCancelled(err error) bool {
	var ce *classError
	return errors.As(err, &ce) && ce.class == "cancelled"
}
