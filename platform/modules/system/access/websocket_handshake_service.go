package access

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/yzxingtu2026/ainp/platform/framework/auth"
	"github.com/yzxingtu2026/ainp/platform/framework/errs"
	"github.com/yzxingtu2026/ainp/platform/modules/system/access/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const webSocketHandshakeGrantTTL = 30 * time.Second

type WebSocketHandshakeService struct {
	db            *gorm.DB
	authorization auth.AuthorizationProvider
	surfaces      *auth.ClientSurfaceRegistry
	targets       *auth.WebSocketHandshakeTargetRegistry
	now           func() time.Time
	cleanupMu     sync.Mutex
	nextCleanup   time.Time
}

func NewWebSocketHandshakeService(db *gorm.DB, authorization auth.AuthorizationProvider, surfaces *auth.ClientSurfaceRegistry, targets *auth.WebSocketHandshakeTargetRegistry) *WebSocketHandshakeService {
	return &WebSocketHandshakeService{db: db, authorization: authorization, surfaces: surfaces, targets: targets, now: func() time.Time { return time.Now().UTC() }}
}

func (s *WebSocketHandshakeService) Issue(ctx context.Context, targetPath string) (*auth.WebSocketHandshakeGrant, error) {
	ac, ok := auth.FromContext(ctx)
	if !ok || ac == nil || !ac.Authenticated || ac.TenantID == "" || !ac.Principal.Valid() || ac.SessionID == "" || ac.AuthVersion == 0 {
		return nil, errs.NewKey(errs.CodeUnauthorized, "common.unauthorized", "unauthorized")
	}
	target, surface, ok := s.resolveTarget(targetPath)
	if !ok || surface.ID != ac.ClientSurface || surface.Audience != ac.Audience || !surface.AllowsKind(ac.Principal.Kind) {
		return nil, errs.NewKey(errs.CodeValidation, "auth.websocket_target_invalid", "websocket handshake target is not allowed")
	}
	grant, id, hash, err := newWebSocketHandshakeGrant()
	if err != nil {
		return nil, err
	}
	now := s.now()
	row := model.WebSocketHandshakeGrant{
		ID: id, SecretHash: hash, TenantID: ac.TenantID, PrincipalKind: string(ac.Principal.Kind), PrincipalID: ac.Principal.ID,
		SessionID: ac.SessionID, Realm: string(auth.NormalizeRealm(ac.Realm)), ClientSurface: ac.ClientSurface, Audience: ac.Audience,
		TargetID: target.ID, TargetPath: target.Path, AuthVersion: ac.AuthVersion, ExpiresAt: now.Add(webSocketHandshakeGrantTTL), CreatedAt: now,
	}
	if err := s.db.WithContext(ctx).Create(&row).Error; err != nil {
		return nil, err
	}
	s.cleanup(ctx, now)
	return &auth.WebSocketHandshakeGrant{Grant: grant, ExpiresAt: row.ExpiresAt, Protocols: auth.WebSocketHandshakeProtocols(grant)}, nil
}

func (s *WebSocketHandshakeService) cleanup(ctx context.Context, now time.Time) {
	s.cleanupMu.Lock()
	if now.Before(s.nextCleanup) {
		s.cleanupMu.Unlock()
		return
	}
	s.nextCleanup = now.Add(time.Hour)
	s.cleanupMu.Unlock()
	cutoff := now.Add(-time.Hour)
	_ = s.db.WithContext(ctx).
		Where("expires_at < ? OR consumed_at < ?", cutoff, cutoff).
		Delete(&model.WebSocketHandshakeGrant{}).Error
}

func (s *WebSocketHandshakeService) Consume(ctx context.Context, request auth.WebSocketHandshakeConsumeRequest) (*auth.Context, error) {
	id, hash, ok := parseWebSocketHandshakeGrant(request.Grant)
	if !ok {
		return nil, auth.ErrWebSocketHandshakeGrantInvalid
	}
	target, surface, ok := s.resolveTarget(request.TargetPath)
	if !ok {
		return nil, auth.ErrWebSocketHandshakeGrantInvalid
	}
	now := s.now()
	row := model.WebSocketHandshakeGrant{}
	result := s.db.WithContext(ctx).Clauses(clause.Returning{}).Model(&row).
		Where("id = ? AND secret_hash = ? AND target_id = ? AND target_path = ? AND client_surface = ? AND audience = ? AND consumed_at IS NULL AND expires_at > ?",
			id, hash, target.ID, target.Path, surface.ID, surface.Audience, now).
		Update("consumed_at", now)
	if result.Error != nil || result.RowsAffected != 1 {
		return nil, auth.ErrWebSocketHandshakeGrantInvalid
	}
	principal := auth.PrincipalRef{Kind: auth.PrincipalKind(row.PrincipalKind), ID: row.PrincipalID}
	if row.TargetID != target.ID || row.TargetPath != target.Path || row.ClientSurface != surface.ID || row.Audience != surface.Audience ||
		!principal.Valid() || !surface.AllowsKind(principal.Kind) || row.Realm != string(auth.RealmTenant) || row.AuthVersion == 0 {
		return nil, auth.ErrWebSocketHandshakeGrantInvalid
	}
	snapshot, err := s.authorization.LoadAuthorization(ctx, auth.AuthorizationRequest{
		TenantID: row.TenantID, Principal: principal, PrincipalID: principal.ID, SessionID: row.SessionID,
		Realm: auth.Realm(row.Realm), AuthVersion: row.AuthVersion, Audience: row.Audience, ClientSurface: row.ClientSurface,
	})
	if err != nil || snapshot == nil || snapshot.TenantID != row.TenantID || snapshot.PrincipalID != row.PrincipalID || snapshot.SessionID != row.SessionID ||
		snapshot.AuthVersion != row.AuthVersion || (snapshot.Principal.Valid() && snapshot.Principal != principal) ||
		(snapshot.Audience != "" && snapshot.Audience != row.Audience) || (snapshot.ClientSurface != "" && snapshot.ClientSurface != row.ClientSurface) {
		return nil, auth.ErrWebSocketHandshakeGrantInvalid
	}
	existing, _ := auth.FromContext(ctx)
	ac := &auth.Context{}
	if existing != nil {
		*ac = *existing
	}
	ac.TenantID, ac.Principal, ac.PrincipalID = snapshot.TenantID, principal, snapshot.PrincipalID
	ac.SessionID, ac.AuthVersion, ac.Realm = snapshot.SessionID, snapshot.AuthVersion, auth.Realm(row.Realm)
	ac.ClientSurface, ac.Audience = row.ClientSurface, row.Audience
	ac.OrgID, ac.DeptID = snapshot.OrgID, snapshot.DeptID
	ac.Roles, ac.Permissions = append([]string(nil), snapshot.Roles...), append([]string(nil), snapshot.Permissions...)
	ac.Client, ac.Authenticated = "websocket", true
	return ac, nil
}

func (s *WebSocketHandshakeService) resolveTarget(path string) (auth.WebSocketHandshakeTarget, auth.ClientSurface, bool) {
	if s == nil || s.db == nil || s.authorization == nil || s.surfaces == nil || s.targets == nil {
		return auth.WebSocketHandshakeTarget{}, auth.ClientSurface{}, false
	}
	target, ok := s.targets.ResolvePath(strings.TrimSpace(path))
	if !ok {
		return auth.WebSocketHandshakeTarget{}, auth.ClientSurface{}, false
	}
	surface, ok := s.surfaces.ResolvePath(target.Path)
	return target, surface, ok
}

func newWebSocketHandshakeGrant() (token, id, hash string, err error) {
	secret := make([]byte, 32)
	if _, err = rand.Read(secret); err != nil {
		return "", "", "", err
	}
	id = uuid.NewString()
	token = id + "." + base64.RawURLEncoding.EncodeToString(secret)
	sum := sha256.Sum256([]byte(token))
	return token, id, hex.EncodeToString(sum[:]), nil
}

func parseWebSocketHandshakeGrant(token string) (id, hash string, ok bool) {
	id, secret, ok := strings.Cut(strings.TrimSpace(token), ".")
	if !ok || id == "" || secret == "" {
		return "", "", false
	}
	if _, err := uuid.Parse(id); err != nil {
		return "", "", false
	}
	decoded, err := base64.RawURLEncoding.DecodeString(secret)
	if err != nil || len(decoded) != 32 {
		return "", "", false
	}
	sum := sha256.Sum256([]byte(token))
	return id, hex.EncodeToString(sum[:]), true
}

var _ auth.WebSocketHandshakeGrantService = (*WebSocketHandshakeService)(nil)
