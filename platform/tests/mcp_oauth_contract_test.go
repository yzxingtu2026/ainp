package tests

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/yzxingtu2026/ainp/platform/framework/auth"
	"github.com/yzxingtu2026/ainp/platform/framework/mcpserver"
)

func TestMCPOAuthMetadataAndProtectedTransport(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	service := mcpserver.NewService(nil, nil, "test-secret", "ainp-test", "https://api.ainp.example")
	mcpserver.NewHTTPServer(service, "https://api.ainp.example", "https://ainp.example").Register(router)

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

func TestWebTokenRejectsMCPAudience(t *testing.T) {
	web, _ := auth.NewTokenService("test-secret", "ainp-test", time.Minute)
	now := time.Now().UTC()
	claims := auth.TokenClaims{TenantID: "tenant-a", UserID: "user-a", SessionID: "grant-a", Realm: auth.RealmTenant, AuthVersion: 1, RegisteredClaims: jwt.RegisteredClaims{Issuer: "ainp-test", Subject: "user-a", Audience: jwt.ClaimStrings{mcpserver.Audience}, IssuedAt: jwt.NewNumericDate(now), ExpiresAt: jwt.NewNumericDate(now.Add(time.Minute))}}
	raw, _ := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte("test-secret"))
	if _, err := web.Parse(raw); err == nil {
		t.Fatal("tenant-console parser accepted MCP audience")
	}
}
