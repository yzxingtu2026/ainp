package tests

import (
	"context"
	"testing"

	"github.com/yzxingtu2026/ainp/platform/framework/authentication"
)

type fakeAuthenticator struct {
	method       string
	capabilities authentication.Capabilities
	available    *bool
}

func (f *fakeAuthenticator) Available(context.Context) bool {
	return f.available == nil || *f.available
}

func (f *fakeAuthenticator) Method() string                            { return f.method }
func (f *fakeAuthenticator) Capabilities() authentication.Capabilities { return f.capabilities }
func (f *fakeAuthenticator) Begin(context.Context, authentication.BeginRequest) (*authentication.Challenge, error) {
	return &authentication.Challenge{}, nil
}
func (f *fakeAuthenticator) Complete(context.Context, authentication.CompleteRequest) (*authentication.VerifiedIdentity, error) {
	return &authentication.VerifiedIdentity{}, nil
}

func TestAuthenticatorRegistryFailsClosed(t *testing.T) {
	registry := authentication.NewRegistry()
	if err := registry.Register(&fakeAuthenticator{method: "password"}); err != nil {
		t.Fatal(err)
	}
	if _, ok := registry.Get("unknown"); ok {
		t.Fatal("unknown method must not resolve")
	}
	if err := registry.Register(&fakeAuthenticator{method: "password"}); err == nil {
		t.Fatal("duplicate method must fail")
	}
	if err := registry.Register(&fakeAuthenticator{method: "Bad Method"}); err == nil {
		t.Fatal("invalid method must fail")
	}
	if err := registry.Register(&fakeAuthenticator{method: "oidc", capabilities: authentication.Capabilities{Redirect: true}}); err == nil {
		t.Fatal("redirect without challenge must fail")
	}
	if err := registry.Register(&fakeAuthenticator{method: "bad-icon", capabilities: authentication.Capabilities{Icon: "<svg>"}}); err == nil {
		t.Fatal("unsafe icon metadata must fail")
	}
	methods := registry.Methods()
	if len(methods) != 1 || methods[0].Method != "password" {
		t.Fatalf("unexpected methods: %#v", methods)
	}
}

func TestAuthenticatorAvailabilityFailsClosed(t *testing.T) {
	available := false
	provider := &fakeAuthenticator{method: "oidc", capabilities: authentication.Capabilities{Challenge: true, Redirect: true}, available: &available}
	if authentication.Available(context.Background(), provider) {
		t.Fatal("disabled authenticator must be unavailable")
	}
	panicking := &panickingAuthenticator{fakeAuthenticator: fakeAuthenticator{method: "broken"}}
	if authentication.Available(context.Background(), panicking) {
		t.Fatal("availability panic must fail closed")
	}
}

func TestAuthenticatorMetadataIsSnapshottedAtRegistration(t *testing.T) {
	provider := &fakeAuthenticator{method: "oidc", capabilities: authentication.Capabilities{Challenge: true, Redirect: true, Label: "OIDC", Order: 20}}
	registry := authentication.NewRegistry()
	if err := registry.Register(provider); err != nil { t.Fatal(err) }
	provider.capabilities.Label = "changed"
	provider.capabilities.Icon = "<svg>"
	methods := registry.Methods()
	if len(methods) != 1 || methods[0].Capabilities.Label != "OIDC" || methods[0].Capabilities.Icon != "" {
		t.Fatalf("registered metadata must remain immutable: %#v", methods)
	}
}

type panickingAuthenticator struct{ fakeAuthenticator }

func (*panickingAuthenticator) Available(context.Context) bool { panic("secret database failure") }
