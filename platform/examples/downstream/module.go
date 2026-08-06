package downstream

import (
	"context"
	"embed"
	"encoding/json"
	"time"

	"github.com/yzxingtu2026/ainp/platform/framework/action"
	"github.com/yzxingtu2026/ainp/platform/framework/app"
	"github.com/yzxingtu2026/ainp/platform/framework/authentication"
	"github.com/yzxingtu2026/ainp/platform/framework/capability"
	"github.com/yzxingtu2026/ainp/platform/framework/job"
	"github.com/yzxingtu2026/ainp/platform/host"
	"github.com/yzxingtu2026/ainp/platform/migrations"
)

//go:embed migrations/*.sql prompts/**/*.md i18n/*.yaml
var migrationFiles embed.FS

type Module struct{}

func (Module) ID() string { return "example.orders" }

func (Module) Prepare(application *app.App) error {
	return application.Authenticators.Register(exampleAuthenticator{})
}

func (Module) Register(application *app.App) error {
	if err := application.RegisterCapability(capability.Capability{ID: "orders", Name: "Orders", Module: "example.orders"}); err != nil {
		return err
	}
	if err := application.RegisterAction(action.Action{
		ID: "orders.list",
		Handler: func(context.Context, json.RawMessage) (any, error) {
			return []string{}, nil
		},
		Meta: action.Metadata{
			Capability: "orders",
			Access:     action.AccessPermission,
			Name:       "List orders",
			Permission: "orders:read",
			AI:         action.ExcludeFromAI(action.AIExcludePendingSafetyReview),
			HTTP:       &action.HTTPMeta{Method: "GET", Path: "/api/v1/orders"},
		},
	}); err != nil {
		return err
	}
	return application.Jobs.Register(
		job.Definition{Type: "orders.sync", SchemaVersion: 1, Queue: job.QueueDefault, MaxAttempts: 3, Timeout: time.Minute, Description: "Sync downstream orders"},
		job.HandlerFunc{JobType: "orders.sync", Fn: func(context.Context, job.Envelope, json.RawMessage, job.Reporter) error { return nil }},
	)
}

func (Module) MigrationSource() migrations.Source {
	source, err := fsSub(migrationFiles, "migrations")
	if err != nil {
		panic(err)
	}
	return migrations.Source{Owner: "example.orders", FS: source}
}

func (Module) RuntimeAssets() host.RuntimeAssets {
	prompts, err := fsSub(migrationFiles, "prompts")
	if err != nil {
		panic(err)
	}
	translations, err := fsSub(migrationFiles, "i18n")
	if err != nil {
		panic(err)
	}
	return host.RuntimeAssets{Prompts: prompts, I18n: translations}
}

type exampleAuthenticator struct{}

func (exampleAuthenticator) Method() string { return "example-sso" }
func (exampleAuthenticator) Capabilities() authentication.Capabilities {
	return authentication.Capabilities{Challenge: true, Redirect: true, LabelKey: "auth.example_sso"}
}
func (exampleAuthenticator) Begin(context.Context, authentication.BeginRequest) (*authentication.Challenge, error) {
	return &authentication.Challenge{ID: "example", ExpiresIn: 60}, nil
}
func (exampleAuthenticator) Complete(context.Context, authentication.CompleteRequest) (*authentication.VerifiedIdentity, error) {
	return &authentication.VerifiedIdentity{Provider: "example-sso", Subject: "example"}, nil
}
