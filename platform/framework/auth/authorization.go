package auth

import (
	"context"
	"errors"
)

var (
	ErrAuthorizationInvalid = errors.New("authorization identity is invalid")
	ErrAuthorizationStale   = errors.New("authorization version is stale")
)

// AuthorizationRequest contains only identity data verified from a signed token.
type AuthorizationRequest struct {
	TenantID    string
	UserID      string
	SessionID   string
	Realm       Realm
	AuthVersion uint64
}

// AuthorizationSnapshot is the current server-side authorization state.
type AuthorizationSnapshot struct {
	TenantID    string
	UserID      string
	SessionID   string
	OrgID       string
	DeptID      string
	Roles       []string
	Permissions []string
	AuthVersion uint64
}

// AuthorizationProvider validates identity state and loads current authorization.
// Implementations must fail closed and scope every lookup by tenant and user.
type AuthorizationProvider interface {
	LoadAuthorization(context.Context, AuthorizationRequest) (*AuthorizationSnapshot, error)
}
