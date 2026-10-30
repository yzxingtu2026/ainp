package tests

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/yzxingtu2026/ainp/platform/framework/attachment"
	"github.com/yzxingtu2026/ainp/platform/framework/auth"
	"github.com/yzxingtu2026/ainp/platform/framework/errs"
	"github.com/yzxingtu2026/ainp/platform/framework/middleware"
)

type fakeAuthorizationProvider struct {
	snapshot *auth.AuthorizationSnapshot
	err      error
	request  auth.AuthorizationRequest
}

func TestAuthMiddlewareOnlyExposesSessionPolicyErrors(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tokens, _ := auth.NewTokenService("test-secret", "ainp-test", time.Hour)
	token, _, _ := tokens.Issue("tenant-1", "user-1", "session-1", 1)

	for _, test := range []struct {
		name string
		err  error
		key  string
	}{
		{name: "idle", err: errs.NewKey(errs.CodeUnauthorized, "auth.session_idle_expired", "idle"), key: "auth.session_idle_expired"},
		{name: "absolute", err: errs.NewKey(errs.CodeUnauthorized, "auth.session_absolute_expired", "absolute"), key: "auth.session_absolute_expired"},
		{name: "overflow", err: errs.NewKey(errs.CodeUnauthorized, "auth.session_overflow_revoked", "overflow"), key: "auth.session_overflow_revoked"},
		{name: "private provider error", err: errs.NewKey(errs.CodeUnauthorized, "auth.provider_private", "private"), key: "common.unauthorized"},
	} {
		t.Run(test.name, func(t *testing.T) {
			provider := &fakeAuthorizationProvider{err: test.err}
			router := gin.New()
			router.Use(middleware.RequestID(), middleware.Auth(middleware.AuthOptions{Tokens: tokens, Authorization: provider}))
			router.GET("/protected", func(c *gin.Context) { c.Status(http.StatusNoContent) })
			req := httptest.NewRequest(http.MethodGet, "/protected", nil)
			req.Header.Set("Authorization", "Bearer "+token)
			resp := httptest.NewRecorder()
			router.ServeHTTP(resp, req)
			if resp.Code != http.StatusUnauthorized {
				t.Fatalf("status=%d body=%s", resp.Code, resp.Body.String())
			}
			var body struct {
				MessageKey string `json:"message_key"`
			}
			if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			if body.MessageKey != test.key {
				t.Fatalf("message_key=%q want=%q", body.MessageKey, test.key)
			}
		})
	}
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

func TestAuthMiddlewareDefersAttachmentTransferBearerToken(t *testing.T) {
	gin.SetMode(gin.TestMode)
	const (
		secret = "shared-test-secret"
		issuer = "ainp-test"
	)
	sessions, err := auth.NewTokenService(secret, issuer, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	transfers, err := attachment.NewTransferTokenService(secret, issuer)
	if err != nil {
		t.Fatal(err)
	}
	token, _, _, err := transfers.Issue(attachment.IssueTransferInput{
		TenantID:       "tenant-1",
		PrincipalID:    "user-1",
		PrincipalKind:  string(auth.PrincipalKindAccount),
		ClientSurface:  auth.TenantConsoleSurface,
		SourceAudience: auth.TenantAudience,
		AttachmentID:   "attachment-1",
		Op:             attachment.OpUpload,
		Size:           12,
		TTL:            time.Minute,
	})
	if err != nil {
		t.Fatal(err)
	}
	surfaces := auth.NewClientSurfaceRegistry()
	if err := surfaces.Register(auth.BuiltinTenantConsoleSurface()); err != nil {
		t.Fatal(err)
	}
	provider := &fakeAuthorizationProvider{}
	router := gin.New()
	router.Use(middleware.RequestID(), middleware.Auth(middleware.AuthOptions{
		Tokens: sessions, Authorization: provider, ClientSurfaces: surfaces,
	}))
	router.PUT("/api/v1/attachments/transfer/:id", func(c *gin.Context) {
		if ac := auth.MustFromContext(c.Request.Context()); ac.Authenticated {
			t.Fatal("transfer token must not create a web session AuthContext")
		}
		c.Status(http.StatusNoContent)
	})

	req := httptest.NewRequest(http.MethodPut, "/api/v1/attachments/transfer/attachment-1", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)
	if resp.Code != http.StatusNoContent {
		t.Fatalf("status=%d want=%d body=%s", resp.Code, http.StatusNoContent, resp.Body.String())
	}
	if provider.request != (auth.AuthorizationRequest{}) {
		t.Fatalf("attachment token reached session authorization provider: %#v", provider.request)
	}
}

func TestAuthMiddlewareBindsPathSurfaceAudienceAndKind(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tokens, _ := auth.NewTokenService("test-secret", "ainp-test", time.Hour)
	ref := auth.PrincipalRef{Kind: "example.customer", ID: "customer-1"}
	token, _, _ := tokens.IssueFor("tenant-1", ref, "example.mobile", "example:mobile", "session-1", 1)
	surfaces := auth.NewClientSurfaceRegistry()
	if err := surfaces.Register(auth.ClientSurface{ID: "example.mobile", Audience: "example:mobile", PathPrefix: "/mobile/", PrincipalKinds: []auth.PrincipalKind{"example.customer"}, Methods: []string{"example-sso"}}); err != nil {
		t.Fatal(err)
	}
	provider := &fakeAuthorizationProvider{snapshot: &auth.AuthorizationSnapshot{TenantID: "tenant-1", Principal: ref, PrincipalID: ref.ID, SessionID: "session-1", AuthVersion: 1}}
	router := gin.New()
	router.Use(middleware.RequestID(), middleware.Auth(middleware.AuthOptions{Tokens: tokens, Authorization: provider, ClientSurfaces: surfaces, DedicatedBearerPaths: []string{"/mcp"}}))
	router.GET("/mobile/orders", func(c *gin.Context) { c.Status(http.StatusNoContent) })
	router.GET("/mcp", func(c *gin.Context) { c.Status(http.StatusNoContent) })
	router.GET("/mcp/extra", func(c *gin.Context) { c.Status(http.StatusNoContent) })
	router.GET("/unknown", func(c *gin.Context) { c.Status(http.StatusNoContent) })

	valid := httptest.NewRequest(http.MethodGet, "/mobile/orders", nil)
	valid.Header.Set("Authorization", "Bearer "+token)
	validResp := httptest.NewRecorder()
	router.ServeHTTP(validResp, valid)
	if validResp.Code != http.StatusNoContent || provider.request.ClientSurface != "example.mobile" || provider.request.Audience != "example:mobile" || provider.request.Principal != ref {
		t.Fatalf("surface request failed: status=%d req=%#v", validResp.Code, provider.request)
	}

	legacy, _, _ := tokens.Issue("tenant-1", "customer-1", "session-1", 1)
	cross := httptest.NewRequest(http.MethodGet, "/mobile/orders", nil)
	cross.Header.Set("Authorization", "Bearer "+legacy)
	crossResp := httptest.NewRecorder()
	router.ServeHTTP(crossResp, cross)
	if crossResp.Code != http.StatusUnauthorized {
		t.Fatalf("cross-audience status=%d", crossResp.Code)
	}

	unknown := httptest.NewRequest(http.MethodGet, "/unknown", nil)
	unknown.Header.Set("Authorization", "Bearer "+token)
	unknownResp := httptest.NewRecorder()
	router.ServeHTTP(unknownResp, unknown)
	if unknownResp.Code != http.StatusUnauthorized {
		t.Fatalf("unknown path status=%d", unknownResp.Code)
	}

	dedicated := httptest.NewRequest(http.MethodGet, "/mcp", nil)
	dedicated.Header.Set("Authorization", "Bearer "+token)
	dedicatedResp := httptest.NewRecorder()
	router.ServeHTTP(dedicatedResp, dedicated)
	if dedicatedResp.Code != http.StatusNoContent {
		t.Fatalf("dedicated bearer path status=%d", dedicatedResp.Code)
	}

	nested := httptest.NewRequest(http.MethodGet, "/mcp/extra", nil)
	nested.Header.Set("Authorization", "Bearer "+token)
	nestedResp := httptest.NewRecorder()
	router.ServeHTTP(nestedResp, nested)
	if nestedResp.Code != http.StatusUnauthorized {
		t.Fatalf("dedicated bearer path must match exactly: status=%d", nestedResp.Code)
	}
}
