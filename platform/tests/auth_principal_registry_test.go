package tests

import (
	"context"
	"testing"
	"time"

	"github.com/yzxingtu2026/ainp/platform/framework/auth"
)

type testPrincipalProvider struct{ kind auth.PrincipalKind }

func (p testPrincipalProvider) Kind() auth.PrincipalKind { return p.kind }
func (p testPrincipalProvider) LoadAuthorization(_ context.Context, req auth.AuthorizationRequest) (*auth.AuthorizationSnapshot, error) {
	return &auth.AuthorizationSnapshot{TenantID: req.TenantID, Principal: req.PrincipalRef(), PrincipalID: req.PrincipalRef().ID, SessionID: req.SessionID, AuthVersion: req.AuthVersion, Audience: req.Audience, ClientSurface: req.ClientSurface}, nil
}

func TestClientSurfaceRegistryValidatesSessionPolicy(t *testing.T) {
	base := auth.ClientSurface{ID: "example.client", Audience: "example:client", PathPrefix: "/api/", PrincipalKinds: []auth.PrincipalKind{"example.customer"}, Methods: []string{"example-sso"}}
	for name, mutate := range map[string]func(*auth.ClientSurface){
		"negative absolute ttl": func(surface *auth.ClientSurface) { surface.AbsoluteTTL = -time.Second },
		"negative idle ttl":     func(surface *auth.ClientSurface) { surface.IdleTTL = -time.Second },
		"negative max sessions": func(surface *auth.ClientSurface) { surface.MaxActiveSessions = -1 },
		"unknown overflow":      func(surface *auth.ClientSurface) { surface.OverflowPolicy = "drop_random" },
	} {
		t.Run(name, func(t *testing.T) {
			surface := base
			mutate(&surface)
			if err := auth.NewClientSurfaceRegistry().Register(surface); err == nil {
				t.Fatal("invalid session policy must be rejected")
			}
		})
	}

	base.MaxActiveSessions = 2
	registry := auth.NewClientSurfaceRegistry()
	if err := registry.Register(base); err != nil {
		t.Fatal(err)
	}
	got, ok := registry.Get(base.ID)
	if !ok || got.OverflowPolicy != auth.SessionOverflowReject {
		t.Fatalf("default overflow policy = %q, want reject", got.OverflowPolicy)
	}
}

func TestPrincipalRegistryFailsClosedAndSeals(t *testing.T) {
	r := auth.NewPrincipalRegistry()
	if err := r.Register(testPrincipalProvider{kind: "example.customer"}); err != nil {
		t.Fatal(err)
	}
	if err := r.Register(testPrincipalProvider{kind: "example.customer"}); err == nil {
		t.Fatal("duplicate kind must fail")
	}
	if err := r.Register(testPrincipalProvider{kind: "Bad Kind"}); err == nil {
		t.Fatal("invalid kind must fail")
	}
	r.Seal()
	if err := r.Register(testPrincipalProvider{kind: "example.staff"}); err == nil {
		t.Fatal("sealed registry must reject mutation")
	}
	if _, err := r.LoadAuthorization(context.Background(), auth.AuthorizationRequest{Principal: auth.PrincipalRef{Kind: "unknown.kind", ID: "1"}}); err == nil {
		t.Fatal("unknown kind must fail")
	}
}

func TestClientSurfaceRegistryLongestPrefixAndConflicts(t *testing.T) {
	r := auth.NewClientSurfaceRegistry()
	base := auth.ClientSurface{ID: "example.client", Audience: "example:client", PathPrefix: "/api/", PrincipalKinds: []auth.PrincipalKind{"example.customer"}, Methods: []string{"example-sso"}}
	if err := r.Register(base); err != nil {
		t.Fatal(err)
	}
	admin := auth.ClientSurface{ID: "example.admin", Audience: "example:admin", PathPrefix: "/api/admin/", PrincipalKinds: []auth.PrincipalKind{auth.PrincipalKindAccount}, Methods: []string{"password"}}
	if err := r.Register(admin); err != nil {
		t.Fatal(err)
	}
	got, ok := r.ResolvePath("/api/admin/orders")
	if !ok || got.ID != admin.ID {
		t.Fatalf("longest prefix resolved %#v", got)
	}
	duplicateAudience := base
	duplicateAudience.ID = "example.other"
	duplicateAudience.PathPrefix = "/other/"
	if err := r.Register(duplicateAudience); err == nil {
		t.Fatal("duplicate audience must fail")
	}
	duplicatePrefix := base
	duplicatePrefix.ID = "example.other"
	duplicatePrefix.Audience = "example:other"
	if err := r.Register(duplicatePrefix); err == nil {
		t.Fatal("duplicate prefix must fail")
	}
	if _, ok := r.ResolvePath("/unknown"); ok {
		t.Fatal("unknown path must not fall back")
	}
}

func TestTokenServiceBindsPrincipalAudienceAndSurface(t *testing.T) {
	tokens, err := auth.NewTokenService("test-secret", "ainp-test", 0)
	if err != nil {
		t.Fatal(err)
	}
	ref := auth.PrincipalRef{Kind: "example.customer", ID: "customer-1"}
	token, _, err := tokens.IssueFor("tenant-1", ref, "example.mobile", "example:mobile", "session-1", 2)
	if err != nil {
		t.Fatal(err)
	}
	claims, err := tokens.ParseFor(token, "example:mobile")
	if err != nil {
		t.Fatal(err)
	}
	if claims.Subject != ref.ID || claims.PrincipalKind != ref.Kind || claims.ClientSurface != "example.mobile" {
		t.Fatalf("claims mismatch: %#v", claims)
	}
	if _, err := tokens.ParseFor(token, auth.TenantAudience); err == nil {
		t.Fatal("cross-audience token must fail")
	}
}
