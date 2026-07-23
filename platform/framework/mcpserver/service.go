package mcpserver

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/yzxingtu2026/ainp/platform/framework/audit"
	"github.com/yzxingtu2026/ainp/platform/framework/auth"
	"github.com/yzxingtu2026/ainp/platform/framework/tenant"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var ErrInvalidGrant = errors.New("invalid OAuth grant")

type accessClaims struct {
	TenantID, UserID, GrantID, ClientID string
	AuthVersion                         uint64
	Scopes                              string
	jwt.RegisteredClaims
}

type Service struct {
	db            *gorm.DB
	authorization auth.AuthorizationProvider
	secret        []byte
	issuer        string
	baseURL       string
	accessTTL     time.Duration
	refreshTTL    time.Duration
	webBaseURL    string
	auditor       audit.Recorder
}

func NewService(db *gorm.DB, authorization auth.AuthorizationProvider, secret, issuer, baseURL string) *Service {
	registerModels()
	return &Service{db: db, authorization: authorization, secret: []byte(secret), issuer: issuer, baseURL: strings.TrimRight(baseURL, "/"), accessTTL: 15 * time.Minute, refreshTTL: 30 * 24 * time.Hour}
}

func (s *Service) SetWebBaseURL(value string)      { s.webBaseURL = strings.TrimRight(value, "/") }
func (s *Service) SetAuditor(value audit.Recorder) { s.auditor = value }

func (s *Service) systemDB(ctx context.Context) (*gorm.DB, error) {
	systemCtx := auth.WithContext(ctx, &auth.Context{Authenticated: true, Realm: auth.RealmPlatform, IsSystemOperator: true, UserID: "system:mcp-oauth", Client: "mcp-oauth"})
	db, err := tenant.SystemDB(systemCtx, s.db)
	if err != nil {
		return nil, err
	}
	return db.WithContext(systemCtx), nil
}

func AutoMigrate(db *gorm.DB) error {
	return db.AutoMigrate(&Client{}, &Grant{}, &AuthorizationCode{}, &TokenFamily{}, &Confirmation{})
}

func randomToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
func hash(v string) string { sum := sha256.Sum256([]byte(v)); return hex.EncodeToString(sum[:]) }

func validateRedirect(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || u.Fragment != "" || u.Scheme == "" {
		return ErrInvalidGrant
	}
	if u.Scheme != "https" && !(u.Scheme == "http" && (u.Hostname() == "127.0.0.1" || u.Hostname() == "localhost")) {
		return ErrInvalidGrant
	}
	return nil
}

func (s *Service) RegisterClient(ctx context.Context, name string, redirectURIs []string) (*Client, error) {
	name = strings.TrimSpace(name)
	if name == "" || len(redirectURIs) == 0 || len(redirectURIs) > 10 {
		return nil, ErrInvalidGrant
	}
	seen := map[string]bool{}
	for _, uri := range redirectURIs {
		if validateRedirect(uri) != nil || seen[uri] {
			return nil, ErrInvalidGrant
		}
		seen[uri] = true
	}
	raw, _ := json.Marshal(redirectURIs)
	row := &Client{ID: uuid.NewString(), Name: name, RedirectURIs: raw}
	db, err := s.systemDB(ctx)
	if err != nil {
		return nil, err
	}
	return row, db.Create(row).Error
}

func (s *Service) Client(ctx context.Context, id, redirectURI string) (*Client, error) {
	var row Client
	db, err := s.systemDB(ctx)
	if err != nil {
		return nil, err
	}
	if err := db.Where("id = ?", id).First(&row).Error; err != nil {
		return nil, ErrInvalidGrant
	}
	var uris []string
	_ = json.Unmarshal(row.RedirectURIs, &uris)
	for _, candidate := range uris {
		if candidate == redirectURI {
			return &row, nil
		}
	}
	return nil, ErrInvalidGrant
}

func normalizeScopes(raw string) (string, error) {
	allowed := map[string]bool{ScopeRead: true, ScopeWrite: true, ScopeSensitive: true}
	set := map[string]bool{}
	for _, scope := range strings.Fields(raw) {
		if !allowed[scope] {
			return "", ErrInvalidGrant
		}
		set[scope] = true
	}
	if !set[ScopeRead] {
		return "", ErrInvalidGrant
	}
	items := make([]string, 0, len(set))
	for scope := range set {
		items = append(items, scope)
	}
	sort.Strings(items)
	return strings.Join(items, " "), nil
}

func (s *Service) Authorize(ctx context.Context, ac *auth.Context, clientID, redirectURI, scope, challenge, method string) (string, error) {
	if ac == nil || !ac.Authenticated || ac.Realm != auth.RealmTenant || method != "S256" || len(challenge) < 43 || len(challenge) > 128 {
		return "", ErrInvalidGrant
	}
	if _, err := s.Client(ctx, clientID, redirectURI); err != nil {
		return "", err
	}
	scopes, err := normalizeScopes(scope)
	if err != nil {
		return "", err
	}
	code, err := randomToken()
	if err != nil {
		return "", err
	}
	now := time.Now().UTC()
	grant := Grant{ID: uuid.NewString(), TenantID: ac.TenantID, UserID: ac.UserID, ClientID: clientID, Scopes: scopes, AuthVersion: ac.AuthVersion, Status: "active", ExpiresAt: now.Add(30 * 24 * time.Hour)}
	codeRow := AuthorizationCode{ID: uuid.NewString(), CodeHash: hash(code), GrantID: grant.ID, ClientID: clientID, RedirectURI: redirectURI, CodeChallenge: challenge, ExpiresAt: now.Add(5 * time.Minute)}
	db, err := s.systemDB(ctx)
	if err != nil {
		return "", err
	}
	err = db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&grant).Error; err != nil {
			return err
		}
		return tx.Create(&codeRow).Error
	})
	return code, err
}

type TokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
	Scope        string `json:"scope"`
}

func verifyPKCE(verifier, challenge string) bool {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:]) == challenge
}

func (s *Service) Exchange(ctx context.Context, code, clientID, redirectURI, verifier string) (*TokenResponse, error) {
	var result *TokenResponse
	db, err := s.systemDB(ctx)
	if err != nil {
		return nil, err
	}
	err = db.Transaction(func(tx *gorm.DB) error {
		var row AuthorizationCode
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("code_hash = ?", hash(code)).First(&row).Error; err != nil || row.ConsumedAt != nil || time.Now().UTC().After(row.ExpiresAt) || row.ClientID != clientID || row.RedirectURI != redirectURI || !verifyPKCE(verifier, row.CodeChallenge) {
			return ErrInvalidGrant
		}
		var grant Grant
		if err := tx.Where("id = ?", row.GrantID).First(&grant).Error; err != nil {
			return ErrInvalidGrant
		}
		now := time.Now().UTC()
		row.ConsumedAt = &now
		if err := tx.Save(&row).Error; err != nil {
			return err
		}
		refresh, err := randomToken()
		if err != nil {
			return err
		}
		family := TokenFamily{ID: uuid.NewString(), GrantID: grant.ID, ExpiresAt: now.Add(s.refreshTTL)}
		refresh = family.ID + "." + refresh
		family.RefreshTokenHash = hash(refresh)
		if err := tx.Create(&family).Error; err != nil {
			return err
		}
		access, err := s.issueAccess(grant)
		if err != nil {
			return err
		}
		result = &TokenResponse{AccessToken: access, RefreshToken: refresh, TokenType: "Bearer", ExpiresIn: int(s.accessTTL.Seconds()), Scope: grant.Scopes}
		return nil
	})
	return result, err
}

func splitFamilyToken(raw string) (string, string, bool) {
	id, secret, ok := strings.Cut(raw, ".")
	return id, secret, ok && id != "" && secret != ""
}

func (s *Service) Refresh(ctx context.Context, raw, clientID string) (*TokenResponse, error) {
	familyID, _, ok := splitFamilyToken(raw)
	if !ok {
		return nil, ErrInvalidGrant
	}
	var result *TokenResponse
	replayed := false
	db, err := s.systemDB(ctx)
	if err != nil {
		return nil, err
	}
	err = db.Transaction(func(tx *gorm.DB) error {
		var family TokenFamily
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", familyID).First(&family).Error; err != nil || family.RevokedAt != nil || time.Now().UTC().After(family.ExpiresAt) {
			return ErrInvalidGrant
		}
		provided := hash(raw)
		if family.PreviousRefreshTokenHash == provided {
			now := time.Now().UTC()
			family.RevokedAt = &now
			if err := tx.Save(&family).Error; err != nil {
				return err
			}
			if err := tx.Model(&Grant{}).Where("id = ?", family.GrantID).Updates(map[string]any{"status": "revoked", "revoked_at": now}).Error; err != nil {
				return err
			}
			replayed = true
			return nil
		}
		if family.RefreshTokenHash != provided {
			return ErrInvalidGrant
		}
		var grant Grant
		if err := tx.Where("id = ? AND client_id = ? AND status = 'active' AND revoked_at IS NULL AND expires_at > ?", family.GrantID, clientID, time.Now().UTC()).First(&grant).Error; err != nil {
			return ErrInvalidGrant
		}
		snapshot, err := s.authorization.LoadAuthorization(ctx, auth.AuthorizationRequest{TenantID: grant.TenantID, UserID: grant.UserID, Realm: auth.RealmTenant, AuthVersion: grant.AuthVersion})
		if err != nil || snapshot == nil || snapshot.TenantID != grant.TenantID || snapshot.UserID != grant.UserID || snapshot.AuthVersion != grant.AuthVersion {
			return ErrInvalidGrant
		}
		secret, err := randomToken()
		if err != nil {
			return err
		}
		next := family.ID + "." + secret
		family.PreviousRefreshTokenHash = family.RefreshTokenHash
		family.RefreshTokenHash = hash(next)
		if err := tx.Save(&family).Error; err != nil {
			return err
		}
		access, err := s.issueAccess(grant)
		if err != nil {
			return err
		}
		result = &TokenResponse{AccessToken: access, RefreshToken: next, TokenType: "Bearer", ExpiresIn: int(s.accessTTL.Seconds()), Scope: grant.Scopes}
		return nil
	})
	if err == nil && replayed {
		return nil, ErrInvalidGrant
	}
	return result, err
}

func (s *Service) Revoke(ctx context.Context, raw string) error {
	now := time.Now().UTC()
	if familyID, _, ok := splitFamilyToken(raw); ok {
		db, err := s.systemDB(ctx)
		if err != nil {
			return err
		}
		return db.Transaction(func(tx *gorm.DB) error {
			var family TokenFamily
			if err := tx.Where("id = ?", familyID).First(&family).Error; err != nil {
				return nil
			}
			if err := tx.Model(&family).Updates(map[string]any{"revoked_at": now}).Error; err != nil {
				return err
			}
			return tx.Model(&Grant{}).Where("id = ?", family.GrantID).Updates(map[string]any{"status": "revoked", "revoked_at": now}).Error
		})
	}
	parsed, err := jwt.ParseWithClaims(raw, &accessClaims{}, func(t *jwt.Token) (any, error) { return s.secret, nil }, jwt.WithIssuer(s.issuer), jwt.WithAudience(Audience))
	if err != nil {
		return nil
	}
	claims, ok := parsed.Claims.(*accessClaims)
	if !ok {
		return nil
	}
	db, err := s.systemDB(ctx)
	if err != nil {
		return err
	}
	return db.Model(&Grant{}).Where("id = ? AND client_id = ?", claims.GrantID, claims.ClientID).Updates(map[string]any{"status": "revoked", "revoked_at": now}).Error
}

func (s *Service) issueAccess(grant Grant) (string, error) {
	now := time.Now().UTC()
	claims := accessClaims{TenantID: grant.TenantID, UserID: grant.UserID, GrantID: grant.ID, ClientID: grant.ClientID, AuthVersion: grant.AuthVersion, Scopes: grant.Scopes, RegisteredClaims: jwt.RegisteredClaims{Issuer: s.issuer, Subject: grant.UserID, Audience: jwt.ClaimStrings{Audience}, IssuedAt: jwt.NewNumericDate(now), ExpiresAt: jwt.NewNumericDate(now.Add(s.accessTTL))}}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(s.secret)
}

func (s *Service) parseAccess(raw string) (*accessClaims, error) {
	token, err := jwt.ParseWithClaims(raw, &accessClaims{}, func(t *jwt.Token) (any, error) {
		if t.Method != jwt.SigningMethodHS256 {
			return nil, ErrInvalidGrant
		}
		return s.secret, nil
	}, jwt.WithIssuer(s.issuer), jwt.WithAudience(Audience))
	if err != nil || !token.Valid {
		return nil, ErrInvalidGrant
	}
	claims, ok := token.Claims.(*accessClaims)
	if !ok || claims.TenantID == "" || claims.UserID == "" || claims.GrantID == "" || claims.ClientID == "" || claims.AuthVersion == 0 {
		return nil, ErrInvalidGrant
	}
	return claims, nil
}

type Principal struct {
	Auth   *auth.Context
	Scopes map[string]bool
}

func (s *Service) Authenticate(ctx context.Context, raw string) (*Principal, error) {
	requestContext := auth.MustFromContext(ctx)
	claims, err := s.parseAccess(raw)
	if err != nil {
		return nil, err
	}
	var grant Grant
	db, err := s.systemDB(ctx)
	if err != nil {
		return nil, err
	}
	if err := db.Where("id = ? AND status = 'active' AND revoked_at IS NULL AND expires_at > ?", claims.GrantID, time.Now().UTC()).First(&grant).Error; err != nil || grant.TenantID != claims.TenantID || grant.UserID != claims.UserID || grant.ClientID != claims.ClientID || grant.AuthVersion != claims.AuthVersion || grant.Scopes != claims.Scopes {
		return nil, ErrInvalidGrant
	}
	snapshot, err := s.authorization.LoadAuthorization(ctx, auth.AuthorizationRequest{TenantID: claims.TenantID, UserID: claims.UserID, Realm: auth.RealmTenant, AuthVersion: claims.AuthVersion})
	if err != nil || snapshot == nil || snapshot.TenantID != claims.TenantID || snapshot.UserID != claims.UserID || snapshot.AuthVersion != claims.AuthVersion {
		return nil, ErrInvalidGrant
	}
	now := time.Now().UTC()
	_ = db.Model(&Grant{}).Where("id = ?", grant.ID).Update("last_used_at", now).Error
	scopes := make(map[string]bool)
	for _, scope := range strings.Fields(grant.Scopes) {
		scopes[scope] = true
	}
	return &Principal{
		Auth:   &auth.Context{RequestID: requestContext.RequestID, TraceID: requestContext.TraceID, Language: requestContext.Language, TenantID: snapshot.TenantID, UserID: snapshot.UserID, Realm: auth.RealmTenant, AuthVersion: snapshot.AuthVersion, Roles: snapshot.Roles, Permissions: snapshot.Permissions, OrgID: snapshot.OrgID, DeptID: snapshot.DeptID, SessionID: grant.ID, Client: "mcp", MCPClientID: grant.ClientID, MCPGrantID: grant.ID, Authenticated: true},
		Scopes: scopes,
	}, nil
}
