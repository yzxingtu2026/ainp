// Package platformresource defines the stable, provider-neutral platform
// resource contracts available to downstream modules.
package platformresource

import (
	"context"
	"errors"
	"io"
	"regexp"
	"strings"
	"time"

	"github.com/yzxingtu2026/ainp/platform/framework/auth"
)

var (
	ErrUnavailable = errors.New("platform resource unavailable")
	ErrForbidden   = errors.New("platform resource forbidden")
	ErrConflict    = errors.New("platform resource revision conflict")
	ErrInvalid     = errors.New("platform resource input invalid")
	ErrNotFound    = errors.New("platform resource not found")
	ErrDisabled    = errors.New("platform resource disabled")
)

var accountIdentityProviderPattern = regexp.MustCompile(`^[a-z][a-z0-9._-]{0,63}$`)

type AttachmentVariant string

const (
	AttachmentVariantOriginal AttachmentVariant = "original"
	AttachmentVariantThumb    AttachmentVariant = "thumb"
)

type AttachmentBindingQuery struct {
	ResourceType string `json:"resource_type"`
	ResourceID   string `json:"resource_id"`
	Field        string `json:"field,omitempty"`
}

type AttachmentOpenRequest struct {
	AttachmentID string                 `json:"attachment_id"`
	Binding      AttachmentBindingQuery `json:"binding"`
	Variant      AttachmentVariant      `json:"variant,omitempty"`
}

// OwnedUploadOpenRequest opens the original bytes of an attachment uploaded by
// the current principal. MaxBytes is required so downstream callers make their
// processing budget explicit before any object is opened.
type OwnedUploadOpenRequest struct {
	AttachmentID string `json:"attachment_id"`
	MaxBytes     int64  `json:"max_bytes"`
}

type AttachmentMetadata struct {
	ID           string            `json:"id"`
	OriginalName string            `json:"original_name"`
	ContentType  string            `json:"content_type"`
	Size         int64             `json:"size"`
	SHA256       string            `json:"sha256,omitempty"`
	Variant      AttachmentVariant `json:"variant"`
	CreatedAt    time.Time         `json:"created_at"`
}

type AttachmentContent struct {
	Metadata AttachmentMetadata `json:"metadata"`
	Reader   io.ReadCloser      `json:"-"`
}

type AttachmentBinding struct {
	Attachment   AttachmentMetadata `json:"attachment"`
	ResourceType string             `json:"resource_type"`
	ResourceID   string             `json:"resource_id"`
	Field        string             `json:"field,omitempty"`
}

type AttachmentReader interface {
	Open(context.Context, AttachmentOpenRequest) (*AttachmentContent, error)
	OpenOwnedUpload(context.Context, OwnedUploadOpenRequest) (*AttachmentContent, error)
	BindingExists(context.Context, AttachmentOpenRequest) (bool, error)
	ListBindings(context.Context, AttachmentBindingQuery) ([]AttachmentBinding, error)
}

type PrincipalProjection struct {
	Principal   auth.PrincipalRef `json:"principal"`
	DisplayName string            `json:"display_name"`
	Active      bool              `json:"active"`
	Revision    uint64            `json:"revision"`
}

type PrincipalInvalidation struct {
	Principal          auth.PrincipalRef `json:"principal"`
	ExpectedRevision   uint64            `json:"expected_revision"`
	RequiredPermission string            `json:"-"`
	Reason             string            `json:"reason"`
}

type PrincipalService interface {
	Get(context.Context, auth.PrincipalRef) (*PrincipalProjection, error)
	Invalidate(context.Context, PrincipalInvalidation) (*PrincipalProjection, error)
}

// AccountIdentity is the minimum external-identity projection exposed to the
// currently authenticated AINP account. Tenant and account ownership are
// intentionally absent because both are derived from trusted AuthContext.
type AccountIdentity struct {
	ID                   string    `json:"id"`
	Provider             string    `json:"provider"`
	Subject              string    `json:"subject"`
	NormalizedIdentifier string    `json:"normalized_identifier"`
	VerifiedAt           time.Time `json:"verified_at"`
}

// AccountIdentityBindRequest must be assembled by trusted server code from a
// successfully verified external identity. It is not a client credential or
// proof and must not be exposed as a direct bind endpoint payload.
type AccountIdentityBindRequest struct {
	Provider             string `json:"provider"`
	Subject              string `json:"subject"`
	NormalizedIdentifier string `json:"normalized_identifier"`
}

func (r AccountIdentityBindRequest) Validate() error {
	if !accountIdentityProviderPattern.MatchString(r.Provider) || r.Provider == "password" ||
		!validAccountIdentityValue(r.Subject, 255) || !validAccountIdentityValue(r.NormalizedIdentifier, 255) {
		return ErrInvalid
	}
	return nil
}

type AccountIdentityUnbindRequest struct {
	IdentityID string `json:"identity_id"`
}

func (r AccountIdentityUnbindRequest) Validate() error {
	if !validAccountIdentityValue(r.IdentityID, 64) {
		return ErrInvalid
	}
	return nil
}

func validAccountIdentityValue(value string, max int) bool {
	if value == "" || value != strings.TrimSpace(value) || len(value) > max {
		return false
	}
	return !strings.ContainsFunc(value, func(r rune) bool { return r < 0x20 || r == 0x7f })
}

type AccountIdentityService interface {
	List(context.Context) ([]AccountIdentity, error)
	Bind(context.Context, AccountIdentityBindRequest) (*AccountIdentity, error)
	Unbind(context.Context, AccountIdentityUnbindRequest) error
}

// ManagedAccountIdentity is the redacted projection exposed to authorized
// account administrators. Raw subjects and normalized identifiers are never
// returned from the management surface.
type ManagedAccountIdentity struct {
	ID                         string    `json:"id"`
	Provider                   string    `json:"provider"`
	MaskedSubject              string    `json:"masked_subject"`
	MaskedNormalizedIdentifier string    `json:"masked_normalized_identifier"`
	VerifiedAt                 time.Time `json:"verified_at"`
}

type AccountIdentityAdminListRequest struct {
	AccountID string `json:"account_id" validate:"required"`
}

func (r AccountIdentityAdminListRequest) Validate() error {
	if !validAccountIdentityValue(r.AccountID, 64) {
		return ErrInvalid
	}
	return nil
}

type AccountIdentityAdminBindRequest struct {
	AccountID            string `json:"account_id" validate:"required"`
	Provider             string `json:"provider" validate:"required"`
	Subject              string `json:"subject" validate:"required"`
	NormalizedIdentifier string `json:"normalized_identifier" validate:"required"`
}

func (r AccountIdentityAdminBindRequest) Validate() error {
	if err := (AccountIdentityAdminListRequest{AccountID: r.AccountID}).Validate(); err != nil {
		return err
	}
	return (AccountIdentityBindRequest{
		Provider: r.Provider, Subject: r.Subject, NormalizedIdentifier: r.NormalizedIdentifier,
	}).Validate()
}

type AccountIdentityAdminUnbindRequest struct {
	AccountID  string `json:"account_id" validate:"required"`
	IdentityID string `json:"identity_id" validate:"required"`
}

func (r AccountIdentityAdminUnbindRequest) Validate() error {
	if err := (AccountIdentityAdminListRequest{AccountID: r.AccountID}).Validate(); err != nil {
		return err
	}
	return (AccountIdentityUnbindRequest{IdentityID: r.IdentityID}).Validate()
}

// AccountIdentityAdministrationService is an additive management contract.
// It is separate from AccountIdentityService so existing self-service
// integrations keep their current source-level contract.
type AccountIdentityAdministrationService interface {
	ListForAccount(context.Context, AccountIdentityAdminListRequest) ([]ManagedAccountIdentity, error)
	BindForAccount(context.Context, AccountIdentityAdminBindRequest) (*ManagedAccountIdentity, error)
	UnbindForAccount(context.Context, AccountIdentityAdminUnbindRequest) error
}

// Services is the immutable platform-resource surface injected into API and
// Worker module registration through app.App.
type Services struct {
	Attachments                   AttachmentReader
	Principals                    PrincipalService
	AccountIdentities             AccountIdentityService
	AccountIdentityAdministration AccountIdentityAdministrationService
}

func (s Services) Validate() error {
	if s.Attachments == nil || s.Principals == nil || s.AccountIdentities == nil || s.AccountIdentityAdministration == nil {
		return ErrUnavailable
	}
	return nil
}
