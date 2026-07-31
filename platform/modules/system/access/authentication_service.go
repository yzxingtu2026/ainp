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
	user, err := p.service.FindUserByUsername(ctx, tenantRow.ID, username)
	if err != nil || bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(credentials.Password)) != nil {
		return nil, errs.NewKey(errs.CodeInvalidCredentials, "auth.invalid_credentials", "invalid username or password")
	}
	return &authentication.VerifiedIdentity{
		Provider: passwordMethod, Subject: user.Username,
		NormalizedIdentifier: strings.ToLower(user.Username), TenantID: user.TenantID,
	}, nil
}

type SessionService struct {
	preAuth    *preAuthStore
	tokens     *auth.TokenService
	auth       *AuthService
	refreshTTL time.Duration
}

func NewSessionService(db *gorm.DB, tokens *auth.TokenService, authService *AuthService, configuredTTL ...time.Duration) *SessionService {
	ttl := refreshTTL
	if len(configuredTTL) > 0 && configuredTTL[0] > 0 {
		ttl = configuredTTL[0]
	}
	return &SessionService{preAuth: newPreAuthStore(db), tokens: tokens, auth: authService, refreshTTL: ttl}
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

func (s *SessionService) Create(ctx context.Context, user *model.User, client, device string) (*SessionTokens, error) {
	now := time.Now().UTC()
	sessionID := uuid.NewString()
	refresh, refreshHash, err := newRefreshToken(sessionID)
	if err != nil {
		return nil, err
	}
	row := model.AuthSession{
		ID: sessionID, TenantID: user.TenantID, UserID: user.ID, Realm: string(auth.RealmTenant),
		Client: normalizeClient(client), DeviceSummary: strings.TrimSpace(device), RefreshTokenHash: refreshHash,
		AuthVersion: user.AuthVersion, ExpiresAt: now.Add(s.refreshTTL), LastSeenAt: now,
	}
	if err := s.preAuth.createSession(ctx, &row); err != nil {
		return nil, err
	}
	access, exp, err := s.tokens.Issue(user.TenantID, user.ID, sessionID, user.AuthVersion)
	if err != nil {
		return nil, err
	}
	return &SessionTokens{AccessToken: access, RefreshToken: refresh, TokenType: "Bearer", ExpiresIn: int(time.Until(exp).Seconds()), ExpiresAt: exp, SessionID: sessionID}, nil
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

func (s *SessionService) Refresh(ctx context.Context, token string) (*SessionTokens, error) {
	sessionID := refreshSessionID(token)
	if sessionID == "" {
		return nil, errs.NewKey(errs.CodeUnauthorized, "auth.session_invalid", "invalid session")
	}
	providedHash := hashRefreshToken(token)
	var output *SessionTokens
	replayed := false
	err := s.preAuth.rotateSession(ctx, sessionID, func(tx *gorm.DB, session *model.AuthSession) error {
		now := time.Now().UTC()
		if session.RevokedAt != nil || !session.ExpiresAt.After(now) {
			return auth.ErrAuthorizationInvalid
		}
		if session.PreviousRefreshTokenHash == providedHash {
			session.RevokedAt = &now
			if err := tx.Model(session).Update("revoked_at", now).Error; err != nil {
				return err
			}
			replayed = true
			return nil
		}
		if session.RefreshTokenHash != providedHash {
			return auth.ErrAuthorizationInvalid
		}
		user, err := s.loadActiveUser(ctx, tx, session.TenantID, session.UserID)
		if err != nil || user.AuthVersion != session.AuthVersion {
			return auth.ErrAuthorizationInvalid
		}
		refresh, refreshHash, err := newRefreshToken(session.ID)
		if err != nil {
			return err
		}
		access, exp, err := s.tokens.Issue(user.TenantID, user.ID, session.ID, user.AuthVersion)
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
	if replayed {
		return nil, errs.NewKey(errs.CodeUnauthorized, "auth.session_invalid", "invalid session")
	}
	return output, nil
}

func (s *SessionService) loadActiveUser(ctx context.Context, db *gorm.DB, tenantID, userID string) (*model.User, error) {
	user, err := s.preAuth.findUserByID(ctx, db, tenantID, userID)
	if err != nil || user.Status != status.ActivationEnabled {
		if err == nil {
			err = auth.ErrAuthorizationInvalid
		}
		return nil, err
	}
	tenantRow, err := s.auth.FindTenant(ctx, tenantID)
	if err != nil || tenantRow.Status != status.ActivationEnabled {
		if err == nil {
			err = auth.ErrAuthorizationInvalid
		}
		return nil, err
	}
	return user, nil
}
