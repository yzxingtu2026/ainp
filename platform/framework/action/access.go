package action

import (
	"github.com/yzxingtu2026/ainp/platform/framework/auth"
)

// NormalizeActor fills historical Account defaults on a copy of the trusted
// AuthContext. It must not mutate the caller's context; Tool Scope compares
// the original PrincipalKind/ClientSurface across search/load/execute.
func NormalizeActor(ac *auth.Context) *auth.Context {
	if ac == nil {
		return &auth.Context{}
	}
	normalized := *ac
	if !normalized.Principal.Valid() && normalized.PrincipalID != "" {
		normalized.Principal = auth.PrincipalRef{Kind: auth.PrincipalKindAccount, ID: normalized.PrincipalID}
	}
	if normalized.Authenticated && normalized.Principal.Kind == "" {
		normalized.Principal.Kind = auth.PrincipalKindAccount
	}
	if normalized.ClientSurface == "" && normalized.Principal.Kind == auth.PrincipalKindAccount {
		normalized.ClientSurface = auth.TenantConsoleSurface
	}
	return &normalized
}

// AllowsAccessSurface reports whether the current actor may see or invoke the
// Action on this realm, PrincipalKind and Client Surface. Public Actions skip
// kind/surface matching. Permission is checked separately.
func AllowsAccessSurface(ac *auth.Context, meta Metadata) bool {
	ac = NormalizeActor(ac)
	if auth.NormalizeRealm(ac.Realm) != meta.Surface {
		return false
	}
	if meta.AccessMode() == AccessPublic {
		return true
	}
	if !ac.Authenticated {
		return false
	}
	kindAllowed := false
	for _, kind := range meta.PrincipalKinds {
		if ac.Principal.Kind == kind {
			kindAllowed = true
			break
		}
	}
	surfaceAllowed := false
	for _, surface := range meta.ClientSurfaces {
		if ac.ClientSurface == surface {
			surfaceAllowed = true
			break
		}
	}
	return kindAllowed && surfaceAllowed
}

// AllowsActor is the shared search/load/invoke gate: realm + PrincipalKind +
// ClientSurface + permission. Authenticated Actions without permission refs
// still require a matching access surface.
func AllowsActor(ac *auth.Context, meta Metadata, actionID string) bool {
	if !AllowsAccessSurface(ac, meta) {
		return false
	}
	if meta.AccessMode() == AccessPublic {
		return true
	}
	ac = NormalizeActor(ac)
	if !ac.Authenticated {
		return false
	}
	if meta.AccessMode() == AccessPermission && !ac.HasPermission(meta.PermissionCode(actionID)) {
		return false
	}
	return true
}
