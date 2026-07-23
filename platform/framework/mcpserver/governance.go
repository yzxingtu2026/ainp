package mcpserver

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/yzxingtu2026/ainp/platform/framework/aitool"
	"github.com/yzxingtu2026/ainp/platform/framework/audit"
	"github.com/yzxingtu2026/ainp/platform/framework/auth"
	"github.com/yzxingtu2026/ainp/platform/framework/errs"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type ConfirmationRequest struct {
	ID              string    `json:"-"`
	ConfirmationURL string    `json:"confirmation_url"`
	RecoveryToken   string    `json:"confirmation_token"`
	ExpiresAt       time.Time `json:"expires_at"`
}

type ConfirmationView struct {
	ID          string     `json:"id"`
	ClientName  string     `json:"client_name"`
	Title       string     `json:"title"`
	Description string     `json:"description"`
	Risk        string     `json:"risk"`
	SideEffect  string     `json:"side_effect"`
	Status      string     `json:"status"`
	ExpiresAt   time.Time  `json:"expires_at"`
	DecidedAt   *time.Time `json:"decided_at,omitempty"`
}

type GrantView struct {
	ID         string     `json:"id"`
	ClientID   string     `json:"client_id"`
	ClientName string     `json:"client_name"`
	Scopes     []string   `json:"scopes"`
	Status     string     `json:"status"`
	ExpiresAt  time.Time  `json:"expires_at"`
	LastUsedAt *time.Time `json:"last_used_at,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
}

type GrantRevokeRequest struct {
	GrantID  string `json:"grant_id"`
	ClientID string `json:"client_id"`
	All      bool   `json:"all"`
}

func canonicalHash(raw json.RawMessage) (string, error) {
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return "", err
	}
	canonical, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(canonical)
	return hex.EncodeToString(sum[:]), nil
}

func definitionHash(definition aitool.Definition) string {
	sum := sha256.Sum256([]byte(definition.RuntimeRef + "\x00" + string(definition.InputSchema) + "\x00" + string(definition.OutputSchema)))
	return hex.EncodeToString(sum[:])
}

func (s *Service) CreateConfirmation(ctx context.Context, principal *Principal, definition aitool.Definition, toolName string, arguments json.RawMessage) (*ConfirmationRequest, error) {
	if principal == nil || principal.Auth == nil || !principal.Scopes[ScopeSensitive] || principal.Auth.MCPGrantID == "" || principal.Auth.MCPClientID == "" {
		return nil, errs.New(errs.CodeForbidden, "tool is unavailable")
	}
	argumentsHash, err := canonicalHash(arguments)
	if err != nil {
		return nil, errs.New(errs.CodeValidation, "invalid tool arguments")
	}
	recovery, err := randomToken()
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	row := Confirmation{
		ID: uuid.NewString(), TenantID: principal.Auth.TenantID, UserID: principal.Auth.UserID,
		GrantID: principal.Auth.MCPGrantID, ClientID: principal.Auth.MCPClientID,
		ActionID: definition.RuntimeRef, ToolName: toolName, SchemaHash: definitionHash(definition), ArgumentsHash: argumentsHash,
		RecoveryHash: hash(recovery), Title: definition.TitleForHuman, Description: definition.DescriptionForHuman,
		Risk: string(definition.Risk), SideEffect: string(definition.SideEffect), Status: "pending", ExpiresAt: now.Add(10 * time.Minute),
	}
	db, err := s.systemDB(ctx)
	if err != nil {
		return nil, err
	}
	if err := db.Create(&row).Error; err != nil {
		return nil, err
	}
	base := s.webBaseURL
	if base == "" {
		base = s.baseURL
	}
	return &ConfirmationRequest{ID: row.ID, ConfirmationURL: base + "/mcp/confirm?token=" + url.QueryEscape(recovery), RecoveryToken: recovery, ExpiresAt: row.ExpiresAt}, nil
}

func (s *Service) ConsumeConfirmation(ctx context.Context, principal *Principal, recovery string, definition aitool.Definition, toolName string, arguments json.RawMessage) error {
	if principal == nil || principal.Auth == nil || !principal.Scopes[ScopeSensitive] {
		return errs.New(errs.CodeForbidden, "confirmation is unavailable")
	}
	argumentsHash, err := canonicalHash(arguments)
	if err != nil {
		return errs.New(errs.CodeValidation, "invalid tool arguments")
	}
	db, err := s.systemDB(ctx)
	if err != nil {
		return err
	}
	err = db.Transaction(func(tx *gorm.DB) error {
		var row Confirmation
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("recovery_hash = ?", hash(recovery)).First(&row).Error; err != nil {
			return errs.New(errs.CodeForbidden, "confirmation is unavailable")
		}
		if row.Status != "approved" || time.Now().UTC().After(row.ExpiresAt) || row.TenantID != principal.Auth.TenantID || row.UserID != principal.Auth.UserID || row.GrantID != principal.Auth.MCPGrantID || row.ClientID != principal.Auth.MCPClientID || row.ActionID != definition.RuntimeRef || row.ToolName != toolName || row.SchemaHash != definitionHash(definition) || row.ArgumentsHash != argumentsHash {
			return errs.New(errs.CodeForbidden, "confirmation is unavailable")
		}
		now := time.Now().UTC()
		return tx.Model(&row).Updates(map[string]any{"status": "consumed", "consumed_at": now}).Error
	})
	return err
}

func (s *Service) ConfirmationInfo(ctx context.Context, ac *auth.Context, recovery string) (*ConfirmationView, error) {
	row, clientName, err := s.ownedConfirmation(ctx, ac, recovery)
	if err != nil {
		return nil, err
	}
	return confirmationView(row, clientName), nil
}

func (s *Service) DecideConfirmation(ctx context.Context, ac *auth.Context, recovery, decision string) (*ConfirmationView, error) {
	if decision != "approve" && decision != "reject" {
		return nil, errs.New(errs.CodeValidation, "invalid confirmation decision")
	}
	row, clientName, err := s.ownedConfirmation(ctx, ac, recovery)
	if err != nil {
		return nil, err
	}
	if row.Status != "pending" || time.Now().UTC().After(row.ExpiresAt) {
		return nil, errs.New(errs.CodeConflict, "confirmation is not pending")
	}
	status := "approved"
	if decision == "reject" {
		status = "rejected"
	}
	now := time.Now().UTC()
	db, err := s.systemDB(ctx)
	if err != nil {
		return nil, err
	}
	result := db.Model(&Confirmation{}).Where("id = ? AND tenant_id = ? AND user_id = ? AND status = 'pending'", row.ID, ac.TenantID, ac.UserID).Updates(map[string]any{"status": status, "decided_at": now})
	if result.Error != nil {
		return nil, result.Error
	}
	if result.RowsAffected != 1 {
		return nil, errs.New(errs.CodeConflict, "confirmation is not pending")
	}
	row.Status, row.DecidedAt = status, &now
	s.writeGovernanceAudit(ctx, ac, row, "mcp.confirmation_"+decision, "success", false)
	return confirmationView(row, clientName), nil
}

func (s *Service) ownedConfirmation(ctx context.Context, ac *auth.Context, recovery string) (*Confirmation, string, error) {
	if ac == nil || !ac.Authenticated || ac.Realm != auth.RealmTenant || strings.TrimSpace(recovery) == "" {
		return nil, "", errs.New(errs.CodeForbidden, "confirmation is unavailable")
	}
	db, err := s.systemDB(ctx)
	if err != nil {
		return nil, "", err
	}
	var row Confirmation
	if err := db.Where("recovery_hash = ? AND tenant_id = ? AND user_id = ?", hash(recovery), ac.TenantID, ac.UserID).First(&row).Error; err != nil {
		return nil, "", errs.New(errs.CodeForbidden, "confirmation is unavailable")
	}
	if row.Status == "pending" && time.Now().UTC().After(row.ExpiresAt) {
		row.Status = "expired"
		_ = db.Model(&row).Update("status", "expired").Error
	}
	var client Client
	_ = db.Where("id = ?", row.ClientID).First(&client).Error
	return &row, client.Name, nil
}

func confirmationView(row *Confirmation, clientName string) *ConfirmationView {
	return &ConfirmationView{ID: row.ID, ClientName: clientName, Title: row.Title, Description: row.Description, Risk: row.Risk, SideEffect: row.SideEffect, Status: row.Status, ExpiresAt: row.ExpiresAt, DecidedAt: row.DecidedAt}
}

func (s *Service) ListGrants(ctx context.Context, ac *auth.Context) ([]GrantView, error) {
	if ac == nil || !ac.Authenticated || ac.Realm != auth.RealmTenant {
		return nil, errs.New(errs.CodeUnauthorized, "unauthorized")
	}
	db, err := s.systemDB(ctx)
	if err != nil {
		return nil, err
	}
	type row struct {
		Grant
		ClientName string `gorm:"column:client_name"`
	}
	var rows []row
	if err := db.Table("mcp_oauth_grants AS g").Select("g.*, c.name AS client_name").Joins("JOIN mcp_oauth_clients AS c ON c.id = g.client_id").Where("g.tenant_id = ? AND g.user_id = ?", ac.TenantID, ac.UserID).Order("g.created_at DESC").Limit(100).Scan(&rows).Error; err != nil {
		return nil, err
	}
	result := make([]GrantView, 0, len(rows))
	for _, item := range rows {
		status := item.Status
		if status == "active" && time.Now().UTC().After(item.ExpiresAt) {
			status = "expired"
		}
		result = append(result, GrantView{ID: item.ID, ClientID: item.ClientID, ClientName: item.ClientName, Scopes: strings.Fields(item.Scopes), Status: status, ExpiresAt: item.ExpiresAt, LastUsedAt: item.LastUsedAt, CreatedAt: item.CreatedAt})
	}
	return result, nil
}

func (s *Service) RevokeOwnedGrants(ctx context.Context, ac *auth.Context, request GrantRevokeRequest) error {
	if ac == nil || !ac.Authenticated || ac.Realm != auth.RealmTenant {
		return errs.New(errs.CodeUnauthorized, "unauthorized")
	}
	if !request.All && strings.TrimSpace(request.GrantID) == "" && strings.TrimSpace(request.ClientID) == "" {
		return errs.New(errs.CodeValidation, "grant_id, client_id or all is required")
	}
	db, err := s.systemDB(ctx)
	if err != nil {
		return err
	}
	changed := false
	err = db.Transaction(func(tx *gorm.DB) error {
		query := tx.Model(&Grant{}).Where("tenant_id = ? AND user_id = ? AND status = 'active'", ac.TenantID, ac.UserID)
		if !request.All {
			if request.GrantID != "" {
				query = query.Where("id = ?", request.GrantID)
			}
			if request.ClientID != "" {
				query = query.Where("client_id = ?", request.ClientID)
			}
		}
		var ids []string
		if err := query.Pluck("id", &ids).Error; err != nil {
			return err
		}
		if len(ids) == 0 {
			return nil
		}
		changed = true
		now := time.Now().UTC()
		if err := tx.Model(&Grant{}).Where("id IN ?", ids).Updates(map[string]any{"status": "revoked", "revoked_at": now}).Error; err != nil {
			return err
		}
		if err := tx.Model(&TokenFamily{}).Where("grant_id IN ? AND revoked_at IS NULL", ids).Update("revoked_at", now).Error; err != nil {
			return err
		}
		return tx.Model(&Confirmation{}).Where("grant_id IN ? AND status IN ?", ids, []string{"pending", "approved"}).Update("status", "revoked").Error
	})
	if err == nil && changed {
		s.writeGovernanceAudit(ctx, ac, nil, "mcp.grant_revoke", "success", false)
	}
	return err
}

func (s *Service) writeGovernanceAudit(ctx context.Context, ac *auth.Context, confirmation *Confirmation, actionID, result string, replay bool) {
	if s.auditor == nil || ac == nil {
		return
	}
	record := audit.Record{OccurredAt: time.Now().UTC(), TenantID: ac.TenantID, Realm: string(auth.RealmTenant), UserID: ac.UserID, ActorType: "user", ActionID: actionID, Result: result, RequestID: ac.RequestID, Client: ac.Client, Replay: replay, Summary: "MCP authorization governance updated"}
	if confirmation != nil {
		record.MCPClientID, record.MCPGrantID = confirmation.ClientID, confirmation.GrantID
	}
	_ = s.auditor.Write(ctx, record)
}
