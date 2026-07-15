package tests

import (
	"testing"

	"github.com/yzxingtu2026/ainp/platform/framework/app"
	"github.com/yzxingtu2026/ainp/platform/framework/attachment"
	"github.com/yzxingtu2026/ainp/platform/framework/storage"
)

func attachStorage(t *testing.T, application *app.App) {
	t.Helper()
	store, err := storage.NewLocalObjectStore(storage.LocalConfig{RootDir: t.TempDir()}, nil)
	if err != nil {
		t.Fatal(err)
	}
	tokens, err := attachment.NewTransferTokenService("test-secret-for-attachments", "ainp")
	if err != nil {
		t.Fatal(err)
	}
	application.Storage = store
	application.TransferTokens = tokens
	if application.AttachmentPolicies == nil {
		application.AttachmentPolicies = attachment.NewPolicyRegistry()
	}
}
