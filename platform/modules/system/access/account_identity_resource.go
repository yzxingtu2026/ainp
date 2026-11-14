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

func NewAccountIdentityResourceService(db *gorm.DB, auditor audit.Recorder) *AccountIdentityResourceService {
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

func accountIdentityAdminContext(ctx context.Context) (*auth.Context, error) {
	ac := auth.MustFromContext(ctx)
	if !ac.Authenticated || auth.NormalizeRealm(ac.Realm) != auth.RealmTenant || strings.TrimSpace(ac.TenantID) == "" ||
		ac.Principal.Kind != auth.PrincipalKindAccount || !ac.Principal.Valid() || ac.Principal.ID != ac.PrincipalID {
		return nil, platformresource.ErrForbidden
	}
	if !ac.HasPermission(ActionAccountIdentityManage) {
		return nil, platformresource.ErrForbidden
	}
	return ac, nil
}

func managedAccountIdentityDB(ctx context.Context, db *gorm.DB, accountID string, lock bool) (tenant.TenantDatabase, error) {
	tdb, err := tenant.TenantDB(ctx, db)
	if err != nil {
		return tenant.TenantDatabase{}, platformresource.ErrUnavailable
	}
	query := `SELECT a.status FROM tenant_accounts a
JOIN tenants t ON t.id = a.tenant_id
WHERE a.tenant_id = ? AND a.id = ? AND t.status = ?`
	if lock {
		query += " FOR UPDATE OF a"
	}
	rows, err := tenant.TenantRaw[struct{ Status status.ActivationStatus }](tdb, query, accountID, status.ActivationEnabled)
	if err != nil {
		return tenant.TenantDatabase{}, platformresource.ErrUnavailable
	}
	if len(rows) != 1 {
		return tenant.TenantDatabase{}, platformresource.ErrNotFound
	}
	if rows[0].Status != status.ActivationEnabled {
		return tenant.TenantDatabase{}, platformresource.ErrDisabled
	}
	return tdb, nil
}

func (s *AccountIdentityResourceService) ListForAccount(ctx context.Context, req platformresource.AccountIdentityAdminListRequest) ([]platformresource.ManagedAccountIdentity, error) {
	if s == nil || s.db == nil {
		return nil, platformresource.ErrUnavailable
	}
	if err := req.Validate(); err != nil {
		return nil, err
	}
	if _, err := accountIdentityAdminContext(ctx); err != nil {
		return nil, err
	}
	tdb, err := managedAccountIdentityDB(ctx, s.db, req.AccountID, false)
	if err != nil {
		return nil, err
	}
	rows, err := tenant.TenantRaw[accountIdentityResourceRow](tdb, `SELECT id, provider, subject, normalized_identifier, verified_at
FROM auth_identities
WHERE tenant_id = ? AND account_id = ? AND status = ? AND provider <> ?
ORDER BY created_at ASC, id ASC`, req.AccountID, status.ActivationEnabled, passwordMethod)
	if err != nil {
		return nil, platformresource.ErrUnavailable
	}
	result := make([]platformresource.ManagedAccountIdentity, len(rows))
	for i := range rows {
		result[i] = managedAccountIdentityProjection(rows[i])
	}
	return result, nil
}

func (s *AccountIdentityResourceService) BindForAccount(ctx context.Context, req platformresource.AccountIdentityAdminBindRequest) (*platformresource.ManagedAccountIdentity, error) {
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
	ac, err := accountIdentityAdminContext(ctx)
	if err != nil {
		return nil, err
	}
	var row accountIdentityResourceRow
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		tdb, scopeErr := managedAccountIdentityDB(ctx, tx, req.AccountID, true)
		if scopeErr != nil {
			return scopeErr
		}
		existingProvider, queryErr := tenant.TenantRaw[struct{ ID string }](tdb, `SELECT id FROM auth_identities
WHERE tenant_id = ? AND account_id = ? AND provider = ? LIMIT 1`, req.AccountID, req.Provider)
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
RETURNING id, provider, subject, normalized_identifier, verified_at`, identityID, req.AccountID, req.Provider, req.Subject, req.NormalizedIdentifier, status.ActivationEnabled, now, now, now)
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
		if auditErr := transactional.WithDB(tx).Write(ctx, managedAccountIdentityAudit(ac, req.AccountID, row.ID, "bind")); auditErr != nil {
			return platformresource.ErrUnavailable
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	projection := managedAccountIdentityProjection(row)
	return &projection, nil
}

func (s *AccountIdentityResourceService) UnbindForAccount(ctx context.Context, req platformresource.AccountIdentityAdminUnbindRequest) error {
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
	ac, err := accountIdentityAdminContext(ctx)
	if err != nil {
		return err
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		tdb, scopeErr := managedAccountIdentityDB(ctx, tx, req.AccountID, true)
		if scopeErr != nil {
			return scopeErr
		}
		rows, queryErr := tenant.TenantRaw[accountIdentityResourceRow](tdb, `SELECT id, provider, subject, normalized_identifier, verified_at
FROM auth_identities WHERE tenant_id = ? AND account_id = ? AND id = ? AND status = ? AND provider <> ? FOR UPDATE`, req.AccountID, req.IdentityID, status.ActivationEnabled, passwordMethod)
		if queryErr != nil {
			return platformresource.ErrUnavailable
		}
		if len(rows) != 1 {
			return platformresource.ErrNotFound
		}
		if err := tenant.TenantExec(tdb, `DELETE FROM auth_identities WHERE tenant_id = ? AND account_id = ? AND id = ?`, req.AccountID, req.IdentityID); err != nil {
			return platformresource.ErrUnavailable
		}
		if auditErr := transactional.WithDB(tx).Write(ctx, managedAccountIdentityAudit(ac, req.AccountID, req.IdentityID, "unbind")); auditErr != nil {
			return platformresource.ErrUnavailable
		}
		return nil
	})
}

func accountIdentityProjection(row accountIdentityResourceRow) platformresource.AccountIdentity {
	return platformresource.AccountIdentity{ID: row.ID, Provider: row.Provider, Subject: row.Subject, NormalizedIdentifier: row.NormalizedIdentifier, VerifiedAt: row.VerifiedAt}
}

func managedAccountIdentityProjection(row accountIdentityResourceRow) platformresource.ManagedAccountIdentity {
	return platformresource.ManagedAccountIdentity{
		ID: row.ID, Provider: row.Provider, MaskedSubject: maskAccountIdentityValue(row.Subject),
		MaskedNormalizedIdentifier: maskAccountIdentityValue(row.NormalizedIdentifier), VerifiedAt: row.VerifiedAt,
	}
}

func maskAccountIdentityValue(value string) string {
	runes := []rune(value)
	if len(runes) == 0 {
		return ""
	}
	if at := strings.LastIndex(value, "@"); at > 0 && at < len(value)-1 {
		local := []rune(value[:at])
		prefix := string(local[:1])
		return prefix + "***@" + value[at+1:]
	}
	digits := make([]rune, 0, len(runes))
	for _, r := range runes {
		if r >= '0' && r <= '9' {
			digits = append(digits, r)
		}
	}
	if len(digits) >= 7 && len(digits) == len(runes) {
		return string(digits[:3]) + "****" + string(digits[len(digits)-4:])
	}
	if len(runes) <= 2 {
		return strings.Repeat("*", len(runes))
	}
	if len(runes) <= 6 {
		return string(runes[:1]) + "***" + string(runes[len(runes)-1:])
	}
	return string(runes[:2]) + "****" + string(runes[len(runes)-2:])
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

func managedAccountIdentityAudit(ac *auth.Context, accountID, identityID, operation string) audit.Record {
	return audit.Record{
		TenantID: ac.TenantID, Realm: string(auth.RealmTenant), ActorID: ac.PrincipalID, ActorType: "principal",
		PrincipalKind: string(ac.Principal.Kind), ClientSurface: ac.ClientSurface, Audience: ac.Audience,
		ActionID: "system.platform_resource_account_identity_admin_" + operation, Result: "success", RequestID: ac.RequestID,
		Client: ac.Client, Permission: ActionAccountIdentityManage, ResourceType: "system.account_identity", ResourceID: identityID,
		Summary: "managed account external identity " + operation + " completed for account " + accountID,
	}
}

var _ platformresource.AccountIdentityService = (*AccountIdentityResourceService)(nil)
var _ platformresource.AccountIdentityAdministrationService = (*AccountIdentityResourceService)(nil)
