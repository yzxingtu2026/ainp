package access

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/yzxingtu2026/ainp/platform/framework/audit"
	"github.com/yzxingtu2026/ainp/platform/framework/auth"
	"github.com/yzxingtu2026/ainp/platform/framework/authentication"
	"github.com/yzxingtu2026/ainp/platform/framework/errs"
	"github.com/yzxingtu2026/ainp/platform/framework/status"
	"github.com/yzxingtu2026/ainp/platform/modules/system/access/model"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

const (
	passwordMethod = "password"
	refreshTTL     = 30 * 24 * time.Hour

	sessionRevokeIdle     = "idle_expired"
	sessionRevokeAbsolute = "absolute_expired"
	sessionRevokeOverflow = "overflow"
)

type passwordCredentials struct {
	Username string `json:"username" validate:"required,max=64"`
	Password string `json:"password" validate:"required,max=72"`
}

type PasswordAuthenticator struct{ service *AuthService }

func NewPasswordAuthenticator(service *AuthService) *PasswordAuthenticator {
	return &PasswordAuthenticator{service: service}
}

func (*PasswordAuthenticator) Method() string { return passwordMethod }
func (*PasswordAuthenticator) Capabilities() authentication.Capabilities {
	return authentication.Capabilities{LabelKey: "auth.method.password"}
}
func (*PasswordAuthenticator) Begin(context.Context, authentication.BeginRequest) (*authentication.Challenge, error) {
	return nil, errs.NewKey(errs.CodeValidation, "auth.challenge_not_supported", "authentication method does not use challenge")
}
func (p *PasswordAuthenticator) Complete(ctx context.Context, req authentication.CompleteRequest) (*authentication.VerifiedIdentity, error) {
	var credentials passwordCredentials
	if err := json.Unmarshal(req.Payload, &credentials); err != nil || strings.TrimSpace(credentials.Username) == "" || credentials.Password == "" {
		return nil, errs.NewKey(errs.CodeInvalidCredentials, "auth.invalid_credentials", "invalid username or password")
	}
	tenantRow, err := p.service.FindDefaultTenant(ctx)
	if err != nil || tenantRow.Status != status.ActivationEnabled {
		return nil, errs.NewKey(errs.CodeInvalidCredentials, "auth.invalid_credentials", "invalid username or password")
	}
	username := strings.TrimSpace(credentials.Username)
	account, err := p.service.FindAccountByUsername(ctx, tenantRow.ID, username)
	if err != nil || bcrypt.CompareHashAndPassword([]byte(account.PasswordHash), []byte(credentials.Password)) != nil {
		return nil, errs.NewKey(errs.CodeInvalidCredentials, "auth.invalid_credentials", "invalid username or password")
	}
	return &authentication.VerifiedIdentity{
		Provider: passwordMethod, Subject: account.Username,
		NormalizedIdentifier: strings.ToLower(account.Username), TenantID: account.TenantID,
	}, nil
}

type SessionService struct {
	preAuth    *preAuthStore
	tokens     *auth.TokenService
	auth       *AuthService
	principals *auth.PrincipalRegistry
	surfaces   *auth.ClientSurfaceRegistry
	refreshTTL time.Duration
	auditor    audit.Recorder
}

func (s *SessionService) Configure(principals *auth.PrincipalRegistry, surfaces *auth.ClientSurfaceRegistry) {
	s.principals, s.surfaces = principals, surfaces
}

func NewSessionService(db *gorm.DB, tokens *auth.TokenService, authService *AuthService, configuredTTL ...time.Duration) *SessionService {
	ttl := refreshTTL
	if len(configuredTTL) > 0 && configuredTTL[0] > 0 {
		ttl = configuredTTL[0]
	}
	principals := auth.NewPrincipalRegistry()
	if authService != nil {
		_ = principals.Register(authService)
	}
	surfaces := auth.NewClientSurfaceRegistry()
	_ = surfaces.Register(auth.BuiltinTenantConsoleSurface())
	return &SessionService{preAuth: newPreAuthStore(db), tokens: tokens, auth: authService, principals: principals, surfaces: surfaces, refreshTTL: ttl, auditor: audit.NewGormRecorder(db)}
}

type SessionTokens struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
	TokenType    string    `json:"token_type"`
	ExpiresIn    int       `json:"expires_in"`
	ExpiresAt    time.Time `json:"expires_at"`
	SessionID    string    `json:"session_id"`
}

func newRefreshToken(sessionID string) (string, string, error) {
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		return "", "", err
	}
	token := sessionID + "." + base64.RawURLEncoding.EncodeToString(secret)
	return token, hashRefreshToken(token), nil
}

func hashRefreshToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func refreshSessionID(token string) string {
	id, _, ok := strings.Cut(token, ".")
	if !ok {
		return ""
	}
	return id
}

func (s *SessionService) Create(ctx context.Context, account *model.TenantAccount, client, device string) (*SessionTokens, error) {
	if account == nil {
		return nil, auth.ErrAuthorizationInvalid
	}
	snapshot := &auth.AuthorizationSnapshot{TenantID: account.TenantID, Principal: auth.PrincipalRef{Kind: auth.PrincipalKindAccount, ID: account.ID}, PrincipalID: account.ID, AuthVersion: account.AuthVersion}
	return s.CreatePrincipal(ctx, snapshot, auth.BuiltinTenantConsoleSurface(), client, device)
}

func (s *SessionService) CreatePrincipal(ctx context.Context, snapshot *auth.AuthorizationSnapshot, surface auth.ClientSurface, client, device string) (*SessionTokens, error) {
	if snapshot == nil || snapshot.TenantID == "" || !snapshot.Principal.Valid() || snapshot.AuthVersion == 0 || surface.ID == "" || !surface.AllowsKind(snapshot.Principal.Kind) {
		return nil, auth.ErrAuthorizationInvalid
	}
	if s.principals == nil || s.surfaces == nil {
		return nil, auth.ErrAuthorizationInvalid
	}
	registeredSurface, ok := s.surfaces.Get(surface.ID)
	if !ok || registeredSurface.Audience != surface.Audience || !registeredSurface.AllowsKind(snapshot.Principal.Kind) {
		return nil, auth.ErrAuthorizationInvalid
	}
	verified, err := s.principals.LoadAuthorization(ctx, auth.AuthorizationRequest{TenantID: snapshot.TenantID, Principal: snapshot.Principal, PrincipalID: snapshot.Principal.ID, Realm: auth.RealmTenant, AuthVersion: snapshot.AuthVersion, Audience: surface.Audience, ClientSurface: surface.ID})
	if err != nil || verified == nil || verified.TenantID != snapshot.TenantID || verified.Principal != snapshot.Principal || verified.AuthVersion != snapshot.AuthVersion {
		return nil, auth.ErrAuthorizationInvalid
	}
	snapshot = verified
	surface = registeredSurface
	now := time.Now().UTC()
	sessionID := uuid.NewString()
	refresh, refreshHash, err := newRefreshToken(sessionID)
	if err != nil {
		return nil, err
	}
	var accountID *string
	if snapshot.Principal.Kind == auth.PrincipalKindAccount {
		value := snapshot.Principal.ID
		accountID = &value
	}
	ttl := s.refreshTTL
	if surface.AbsoluteTTL > 0 {
		ttl = surface.AbsoluteTTL
	}
	row := model.AuthSession{
		ID: sessionID, TenantID: snapshot.TenantID, PrincipalKind: string(snapshot.Principal.Kind), PrincipalID: snapshot.Principal.ID, AccountID: accountID, Realm: string(auth.RealmTenant),
		Client: normalizeClient(client), ClientSurface: surface.ID, Audience: surface.Audience, TokenFamilyID: sessionID,
		DeviceSummary: strings.TrimSpace(device), RefreshTokenHash: refreshHash,
		AuthVersion: snapshot.AuthVersion, ExpiresAt: now.Add(ttl), LastSeenAt: now,
	}
	if err := s.createSession(ctx, &row, surface); err != nil {
		return nil, err
	}
	access, exp, err := s.tokens.IssueFor(snapshot.TenantID, snapshot.Principal, surface.ID, surface.Audience, sessionID, snapshot.AuthVersion)
	if err != nil {
		return nil, err
	}
	return &SessionTokens{AccessToken: access, RefreshToken: refresh, TokenType: "Bearer", ExpiresIn: int(time.Until(exp).Seconds()), ExpiresAt: exp, SessionID: sessionID}, nil
}

func (s *SessionService) createSession(ctx context.Context, row *model.AuthSession, surface auth.ClientSurface) error {
	if surface.MaxActiveSessions == 0 {
		return s.preAuth.createSession(ctx, row)
	}
	return s.preAuth.createSessionWithPolicy(ctx, row, surface, func(tx *gorm.DB, revoked *model.AuthSession, now time.Time) error {
		return s.writeSessionAudit(ctx, tx, revoked, sessionRevokeOverflow, now)
	})
}

func normalizeClient(client string) string {
	client = strings.TrimSpace(client)
	if client == "" {
		return "web"
	}
	if len(client) > 64 {
		return client[:64]
	}
	return client
}

func sessionPolicyError(reason string) *errs.AppError {
	switch reason {
	case sessionRevokeIdle:
		return errs.NewKey(errs.CodeUnauthorized, "auth.session_idle_expired", "session idle timeout exceeded")
	case sessionRevokeAbsolute:
		return errs.NewKey(errs.CodeUnauthorized, "auth.session_absolute_expired", "session absolute timeout exceeded")
	case sessionRevokeOverflow:
		return errs.NewKey(errs.CodeUnauthorized, "auth.session_overflow_revoked", "session revoked by active session limit")
	default:
		return nil
	}
}

func sessionExpiryReason(session *model.AuthSession, surface auth.ClientSurface, now time.Time) string {
	if surface.AbsoluteTTL > 0 && !session.ExpiresAt.After(now) {
		return sessionRevokeAbsolute
	}
	if surface.IdleTTL > 0 && !session.LastSeenAt.Add(surface.IdleTTL).After(now) {
		return sessionRevokeIdle
	}
	return ""
}

func (s *SessionService) writeSessionAudit(ctx context.Context, tx *gorm.DB, session *model.AuthSession, reason string, now time.Time) error {
	transactional, ok := s.auditor.(audit.TransactionalRecorder)
	if !ok {
		return errors.New("session audit recorder is not transactional")
	}
	ac := auth.MustFromContext(ctx)
	requestID := ac.RequestID
	if requestID == "" {
		requestID = uuid.NewString()
	}
	return transactional.WithDB(tx).Write(ctx, audit.Record{
		OccurredAt: now, TenantID: session.TenantID, Realm: session.Realm,
		ActorID: session.PrincipalID, ActorType: "principal", PrincipalKind: session.PrincipalKind,
		ClientSurface: session.ClientSurface, Audience: session.Audience,
		ActionID: "system.auth_session_policy_revoke", Result: "success", RequestID: requestID,
		ResourceType: "auth_session", ResourceID: session.ID, Client: session.Client, Summary: reason,
	})
}

func (s *SessionService) LoadAuthorization(ctx context.Context, req auth.AuthorizationRequest) (*auth.AuthorizationSnapshot, error) {
	if s.principals == nil || s.surfaces == nil {
		return nil, auth.ErrAuthorizationInvalid
	}
	if req.SessionID == "" {
		return s.principals.LoadAuthorization(ctx, req)
	}
	var policyErr *errs.AppError
	err := s.preAuth.rotateSession(ctx, req.SessionID, func(tx *gorm.DB, session *model.AuthSession) error {
		principal := req.PrincipalRef()
		if session.TenantID != req.TenantID || session.PrincipalKind != string(principal.Kind) || session.PrincipalID != principal.ID ||
			session.AuthVersion != req.AuthVersion || session.ClientSurface != req.ClientSurface || session.Audience != req.Audience {
			return auth.ErrAuthorizationInvalid
		}
		if session.RevokedAt != nil {
			policyErr = sessionPolicyError(session.RevokedReason)
			if policyErr != nil {
				return nil
			}
			return auth.ErrAuthorizationInvalid
		}
		surface, ok := s.surfaces.Get(session.ClientSurface)
		if !ok || !surface.AllowsKind(principal.Kind) || surface.Audience != session.Audience {
			return auth.ErrAuthorizationInvalid
		}
		now := time.Now().UTC()
		if reason := sessionExpiryReason(session, surface, now); reason != "" {
			if err := tx.Model(session).Updates(map[string]any{"revoked_at": now, "revoked_reason": reason, "updated_at": now}).Error; err != nil {
				return err
			}
			if err := s.writeSessionAudit(ctx, tx, session, reason, now); err != nil {
				return err
			}
			policyErr = sessionPolicyError(reason)
			return nil
		}
		if !session.ExpiresAt.After(now) {
			return auth.ErrAuthorizationInvalid
		}
		if surface.IdleTTL > 0 {
			return tx.Model(session).Update("last_seen_at", now).Error
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if policyErr != nil {
		return nil, policyErr
	}
	snapshot, err := s.principals.LoadAuthorization(ctx, req)
	if err != nil || snapshot == nil || snapshot.AuthVersion != req.AuthVersion {
		return nil, auth.ErrAuthorizationInvalid
	}
	return snapshot, nil
}

func (s *SessionService) Refresh(ctx context.Context, token string) (*SessionTokens, error) {
	sessionID := refreshSessionID(token)
	if sessionID == "" {
		return nil, errs.NewKey(errs.CodeUnauthorized, "auth.session_invalid", "invalid session")
	}
	providedHash := hashRefreshToken(token)
	var output *SessionTokens
	replayed := false
	var policyErr *errs.AppError
	err := s.preAuth.rotateSession(ctx, sessionID, func(tx *gorm.DB, session *model.AuthSession) error {
		now := time.Now().UTC()
		if expected := auth.MustFromContext(ctx).ClientSurface; expected != "" && expected != session.ClientSurface {
			return auth.ErrAuthorizationInvalid
		}
		if session.RevokedAt != nil {
			policyErr = sessionPolicyError(session.RevokedReason)
			if policyErr != nil {
				return nil
			}
			return auth.ErrAuthorizationInvalid
		}
		principal := auth.PrincipalRef{Kind: auth.PrincipalKind(session.PrincipalKind), ID: session.PrincipalID}
		if !principal.Valid() || s.principals == nil || s.surfaces == nil {
			return auth.ErrAuthorizationInvalid
		}
		surface, ok := s.surfaces.Get(session.ClientSurface)
		if !ok || surface.Audience != session.Audience || !surface.AllowsKind(principal.Kind) {
			return auth.ErrAuthorizationInvalid
		}
		if reason := sessionExpiryReason(session, surface, now); reason != "" {
			if err := tx.Model(session).Updates(map[string]any{"revoked_at": now, "revoked_reason": reason, "updated_at": now}).Error; err != nil {
				return err
			}
			if err := s.writeSessionAudit(ctx, tx, session, reason, now); err != nil {
				return err
			}
			policyErr = sessionPolicyError(reason)
			return nil
		}
		if !session.ExpiresAt.After(now) {
			return auth.ErrAuthorizationInvalid
		}
		var usedCount int64
		if err := tx.Model(&model.AuthRefreshTokenUse{}).Where("token_hash = ? AND token_family_id = ?", providedHash, session.TokenFamilyID).Count(&usedCount).Error; err != nil {
			return err
		}
		if usedCount == 1 || session.PreviousRefreshTokenHash == providedHash {
			session.RevokedAt = &now
			if err := tx.Model(&model.AuthSession{}).Where("token_family_id = ? AND revoked_at IS NULL", session.TokenFamilyID).Updates(map[string]any{"revoked_at": now, "updated_at": now}).Error; err != nil {
				return err
			}
			replayed = true
			return nil
		}
		if session.RefreshTokenHash != providedHash {
			return auth.ErrAuthorizationInvalid
		}
		if err := tx.Create(&model.AuthRefreshTokenUse{TokenHash: providedHash, SessionID: session.ID, TokenFamilyID: session.TokenFamilyID, UsedAt: now}).Error; err != nil {
			return err
		}
		snapshot, err := s.principals.LoadAuthorization(ctx, auth.AuthorizationRequest{TenantID: session.TenantID, Principal: principal, PrincipalID: principal.ID, SessionID: session.ID, Realm: auth.Realm(session.Realm), AuthVersion: session.AuthVersion, Audience: session.Audience, ClientSurface: session.ClientSurface})
		if err != nil || snapshot == nil || snapshot.AuthVersion != session.AuthVersion {
			return auth.ErrAuthorizationInvalid
		}
		refresh, refreshHash, err := newRefreshToken(session.ID)
		if err != nil {
			return err
		}
		access, exp, err := s.tokens.IssueFor(session.TenantID, principal, session.ClientSurface, session.Audience, session.ID, session.AuthVersion)
		if err != nil {
			return err
		}
		result := tx.Model(session).Where("refresh_token_hash = ? AND revoked_at IS NULL", providedHash).Updates(map[string]any{
			"previous_refresh_token_hash": providedHash, "refresh_token_hash": refreshHash, "last_seen_at": now,
		})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return auth.ErrAuthorizationInvalid
		}
		output = &SessionTokens{AccessToken: access, RefreshToken: refresh, TokenType: "Bearer", ExpiresIn: int(time.Until(exp).Seconds()), ExpiresAt: exp, SessionID: session.ID}
		return nil
	})
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) || errors.Is(err, auth.ErrAuthorizationInvalid) {
			return nil, errs.NewKey(errs.CodeUnauthorized, "auth.session_invalid", "invalid session")
		}
		return nil, err
	}
	if policyErr != nil {
		return nil, policyErr
	}
	if replayed {
		return nil, errs.NewKey(errs.CodeUnauthorized, "auth.session_invalid", "invalid session")
	}
	return output, nil
}
