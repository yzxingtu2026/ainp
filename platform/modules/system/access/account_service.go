package access

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/yzxingtu2026/ainp/platform/framework/action"
	"github.com/yzxingtu2026/ainp/platform/framework/auth"
	"github.com/yzxingtu2026/ainp/platform/framework/errs"
	"github.com/yzxingtu2026/ainp/platform/framework/i18n"
	"github.com/yzxingtu2026/ainp/platform/framework/logging"
	"github.com/yzxingtu2026/ainp/platform/framework/outbox"
	"github.com/yzxingtu2026/ainp/platform/framework/pagination"
	"github.com/yzxingtu2026/ainp/platform/framework/status"
	"github.com/yzxingtu2026/ainp/platform/framework/tenant"
	"github.com/yzxingtu2026/ainp/platform/modules/system/access/model"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

var usernamePattern = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9_]{2,63}$`)

type AccountService struct {
	db           *gorm.DB
	auth         *AuthService
	outboxWriter outbox.Writer
}

func NewAccountService(db *gorm.DB, authService *AuthService) *AccountService {
	return &AccountService{db: db, auth: authService}
}

// SetOutboxWriter injects the outbox writer for reliable notification dispatch.
func (s *AccountService) SetOutboxWriter(w outbox.Writer) {
	s.outboxWriter = w
}

type accountListRequest struct {
	pagination.Request
	Keyword string                  `json:"keyword,omitempty" validate:"omitempty"`
	Status  status.ActivationStatus `json:"status,omitempty" validate:"omitempty,oneof=enabled disabled"`
	RoleID  string                  `json:"role_id,omitempty" validate:"omitempty"`
}

type accountIDRequest struct {
	ID string `json:"id" validate:"required"`
}

type accountCreateRequest struct {
	Username    string                  `json:"username" validate:"required,min=3,max=64"`
	DisplayName string                  `json:"display_name" validate:"required,max=128"`
	Password    string                  `json:"password" validate:"required"`
	Status      status.ActivationStatus `json:"status" validate:"omitempty,oneof=enabled disabled"`
	RoleIDs     []string                `json:"role_ids,omitempty" validate:"omitempty,max=20,dive,required"`
	OrgID       string                  `json:"org_id,omitempty" validate:"omitempty"`
	DeptID      string                  `json:"dept_id,omitempty" validate:"omitempty"`
}

type accountUpdateRequest struct {
	ID          string  `json:"id" validate:"required"`
	DisplayName string  `json:"display_name" validate:"required,max=128"`
	OrgID       *string `json:"org_id,omitempty" validate:"omitempty"`
	DeptID      *string `json:"dept_id,omitempty" validate:"omitempty"`
}

type selfProfileUpdateRequest struct {
	DisplayName        string  `json:"display_name" validate:"required,max=128"`
	AvatarAttachmentID *string `json:"avatar_attachment_id,omitempty" validate:"omitempty"`
}

type selfPasswordUpdateRequest struct {
	CurrentPassword string `json:"current_password" validate:"required"`
	NewPassword     string `json:"new_password" validate:"required"`
}

type AccountSelfProfileView struct {
	AccountView
	AvatarAttachmentID     *string `json:"avatar_attachment_id,omitempty"`
	MemoryPreferredAddress string  `json:"memory_preferred_address"`
	EffectiveAddress       string  `json:"effective_address"`
	AddressSource          string  `json:"address_source"`
}

func (s *AccountService) SelfProfile(ctx context.Context, _ json.RawMessage) (any, error) {
	_, accountID, err := tenantIdentity(ctx)
	if err != nil {
		return nil, err
	}
	tdb, err := tenant.TenantDB(ctx, s.db)
	if err != nil {
		return nil, err
	}
	account, err := s.findAccount(tdb, accountID, false)
	if err != nil {
		return nil, err
	}
	roleMap, err := s.loadAccountRoles(tdb, []string{accountID})
	if err != nil {
		return nil, err
	}
	view := AccountSelfProfileView{AccountView: accountView(*account, roleMap[accountID]), AvatarAttachmentID: account.AvatarAttachmentID}
	rows, qerr := tenant.TenantRaw[struct{ PreferredAddress string }](tdb, `SELECT COALESCE(profile_json->>'preferred_address','') AS preferred_address
FROM ai_memory_current_profiles WHERE tenant_id = ? AND user_id = ? AND agent_id IS NULL AND status = 'ready'
ORDER BY source_revision DESC LIMIT 1`, accountID)
	if qerr == nil && len(rows) > 0 {
		view.MemoryPreferredAddress = strings.TrimSpace(rows[0].PreferredAddress)
	}
	view.EffectiveAddress, view.AddressSource = view.MemoryPreferredAddress, "memory_profile"
	if view.EffectiveAddress == "" {
		view.EffectiveAddress, view.AddressSource = account.DisplayName, "display_name"
	}
	if view.EffectiveAddress == "" {
		view.EffectiveAddress, view.AddressSource = account.Username, "username"
	}
	return view, nil
}

func (s *AccountService) UpdateSelfProfile(ctx context.Context, input json.RawMessage) (any, error) {
	_, accountID, err := tenantIdentity(ctx)
	if err != nil {
		return nil, err
	}
	var req selfProfileUpdateRequest
	if err := decodeAccountRequest(input, &req); err != nil {
		return nil, err
	}
	req.DisplayName = strings.TrimSpace(req.DisplayName)
	if req.AvatarAttachmentID != nil {
		trimmed := strings.TrimSpace(*req.AvatarAttachmentID)
		req.AvatarAttachmentID = &trimmed
	}
	if req.DisplayName == "" {
		return nil, errs.ValidationKey("common.validation_failed", "display_name is required")
	}
	setAccountAudit(ctx, accountID, "self profile updated")
	tdb, err := tenant.TenantDB(ctx, s.db)
	if err != nil {
		return nil, err
	}
	if req.AvatarAttachmentID != nil && *req.AvatarAttachmentID != "" {
		rows, queryErr := tenant.TenantRaw[struct{ ID string }](tdb, `SELECT a.id
FROM attachments a
JOIN attachment_links l ON l.tenant_id = a.tenant_id AND l.attachment_id = a.id
WHERE a.tenant_id = ? AND a.id = ? AND a.uploader_id = ? AND a.status = 'ready'
  AND a.content_type LIKE 'image/%'
  AND l.resource_type = 'system.user_profile' AND l.resource_id = ? AND l.field = 'avatar'
LIMIT 1`, *req.AvatarAttachmentID, accountID, accountID)
		if queryErr != nil || len(rows) != 1 {
			return nil, errs.ValidationKey("common.validation_failed", "avatar attachment must be a bound ready image uploaded by the current user")
		}
	}
	var updateErr error
	if req.AvatarAttachmentID == nil {
		updateErr = tenant.TenantExec(tdb, `WITH scoped_account AS (
  SELECT id FROM tenant_accounts WHERE tenant_id = ? AND id = ?
)
UPDATE tenant_accounts SET display_name = ?, updated_at = ?
WHERE id IN (SELECT id FROM scoped_account)`, accountID, req.DisplayName, time.Now().UTC())
	} else {
		updateErr = tenant.TenantExec(tdb, `WITH scoped_account AS (
  SELECT id FROM tenant_accounts WHERE tenant_id = ? AND id = ?
)
UPDATE tenant_accounts SET display_name = ?, avatar_attachment_id = NULLIF(?, ''), updated_at = ?
WHERE id IN (SELECT id FROM scoped_account)`, accountID, req.DisplayName, *req.AvatarAttachmentID, time.Now().UTC())
	}
	if updateErr != nil {
		return nil, normalizeAccountError(updateErr, "update self profile failed")
	}
	return s.SelfProfile(ctx, nil)
}

func (s *AccountService) UpdateSelfPassword(ctx context.Context, input json.RawMessage) (any, error) {
	_, accountID, err := tenantIdentity(ctx)
	if err != nil {
		return nil, err
	}
	var req selfPasswordUpdateRequest
	if err := decodeAccountRequest(input, &req); err != nil {
		return nil, err
	}
	if req.CurrentPassword == "" {
		return nil, errs.ValidationKey("common.validation_failed", "current password is required")
	}
	if err := ValidatePassword(req.NewPassword); err != nil {
		return nil, err
	}
	setAccountAudit(ctx, accountID, "self password updated")
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		tdb, scopeErr := tenant.TenantDB(ctx, tx)
		if scopeErr != nil {
			return scopeErr
		}
		account, findErr := s.findAccount(tdb, accountID, true)
		if findErr != nil {
			return findErr
		}
		if bcrypt.CompareHashAndPassword([]byte(account.PasswordHash), []byte(req.CurrentPassword)) != nil {
			return errs.NewKey(errs.CodeInvalidCredentials, "auth.current_password_invalid", "current password is invalid")
		}
		if bcrypt.CompareHashAndPassword([]byte(account.PasswordHash), []byte(req.NewPassword)) == nil {
			return errs.ValidationKey("user.password_unchanged", "new password must differ from current password")
		}
		hash, hashErr := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
		if hashErr != nil {
			return errs.Wrap(errs.CodeInternal, "hash password failed", hashErr)
		}
		if execErr := tenant.TenantExec(tdb, `WITH scoped_account AS (
  SELECT id FROM tenant_accounts WHERE tenant_id = ? AND id = ?
)
UPDATE tenant_accounts SET password_hash = ?, updated_at = ? WHERE id IN (SELECT id FROM scoped_account)`,
			accountID, string(hash), time.Now().UTC()); execErr != nil {
			return execErr
		}
		return s.auth.BumpAuthVersionScoped(tdb, accountID)
	})
	if err != nil {
		return nil, normalizeAccountError(err, "update self password failed")
	}
	return map[string]any{"reauthentication_required": true}, nil
}

type accountStatusRequest struct {
	ID     string                  `json:"id" validate:"required"`
	Status status.ActivationStatus `json:"status" validate:"required,oneof=enabled disabled"`
}

type accountRolesRequest struct {
	ID      string   `json:"id" validate:"required"`
	RoleIDs []string `json:"role_ids" validate:"required,max=20,dive,required"`
}

type accountPasswordRequest struct {
	ID       string `json:"id" validate:"required"`
	Password string `json:"password" validate:"required"`
}

type AccountRoleBrief struct {
	ID        string                  `json:"id"`
	Code      string                  `json:"code"`
	Name      string                  `json:"name"`
	Status    status.ActivationStatus `json:"status"`
	IsBuiltin bool                    `json:"is_builtin"`
}

type AccountView struct {
	ID          string                  `json:"id"`
	Username    string                  `json:"username"`
	DisplayName string                  `json:"display_name"`
	Status      status.ActivationStatus `json:"status"`
	OrgID       string                  `json:"org_id,omitempty"`
	DeptID      string                  `json:"dept_id,omitempty"`
	Roles       []AccountRoleBrief      `json:"roles"`
	IsAdmin     bool                    `json:"is_admin"`
	CreatedAt   time.Time               `json:"created_at"`
	UpdatedAt   time.Time               `json:"updated_at"`
}

func decodeAccountRequest(input json.RawMessage, target any) error {
	if err := json.Unmarshal(input, target); err != nil {
		return errs.ValidationKey("common.validation_failed", "invalid json")
	}
	return nil
}

func validateAccountStatus(value status.ActivationStatus) bool {
	return value == status.ActivationEnabled || value == status.ActivationDisabled
}

func (s *AccountService) List(ctx context.Context, input json.RawMessage) (any, error) {
	return action.JSON(s.list)(ctx, input)
}

func (s *AccountService) list(ctx context.Context, req accountListRequest) (any, error) {
	_, _, err := tenantIdentity(ctx)
	if err != nil {
		return nil, err
	}
	tdb, err := tenant.TenantDB(ctx, s.db)
	if err != nil {
		return nil, err
	}
	keyword, roleID := strings.TrimSpace(req.Keyword), strings.TrimSpace(req.RoleID)
	like := "%" + keyword + "%"
	counts, err := tenant.TenantRaw[struct{ Count int64 }](tdb, `SELECT COUNT(*) AS count FROM tenant_accounts u
WHERE u.tenant_id = ? AND (? = '' OR u.username ILIKE ? OR u.display_name ILIKE ?)
  AND (? = '' OR u.status = ?)
  AND (? = '' OR EXISTS (
    SELECT 1 FROM tenant_account_roles ur WHERE ur.tenant_id = u.tenant_id AND ur.account_id = u.id AND ur.role_id = ?
  ))`, keyword, like, like, req.Status, req.Status, roleID, roleID)
	if err != nil {
		return nil, errs.Wrap(errs.CodeInternal, "count users failed", err)
	}
	var total int64
	if len(counts) == 1 {
		total = counts[0].Count
	}
	accounts, err := tenant.TenantRaw[model.TenantAccount](tdb, `SELECT u.* FROM tenant_accounts u
WHERE u.tenant_id = ? AND (? = '' OR u.username ILIKE ? OR u.display_name ILIKE ?)
  AND (? = '' OR u.status = ?)
  AND (? = '' OR EXISTS (
    SELECT 1 FROM tenant_account_roles ur WHERE ur.tenant_id = u.tenant_id AND ur.account_id = u.id AND ur.role_id = ?
  ))
ORDER BY u.created_at ASC, u.id ASC OFFSET ? LIMIT ?`, keyword, like, like, req.Status, req.Status, roleID, roleID, req.Offset(), req.PageSize)
	if err != nil {
		return nil, errs.Wrap(errs.CodeInternal, "list users failed", err)
	}
	roleMap, err := s.loadAccountRoles(tdb, accountIDs(accounts))
	if err != nil {
		return nil, err
	}
	records := make([]AccountView, 0, len(accounts))
	for _, account := range accounts {
		records = append(records, accountView(account, roleMap[account.ID]))
	}
	return pagination.NewResult(records, total, req.Request), nil
}

func (s *AccountService) Get(ctx context.Context, input json.RawMessage) (any, error) {
	_, _, err := tenantIdentity(ctx)
	if err != nil {
		return nil, err
	}
	var req accountIDRequest
	if err := decodeAccountRequest(input, &req); err != nil {
		return nil, err
	}
	tdb, err := tenant.TenantDB(ctx, s.db)
	if err != nil {
		return nil, err
	}
	account, err := s.findAccount(tdb, req.ID, false)
	if err != nil {
		return nil, err
	}
	roleMap, err := s.loadAccountRoles(tdb, []string{account.ID})
	if err != nil {
		return nil, err
	}
	return accountView(*account, roleMap[account.ID]), nil
}

func (s *AccountService) Create(ctx context.Context, input json.RawMessage) (any, error) {
	tenantID, _, err := tenantIdentity(ctx)
	if err != nil {
		return nil, err
	}
	var req accountCreateRequest
	if err := decodeAccountRequest(input, &req); err != nil {
		return nil, err
	}
	req.Username = strings.TrimSpace(req.Username)
	req.DisplayName = strings.TrimSpace(req.DisplayName)
	req.OrgID = strings.TrimSpace(req.OrgID)
	req.DeptID = strings.TrimSpace(req.DeptID)
	if !usernamePattern.MatchString(req.Username) || req.DisplayName == "" {
		return nil, errs.ValidationKey("common.validation_failed", "invalid username or display_name")
	}
	if err := ValidatePassword(req.Password); err != nil {
		return nil, err
	}
	if req.Status == "" {
		req.Status = status.ActivationEnabled
	}
	if !validateAccountStatus(req.Status) {
		return nil, errs.ValidationKey("common.validation_failed", "invalid status")
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		return nil, errs.Wrap(errs.CodeInternal, "hash password failed", err)
	}
	account := model.TenantAccount{
		ID: uuid.NewString(), TenantID: tenantID, Username: req.Username, PasswordHash: string(hash),
		DisplayName: req.DisplayName, Status: req.Status, AuthVersion: 1, OrgID: req.OrgID, DeptID: req.DeptID,
	}
	var roles []model.Role
	setAccountAudit(ctx, account.ID, "user created")
	if err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		tdb, scopeErr := tenant.TenantDB(ctx, tx)
		if scopeErr != nil {
			return scopeErr
		}
		roles, err = s.validateAssignableRoles(tdb, req.RoleIDs)
		if err != nil {
			return err
		}
		now := time.Now().UTC()
		account.CreatedAt, account.UpdatedAt = now, now
		if err := tenant.TenantExec(tdb, `WITH scoped_tenant AS (SELECT ?::text AS tenant_id)
INSERT INTO tenant_accounts (id, tenant_id, username, password_hash, display_name, status, auth_version, org_id, dept_id, created_at, updated_at)
SELECT ?, tenant_id, ?, ?, ?, ?, 1, ?, ?, ?, ? FROM scoped_tenant`,
			account.ID, account.Username, account.PasswordHash, account.DisplayName, account.Status, account.OrgID, account.DeptID, now, now); err != nil {
			if strings.Contains(strings.ToLower(err.Error()), "duplicate") {
				return errs.NewKey(errs.CodeConflict, "user.username_conflict", "username already exists")
			}
			return err
		}
		return replaceAccountRoles(tdb, account.ID, roleIDs(roles))
	}); err != nil {
		return nil, normalizeAccountError(err, "create user failed")
	}
	return accountView(account, toRoleBriefs(roles)), nil
}

func (s *AccountService) Update(ctx context.Context, input json.RawMessage) (any, error) {
	_, _, err := tenantIdentity(ctx)
	if err != nil {
		return nil, err
	}
	var req accountUpdateRequest
	if err := decodeAccountRequest(input, &req); err != nil {
		return nil, err
	}
	req.DisplayName = strings.TrimSpace(req.DisplayName)
	if req.OrgID != nil {
		value := strings.TrimSpace(*req.OrgID)
		req.OrgID = &value
	}
	if req.DeptID != nil {
		value := strings.TrimSpace(*req.DeptID)
		req.DeptID = &value
	}
	if req.DisplayName == "" {
		return nil, errs.ValidationKey("common.validation_failed", "display_name is required")
	}
	var account *model.TenantAccount
	setAccountAudit(ctx, req.ID, "user updated")
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		tdb, scopeErr := tenant.TenantDB(ctx, tx)
		if scopeErr != nil {
			return scopeErr
		}
		current, err := s.findAccount(tdb, req.ID, true)
		if err != nil {
			return err
		}
		assignments := []string{"display_name = ?"}
		arguments := []any{current.ID, req.DisplayName}
		if req.OrgID != nil {
			assignments = append(assignments, "org_id = ?")
			arguments = append(arguments, *req.OrgID)
		}
		if req.DeptID != nil {
			assignments = append(assignments, "dept_id = ?")
			arguments = append(arguments, *req.DeptID)
		}
		assignments = append(assignments, "updated_at = ?")
		arguments = append(arguments, time.Now().UTC())
		if err := tenant.TenantExec(tdb, `WITH scoped_account AS (
  SELECT id FROM tenant_accounts WHERE tenant_id = ? AND id = ?
)
UPDATE tenant_accounts SET `+strings.Join(assignments, ", ")+`
WHERE id IN (SELECT id FROM scoped_account)`, arguments...); err != nil {
			return err
		}
		current.DisplayName = req.DisplayName
		if req.OrgID != nil {
			current.OrgID = *req.OrgID
		}
		if req.DeptID != nil {
			current.DeptID = *req.DeptID
		}
		current.UpdatedAt = time.Now().UTC()
		account = current
		return nil
	})
	if err != nil {
		return nil, normalizeAccountError(err, "update user failed")
	}
	tdb, err := tenant.TenantDB(ctx, s.db)
	if err != nil {
		return nil, err
	}
	roleMap, err := s.loadAccountRoles(tdb, []string{account.ID})
	if err != nil {
		return nil, err
	}
	return accountView(*account, roleMap[account.ID]), nil
}

func (s *AccountService) UpdateStatus(ctx context.Context, input json.RawMessage) (any, error) {
	_, actorID, err := tenantIdentity(ctx)
	if err != nil {
		return nil, err
	}
	var req accountStatusRequest
	if err := decodeAccountRequest(input, &req); err != nil {
		return nil, err
	}
	if !validateAccountStatus(req.Status) {
		return nil, errs.ValidationKey("common.validation_failed", "invalid status")
	}
	if req.ID == actorID && req.Status == status.ActivationDisabled {
		return nil, errs.NewKey(errs.CodeConflict, "user.self_disable_forbidden", "cannot disable yourself")
	}
	setAccountAudit(ctx, req.ID, "user status updated")
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		tdb, scopeErr := tenant.TenantDB(ctx, tx)
		if scopeErr != nil {
			return scopeErr
		}
		account, err := s.findAccount(tdb, req.ID, true)
		if err != nil {
			return err
		}
		if account.Status == req.Status {
			return nil
		}
		if req.Status == status.ActivationDisabled {
			if err := s.protectLastAdmin(tdb, account.ID, false); err != nil {
				return err
			}
		}
		if err := tenant.TenantExec(tdb, `WITH scoped_account AS (
  SELECT id FROM tenant_accounts WHERE tenant_id = ? AND id = ?
)
UPDATE tenant_accounts SET status = ?, updated_at = ? WHERE id IN (SELECT id FROM scoped_account)`,
			account.ID, req.Status, time.Now().UTC()); err != nil {
			return err
		}
		return s.auth.BumpAuthVersionScoped(tdb, account.ID)
	})
	if err != nil {
		return nil, normalizeAccountError(err, "update user status failed")
	}
	return map[string]any{"id": req.ID, "status": req.Status}, nil
}

func (s *AccountService) UpdateRoles(ctx context.Context, input json.RawMessage) (any, error) {
	_, _, err := tenantIdentity(ctx)
	if err != nil {
		return nil, err
	}
	var req accountRolesRequest
	if err := decodeAccountRequest(input, &req); err != nil {
		return nil, err
	}
	var roles []model.Role
	var targetUsername string
	setAccountAudit(ctx, req.ID, "user roles updated")
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		tdb, scopeErr := tenant.TenantDB(ctx, tx)
		if scopeErr != nil {
			return scopeErr
		}
		account, err := s.findAccount(tdb, req.ID, true)
		if err != nil {
			return err
		}
		targetUsername = account.Username
		roles, err = s.validateAssignableRoles(tdb, req.RoleIDs)
		if err != nil {
			return err
		}
		remainsAdmin := account.Status == status.ActivationEnabled && hasBuiltinAdmin(roles)
		if err := s.protectLastAdmin(tdb, account.ID, remainsAdmin); err != nil {
			return err
		}
		if err := replaceAccountRoles(tdb, account.ID, roleIDs(roles)); err != nil {
			return err
		}
		if err := s.auth.BumpAuthVersionScoped(tdb, account.ID); err != nil {
			return err
		}
		return s.notifyRoleChanged(ctx, tx, account.ID, targetUsername, roles)
	})
	if err != nil {
		return nil, normalizeAccountError(err, "update user roles failed")
	}
	return map[string]any{"id": req.ID, "roles": toRoleBriefs(roles)}, nil
}

func (s *AccountService) ResetPassword(ctx context.Context, input json.RawMessage) (any, error) {
	_, _, err := tenantIdentity(ctx)
	if err != nil {
		return nil, err
	}
	var req accountPasswordRequest
	if err := decodeAccountRequest(input, &req); err != nil {
		return nil, err
	}
	if err := ValidatePassword(req.Password); err != nil {
		return nil, err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		return nil, errs.Wrap(errs.CodeInternal, "hash password failed", err)
	}
	var targetUsername string
	setAccountAudit(ctx, req.ID, "user password reset")
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		tdb, scopeErr := tenant.TenantDB(ctx, tx)
		if scopeErr != nil {
			return scopeErr
		}
		account, err := s.findAccount(tdb, req.ID, true)
		if err != nil {
			return err
		}
		targetUsername = account.Username
		if err := tenant.TenantExec(tdb, `WITH scoped_account AS (
  SELECT id FROM tenant_accounts WHERE tenant_id = ? AND id = ?
)
UPDATE tenant_accounts SET password_hash = ?, updated_at = ? WHERE id IN (SELECT id FROM scoped_account)`,
			account.ID, string(hash), time.Now().UTC()); err != nil {
			return err
		}
		if err := s.auth.BumpAuthVersionScoped(tdb, account.ID); err != nil {
			return err
		}
		return s.notifyPasswordReset(ctx, tx, account.ID, targetUsername)
	})
	if err != nil {
		return nil, normalizeAccountError(err, "reset user password failed")
	}
	return map[string]any{"id": req.ID}, nil
}

func (s *AccountService) findAccount(db tenant.TenantDatabase, accountID string, lock bool) (*model.TenantAccount, error) {
	if accountID == "" {
		return nil, errs.ValidationKey("common.validation_failed", "user id is required")
	}
	sql := `SELECT * FROM tenant_accounts WHERE tenant_id = ? AND id = ?`
	if lock {
		sql += ` FOR UPDATE`
	}
	rows, err := tenant.TenantRaw[model.TenantAccount](db, sql, accountID)
	if err != nil {
		return nil, err
	}
	if len(rows) != 1 {
		return nil, errs.NewKey(errs.CodeNotFound, "user.not_found", "user not found")
	}
	return &rows[0], nil
}

func (s *AccountService) resolveAssignableAccountRoles(ctx context.Context, accountID string, roleIDs []string) (*model.TenantAccount, []model.Role, error) {
	tdb, err := tenant.TenantDB(ctx, s.db)
	if err != nil {
		return nil, nil, err
	}
	account, err := s.findAccount(tdb, accountID, false)
	if err != nil {
		return nil, nil, err
	}
	roles, err := s.validateAssignableRoles(tdb, roleIDs)
	if err != nil {
		return nil, nil, fmt.Errorf("resolve roles: %w", err)
	}
	return account, roles, nil
}

func (s *AccountService) validateAssignableRoles(db tenant.TenantDatabase, values []string) ([]model.Role, error) {
	seen := make(map[string]struct{})
	ids := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		ids = append(ids, value)
	}
	if len(ids) == 0 {
		return []model.Role{}, nil
	}
	roles, err := tenant.TenantRaw[model.Role](db,
		`SELECT * FROM roles WHERE tenant_id = ? AND id IN ? AND status = ?`, ids, status.ActivationEnabled)
	if err != nil {
		return nil, err
	}
	if len(roles) != len(ids) {
		return nil, errs.ValidationKey("user.role_invalid", "role is invalid or not assignable")
	}
	sort.Slice(roles, func(i, j int) bool { return roles[i].Code < roles[j].Code })
	return roles, nil
}

func (s *AccountService) loadAccountRoles(db tenant.TenantDatabase, ids []string) (map[string][]AccountRoleBrief, error) {
	result := make(map[string][]AccountRoleBrief, len(ids))
	if len(ids) == 0 {
		return result, nil
	}
	rows, err := tenant.TenantRaw[struct {
		AccountID string                  `gorm:"column:account_id"`
		ID        string                  `gorm:"column:id"`
		Code      string                  `gorm:"column:code"`
		Name      string                  `gorm:"column:name"`
		Status    status.ActivationStatus `gorm:"column:status"`
		IsBuiltin bool                    `gorm:"column:is_builtin"`
	}](db, `SELECT ur.account_id, r.id, r.code, r.name, r.status, r.is_builtin
FROM tenant_account_roles ur JOIN roles r ON r.id = ur.role_id AND r.tenant_id = ur.tenant_id
WHERE ur.tenant_id = ? AND ur.account_id IN ? ORDER BY r.is_builtin DESC, r.code ASC`, ids)
	if err != nil {
		return nil, errs.Wrap(errs.CodeInternal, "load user roles failed", err)
	}
	for _, row := range rows {
		result[row.AccountID] = append(result[row.AccountID], AccountRoleBrief{
			ID: row.ID, Code: row.Code, Name: row.Name, Status: row.Status, IsBuiltin: row.IsBuiltin,
		})
	}
	return result, nil
}

// protectLastAdmin 在事务内保证租户至少保留一名 enabled 的内置 admin 用户。
// remainsAdmin 表示变更后目标用户是否仍是 enabled 内置管理员。
func (s *AccountService) protectLastAdmin(db tenant.TenantDatabase, accountID string, remainsAdmin bool) error {
	isAdmin, err := s.isEnabledBuiltinAdmin(db, accountID)
	if err != nil {
		return err
	}
	if !isAdmin || remainsAdmin {
		return nil
	}
	count, err := s.countEnabledBuiltinAdmins(db)
	if err != nil {
		return err
	}
	if count <= 1 {
		return errs.NewKey(errs.CodeConflict, "user.last_admin_required", "tenant must retain at least one enabled admin")
	}
	return nil
}

func (s *AccountService) isEnabledBuiltinAdmin(db tenant.TenantDatabase, accountID string) (bool, error) {
	rows, err := tenant.TenantRaw[struct{ Count int64 }](db, `SELECT COUNT(*) AS count FROM tenant_accounts u
JOIN tenant_account_roles ur ON ur.account_id = u.id AND ur.tenant_id = u.tenant_id
JOIN roles r ON r.id = ur.role_id AND r.tenant_id = u.tenant_id
WHERE u.tenant_id = ? AND u.id = ? AND u.status = ?
  AND r.code = ? AND r.is_builtin = TRUE AND r.status = ?`,
		accountID, status.ActivationEnabled, BuiltinAdminCode, status.ActivationEnabled)
	return len(rows) == 1 && rows[0].Count > 0, err
}

func (s *AccountService) countEnabledBuiltinAdmins(db tenant.TenantDatabase) (int64, error) {
	rows, err := tenant.TenantRaw[struct{ Count int64 }](db, `SELECT COUNT(DISTINCT u.id) AS count
FROM tenant_accounts u JOIN tenant_account_roles ur ON ur.account_id = u.id AND ur.tenant_id = u.tenant_id
JOIN roles r ON r.id = ur.role_id AND r.tenant_id = u.tenant_id
WHERE u.tenant_id = ? AND u.status = ?
  AND r.code = ? AND r.is_builtin = TRUE AND r.status = ?`,
		status.ActivationEnabled, BuiltinAdminCode, status.ActivationEnabled)
	if err != nil || len(rows) != 1 {
		return 0, err
	}
	return rows[0].Count, nil
}

func replaceAccountRoles(db tenant.TenantDatabase, accountID string, roleIDs []string) error {
	if err := tenant.TenantExec(db, `DELETE FROM tenant_account_roles WHERE tenant_id = ? AND account_id = ?`, accountID); err != nil {
		return err
	}
	for _, roleID := range roleIDs {
		if err := tenant.TenantExec(db, `WITH scoped_role AS (
  SELECT id, tenant_id FROM roles WHERE tenant_id = ? AND id = ?
)
INSERT INTO tenant_account_roles (id, tenant_id, account_id, role_id)
SELECT ?, tenant_id, ?, id FROM scoped_role`, roleID, uuid.NewString(), accountID); err != nil {
			return err
		}
	}
	return nil
}

func accountIDs(accounts []model.TenantAccount) []string {
	ids := make([]string, 0, len(accounts))
	for _, account := range accounts {
		ids = append(ids, account.ID)
	}
	return ids
}

func roleIDs(roles []model.Role) []string {
	ids := make([]string, 0, len(roles))
	for _, role := range roles {
		ids = append(ids, role.ID)
	}
	return ids
}

func hasBuiltinAdmin(roles []model.Role) bool {
	for _, role := range roles {
		if role.Code == BuiltinAdminCode && role.IsBuiltin && role.Status == status.ActivationEnabled {
			return true
		}
	}
	return false
}

func toRoleBriefs(roles []model.Role) []AccountRoleBrief {
	result := make([]AccountRoleBrief, 0, len(roles))
	for _, role := range roles {
		result = append(result, AccountRoleBrief{
			ID: role.ID, Code: role.Code, Name: role.Name, Status: role.Status, IsBuiltin: role.IsBuiltin,
		})
	}
	return result
}

func accountView(account model.TenantAccount, roles []AccountRoleBrief) AccountView {
	if roles == nil {
		roles = []AccountRoleBrief{}
	}
	isAdmin := false
	for _, role := range roles {
		if role.Code == BuiltinAdminCode && role.IsBuiltin && role.Status == status.ActivationEnabled {
			isAdmin = true
			break
		}
	}
	return AccountView{
		ID: account.ID, Username: account.Username, DisplayName: account.DisplayName, Status: account.Status,
		OrgID: account.OrgID, DeptID: account.DeptID, Roles: roles, IsAdmin: isAdmin,
		CreatedAt: account.CreatedAt, UpdatedAt: account.UpdatedAt,
	}
}

func normalizeAccountError(err error, fallback string) error {
	if _, ok := errs.AsAppError(err); ok {
		return err
	}
	return errs.Wrap(errs.CodeInternal, fallback, err)
}

func setAccountAudit(ctx context.Context, accountID, summary string) {
	if ac := auth.MustFromContext(ctx); ac != nil {
		ac.AuditResourceType = "user"
		ac.AuditResourceID = accountID
		ac.AuditSummary = summary
	}
}

// resolveLocale extracts the locale from the request context (Accept-Language header),
// falling back to the system default language.
func (s *AccountService) resolveLocale(ctx context.Context) string {
	if lang := i18n.FromCtx(ctx); lang != "" {
		return lang
	}
	return i18n.DefaultLang()
}

// ── Notification helpers ──

// notifyRoleChanged writes a system.role_changed outbox event.
func (s *AccountService) notifyRoleChanged(ctx context.Context, tx *gorm.DB, accountID, username string, roles []model.Role) error {
	if s.outboxWriter == nil {
		return nil
	}
	roleNames := make([]string, len(roles))
	for i, r := range roles {
		roleNames[i] = r.Name
	}
	payload := map[string]any{
		"definition_key": "system.role_changed",
		"recipients": []map[string]any{
			{"type": "user", "id": accountID},
		},
		"variables": map[string]any{
			"username":  username,
			"role_name": strings.Join(roleNames, ", "),
			"action":    "assigned",
		},
		"locale":          s.resolveLocale(ctx),
		"idempotency_key": fmt.Sprintf("role_changed:%s:%d", accountID, time.Now().UnixMilli()),
	}
	if _, err := s.outboxWriter.Append(ctx, tx, outbox.AppendInput{
		EventType:     "notification.requested.v1",
		AggregateType: "user",
		AggregateID:   accountID,
		Payload:       payload,
	}); err != nil {
		logging.Warn(ctx, "notify role_changed outbox append failed", "user_id", accountID, "error", err)
		return fmt.Errorf("append role changed notification: %w", err)
	}
	return nil
}

// notifyPasswordReset writes a user.password_reset outbox event.
func (s *AccountService) notifyPasswordReset(ctx context.Context, tx *gorm.DB, accountID, username string) error {
	if s.outboxWriter == nil {
		return nil
	}
	payload := map[string]any{
		"definition_key": "user.password_reset",
		"recipients": []map[string]any{
			{"type": "user", "id": accountID},
		},
		"variables": map[string]any{
			"username":   username,
			"reset_link": "", // Phase 1: in-app only, no email link
			"expires_in": "30 minutes",
		},
		"locale":          s.resolveLocale(ctx),
		"idempotency_key": fmt.Sprintf("password_reset:%s:%d", accountID, time.Now().UnixMilli()),
	}
	if _, err := s.outboxWriter.Append(ctx, tx, outbox.AppendInput{
		EventType:     "notification.requested.v1",
		AggregateType: "user",
		AggregateID:   accountID,
		Payload:       payload,
	}); err != nil {
		logging.Warn(ctx, "notify password_reset outbox append failed", "user_id", accountID, "error", err)
		return fmt.Errorf("append password reset notification: %w", err)
	}
	return nil
}
