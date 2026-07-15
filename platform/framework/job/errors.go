package job

import (
	"errors"
	"fmt"
)

// Typed errors for transport mapping. Causes are for logs only (must be sanitized before storage).
var (
	ErrUnavailable = errors.New("job: unavailable")
	ErrNotFound    = errors.New("job: not found")
	ErrConflict    = errors.New("job: conflict")
)

type classError struct {
	class string
	err   error
}

func (e *classError) Error() string {
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
