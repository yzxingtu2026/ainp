package tests

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/yzxingtu2026/ainp/platform/framework/action"
	"github.com/yzxingtu2026/ainp/platform/framework/aitool"
	"github.com/yzxingtu2026/ainp/platform/framework/auth"
	"github.com/yzxingtu2026/ainp/platform/framework/httpserver"
	"github.com/yzxingtu2026/ainp/platform/framework/mcpserver"
	"github.com/yzxingtu2026/ainp/platform/framework/permission"
	"github.com/yzxingtu2026/ainp/platform/framework/tenant"
)

type mcpAuthorizationProvider struct {
	disabled    bool
	permissions []string
}

func (p *mcpAuthorizationProvider) LoadAuthorization(_ context.Context, req auth.AuthorizationRequest) (*auth.AuthorizationSnapshot, error) {
	if p.disabled {
		return nil, auth.ErrAuthorizationStale
	}
	principal := req.PrincipalRef()
	return &auth.AuthorizationSnapshot{TenantID: req.TenantID, Principal: principal, PrincipalID: principal.ID, SessionID: req.SessionID, AuthVersion: req.AuthVersion, Audience: req.Audience, ClientSurface: req.ClientSurface, Permissions: append([]string(nil), p.permissions...)}, nil
}

type bearerRoundTripper struct {
	token string
	base  http.RoundTripper
}

func (t bearerRoundTripper) RoundTrip(request *http.Request) (*http.Response, error) {
	clone := request.Clone(request.Context())
	clone.Header.Set("Authorization", "Bearer "+t.token)
	return t.base.RoundTrip(clone)
}

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

func TestMCPTransportBearerTokenReachesDedicatedAuthenticator(t *testing.T) {
	db := openOutboxTestDB(t)
	provider := &mcpAuthorizationProvider{permissions: []string{"demo.read"}}
	service := mcpserver.NewService(db, provider, "test-secret", "ainp-test", "https://api.ainp.example")
	if err := mcpserver.AutoMigrate(db); err != nil {
		t.Fatal(err)
	}

	registry := action.NewRegistry(permission.NewChecker(), nil)
	registerMCPTestAction(t, registry, "demo.read", action.AccessPermission, "demo.read", "read", "low", nil)
	client, err := service.RegisterClient(context.Background(), "Codex", []string{"http://127.0.0.1/callback"})
	if err != nil {
		t.Fatal(err)
	}
	verifier := strings.Repeat("v", 43)
	sum := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(sum[:])
	web := &auth.Context{Authenticated: true, Realm: auth.RealmTenant, TenantID: "tenant-a", PrincipalID: "account-a", Principal: auth.PrincipalRef{Kind: auth.PrincipalKindAccount, ID: "account-a"}, ClientSurface: auth.TenantConsoleSurface, Audience: auth.TenantAudience, AuthVersion: 7}
	code, err := service.Authorize(context.Background(), web, client.ID, "http://127.0.0.1/callback", mcpserver.ScopeRead, challenge, "S256")
	if err != nil {
		t.Fatal(err)
	}
	tokens, err := service.Exchange(context.Background(), code, client.ID, "http://127.0.0.1/callback", verifier)
	if err != nil {
		t.Fatal(err)
	}

	webTokens, err := auth.NewTokenService("test-secret", "ainp-test", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	surfaces := auth.NewClientSurfaceRegistry()
	if err = surfaces.Register(auth.BuiltinTenantConsoleSurface()); err != nil {
		t.Fatal(err)
	}
	installed := true
	mcpHTTP := mcpserver.NewHTTPServer(service, registry, "https://api.ainp.example", "https://ainp.example")
	router := httpserver.New(httpserver.Options{Mode: "test", Installed: &installed, Tokens: webTokens, Authorization: provider, ClientSurfaces: surfaces, Actions: registry, DB: db, OnHTTP: mcpHTTP.Register})
	testServer := httptest.NewServer(router)
	defer testServer.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	httpClient := &http.Client{Transport: bearerRoundTripper{token: tokens.AccessToken, base: http.DefaultTransport}}
	transport := &mcp.StreamableClientTransport{Endpoint: testServer.URL + "/mcp", HTTPClient: httpClient, DisableStandaloneSSE: true}
	session, err := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "1"}, nil).Connect(ctx, transport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	result, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, tool := range result.Tools {
		if tool.Name == mcpserver.MCPToolName("demo.read") {
			return
		}
	}
	t.Fatalf("MCP tools/list did not expose demo.read: %#v", result.Tools)
}

func TestMCPSensitiveConfirmationIsBoundAndSingleUse(t *testing.T) {
	db := openOutboxTestDB(t)
	service := mcpserver.NewService(db, nil, "test-secret", "ainp-test", "https://api.ainp.example")
	service.SetWebBaseURL("https://ainp.example")
	if err := mcpserver.AutoMigrate(db); err != nil {
		t.Fatal(err)
	}
	platformCtx := auth.WithContext(context.Background(), &auth.Context{Authenticated: true, Realm: auth.RealmPlatform, IsSystemOperator: true, PrincipalID: "test"})
	sdb, err := tenant.SystemDB(platformCtx, db)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	if err = sdb.WithContext(platformCtx).Create(&mcpserver.Client{ID: "client-a", Name: "Codex", RedirectURIs: json.RawMessage(`[]`)}).Error; err != nil {
		t.Fatal(err)
	}
	if err = sdb.WithContext(platformCtx).Create(&mcpserver.Grant{ID: "grant-a", TenantID: "tenant-a", AccountID: "user-a", ClientID: "client-a", Scopes: "actions:read actions:sensitive", AuthVersion: 1, Status: "active", ExpiresAt: now.Add(time.Hour)}).Error; err != nil {
		t.Fatal(err)
	}
	principal := &mcpserver.Principal{Auth: &auth.Context{Authenticated: true, Realm: auth.RealmTenant, TenantID: "tenant-a", PrincipalID: "user-a", Client: "mcp", MCPClientID: "client-a", MCPGrantID: "grant-a"}, Scopes: map[string]bool{mcpserver.ScopeSensitive: true}}
	definition := aitool.Definition{RuntimeRef: "demo.destroy", TitleForHuman: "删除演示记录", DescriptionForHuman: "删除指定演示记录", Risk: aitool.RiskHigh, SideEffect: aitool.SideEffectDestructive, InputSchema: json.RawMessage(`{"type":"object"}`)}
	pending, err := service.CreateConfirmation(context.Background(), principal, definition, "ainp_demo_destroy", json.RawMessage(`{"id":"one"}`))
	if err != nil || !strings.HasPrefix(pending.ConfirmationURL, "https://ainp.example/mcp/confirm?token=") {
		t.Fatalf("create confirmation: %#v %v", pending, err)
	}
	if err = service.ConsumeConfirmation(context.Background(), principal, pending.RecoveryToken, definition, "ainp_demo_destroy", json.RawMessage(`{"id":"one"}`)); err == nil {
		t.Fatal("pending confirmation was consumed")
	}
	web := &auth.Context{Authenticated: true, Realm: auth.RealmTenant, TenantID: "tenant-a", PrincipalID: "user-a"}
	if _, err = service.DecideConfirmation(context.Background(), web, pending.RecoveryToken, "approve"); err != nil {
		t.Fatal(err)
	}
	impostor := &auth.Context{Authenticated: true, Realm: auth.RealmTenant, TenantID: "tenant-a", PrincipalID: "user-a", Principal: auth.PrincipalRef{Kind: "example.customer", ID: "user-a"}, ClientSurface: "example.customer-client", Audience: "example:customer"}
	if _, err = service.ConfirmationInfo(context.Background(), impostor, pending.RecoveryToken); err == nil {
		t.Fatal("same principal ID from another kind/surface accessed MCP confirmation")
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

	pending, err = service.CreateConfirmation(context.Background(), principal, definition, "ainp_demo_destroy", json.RawMessage(`{"id":"two","nested":{"enabled":true}}`))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = service.DecideConfirmation(context.Background(), web, pending.RecoveryToken, "approve"); err != nil {
		t.Fatal(err)
	}
	stored, err := service.ConsumeStoredConfirmation(context.Background(), principal, pending.ID, definition, "ainp_demo_destroy")
	if err != nil || string(stored) != `{"id":"two","nested":{"enabled":true}}` {
		t.Fatalf("stored confirmation arguments were not recovered: %s %v", stored, err)
	}
	var consumed mcpserver.Confirmation
	if err = sdb.WithContext(platformCtx).First(&consumed, "id = ?", pending.ID).Error; err != nil || consumed.ArgumentsCiphertext != "" {
		t.Fatalf("consumed confirmation retained encrypted arguments: %#v %v", consumed, err)
	}
	if _, err = service.ConsumeStoredConfirmation(context.Background(), principal, pending.ID, definition, "ainp_demo_destroy"); err == nil {
		t.Fatal("stored confirmation replay was accepted")
	}

	registry := action.NewRegistry(permission.NewChecker(), nil)
	var calls int
	registerMCPTestAction(t, registry, "demo.resume", action.AccessPermission, "demo.resume", "destructive", "high", func(ctx context.Context, _ json.RawMessage) (any, error) {
		calls++
		if auth.MustFromContext(ctx).IdempotencyKey == "" {
			t.Fatal("confirmation execution did not set an idempotency key")
		}
		return map[string]any{"deleted": true}, nil
	})
	definitions, err := (&aitool.ActionProvider{Actions: registry}).List(context.Background())
	if err != nil || len(definitions) != 1 {
		t.Fatalf("project resume action: %#v %v", definitions, err)
	}
	principal.Auth.Permissions = []string{"demo.resume"}
	resumePending, err := service.CreateConfirmation(context.Background(), principal, definitions[0], mcpserver.MCPToolName("demo.resume"), json.RawMessage(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = service.DecideConfirmation(context.Background(), web, resumePending.RecoveryToken, "approve"); err != nil {
		t.Fatal(err)
	}
	ctx := auth.WithContext(context.Background(), principal.Auth)
	ctx = mcpserver.ContextWithPrincipalForTest(ctx, principal.Scopes)
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	serverSession, err := mcpserver.NewActionServer(registry, service).Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer serverSession.Close()
	clientSession, err := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "1"}, nil).Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer clientSession.Close()
	result, err := clientSession.CallTool(ctx, &mcp.CallToolParams{Name: "ainp_confirmation_execute", Arguments: map[string]any{"confirmation_id": resumePending.ID}})
	if err != nil || result.IsError || calls != 1 {
		t.Fatalf("confirmed action was not executed: result=%#v err=%v calls=%d", result, err, calls)
	}
	result, err = clientSession.CallTool(ctx, &mcp.CallToolParams{Name: "ainp_confirmation_execute", Arguments: map[string]any{"confirmation_id": resumePending.ID}})
	if err != nil || !result.IsError || calls != 1 {
		t.Fatalf("confirmed action replay was accepted: result=%#v err=%v calls=%d", result, err, calls)
	}

	pending, err = service.CreateConfirmation(context.Background(), principal, definition, "ainp_demo_destroy", json.RawMessage(`{"id":"three"}`))
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
	claims := auth.TokenClaims{TenantID: "tenant-a", LegacyUserID: "user-a", SessionID: "grant-a", Realm: auth.RealmTenant, AuthVersion: 1, RegisteredClaims: jwt.RegisteredClaims{Issuer: "ainp-test", Subject: "user-a", Audience: jwt.ClaimStrings{mcpserver.Audience}, IssuedAt: jwt.NewNumericDate(now), ExpiresAt: jwt.NewNumericDate(now.Add(time.Minute))}}
	raw, _ := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte("test-secret"))
	if _, err := web.Parse(raw); err == nil {
		t.Fatal("tenant-console parser accepted MCP audience")
	}
}

func TestMCPAuthorizeRejectsNonAccountOrNonTenantSurface(t *testing.T) {
	service := mcpserver.NewService(nil, nil, "test-secret", "ainp-test", "https://api.ainp.example")
	for _, ac := range []*auth.Context{
		{Authenticated: true, Realm: auth.RealmTenant, TenantID: "tenant-a", Principal: auth.PrincipalRef{Kind: "example.customer", ID: "same-id"}, PrincipalID: "same-id", ClientSurface: "example.customer-client", Audience: "example:customer"},
		{Authenticated: true, Realm: auth.RealmTenant, TenantID: "tenant-a", Principal: auth.PrincipalRef{Kind: auth.PrincipalKindAccount, ID: "account-a"}, PrincipalID: "account-a", ClientSurface: "example.staff-client", Audience: "example:staff"},
	} {
		if _, err := service.Authorize(context.Background(), ac, "client-a", "https://client.example/cb", "actions:read", strings.Repeat("a", 43), "S256"); err == nil {
			t.Fatal("MCP authorize accepted an unsupported principal or surface")
		}
	}
}

func TestMCPExchangeRechecksGrantAndLiveAuthorization(t *testing.T) {
	db := openOutboxTestDB(t)
	provider := &mcpAuthorizationProvider{}
	service := mcpserver.NewService(db, provider, "test-secret", "ainp-test", "https://api.ainp.example")
	if err := mcpserver.AutoMigrate(db); err != nil {
		t.Fatal(err)
	}
	client, err := service.RegisterClient(context.Background(), "Codex", []string{"https://client.example/cb"})
	if err != nil {
		t.Fatal(err)
	}
	verifier := strings.Repeat("v", 43)
	sum := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(sum[:])
	web := &auth.Context{Authenticated: true, Realm: auth.RealmTenant, TenantID: "tenant-a", PrincipalID: "account-a", Principal: auth.PrincipalRef{Kind: auth.PrincipalKindAccount, ID: "account-a"}, ClientSurface: auth.TenantConsoleSurface, Audience: auth.TenantAudience, AuthVersion: 7}
	code, err := service.Authorize(context.Background(), web, client.ID, "https://client.example/cb", mcpserver.ScopeRead, challenge, "S256")
	if err != nil {
		t.Fatal(err)
	}
	provider.disabled = true
	if _, err = service.Exchange(context.Background(), code, client.ID, "https://client.example/cb", verifier); err == nil {
		t.Fatal("MCP Exchange issued tokens after live authorization became stale")
	}
	provider.disabled = false
	if _, err = service.Exchange(context.Background(), code, client.ID, "https://client.example/cb", verifier); err != nil {
		t.Fatalf("failed Exchange consumed the authorization code: %v", err)
	}
}
