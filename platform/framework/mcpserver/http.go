package mcpserver

import (
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/yzxingtu2026/ainp/platform/framework/action"
	"github.com/yzxingtu2026/ainp/platform/framework/auth"
	"github.com/yzxingtu2026/ainp/platform/framework/authentication"
	"github.com/yzxingtu2026/ainp/platform/framework/errs"
	"github.com/yzxingtu2026/ainp/platform/framework/response"
)

type HTTPServer struct {
	service    *Service
	handler    http.Handler
	baseURL    string
	webBaseURL string
	limiter    authentication.RateLimiter
}

func NewHTTPServer(service *Service, actions *action.Registry, baseURL, webBaseURL string) *HTTPServer {
	service.SetWebBaseURL(webBaseURL)
	return &HTTPServer{service: service, baseURL: strings.TrimRight(baseURL, "/"), webBaseURL: strings.TrimRight(webBaseURL, "/"), limiter: authentication.NewMemoryRateLimiter(30, time.Minute), handler: mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return NewActionServer(actions, service) }, &mcp.StreamableHTTPOptions{})}
}

func (h *HTTPServer) Register(r *gin.Engine) {
	r.GET("/.well-known/oauth-protected-resource", h.resourceMetadata)
	r.GET("/.well-known/oauth-authorization-server", h.serverMetadata)
	r.POST("/oauth/register", h.registerClient)
	r.GET("/api/v1/mcp/oauth/authorize", h.authorizeInfo)
	r.POST("/api/v1/mcp/oauth/authorize", h.authorize)
	r.POST("/oauth/token", h.token)
	r.POST("/oauth/revoke", h.revoke)
	r.GET("/api/v1/mcp/confirmations/info", h.confirmationInfo)
	r.POST("/api/v1/mcp/confirmations/decision", h.confirmationDecision)
	r.GET("/api/v1/mcp/grants", h.grants)
	r.POST("/api/v1/mcp/grants/revoke", h.revokeGrant)
	r.Any("/mcp", h.mcp)
}

func (h *HTTPServer) confirmationInfo(c *gin.Context) {
	view, err := h.service.ConfirmationInfo(c.Request.Context(), auth.MustFromContext(c.Request.Context()), c.Query("token"))
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, view)
}

func (h *HTTPServer) confirmationDecision(c *gin.Context) {
	var req struct {
		Token    string `json:"token"`
		Decision string `json:"decision"`
	}
	if c.ShouldBindJSON(&req) != nil {
		response.Fail(c, errs.New(errs.CodeValidation, "invalid confirmation request"))
		return
	}
	view, err := h.service.DecideConfirmation(c.Request.Context(), auth.MustFromContext(c.Request.Context()), req.Token, req.Decision)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, view)
}

func (h *HTTPServer) grants(c *gin.Context) {
	items, err := h.service.ListGrants(c.Request.Context(), auth.MustFromContext(c.Request.Context()))
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, items)
}

func (h *HTTPServer) revokeGrant(c *gin.Context) {
	var req GrantRevokeRequest
	if c.ShouldBindJSON(&req) != nil {
		response.Fail(c, errs.New(errs.CodeValidation, "invalid grant revoke request"))
		return
	}
	if err := h.service.RevokeOwnedGrants(c.Request.Context(), auth.MustFromContext(c.Request.Context()), req); err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, gin.H{"status": "revoked"})
}

func (h *HTTPServer) allow(c *gin.Context, operation string) bool {
	if h.limiter != nil && !h.limiter.Allow(c.Request.Context(), operation+":"+c.ClientIP()) {
		c.Header("Retry-After", "60")
		c.JSON(http.StatusTooManyRequests, gin.H{"error": "temporarily_unavailable"})
		return false
	}
	return true
}

func (h *HTTPServer) resourceMetadata(c *gin.Context) {
	c.JSON(200, gin.H{"resource": h.baseURL + "/mcp", "authorization_servers": []string{h.baseURL}, "scopes_supported": []string{ScopeRead, ScopeWrite, ScopeSensitive}, "bearer_methods_supported": []string{"header"}})
}
func (h *HTTPServer) serverMetadata(c *gin.Context) {
	c.JSON(200, gin.H{"issuer": h.baseURL, "authorization_endpoint": h.webBaseURL + "/oauth/authorize", "token_endpoint": h.baseURL + "/oauth/token", "registration_endpoint": h.baseURL + "/oauth/register", "revocation_endpoint": h.baseURL + "/oauth/revoke", "response_types_supported": []string{"code"}, "grant_types_supported": []string{"authorization_code", "refresh_token"}, "code_challenge_methods_supported": []string{"S256"}, "token_endpoint_auth_methods_supported": []string{"none"}, "scopes_supported": []string{ScopeRead, ScopeWrite, ScopeSensitive}})
}
func (h *HTTPServer) registerClient(c *gin.Context) {
	if !h.allow(c, "register") {
		return
	}
	var req struct {
		Name         string   `json:"client_name"`
		RedirectURIs []string `json:"redirect_uris"`
	}
	if c.ShouldBindJSON(&req) != nil {
		c.JSON(400, gin.H{"error": "invalid_client_metadata"})
		return
	}
	row, err := h.service.RegisterClient(c, req.Name, req.RedirectURIs)
	if err != nil {
		c.JSON(400, gin.H{"error": "invalid_client_metadata"})
		return
	}
	c.JSON(201, gin.H{"client_id": row.ID, "client_name": row.Name, "redirect_uris": req.RedirectURIs, "token_endpoint_auth_method": "none"})
}
func (h *HTTPServer) authorizeInfo(c *gin.Context) {
	ctx := c.Request.Context()
	ac := auth.MustFromContext(ctx)
	if !ac.Authenticated {
		c.JSON(401, gin.H{"error": "login_required"})
		return
	}
	client, err := h.service.Client(ctx, c.Query("client_id"), c.Query("redirect_uri"))
	if err != nil || c.Query("response_type") != "code" || c.Query("code_challenge_method") != "S256" {
		c.JSON(400, gin.H{"error": "invalid_request"})
		return
	}
	c.JSON(200, gin.H{"client_id": client.ID, "client_name": client.Name, "redirect_uri": c.Query("redirect_uri"), "requested_scope": strings.Fields(c.Query("scope")), "account_id": ac.PrincipalID, "tenant_id": ac.TenantID})
}
func (h *HTTPServer) authorize(c *gin.Context) {
	if !h.allow(c, "authorize") {
		return
	}
	ctx := c.Request.Context()
	ac := auth.MustFromContext(ctx)
	if !ac.Authenticated {
		c.JSON(401, gin.H{"error": "login_required"})
		return
	}
	var req struct {
		ClientID            string `json:"client_id"`
		RedirectURI         string `json:"redirect_uri"`
		Scope               string `json:"scope"`
		CodeChallenge       string `json:"code_challenge"`
		CodeChallengeMethod string `json:"code_challenge_method"`
	}
	if c.ShouldBindJSON(&req) != nil {
		c.JSON(400, gin.H{"error": "invalid_request"})
		return
	}
	code, err := h.service.Authorize(ctx, ac, req.ClientID, req.RedirectURI, req.Scope, req.CodeChallenge, req.CodeChallengeMethod)
	if err != nil {
		c.JSON(400, gin.H{"error": "invalid_request"})
		return
	}
	c.JSON(200, gin.H{"code": code})
}
func (h *HTTPServer) token(c *gin.Context) {
	if !h.allow(c, "token") {
		return
	}
	if err := c.Request.ParseForm(); err != nil {
		c.JSON(400, gin.H{"error": "invalid_request"})
		return
	}
	var out *TokenResponse
	var err error
	switch c.PostForm("grant_type") {
	case "authorization_code":
		out, err = h.service.Exchange(c.Request.Context(), c.PostForm("code"), c.PostForm("client_id"), c.PostForm("redirect_uri"), c.PostForm("code_verifier"))
	case "refresh_token":
		out, err = h.service.Refresh(c.Request.Context(), c.PostForm("refresh_token"), c.PostForm("client_id"))
	default:
		c.JSON(400, gin.H{"error": "unsupported_grant_type"})
		return
	}
	if err != nil {
		c.JSON(400, gin.H{"error": "invalid_grant"})
		return
	}
	c.JSON(200, out)
}
func (h *HTTPServer) revoke(c *gin.Context) {
	if !h.allow(c, "revoke") {
		return
	}
	_ = c.Request.ParseForm()
	_ = h.service.Revoke(c.Request.Context(), c.PostForm("token"))
	c.Status(http.StatusOK)
}
func (h *HTTPServer) mcp(c *gin.Context) {
	base, _ := url.Parse(h.baseURL)
	if origin := c.GetHeader("Origin"); origin != "" && origin != base.Scheme+"://"+base.Host {
		c.Status(http.StatusForbidden)
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 1<<20)
	raw := strings.TrimSpace(strings.TrimPrefix(c.GetHeader("Authorization"), "Bearer "))
	principal, err := h.service.Authenticate(c.Request.Context(), raw)
	if err != nil {
		c.Header("WWW-Authenticate", `Bearer resource_metadata="`+h.baseURL+`/.well-known/oauth-protected-resource"`)
		c.Status(401)
		return
	}
	ctx := auth.WithContext(c.Request.Context(), principal.Auth)
	ctx = withPrincipal(ctx, principal)
	h.handler.ServeHTTP(c.Writer, c.Request.WithContext(ctx))
}
