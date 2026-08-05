package tests

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/yzxingtu2026/ainp/platform/framework/auth"
	"github.com/yzxingtu2026/ainp/platform/framework/middleware"
)

type fakeAuthorizationProvider struct {
	snapshot *auth.AuthorizationSnapshot
	err      error
	request  auth.AuthorizationRequest
}

func (f *fakeAuthorizationProvider) LoadAuthorization(_ context.Context, req auth.AuthorizationRequest) (*auth.AuthorizationSnapshot, error) {
	f.request = req
	return f.snapshot, f.err
}

func TestAuthMiddlewareUsesCurrentAuthorizationSnapshot(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tokens, err := auth.NewTokenService("test-secret", "ainp-test", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	token, _, err := tokens.Issue("tenant-1", "user-1", "session-1", 7)
	if err != nil {
		t.Fatal(err)
	}
	provider := &fakeAuthorizationProvider{snapshot: &auth.AuthorizationSnapshot{
		TenantID: "tenant-1", PrincipalID: "user-1", SessionID: "session-1", AuthVersion: 7,
		Roles: []string{"current-role"}, Permissions: []string{"current.permission"},
	}}
	router := gin.New()
	router.Use(middleware.RequestID(), middleware.Auth(middleware.AuthOptions{Tokens: tokens, Authorization: provider}))
	router.GET("/protected", func(c *gin.Context) {
		ac := auth.MustFromContext(c.Request.Context())
		if !ac.Authenticated || !ac.HasPermission("current.permission") {
			c.Status(http.StatusUnauthorized)
			return
		}
		c.Status(http.StatusNoContent)
	})

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)
	if resp.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", resp.Code, http.StatusNoContent)
	}
	if provider.request.TenantID != "tenant-1" || provider.request.PrincipalID != "user-1" || provider.request.SessionID != "session-1" || provider.request.AuthVersion != 7 {
		t.Fatalf("unexpected provider request: %#v", provider.request)
	}
}

func TestAuthMiddlewareFailsClosedWhenAuthorizationIsStale(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tokens, _ := auth.NewTokenService("test-secret", "ainp-test", time.Hour)
	token, _, _ := tokens.Issue("tenant-1", "user-1", "session-1", 1)
	provider := &fakeAuthorizationProvider{err: auth.ErrAuthorizationStale}
	router := gin.New()
	router.Use(middleware.RequestID(), middleware.Auth(middleware.AuthOptions{Tokens: tokens, Authorization: provider}))
	router.GET("/protected", func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)
	if resp.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", resp.Code, http.StatusUnauthorized)
	}
}

func TestAuthMiddlewareRejectsMismatchedAuthorizationSnapshot(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tokens, _ := auth.NewTokenService("test-secret", "ainp-test", time.Hour)
	token, _, _ := tokens.Issue("tenant-1", "user-1", "session-1", 1)
	provider := &fakeAuthorizationProvider{snapshot: &auth.AuthorizationSnapshot{
		TenantID: "tenant-2", PrincipalID: "user-1", SessionID: "session-1", AuthVersion: 1, Permissions: []string{"*"},
	}}
	router := gin.New()
	router.Use(middleware.RequestID(), middleware.Auth(middleware.AuthOptions{Tokens: tokens, Authorization: provider}))
	router.GET("/protected", func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)
	if resp.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", resp.Code, http.StatusUnauthorized)
	}
}
