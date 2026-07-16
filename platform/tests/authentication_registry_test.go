package tests

import (
	"context"
	"testing"

	"github.com/yzxingtu2026/ainp/platform/framework/authentication"
)

type fakeAuthenticator struct {
	method       string
	capabilities authentication.Capabilities
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
	methods := registry.Methods()
	if len(methods) != 1 || methods[0].Method != "password" {
		t.Fatalf("unexpected methods: %#v", methods)
	}
}
