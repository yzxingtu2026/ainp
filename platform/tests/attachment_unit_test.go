package tests

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/yzxingtu2026/ainp/platform/framework/attachment"
	"github.com/yzxingtu2026/ainp/platform/modules/system"
)

func TestAttachmentTransferTokenIssueAndParse(t *testing.T) {
	svc, err := attachment.NewTransferTokenService("unit-secret", "ainp")
	if err != nil {
		t.Fatal(err)
	}
	token, jti, exp, err := svc.Issue(attachment.IssueTransferInput{
		TenantID: "t1", UserID: "u1", AttachmentID: "a1",
		Op: attachment.OpUpload, Size: 12, TTL: time.Minute,
	})
	if err != nil || token == "" || jti == "" || exp.Before(time.Now()) {
		t.Fatalf("issue = %q %q %v %v", token, jti, exp, err)
	}
	claims, err := svc.Parse(token)
	if err != nil {
		t.Fatal(err)
	}
	if claims.TenantID != "t1" || claims.UserID != "u1" || claims.AttachmentID != "a1" || claims.Op != attachment.OpUpload || claims.JTI != jti {
		t.Fatalf("claims = %#v", claims)
	}
	if _, err := svc.Parse(token + "x"); err == nil {
		t.Fatal("expected parse failure")
	}
}

func TestAttachmentAccessPolicyDenyUnregistered(t *testing.T) {
	reg := attachment.NewPolicyRegistry()
	if err := reg.Allow(context.Background(), "unknown.resource", "t1", "u1", "r1", ""); err == nil {
		t.Fatal("expected deny for unregistered resource_type")
	}
	if err := reg.Register(attachment.FuncPolicy{
		Type: "demo.task",
		Fn: func(_ context.Context, _, _, resourceID, _ string) error {
			if resourceID != "ok" {
				return context.Canceled
			}
			return nil
		},
	}); err != nil {
		t.Fatal(err)
	}
	if err := reg.Allow(context.Background(), "demo.task", "t1", "u1", "ok", ""); err != nil {
		t.Fatal(err)
	}
	if err := reg.Allow(context.Background(), "demo.task", "t1", "u1", "no", ""); err == nil {
		t.Fatal("expected policy deny")
	}
}

func TestAttachmentContentDispositionSafe(t *testing.T) {
	got := system.ContentDisposition("a\r\nb\"c.pdf")
	if strings.Contains(got, "\r") || strings.Contains(got, "\n") {
		t.Fatalf("CRLF leaked: %q", got)
	}
	if !strings.HasPrefix(got, `attachment; filename="`) || !strings.HasSuffix(got, `"`) {
		t.Fatalf("unexpected disposition: %q", got)
	}
}
