package errs

import (
	"errors"
	"fmt"
)

const (
	CodeOK = 0

	CodeValidation      = 10001
	CodeUnauthorized    = 10002
	CodeForbidden       = 10003
	CodeNotFound        = 10004
	CodeConflict        = 10005
	CodeTooManyRequests = 10006

	CodeTenantDisabled     = 20001
	CodeInvalidCredentials = 20002
	CodeUserDisabled       = 20003

	CodeActionNotFound  = 30001
	CodeCodeSetNotFound = 30002

	CodeIdempotencyKeyRequired = 40001
	CodeIdempotencyConflict    = 40002
	CodeOperationInProgress    = 40003
	CodeRevisionConflict       = 40004
	CodeResourceLocked         = 40005

	CodeSetupRequired  = 50003
	CodeSetupInstalled = 50004

	CodeNotificationDefinitionNotFound = 60001
	CodeNotificationTemplateMissing    = 60002
	CodeNotificationRecipientInvalid   = 60003
	CodeNotificationDeliveryFailed     = 60004
	CodeNotificationDuplicateRequest   = 60005

	CodeInternal = 50000
)

// codeDefaultKey 将错误码映射到默认 message_key，供未显式传 key 的旧调用自动关联。
var codeDefaultKey = map[int]string{
	CodeValidation:                     "common.validation_failed",
	CodeUnauthorized:                   "common.unauthorized",
	CodeForbidden:                      "common.forbidden",
	CodeNotFound:                       "common.not_found",
	CodeConflict:                       "common.conflict",
	CodeTooManyRequests:                "common.too_many_requests",
	CodeTenantDisabled:                 "auth.tenant_disabled",
	CodeInvalidCredentials:             "auth.invalid_credentials",
	CodeUserDisabled:                   "auth.user_disabled",
	CodeActionNotFound:                 "action.not_found",
	CodeCodeSetNotFound:                "codeset.not_found",
	CodeIdempotencyKeyRequired:         "idempotency.key_required",
	CodeIdempotencyConflict:            "idempotency.conflict",
	CodeOperationInProgress:            "idempotency.in_progress",
	CodeRevisionConflict:               "common.revision_conflict",
	CodeResourceLocked:                 "common.resource_locked",
	CodeSetupRequired:                  "setup.required",
	CodeSetupInstalled:                 "setup.already_installed",
	CodeNotificationDefinitionNotFound: "notification.definition_not_found",
	CodeNotificationTemplateMissing:    "notification.template_missing",
	CodeNotificationRecipientInvalid:   "notification.recipient_invalid",
	CodeNotificationDeliveryFailed:     "notification.delivery_failed",
	CodeNotificationDuplicateRequest:   "notification.duplicate_request",
	CodeInternal:                       "common.internal_error",
}

type AppError struct {
	Code       int
	MessageKey string
	Message    string
	Fields     []FieldError
	Cause      error
}

type FieldError struct {
	Field      string `json:"field"`
	Message    string `json:"message"`
	MessageKey string `json:"message_key,omitempty"`
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

// Key 返回该错误的 message_key；若未显式设置，则按 code 查默认映射。
func (e *AppError) Key() string {
	if e == nil {
		return ""
	}
	if e.MessageKey != "" {
		return e.MessageKey
	}
	return codeDefaultKey[e.Code]
}

// New 使用硬编码 message 创建 AppError（兼容旧调用）。
// message_key 由 code 自动映射。
func New(code int, message string) *AppError {
	return &AppError{Code: code, Message: message, MessageKey: codeDefaultKey[code]}
}

// NewKey 使用 message_key 创建 AppError；message 为英文兜底。
func NewKey(code int, key string, fallback string) *AppError {
	return &AppError{Code: code, MessageKey: key, Message: fallback}
}

func Wrap(code int, message string, cause error) *AppError {
	return &AppError{Code: code, Message: message, MessageKey: codeDefaultKey[code], Cause: cause}
}

// Validation 校验失败（兼容旧调用）。
func Validation(message string, fields ...FieldError) *AppError {
	return &AppError{Code: CodeValidation, Message: message, MessageKey: codeDefaultKey[CodeValidation], Fields: fields}
}

// ValidationKey 校验失败，使用 message_key。
func ValidationKey(key string, fallback string, fields ...FieldError) *AppError {
	return &AppError{Code: CodeValidation, MessageKey: key, Message: fallback, Fields: fields}
}

func AsAppError(err error) (*AppError, bool) {
	if err == nil {
		return nil, false
	}
	var ae *AppError
	return ae, errors.As(err, &ae)
}
