package errs

import "fmt"

const (
	CodeOK = 0

	CodeValidation   = 10001
	CodeUnauthorized = 10002
	CodeForbidden    = 10003
	CodeNotFound     = 10004

	CodeTenantDisabled     = 20001
	CodeInvalidCredentials = 20002
	CodeUserDisabled       = 20003

	CodeActionNotFound = 30001

	CodeInternal = 50000
)

type AppError struct {
	Code    int
	Message string
	Fields  []FieldError
	Cause   error
}

type FieldError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

func (e *AppError) Error() string {
	if e == nil {
		return ""
	}
	if e.Cause != nil {
		return fmt.Sprintf("%s: %v", e.Message, e.Cause)
	}
	return e.Message
}

func (e *AppError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

func New(code int, message string) *AppError {
	return &AppError{Code: code, Message: message}
}

func Wrap(code int, message string, cause error) *AppError {
	return &AppError{Code: code, Message: message, Cause: cause}
}

func Validation(message string, fields ...FieldError) *AppError {
	return &AppError{Code: CodeValidation, Message: message, Fields: fields}
}

func AsAppError(err error) (*AppError, bool) {
	if err == nil {
		return nil, false
	}
	if ae, ok := err.(*AppError); ok {
		return ae, true
	}
	return nil, false
}
