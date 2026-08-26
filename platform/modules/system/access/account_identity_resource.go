package access

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/yzxingtu2026/ainp/platform/framework/audit"
	"github.com/yzxingtu2026/ainp/platform/framework/auth"
	"github.com/yzxingtu2026/ainp/platform/framework/platformresource"
	"github.com/yzxingtu2026/ainp/platform/framework/status"
	"github.com/yzxingtu2026/ainp/platform/framework/tenant"
	"gorm.io/gorm"
)

type AccountIdentityResourceService struct {
	db      *gorm.DB
	auditor audit.Recorder
}

func NewAccountIdentityResourceService(db *gorm.DB, auditor audit.Recorder) platformresource.AccountIdentityService {
	return &AccountIdentityResourceService{db: db, auditor: auditor}
}

type accountIdentityResourceRow struct {
	ID                   string
	Provider             string
	Subject              string
	NormalizedIdentifier string
	VerifiedAt           time.Time
}

func accountIdentityContext(ctx context.Context) (*auth.Context, error) {
	ac := auth.MustFromContext(ctx)
	if !ac.Authenticated || auth.NormalizeRealm(ac.Realm) != auth.RealmTenant || strings.TrimSpace(ac.TenantID) == "" ||
		ac.Principal.Kind != auth.PrincipalKindAccount || !ac.Principal.Valid() || ac.Principal.ID != ac.PrincipalID {
		return nil, platformresource.ErrForbidden
	}
	return ac, nil
}

func activeAccountIdentityDB(ctx context.Context, db *gorm.DB, accountID string, lock bool) (tenant.TenantDatabase, error) {
	tdb, err := tenant.TenantDB(ctx, db)
	if err != nil {
		return tenant.TenantDatabase{}, platformresource.ErrUnavailable
	}
	query := `SELECT a.id FROM tenant_accounts a
JOIN tenants t ON t.id = a.tenant_id
WHERE a.tenant_id = ? AND a.id = ? AND a.status = ? AND t.status = ?`
	if lock {
		query += " FOR UPDATE OF a"
	}
	rows, err := tenant.TenantRaw[struct{ ID string }](tdb, query, accountID, status.ActivationEnabled, status.ActivationEnabled)
	if err != nil || len(rows) != 1 {
		return tenant.TenantDatabase{}, platformresource.ErrForbidden
	}
	return tdb, nil
}

func (s *AccountIdentityResourceService) List(ctx context.Context) ([]platformresource.AccountIdentity, error) {
	if s == nil || s.db == nil {
		return nil, platformresource.ErrUnavailable
	}
	ac, err := accountIdentityContext(ctx)
	if err != nil {
		return nil, err
	}
	tdb, err := activeAccountIdentityDB(ctx, s.db, ac.PrincipalID, false)
	if err != nil {
		return nil, err
	}
	rows, err := tenant.TenantRaw[accountIdentityResourceRow](tdb, `SELECT id, provider, subject, normalized_identifier, verified_at
FROM auth_identities
WHERE tenant_id = ? AND account_id = ? AND status = ? AND provider <> ?
ORDER BY created_at ASC, id ASC`, ac.PrincipalID, status.ActivationEnabled, passwordMethod)
	if err != nil {
		return nil, platformresource.ErrUnavailable
	}
	result := make([]platformresource.AccountIdentity, len(rows))
	for i := range rows {
		result[i] = accountIdentityProjection(rows[i])
	}
	return result, nil
}

func (s *AccountIdentityResourceService) Bind(ctx context.Context, req platformresource.AccountIdentityBindRequest) (*platformresource.AccountIdentity, error) {
	if s == nil || s.db == nil {
		return nil, platformresource.ErrUnavailable
	}
	transactional, ok := s.auditor.(audit.TransactionalRecorder)
	if !ok {
		return nil, platformresource.ErrUnavailable
	}
	if err := req.Validate(); err != nil {
		return nil, err
	}
	ac, err := accountIdentityContext(ctx)
	if err != nil {
		return nil, err
	}
	var row accountIdentityResourceRow
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		tdb, scopeErr := activeAccountIdentityDB(ctx, tx, ac.PrincipalID, true)
		if scopeErr != nil {
			return scopeErr
		}
		existingProvider, queryErr := tenant.TenantRaw[struct{ ID string }](tdb, `SELECT id FROM auth_identities
WHERE tenant_id = ? AND account_id = ? AND provider = ? LIMIT 1`, ac.PrincipalID, req.Provider)
		if queryErr != nil {
			return platformresource.ErrUnavailable
		}
		if len(existingProvider) != 0 {
			return platformresource.ErrConflict
		}
		now := time.Now().UTC()
		identityID := uuid.NewString()
		created, createErr := tenant.TenantRaw[accountIdentityResourceRow](tdb, `INSERT INTO auth_identities
(tenant_id, id, account_id, provider, subject, normalized_identifier, status, verified_at, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT DO NOTHING
RETURNING id, provider, subject, normalized_identifier, verified_at`, identityID, ac.PrincipalID, req.Provider, req.Subject, req.NormalizedIdentifier, status.ActivationEnabled, now, now, now)
		if createErr != nil {
			return platformresource.ErrUnavailable
		}
		if len(created) == 0 {
			return platformresource.ErrConflict
		}
		if len(created) != 1 {
			return platformresource.ErrUnavailable
		}
		row = created[0]
		if auditErr := transactional.WithDB(tx).Write(ctx, accountIdentityAudit(ac, row.ID, "bind")); auditErr != nil {
			return platformresource.ErrUnavailable
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	projection := accountIdentityProjection(row)
	return &projection, nil
}

func (s *AccountIdentityResourceService) Unbind(ctx context.Context, req platformresource.AccountIdentityUnbindRequest) error {
	if s == nil || s.db == nil {
		return platformresource.ErrUnavailable
	}
	transactional, ok := s.auditor.(audit.TransactionalRecorder)
	if !ok {
		return platformresource.ErrUnavailable
	}
	if err := req.Validate(); err != nil {
		return err
	}
	ac, err := accountIdentityContext(ctx)
	if err != nil {
		return err
	}
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		tdb, scopeErr := activeAccountIdentityDB(ctx, tx, ac.PrincipalID, true)
		if scopeErr != nil {
			return scopeErr
		}
		rows, queryErr := tenant.TenantRaw[accountIdentityResourceRow](tdb, `SELECT id, provider, subject, normalized_identifier, verified_at
FROM auth_identities
WHERE tenant_id = ? AND account_id = ? AND id = ? AND provider <> ?
FOR UPDATE`, ac.PrincipalID, req.IdentityID, passwordMethod)
		if queryErr != nil || len(rows) != 1 {
			return platformresource.ErrUnavailable
		}
		if deleteErr := tenant.TenantExec(tdb, `DELETE FROM auth_identities
WHERE tenant_id = ? AND account_id = ? AND id = ? AND provider <> ?`, ac.PrincipalID, req.IdentityID, passwordMethod); deleteErr != nil {
			return platformresource.ErrUnavailable
		}
		if auditErr := transactional.WithDB(tx).Write(ctx, accountIdentityAudit(ac, req.IdentityID, "unbind")); auditErr != nil {
			return platformresource.ErrUnavailable
		}
		return nil
	})
	if err != nil {
		return err
	}
	return nil
}

func accountIdentityProjection(row accountIdentityResourceRow) platformresource.AccountIdentity {
	return platformresource.AccountIdentity{ID: row.ID, Provider: row.Provider, Subject: row.Subject, NormalizedIdentifier: row.NormalizedIdentifier, VerifiedAt: row.VerifiedAt}
}

func accountIdentityAudit(ac *auth.Context, identityID, operation string) audit.Record {
	return audit.Record{
		TenantID: ac.TenantID, Realm: string(auth.RealmTenant), ActorID: ac.PrincipalID, ActorType: "principal",
		PrincipalKind: string(ac.Principal.Kind), ClientSurface: ac.ClientSurface, Audience: ac.Audience,
		ActionID: "system.platform_resource_account_identity_" + operation, Result: "success", RequestID: ac.RequestID,
		Client: ac.Client, ResourceType: "system.account_identity", ResourceID: identityID,
		Summary: "account external identity " + operation + " completed",
	}
}

var _ platformresource.AccountIdentityService = (*AccountIdentityResourceService)(nil)
