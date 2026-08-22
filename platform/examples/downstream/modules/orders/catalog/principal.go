package catalog

import (
	"context"

	"github.com/yzxingtu2026/ainp/platform/framework/auth"
)

const (
	CustomerKind     auth.PrincipalKind = "example.customer"
	CustomerSurface                     = "example.customer-client"
	CustomerAudience                    = "example:customer-client"
)

type customerProvider struct{}

func (customerProvider) Kind() auth.PrincipalKind { return CustomerKind }
func (customerProvider) LoadAuthorization(_ context.Context, req auth.AuthorizationRequest) (*auth.AuthorizationSnapshot, error) {
	ref := req.PrincipalRef()
	if req.TenantID != "example-tenant" || ref.Kind != CustomerKind || ref.ID != "customer-1" || req.AuthVersion != 1 {
		return nil, auth.ErrAuthorizationInvalid
	}
	return &auth.AuthorizationSnapshot{
		TenantID: req.TenantID, Principal: ref, PrincipalID: ref.ID, SessionID: req.SessionID,
		AuthVersion: 1, Audience: req.Audience, ClientSurface: req.ClientSurface,
		Permissions: []string{"orders:read"},
	}, nil
}
