package action

import (
	"github.com/yzxingtu2026/ainp/platform/framework/auth"
)

// NormalizeActor fills historical Account defaults on a trusted AuthContext.
func NormalizeActor(ac *auth.Context) *auth.Context {
	if ac == nil {
		return &auth.Context{}
	}
	if !ac.Principal.Valid() && ac.PrincipalID != "" {
		ac.Principal = auth.PrincipalRef{Kind: auth.PrincipalKindAccount, ID: ac.PrincipalID}
	}
	if ac.Authenticated && ac.Principal.Kind == "" {
		ac.Principal.Kind = auth.PrincipalKindAccount
	}
	if ac.ClientSurface == "" && ac.Principal.Kind == auth.PrincipalKindAccount {
		ac.ClientSurface = auth.TenantConsoleSurface
	}
	return ac
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
