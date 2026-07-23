package tests

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/yzxingtu2026/ainp/platform/framework/aitool"
	"github.com/yzxingtu2026/ainp/platform/framework/auth"
	"github.com/yzxingtu2026/ainp/platform/framework/mcpserver"
	"github.com/yzxingtu2026/ainp/platform/framework/tenant"
)

func TestMCPOAuthMetadataAndProtectedTransport(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	service := mcpserver.NewService(nil, nil, "test-secret", "ainp-test", "https://api.ainp.example")
	mcpserver.NewHTTPServer(service, nil, "https://api.ainp.example", "https://ainp.example").Register(router)

	for _, path := range []string{"/.well-known/oauth-protected-resource", "/.well-known/oauth-authorization-server"} {
		request := httptest.NewRequest(http.MethodGet, path, nil)
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		if response.Code != http.StatusOK || (path == "/.well-known/oauth-authorization-server" && !strings.Contains(response.Body.String(), "S256")) {
			t.Fatalf("metadata %s: status=%d body=%s", path, response.Code, response.Body.String())
		}
	}
	request := httptest.NewRequest(http.MethodGet, "/.well-known/oauth-authorization-server", nil)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if !strings.Contains(response.Body.String(), `"authorization_endpoint":"https://ainp.example/oauth/authorize"`) {
		t.Fatalf("authorization endpoint must use web base URL: %s", response.Body.String())
	}
	request = httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`))
	response = httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusUnauthorized || !strings.Contains(response.Header().Get("WWW-Authenticate"), "oauth-protected-resource") {
		t.Fatalf("unprotected MCP request status=%d challenge=%q", response.Code, response.Header().Get("WWW-Authenticate"))
	}
}

func TestMCPSensitiveConfirmationIsBoundAndSingleUse(t *testing.T) {
	db := openOutboxTestDB(t)
	service := mcpserver.NewService(db, nil, "test-secret", "ainp-test", "https://api.ainp.example")
	service.SetWebBaseURL("https://ainp.example")
	if err := mcpserver.AutoMigrate(db); err != nil {
		t.Fatal(err)
	}
	platformCtx := auth.WithContext(context.Background(), &auth.Context{Authenticated: true, Realm: auth.RealmPlatform, IsSystemOperator: true, UserID: "test"})
	sdb, err := tenant.SystemDB(platformCtx, db)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	if err = sdb.WithContext(platformCtx).Create(&mcpserver.Client{ID: "client-a", Name: "Codex", RedirectURIs: json.RawMessage(`[]`)}).Error; err != nil {
		t.Fatal(err)
	}
	if err = sdb.WithContext(platformCtx).Create(&mcpserver.Grant{ID: "grant-a", TenantID: "tenant-a", UserID: "user-a", ClientID: "client-a", Scopes: "actions:read actions:sensitive", AuthVersion: 1, Status: "active", ExpiresAt: now.Add(time.Hour)}).Error; err != nil {
		t.Fatal(err)
	}
	principal := &mcpserver.Principal{Auth: &auth.Context{Authenticated: true, Realm: auth.RealmTenant, TenantID: "tenant-a", UserID: "user-a", Client: "mcp", MCPClientID: "client-a", MCPGrantID: "grant-a"}, Scopes: map[string]bool{mcpserver.ScopeSensitive: true}}
	definition := aitool.Definition{RuntimeRef: "demo.destroy", TitleForHuman: "删除演示记录", DescriptionForHuman: "删除指定演示记录", Risk: aitool.RiskHigh, SideEffect: aitool.SideEffectDestructive, InputSchema: json.RawMessage(`{"type":"object"}`)}
	pending, err := service.CreateConfirmation(context.Background(), principal, definition, "ainp_demo_destroy", json.RawMessage(`{"id":"one"}`))
	if err != nil || !strings.HasPrefix(pending.ConfirmationURL, "https://ainp.example/mcp/confirm?token=") {
		t.Fatalf("create confirmation: %#v %v", pending, err)
	}
	if err = service.ConsumeConfirmation(context.Background(), principal, pending.RecoveryToken, definition, "ainp_demo_destroy", json.RawMessage(`{"id":"one"}`)); err == nil {
		t.Fatal("pending confirmation was consumed")
	}
	web := &auth.Context{Authenticated: true, Realm: auth.RealmTenant, TenantID: "tenant-a", UserID: "user-a"}
	if _, err = service.DecideConfirmation(context.Background(), web, pending.RecoveryToken, "approve"); err != nil {
		t.Fatal(err)
	}
	if err = service.ConsumeConfirmation(context.Background(), principal, pending.RecoveryToken, definition, "ainp_demo_destroy", json.RawMessage(`{"id":"changed"}`)); err == nil {
		t.Fatal("changed arguments consumed confirmation")
	}
	if err = service.ConsumeConfirmation(context.Background(), principal, pending.RecoveryToken, definition, "ainp_demo_destroy", json.RawMessage(`{"id":"one"}`)); err != nil {
		t.Fatal(err)
	}
	if err = service.ConsumeConfirmation(context.Background(), principal, pending.RecoveryToken, definition, "ainp_demo_destroy", json.RawMessage(`{"id":"one"}`)); err == nil {
		t.Fatal("confirmation replay was accepted")
	}

	pending, err = service.CreateConfirmation(context.Background(), principal, definition, "ainp_demo_destroy", json.RawMessage(`{"id":"two"}`))
	if err != nil {
		t.Fatal(err)
	}
	if err = sdb.WithContext(platformCtx).Create(&mcpserver.TokenFamily{ID: "family-a", GrantID: "grant-a", RefreshTokenHash: "refresh-a", ExpiresAt: now.Add(time.Hour)}).Error; err != nil {
		t.Fatal(err)
	}
	if err = service.RevokeOwnedGrants(context.Background(), web, mcpserver.GrantRevokeRequest{ClientID: "client-a"}); err != nil {
		t.Fatal(err)
	}
	var grant mcpserver.Grant
	var family mcpserver.TokenFamily
	var confirmation mcpserver.Confirmation
	if err = sdb.WithContext(platformCtx).First(&grant, "id = ?", "grant-a").Error; err != nil {
		t.Fatal(err)
	}
	if err = sdb.WithContext(platformCtx).First(&family, "id = ?", "family-a").Error; err != nil {
		t.Fatal(err)
	}
	if err = sdb.WithContext(platformCtx).Where("grant_id = ? AND status = ?", "grant-a", "revoked").Order("created_at DESC").First(&confirmation).Error; err != nil {
		t.Fatal(err)
	}
	if grant.Status != "revoked" || grant.RevokedAt == nil || family.RevokedAt == nil || confirmation.Status != "revoked" {
		t.Fatalf("grant revocation did not cascade: grant=%#v family=%#v confirmation=%#v", grant, family, confirmation)
	}
}

func TestWebTokenRejectsMCPAudience(t *testing.T) {
	web, _ := auth.NewTokenService("test-secret", "ainp-test", time.Minute)
	now := time.Now().UTC()
	claims := auth.TokenClaims{TenantID: "tenant-a", UserID: "user-a", SessionID: "grant-a", Realm: auth.RealmTenant, AuthVersion: 1, RegisteredClaims: jwt.RegisteredClaims{Issuer: "ainp-test", Subject: "user-a", Audience: jwt.ClaimStrings{mcpserver.Audience}, IssuedAt: jwt.NewNumericDate(now), ExpiresAt: jwt.NewNumericDate(now.Add(time.Minute))}}
	raw, _ := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte("test-secret"))
	if _, err := web.Parse(raw); err == nil {
		t.Fatal("tenant-console parser accepted MCP audience")
	}
}
