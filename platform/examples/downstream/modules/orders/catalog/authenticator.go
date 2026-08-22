package catalog

import (
	"context"

	"github.com/yzxingtu2026/ainp/platform/framework/authentication"
)

type exampleAuthenticator struct{}

func (exampleAuthenticator) Method() string { return "example-sso" }

func (exampleAuthenticator) Capabilities() authentication.Capabilities {
	return authentication.Capabilities{Challenge: true, Redirect: true, LabelKey: "auth.example_sso"}
}

func (exampleAuthenticator) Begin(context.Context, authentication.BeginRequest) (*authentication.Challenge, error) {
	return &authentication.Challenge{ID: "example", ExpiresIn: 60}, nil
}

func (exampleAuthenticator) Complete(context.Context, authentication.CompleteRequest) (*authentication.VerifiedIdentity, error) {
	return &authentication.VerifiedIdentity{Provider: "example-sso", Subject: "customer-1", TenantID: "example-tenant"}, nil
}
