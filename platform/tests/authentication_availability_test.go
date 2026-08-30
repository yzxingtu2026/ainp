package tests

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/yzxingtu2026/ainp/platform/framework/auth"
	"github.com/yzxingtu2026/ainp/platform/framework/authentication"
	system "github.com/yzxingtu2026/ainp/platform/modules/system/access"
)

type availabilityAuthenticator struct{ available bool }

func (*availabilityAuthenticator) Method() string { return "example-redirect" }
func (*availabilityAuthenticator) Capabilities() authentication.Capabilities {
	return authentication.Capabilities{Challenge: true, Redirect: true, Label: "Example", Icon: "mdi:login", Order: 10}
}
func (a *availabilityAuthenticator) Available(context.Context) bool { return a.available }
func (*availabilityAuthenticator) Begin(context.Context, authentication.BeginRequest) (*authentication.Challenge, error) {
	return &authentication.Challenge{ID: "challenge", Data: map[string]any{"authorize_url": "https://login.example/authorize"}}, nil
}
func (*availabilityAuthenticator) Complete(context.Context, authentication.CompleteRequest) (*authentication.VerifiedIdentity, error) {
	return &authentication.VerifiedIdentity{}, nil
}

func TestUnifiedAuthenticationUsesAvailabilityForEveryPublicEntry(t *testing.T) {
	provider := &availabilityAuthenticator{}
	registry := authentication.NewRegistry()
	if err := registry.Register(provider); err != nil {
		t.Fatal(err)
	}
	surfaces := auth.NewClientSurfaceRegistry()
	if err := surfaces.Register(auth.ClientSurface{
		ID: auth.TenantConsoleSurface, Audience: auth.TenantAudience, PathPrefix: "/api/v1/system/",
		PrincipalKinds: []auth.PrincipalKind{auth.PrincipalKindAccount}, Methods: []string{provider.Method()},
	}); err != nil {
		t.Fatal(err)
	}
	service := system.NewUnifiedAuthService(nil, registry, nil, nil, nil)
	service.Configure(nil, surfaces)
	ctx := auth.WithContext(context.Background(), &auth.Context{ClientSurface: auth.TenantConsoleSurface, Audience: auth.TenantAudience})

	methods := service.Methods(ctx).(map[string]any)["methods"].([]authentication.Method)
	if len(methods) != 0 {
		t.Fatalf("unavailable method leaked into catalog: %#v", methods)
	}
	if _, err := service.Begin(ctx, system.ChallengeRequest{Method: provider.Method(), Client: auth.TenantConsoleSurface}); err == nil {
		t.Fatal("unavailable method must not begin a challenge")
	}
	if _, err := service.Authenticate(ctx, system.AuthenticateRequest{Method: provider.Method(), Client: auth.TenantConsoleSurface, Credentials: json.RawMessage(`{}`)}); err == nil {
		t.Fatal("unavailable method must not authenticate")
	}

	provider.available = true
	methods = service.Methods(ctx).(map[string]any)["methods"].([]authentication.Method)
	if len(methods) != 1 || methods[0].Method != provider.Method() {
		t.Fatalf("available method missing: %#v", methods)
	}
	if _, err := service.Begin(ctx, system.ChallengeRequest{Method: provider.Method(), Client: auth.TenantConsoleSurface}); err != nil {
		t.Fatal(err)
	}
}
