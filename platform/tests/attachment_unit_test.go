package tests

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/yzxingtu2026/ainp/platform/framework/attachment"
	"github.com/yzxingtu2026/ainp/platform/modules/system"
)

func TestAttachmentTransferRoutesIncludeMultipartPost(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	system.RegisterAttachmentTransferRoutes(router, &system.AttachmentService{})
	want := map[string]bool{"PUT": false, "POST": false, "GET": false, "HEAD": false}
	for _, route := range router.Routes() {
		if route.Path == "/api/v1/attachments/transfer/:id" {
			if _, ok := want[route.Method]; ok {
				want[route.Method] = true
			}
		}
	}
	for method, found := range want {
		if !found {
			t.Fatalf("missing %s attachment transfer route", method)
		}
	}
}

func TestAttachmentTransferTokenIssueAndParse(t *testing.T) {
	svc, err := attachment.NewTransferTokenService("unit-secret", "ainp")
	if err != nil {
		t.Fatal(err)
	}
	token, jti, exp, err := svc.Issue(attachment.IssueTransferInput{
		TenantID: "t1", PrincipalID: "u1", AttachmentID: "a1",
		Op: attachment.OpUpload, Size: 12, TTL: time.Minute,
	})
	if err != nil || token == "" || jti == "" || exp.Before(time.Now()) {
		t.Fatalf("issue = %q %q %v %v", token, jti, exp, err)
	}
	claims, err := svc.Parse(token)
	if err != nil {
		t.Fatal(err)
	}
	if claims.TenantID != "t1" || claims.PrincipalID != "u1" || claims.AttachmentID != "a1" || claims.Op != attachment.OpUpload || claims.JTI != jti {
		t.Fatalf("claims = %#v", claims)
	}
	if claims.Transport != attachment.TransferTransportRaw || claims.EffectiveUploadTransport() != attachment.TransferTransportRaw {
		t.Fatalf("default upload transport = %#v", claims)
	}
	multipartToken, _, _, err := svc.Issue(attachment.IssueTransferInput{
		TenantID: "t1", PrincipalID: "u1", AttachmentID: "a2",
		Op: attachment.OpUpload, Transport: attachment.TransferTransportMultipart, Size: 12, TTL: time.Minute,
	})
	if err != nil {
		t.Fatal(err)
	}
	multipartClaims, err := svc.Parse(multipartToken)
	if err != nil || multipartClaims.EffectiveUploadTransport() != attachment.TransferTransportMultipart {
		t.Fatalf("multipart claims=%#v err=%v", multipartClaims, err)
	}
	if _, _, _, err := svc.Issue(attachment.IssueTransferInput{
		TenantID: "t1", PrincipalID: "u1", AttachmentID: "a3",
		Op: attachment.OpUpload, Transport: "unknown", Size: 12, TTL: time.Minute,
	}); err == nil {
		t.Fatal("invalid upload transport must be rejected")
	}
	if _, err := svc.Parse(token + "x"); err == nil {
		t.Fatal("expected parse failure")
	}
}

func TestAttachmentTransferTokenLegacyUIDFallbackAndConflict(t *testing.T) {
	svc, err := attachment.NewTransferTokenService("unit-secret", "ainp")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	base := attachment.TransferClaims{
		TenantID: "t1", LegacyUserID: "legacy-account", AttachmentID: "a1", Op: attachment.OpDownload, Variant: "original", JTI: "jti-1",
		RegisteredClaims: jwt.RegisteredClaims{Issuer: "ainp", Audience: jwt.ClaimStrings{attachment.TransferAudience}, IssuedAt: jwt.NewNumericDate(now), ExpiresAt: jwt.NewNumericDate(now.Add(time.Minute))},
	}
	legacyToken, err := jwt.NewWithClaims(jwt.SigningMethodHS256, base).SignedString([]byte("unit-secret"))
	if err != nil {
		t.Fatal(err)
	}
	claims, err := svc.Parse(legacyToken)
	if err != nil || claims.PrincipalID != "legacy-account" {
		t.Fatalf("legacy uid fallback claims=%#v err=%v", claims, err)
	}
	base.PrincipalID = "canonical-account"
	conflictToken, err := jwt.NewWithClaims(jwt.SigningMethodHS256, base).SignedString([]byte("unit-secret"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = svc.Parse(conflictToken); err == nil {
		t.Fatal("conflicting attachment principal claims must be rejected")
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
