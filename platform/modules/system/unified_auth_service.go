package system

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/yzxingtu2026/ainp/platform/framework/auth"
	"github.com/yzxingtu2026/ainp/platform/framework/authentication"
	"github.com/yzxingtu2026/ainp/platform/framework/errs"
	"github.com/yzxingtu2026/ainp/platform/framework/status"
	"github.com/yzxingtu2026/ainp/platform/framework/tenant"
	"github.com/yzxingtu2026/ainp/platform/modules/system/model"
	"gorm.io/gorm"
)

type AuthenticateRequest struct {
	Method        string          `json:"method" validate:"required,max=64"`
	ChallengeID   string          `json:"challenge_id" validate:"max=64"`
	Client        string          `json:"client" validate:"max=64"`
	DeviceSummary string          `json:"device_summary" validate:"max=255"`
	Credentials   json.RawMessage `json:"credentials" validate:"required"`
}

type ChallengeRequest struct {
	Method  string          `json:"method" validate:"required,max=64"`
	Client  string          `json:"client" validate:"max=64"`
	Payload json.RawMessage `json:"payload"`
}

type RefreshRequest struct {
	RefreshToken string `json:"refresh_token" validate:"required"`
}

type LegacyLoginRequest struct {
	Username string `json:"username" validate:"required,max=64"`
	Password string `json:"password" validate:"required,max=72"`
}

type UnifiedAuthService struct {
	db       *gorm.DB
	registry *authentication.Registry
	auth     *AuthService
	sessions *SessionService
	limiter  authentication.RateLimiter
	preAuth  *preAuthStore
}

func NewUnifiedAuthService(db *gorm.DB, registry *authentication.Registry, authService *AuthService, sessions *SessionService, limiter authentication.RateLimiter) *UnifiedAuthService {
	return &UnifiedAuthService{db: db, registry: registry, auth: authService, sessions: sessions, limiter: limiter, preAuth: newPreAuthStore(db)}
}

func (s *UnifiedAuthService) allow(ctx context.Context, key string) error {
	if s.limiter != nil && !s.limiter.Allow(ctx, key) {
		return errs.NewKey(errs.CodeTooManyRequests, "common.too_many_requests", "too many requests")
	}
	return nil
}

func authenticationRateKey(method, client string, payload json.RawMessage) string {
	discriminator := payload
	if method == passwordMethod {
		var credentials passwordCredentials
		if json.Unmarshal(payload, &credentials) == nil {
			discriminator = []byte(strings.ToLower(strings.TrimSpace(credentials.Username)))
		}
	}
	sum := sha256.Sum256(discriminator)
	return method + ":" + normalizeClient(client) + ":" + hex.EncodeToString(sum[:8])
}

func (s *UnifiedAuthService) Methods() any { return map[string]any{"methods": s.registry.Methods()} }

func (s *UnifiedAuthService) Begin(ctx context.Context, req ChallengeRequest) (any, error) {
	if err := s.allow(ctx, "challenge:"+authenticationRateKey(req.Method, req.Client, req.Payload)); err != nil {
		return nil, err
	}
	provider, ok := s.registry.Get(req.Method)
	if !ok {
		return nil, errs.NewKey(errs.CodeNotFound, "auth.method_unavailable", "authentication method unavailable")
	}
	if !provider.Capabilities().Challenge {
		return nil, errs.NewKey(errs.CodeValidation, "auth.challenge_not_supported", "authentication method does not use challenge")
	}
	return provider.Begin(ctx, authentication.BeginRequest{Client: req.Client, Payload: req.Payload})
}

func (s *UnifiedAuthService) Authenticate(ctx context.Context, req AuthenticateRequest) (any, error) {
	if err := s.allow(ctx, "authenticate:"+authenticationRateKey(req.Method, req.Client, req.Credentials)); err != nil {
		return nil, err
	}
	provider, ok := s.registry.Get(req.Method)
	if !ok {
		return nil, errs.NewKey(errs.CodeNotFound, "auth.method_unavailable", "authentication method unavailable")
	}
	verified, err := provider.Complete(ctx, authentication.CompleteRequest{ChallengeID: req.ChallengeID, Client: req.Client, Payload: req.Credentials})
	if err != nil {
		return nil, err
	}
	user, err := s.resolveIdentity(ctx, verified)
	if err != nil {
		return nil, err
	}
	profile, err := s.auth.BuildProfile(ctx, user)
	if err != nil {
		return nil, err
	}
	tokens, err := s.sessions.Create(ctx, user, req.Client, req.DeviceSummary)
	if err != nil {
		return nil, errs.Wrap(errs.CodeInternal, "create session failed", err)
	}
	if ac := auth.MustFromContext(ctx); ac != nil {
		ac.TenantID, ac.UserID, ac.SessionID, ac.Realm = user.TenantID, user.ID, tokens.SessionID, auth.RealmTenant
		ac.Roles, ac.Permissions = append([]string(nil), profile.Roles...), append([]string(nil), profile.Permissions...)
	}
	return map[string]any{
		"access_token": tokens.AccessToken, "refresh_token": tokens.RefreshToken, "token_type": tokens.TokenType,
		"expires_in": tokens.ExpiresIn, "expires_at": tokens.ExpiresAt.UTC().Format(time.RFC3339), "session_id": tokens.SessionID, "user": profile,
	}, nil
}

func (s *UnifiedAuthService) LegacyLogin(ctx context.Context, req LegacyLoginRequest) (any, error) {
	payload, _ := json.Marshal(passwordCredentials{Username: req.Username, Password: req.Password})
	return s.Authenticate(ctx, AuthenticateRequest{Method: passwordMethod, Client: "web", Credentials: payload})
}

func (s *UnifiedAuthService) resolveIdentity(ctx context.Context, identity *authentication.VerifiedIdentity) (*model.User, error) {
	if identity == nil || identity.Provider == "" || identity.Subject == "" || identity.TenantID == "" {
		return nil, errs.NewKey(errs.CodeInvalidCredentials, "auth.invalid_credentials", "invalid credentials")
	}
	var user *model.User
	if identity.Provider == passwordMethod {
		var err error
		if user, err = s.preAuth.findUserByUsername(ctx, identity.TenantID, identity.Subject); err != nil {
			return nil, errs.NewKey(errs.CodeInvalidCredentials, "auth.invalid_credentials", "invalid credentials")
		}
	} else {
		row, err := s.preAuth.findIdentity(ctx, identity.TenantID, identity.Provider, identity.Subject)
		if err != nil {
			return nil, errs.NewKey(errs.CodeInvalidCredentials, "auth.invalid_credentials", "invalid credentials")
		}
		if user, err = s.preAuth.findUserByID(ctx, s.db, row.TenantID, row.UserID); err != nil {
			return nil, errs.NewKey(errs.CodeInvalidCredentials, "auth.invalid_credentials", "invalid credentials")
		}
	}
	if user.Status != status.ActivationEnabled {
		return nil, errs.NewKey(errs.CodeUserDisabled, "auth.user_disabled", "user disabled")
	}
	tenantRow, err := s.auth.FindTenant(ctx, user.TenantID)
	if err != nil || tenantRow.Status != status.ActivationEnabled {
		return nil, errs.NewKey(errs.CodeTenantDisabled, "auth.tenant_disabled", "tenant disabled")
	}
	row := model.AuthIdentity{
		ID: uuid.NewString(), TenantID: user.TenantID, UserID: user.ID, Provider: identity.Provider,
		Subject: identity.Subject, NormalizedIdentifier: identity.NormalizedIdentifier, Status: status.ActivationEnabled, VerifiedAt: time.Now().UTC(),
	}
	if err := s.preAuth.upsertIdentity(ctx, &row); err != nil {
		return nil, errs.Wrap(errs.CodeInternal, "persist verified identity failed", err)
	}
	return user, nil
}

func (s *UnifiedAuthService) Refresh(ctx context.Context, req RefreshRequest) (any, error) {
	if err := s.allow(ctx, "refresh:"+refreshSessionID(req.RefreshToken)); err != nil {
		return nil, err
	}
	return s.sessions.Refresh(ctx, strings.TrimSpace(req.RefreshToken))
}

type SessionView struct {
	ID            string    `json:"id"`
	Client        string    `json:"client"`
	DeviceSummary string    `json:"device_summary,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
	LastSeenAt    time.Time `json:"last_seen_at"`
	ExpiresAt     time.Time `json:"expires_at"`
	IsCurrent     bool      `json:"is_current"`
}

func (s *UnifiedAuthService) ListSessions(ctx context.Context) (any, error) {
	ac := auth.MustFromContext(ctx)
	tdb, err := tenant.TenantDB(ctx, s.db)
	if err != nil {
		return nil, err
	}
	rows, err := tenant.TenantRaw[model.AuthSession](tdb, `SELECT * FROM auth_sessions
WHERE tenant_id = ? AND user_id = ? AND revoked_at IS NULL AND expires_at > ?
ORDER BY last_seen_at DESC`, ac.UserID, time.Now().UTC())
	if err != nil {
		return nil, err
	}
	views := make([]SessionView, 0, len(rows))
	for _, row := range rows {
		views = append(views, SessionView{ID: row.ID, Client: row.Client, DeviceSummary: row.DeviceSummary, CreatedAt: row.CreatedAt, LastSeenAt: row.LastSeenAt, ExpiresAt: row.ExpiresAt, IsCurrent: row.ID == ac.SessionID})
	}
	return map[string]any{"sessions": views}, nil
}

func (s *UnifiedAuthService) RevokeSession(ctx context.Context, sessionID string) (any, error) {
	ac := auth.MustFromContext(ctx)
	tdb, err := tenant.TenantDB(ctx, s.db)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	rows, err := tenant.TenantRaw[struct{ ID string }](tdb, `WITH scoped AS (
  SELECT id FROM auth_sessions WHERE tenant_id = ? AND id = ? AND user_id = ? AND revoked_at IS NULL FOR UPDATE
)
UPDATE auth_sessions AS s SET revoked_at = ?, updated_at = ?
FROM scoped WHERE s.id = scoped.id RETURNING s.id`, sessionID, ac.UserID, now, now)
	if err != nil {
		return nil, err
	}
	if len(rows) != 1 {
		return nil, errs.NewKey(errs.CodeNotFound, "common.not_found", "session not found")
	}
	return map[string]any{"revoked": true}, nil
}

func (s *UnifiedAuthService) RevokeCurrent(ctx context.Context) (any, error) {
	ac := auth.MustFromContext(ctx)
	if ac.SessionID == "" {
		return nil, errs.NewKey(errs.CodeUnauthorized, "common.unauthorized", "unauthorized")
	}
	return s.RevokeSession(ctx, ac.SessionID)
}

func (s *UnifiedAuthService) RevokeAll(ctx context.Context) (any, error) {
	ac := auth.MustFromContext(ctx)
	tdb, err := tenant.TenantDB(ctx, s.db)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	rows, err := tenant.TenantRaw[struct{ ID string }](tdb, `WITH scoped AS (
  SELECT id FROM auth_sessions WHERE tenant_id = ? AND user_id = ? AND revoked_at IS NULL FOR UPDATE
)
UPDATE auth_sessions AS s SET revoked_at = ?, updated_at = ?
FROM scoped WHERE s.id = scoped.id RETURNING s.id`, ac.UserID, now, now)
	if err != nil {
		return nil, err
	}
	return map[string]any{"revoked_count": int64(len(rows))}, nil
}
