package access

import (
	"context"
	"strings"
	"time"

	"github.com/yzxingtu2026/ainp/platform/framework/audit"
	"github.com/yzxingtu2026/ainp/platform/framework/auth"
	"github.com/yzxingtu2026/ainp/platform/framework/platformresource"
	"github.com/yzxingtu2026/ainp/platform/framework/status"
	"github.com/yzxingtu2026/ainp/platform/framework/tenant"
	"gorm.io/gorm"
)

type PrincipalResourceService struct {
	db      *gorm.DB
	auditor audit.Recorder
}

func NewPrincipalResourceService(db *gorm.DB, auditor audit.Recorder) platformresource.PrincipalService {
	return &PrincipalResourceService{db: db, auditor: auditor}
}

type principalResourceRow struct {
	ID          string
	Username    string
	DisplayName string
	Status      status.ActivationStatus
	AuthVersion uint64
}

func (s *PrincipalResourceService) context(ctx context.Context, ref auth.PrincipalRef) (*auth.Context, error) {
	ac := auth.MustFromContext(ctx)
	if !ac.Authenticated || auth.NormalizeRealm(ac.Realm) != auth.RealmTenant || strings.TrimSpace(ac.TenantID) == "" || !ref.Valid() || ref.Kind != auth.PrincipalKindAccount {
		return nil, platformresource.ErrForbidden
	}
	return ac, nil
}

func (s *PrincipalResourceService) load(ctx context.Context, ref auth.PrincipalRef) (*principalResourceRow, error) {
	ac, err := s.context(ctx, ref)
	if err != nil {
		return nil, err
	}
	tdb, err := tenant.TenantDB(ctx, s.db)
	if err != nil {
		return nil, platformresource.ErrUnavailable
	}
	rows, err := tenant.TenantRaw[principalResourceRow](tdb, "SELECT id, username, display_name, status, auth_version FROM tenant_accounts WHERE tenant_id = ? AND id = ? LIMIT 1", ref.ID)
	if err != nil || len(rows) != 1 || ac.TenantID == "" {
		return nil, platformresource.ErrUnavailable
	}
	return &rows[0], nil
}

func (s *PrincipalResourceService) Get(ctx context.Context, ref auth.PrincipalRef) (*platformresource.PrincipalProjection, error) {
	row, err := s.load(ctx, ref)
	if err != nil {
		return nil, err
	}
	return &platformresource.PrincipalProjection{Principal: ref, DisplayName: row.DisplayName, Active: row.Status == status.ActivationEnabled, Revision: row.AuthVersion}, nil
}

func (s *PrincipalResourceService) Invalidate(ctx context.Context, req platformresource.PrincipalInvalidation) (*platformresource.PrincipalProjection, error) {
	ac, err := s.context(ctx, req.Principal)
	if err != nil {
		return nil, err
	}
	permission := req.RequiredPermission
	if permission == "" {
		permission = ActionAccountStatusUpdate
	}
	if req.Principal.ID != ac.PrincipalID && !ac.HasPermission(permission) {
		return nil, platformresource.ErrForbidden
	}
	var row principalResourceRow
	auditWritten := false
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		tdb, scopeErr := tenant.TenantDB(ctx, tx)
		if scopeErr != nil {
			return scopeErr
		}
		rows, queryErr := tenant.TenantRaw[principalResourceRow](tdb, "SELECT id, username, display_name, status, auth_version FROM tenant_accounts WHERE tenant_id = ? AND id = ? FOR UPDATE", req.Principal.ID)
		if queryErr != nil || len(rows) != 1 {
			return platformresource.ErrUnavailable
		}
		row = rows[0]
		if req.ExpectedRevision != 0 && row.AuthVersion != req.ExpectedRevision {
			return platformresource.ErrConflict
		}
		updated, updateErr := tenant.TenantRaw[principalResourceRow](tdb, `WITH scoped AS (
  SELECT id FROM tenant_accounts WHERE tenant_id = ? AND id = ? AND auth_version = ? FOR UPDATE
)
UPDATE tenant_accounts AS a SET auth_version = a.auth_version + 1, updated_at = ?
FROM scoped WHERE a.id = scoped.id
RETURNING a.id, a.username, a.display_name, a.status, a.auth_version`, req.Principal.ID, row.AuthVersion, time.Now().UTC())
		if updateErr != nil || len(updated) != 1 {
			return platformresource.ErrConflict
		}
		row = updated[0]
		if transactional, ok := s.auditor.(audit.TransactionalRecorder); ok {
			if auditErr := transactional.WithDB(tx).Write(ctx, principalInvalidationAudit(ac, req)); auditErr != nil {
				return platformresource.ErrUnavailable
			}
			auditWritten = true
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if s.auditor != nil && !auditWritten {
		if err := s.auditor.Write(ctx, principalInvalidationAudit(ac, req)); err != nil {
			return nil, platformresource.ErrUnavailable
		}
	}
	return &platformresource.PrincipalProjection{Principal: req.Principal, DisplayName: row.DisplayName, Active: row.Status == status.ActivationEnabled, Revision: row.AuthVersion}, nil
}

func principalInvalidationAudit(ac *auth.Context, req platformresource.PrincipalInvalidation) audit.Record {
	return audit.Record{TenantID: ac.TenantID, Realm: string(auth.RealmTenant), ActorID: ac.PrincipalID, ActorType: "principal", PrincipalKind: string(ac.Principal.Kind), ClientSurface: ac.ClientSurface, Audience: ac.Audience, ActionID: "system.platform_resource_principal_invalidate", Result: "success", RequestID: ac.RequestID, Client: ac.Client, ResourceType: "system.principal", ResourceID: req.Principal.ID, Summary: "principal authentication version invalidated"}
}

var _ platformresource.PrincipalService = (*PrincipalResourceService)(nil)
