package system

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
	"github.com/yzxingtu2026/ainp/platform/modules/system/model"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

var usernamePattern = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9_]{2,63}$`)

type UserService struct {
	db           *gorm.DB
	auth         *AuthService
	outboxWriter outbox.Writer
}

func NewUserService(db *gorm.DB, authService *AuthService) *UserService {
	return &UserService{db: db, auth: authService}
}

// SetOutboxWriter injects the outbox writer for reliable notification dispatch.
func (s *UserService) SetOutboxWriter(w outbox.Writer) {
	s.outboxWriter = w
}

type userListRequest struct {
	pagination.Request
	Keyword string                  `json:"keyword,omitempty"`
	Status  status.ActivationStatus `json:"status,omitempty" validate:"omitempty,oneof=enabled disabled"`
	RoleID  string                  `json:"role_id,omitempty"`
}

type userIDRequest struct {
	ID string `json:"id" validate:"required"`
}

type userCreateRequest struct {
	Username    string                  `json:"username" validate:"required,min=3,max=64"`
	DisplayName string                  `json:"display_name" validate:"required,max=128"`
	Password    string                  `json:"password" validate:"required"`
	Status      status.ActivationStatus `json:"status" validate:"omitempty,oneof=enabled disabled"`
	RoleIDs     []string                `json:"role_ids" validate:"max=20,dive,required"`
	OrgID       string                  `json:"org_id"`
	DeptID      string                  `json:"dept_id"`
}

type userUpdateRequest struct {
	ID          string `json:"id" validate:"required"`
	DisplayName string `json:"display_name" validate:"required,max=128"`
	OrgID       string `json:"org_id"`
	DeptID      string `json:"dept_id"`
}

type selfProfileUpdateRequest struct {
	DisplayName string `json:"display_name" validate:"required,max=128"`
}

type SelfProfileView struct {
	UserView
	MemoryPreferredAddress string `json:"memory_preferred_address"`
	EffectiveAddress       string `json:"effective_address"`
	AddressSource          string `json:"address_source"`
}

func (s *UserService) SelfProfile(ctx context.Context, _ json.RawMessage) (any, error) {
	_, userID, err := tenantIdentity(ctx)
	if err != nil {
		return nil, err
	}
	tdb, err := tenant.TenantDB(ctx, s.db)
	if err != nil {
		return nil, err
	}
	user, err := s.findUser(tdb, userID, false)
	if err != nil {
		return nil, err
	}
	roleMap, err := s.loadUserRoles(tdb, []string{userID})
	if err != nil {
		return nil, err
	}
	view := SelfProfileView{UserView: userView(*user, roleMap[userID])}
	rows, qerr := tenant.TenantRaw[struct{ PreferredAddress string }](tdb, `SELECT COALESCE(profile_json->>'preferred_address','') AS preferred_address
FROM ai_memory_current_profiles WHERE tenant_id = ? AND user_id = ? AND agent_id IS NULL AND status = 'ready'
ORDER BY source_revision DESC LIMIT 1`, userID)
	if qerr == nil && len(rows) > 0 {
		view.MemoryPreferredAddress = strings.TrimSpace(rows[0].PreferredAddress)
	}
	view.EffectiveAddress, view.AddressSource = view.MemoryPreferredAddress, "memory_profile"
	if view.EffectiveAddress == "" {
		view.EffectiveAddress, view.AddressSource = user.DisplayName, "display_name"
	}
	if view.EffectiveAddress == "" {
		view.EffectiveAddress, view.AddressSource = user.Username, "username"
	}
	return view, nil
}

func (s *UserService) UpdateSelfProfile(ctx context.Context, input json.RawMessage) (any, error) {
	_, userID, err := tenantIdentity(ctx)
	if err != nil {
		return nil, err
	}
	var req selfProfileUpdateRequest
	if err := decodeUserRequest(input, &req); err != nil {
		return nil, err
	}
	req.DisplayName = strings.TrimSpace(req.DisplayName)
	if req.DisplayName == "" {
		return nil, errs.ValidationKey("common.validation_failed", "display_name is required")
	}
	setUserAudit(ctx, userID, "self profile updated")
	tdb, err := tenant.TenantDB(ctx, s.db)
	if err != nil {
		return nil, err
	}
	if err := tenant.TenantExec(tdb, `WITH scoped_user AS (
  SELECT id FROM users WHERE tenant_id = ? AND id = ?
)
UPDATE users SET display_name = ?, updated_at = ?
WHERE id IN (SELECT id FROM scoped_user)`, userID, req.DisplayName, time.Now().UTC()); err != nil {
		return nil, normalizeUserError(err, "update self profile failed")
	}
	return s.SelfProfile(ctx, nil)
}

type userStatusRequest struct {
	ID     string                  `json:"id" validate:"required"`
	Status status.ActivationStatus `json:"status" validate:"required,oneof=enabled disabled"`
}

type userRolesRequest struct {
	ID      string   `json:"id" validate:"required"`
	RoleIDs []string `json:"role_ids" validate:"max=20,dive,required"`
}

type userPasswordRequest struct {
	ID       string `json:"id" validate:"required"`
	Password string `json:"password" validate:"required"`
}

type UserRoleBrief struct {
	ID        string                  `json:"id"`
	Code      string                  `json:"code"`
	Name      string                  `json:"name"`
	Status    status.ActivationStatus `json:"status"`
	IsBuiltin bool                    `json:"is_builtin"`
}

type UserView struct {
	ID          string                  `json:"id"`
	Username    string                  `json:"username"`
	DisplayName string                  `json:"display_name"`
	Status      status.ActivationStatus `json:"status"`
	OrgID       string                  `json:"org_id,omitempty"`
	DeptID      string                  `json:"dept_id,omitempty"`
	Roles       []UserRoleBrief         `json:"roles"`
	IsAdmin     bool                    `json:"is_admin"`
	CreatedAt   time.Time               `json:"created_at"`
	UpdatedAt   time.Time               `json:"updated_at"`
}

func decodeUserRequest(input json.RawMessage, target any) error {
	if err := json.Unmarshal(input, target); err != nil {
		return errs.ValidationKey("common.validation_failed", "invalid json")
	}
	return nil
}

func validateUserStatus(value status.ActivationStatus) bool {
	return value == status.ActivationEnabled || value == status.ActivationDisabled
}

func (s *UserService) List(ctx context.Context, input json.RawMessage) (any, error) {
	return action.JSON(s.list)(ctx, input)
}

func (s *UserService) list(ctx context.Context, req userListRequest) (any, error) {
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
	counts, err := tenant.TenantRaw[struct{ Count int64 }](tdb, `SELECT COUNT(*) AS count FROM users u
WHERE u.tenant_id = ? AND (? = '' OR u.username ILIKE ? OR u.display_name ILIKE ?)
  AND (? = '' OR u.status = ?)
  AND (? = '' OR EXISTS (
    SELECT 1 FROM user_roles ur WHERE ur.tenant_id = u.tenant_id AND ur.user_id = u.id AND ur.role_id = ?
  ))`, keyword, like, like, req.Status, req.Status, roleID, roleID)
	if err != nil {
		return nil, errs.Wrap(errs.CodeInternal, "count users failed", err)
	}
	var total int64
	if len(counts) == 1 {
		total = counts[0].Count
	}
	users, err := tenant.TenantRaw[model.User](tdb, `SELECT u.* FROM users u
WHERE u.tenant_id = ? AND (? = '' OR u.username ILIKE ? OR u.display_name ILIKE ?)
  AND (? = '' OR u.status = ?)
  AND (? = '' OR EXISTS (
    SELECT 1 FROM user_roles ur WHERE ur.tenant_id = u.tenant_id AND ur.user_id = u.id AND ur.role_id = ?
  ))
ORDER BY u.created_at ASC, u.id ASC OFFSET ? LIMIT ?`, keyword, like, like, req.Status, req.Status, roleID, roleID, req.Offset(), req.PageSize)
	if err != nil {
		return nil, errs.Wrap(errs.CodeInternal, "list users failed", err)
	}
	roleMap, err := s.loadUserRoles(tdb, userIDs(users))
	if err != nil {
		return nil, err
	}
	records := make([]UserView, 0, len(users))
	for _, user := range users {
		records = append(records, userView(user, roleMap[user.ID]))
	}
	return pagination.NewResult(records, total, req.Request), nil
}

func (s *UserService) Get(ctx context.Context, input json.RawMessage) (any, error) {
	_, _, err := tenantIdentity(ctx)
	if err != nil {
		return nil, err
	}
	var req userIDRequest
	if err := decodeUserRequest(input, &req); err != nil {
		return nil, err
	}
	tdb, err := tenant.TenantDB(ctx, s.db)
	if err != nil {
		return nil, err
	}
	user, err := s.findUser(tdb, req.ID, false)
	if err != nil {
		return nil, err
	}
	roleMap, err := s.loadUserRoles(tdb, []string{user.ID})
	if err != nil {
		return nil, err
	}
	return userView(*user, roleMap[user.ID]), nil
}

func (s *UserService) Create(ctx context.Context, input json.RawMessage) (any, error) {
	tenantID, _, err := tenantIdentity(ctx)
	if err != nil {
		return nil, err
	}
	var req userCreateRequest
	if err := decodeUserRequest(input, &req); err != nil {
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
	if !validateUserStatus(req.Status) {
		return nil, errs.ValidationKey("common.validation_failed", "invalid status")
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		return nil, errs.Wrap(errs.CodeInternal, "hash password failed", err)
	}
	user := model.User{
		ID: uuid.NewString(), TenantID: tenantID, Username: req.Username, PasswordHash: string(hash),
		DisplayName: req.DisplayName, Status: req.Status, AuthVersion: 1, OrgID: req.OrgID, DeptID: req.DeptID,
	}
	var roles []model.Role
	setUserAudit(ctx, user.ID, "user created")
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
		user.CreatedAt, user.UpdatedAt = now, now
		if err := tenant.TenantExec(tdb, `WITH scoped_tenant AS (SELECT ?::text AS tenant_id)
INSERT INTO users (id, tenant_id, username, password_hash, display_name, status, auth_version, org_id, dept_id, created_at, updated_at)
SELECT ?, tenant_id, ?, ?, ?, ?, 1, ?, ?, ?, ? FROM scoped_tenant`,
			user.ID, user.Username, user.PasswordHash, user.DisplayName, user.Status, user.OrgID, user.DeptID, now, now); err != nil {
			if strings.Contains(strings.ToLower(err.Error()), "duplicate") {
				return errs.NewKey(errs.CodeConflict, "user.username_conflict", "username already exists")
			}
			return err
		}
		return replaceUserRoles(tdb, user.ID, roleIDs(roles))
	}); err != nil {
		return nil, normalizeUserError(err, "create user failed")
	}
	return userView(user, toRoleBriefs(roles)), nil
}

func (s *UserService) Update(ctx context.Context, input json.RawMessage) (any, error) {
	_, _, err := tenantIdentity(ctx)
	if err != nil {
		return nil, err
	}
	var req userUpdateRequest
	if err := decodeUserRequest(input, &req); err != nil {
		return nil, err
	}
	req.DisplayName = strings.TrimSpace(req.DisplayName)
	req.OrgID = strings.TrimSpace(req.OrgID)
	req.DeptID = strings.TrimSpace(req.DeptID)
	if req.DisplayName == "" {
		return nil, errs.ValidationKey("common.validation_failed", "display_name is required")
	}
	var user *model.User
	setUserAudit(ctx, req.ID, "user updated")
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		tdb, scopeErr := tenant.TenantDB(ctx, tx)
		if scopeErr != nil {
			return scopeErr
		}
		current, err := s.findUser(tdb, req.ID, true)
		if err != nil {
			return err
		}
		if err := tenant.TenantExec(tdb, `WITH scoped_user AS (
  SELECT id FROM users WHERE tenant_id = ? AND id = ?
)
UPDATE users SET display_name = ?, org_id = ?, dept_id = ?, updated_at = ?
WHERE id IN (SELECT id FROM scoped_user)`, current.ID, req.DisplayName, req.OrgID, req.DeptID, time.Now().UTC()); err != nil {
			return err
		}
		current.DisplayName, current.OrgID, current.DeptID = req.DisplayName, req.OrgID, req.DeptID
		current.UpdatedAt = time.Now().UTC()
		user = current
		return nil
	})
	if err != nil {
		return nil, normalizeUserError(err, "update user failed")
	}
	tdb, err := tenant.TenantDB(ctx, s.db)
	if err != nil {
		return nil, err
	}
	roleMap, err := s.loadUserRoles(tdb, []string{user.ID})
	if err != nil {
		return nil, err
	}
	return userView(*user, roleMap[user.ID]), nil
}

func (s *UserService) UpdateStatus(ctx context.Context, input json.RawMessage) (any, error) {
	_, actorID, err := tenantIdentity(ctx)
	if err != nil {
		return nil, err
	}
	var req userStatusRequest
	if err := decodeUserRequest(input, &req); err != nil {
		return nil, err
	}
	if !validateUserStatus(req.Status) {
		return nil, errs.ValidationKey("common.validation_failed", "invalid status")
	}
	if req.ID == actorID && req.Status == status.ActivationDisabled {
		return nil, errs.NewKey(errs.CodeConflict, "user.self_disable_forbidden", "cannot disable yourself")
	}
	setUserAudit(ctx, req.ID, "user status updated")
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		tdb, scopeErr := tenant.TenantDB(ctx, tx)
		if scopeErr != nil {
			return scopeErr
		}
		user, err := s.findUser(tdb, req.ID, true)
		if err != nil {
			return err
		}
		if user.Status == req.Status {
			return nil
		}
		if req.Status == status.ActivationDisabled {
			if err := s.protectLastAdmin(tdb, user.ID, false); err != nil {
				return err
			}
		}
		if err := tenant.TenantExec(tdb, `WITH scoped_user AS (
  SELECT id FROM users WHERE tenant_id = ? AND id = ?
)
UPDATE users SET status = ?, updated_at = ? WHERE id IN (SELECT id FROM scoped_user)`,
			user.ID, req.Status, time.Now().UTC()); err != nil {
			return err
		}
		return s.auth.BumpAuthVersionScoped(tdb, user.ID)
	})
	if err != nil {
		return nil, normalizeUserError(err, "update user status failed")
	}
	return map[string]any{"id": req.ID, "status": req.Status}, nil
}

func (s *UserService) UpdateRoles(ctx context.Context, input json.RawMessage) (any, error) {
	_, _, err := tenantIdentity(ctx)
	if err != nil {
		return nil, err
	}
	var req userRolesRequest
	if err := decodeUserRequest(input, &req); err != nil {
		return nil, err
	}
	var roles []model.Role
	var targetUsername string
	setUserAudit(ctx, req.ID, "user roles updated")
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		tdb, scopeErr := tenant.TenantDB(ctx, tx)
		if scopeErr != nil {
			return scopeErr
		}
		user, err := s.findUser(tdb, req.ID, true)
		if err != nil {
			return err
		}
		targetUsername = user.Username
		roles, err = s.validateAssignableRoles(tdb, req.RoleIDs)
		if err != nil {
			return err
		}
		remainsAdmin := user.Status == status.ActivationEnabled && hasBuiltinAdmin(roles)
		if err := s.protectLastAdmin(tdb, user.ID, remainsAdmin); err != nil {
			return err
		}
		if err := replaceUserRoles(tdb, user.ID, roleIDs(roles)); err != nil {
			return err
		}
		if err := s.auth.BumpAuthVersionScoped(tdb, user.ID); err != nil {
			return err
		}
		return s.notifyRoleChanged(ctx, tx, user.ID, targetUsername, roles)
	})
	if err != nil {
		return nil, normalizeUserError(err, "update user roles failed")
	}
	return map[string]any{"id": req.ID, "roles": toRoleBriefs(roles)}, nil
}

func (s *UserService) ResetPassword(ctx context.Context, input json.RawMessage) (any, error) {
	_, _, err := tenantIdentity(ctx)
	if err != nil {
		return nil, err
	}
	var req userPasswordRequest
	if err := decodeUserRequest(input, &req); err != nil {
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
	setUserAudit(ctx, req.ID, "user password reset")
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		tdb, scopeErr := tenant.TenantDB(ctx, tx)
		if scopeErr != nil {
			return scopeErr
		}
		user, err := s.findUser(tdb, req.ID, true)
		if err != nil {
			return err
		}
		targetUsername = user.Username
		if err := tenant.TenantExec(tdb, `WITH scoped_user AS (
  SELECT id FROM users WHERE tenant_id = ? AND id = ?
)
UPDATE users SET password_hash = ?, updated_at = ? WHERE id IN (SELECT id FROM scoped_user)`,
			user.ID, string(hash), time.Now().UTC()); err != nil {
			return err
		}
		if err := s.auth.BumpAuthVersionScoped(tdb, user.ID); err != nil {
			return err
		}
		return s.notifyPasswordReset(ctx, tx, user.ID, targetUsername)
	})
	if err != nil {
		return nil, normalizeUserError(err, "reset user password failed")
	}
	return map[string]any{"id": req.ID}, nil
}

func (s *UserService) findUser(db tenant.TenantDatabase, userID string, lock bool) (*model.User, error) {
	if userID == "" {
		return nil, errs.ValidationKey("common.validation_failed", "user id is required")
	}
	sql := `SELECT * FROM users WHERE tenant_id = ? AND id = ?`
	if lock {
		sql += ` FOR UPDATE`
	}
	rows, err := tenant.TenantRaw[model.User](db, sql, userID)
	if err != nil {
		return nil, err
	}
	if len(rows) != 1 {
		return nil, errs.NewKey(errs.CodeNotFound, "user.not_found", "user not found")
	}
	return &rows[0], nil
}

func (s *UserService) validateAssignableRoles(db tenant.TenantDatabase, values []string) ([]model.Role, error) {
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

func (s *UserService) loadUserRoles(db tenant.TenantDatabase, ids []string) (map[string][]UserRoleBrief, error) {
	result := make(map[string][]UserRoleBrief, len(ids))
	if len(ids) == 0 {
		return result, nil
	}
	rows, err := tenant.TenantRaw[struct {
		UserID    string                  `gorm:"column:user_id"`
		ID        string                  `gorm:"column:id"`
		Code      string                  `gorm:"column:code"`
		Name      string                  `gorm:"column:name"`
		Status    status.ActivationStatus `gorm:"column:status"`
		IsBuiltin bool                    `gorm:"column:is_builtin"`
	}](db, `SELECT ur.user_id, r.id, r.code, r.name, r.status, r.is_builtin
FROM user_roles ur JOIN roles r ON r.id = ur.role_id AND r.tenant_id = ur.tenant_id
WHERE ur.tenant_id = ? AND ur.user_id IN ? ORDER BY r.is_builtin DESC, r.code ASC`, ids)
	if err != nil {
		return nil, errs.Wrap(errs.CodeInternal, "load user roles failed", err)
	}
	for _, row := range rows {
		result[row.UserID] = append(result[row.UserID], UserRoleBrief{
			ID: row.ID, Code: row.Code, Name: row.Name, Status: row.Status, IsBuiltin: row.IsBuiltin,
		})
	}
	return result, nil
}

// protectLastAdmin 在事务内保证租户至少保留一名 enabled 的内置 admin 用户。
// remainsAdmin 表示变更后目标用户是否仍是 enabled 内置管理员。
func (s *UserService) protectLastAdmin(db tenant.TenantDatabase, userID string, remainsAdmin bool) error {
	isAdmin, err := s.isEnabledBuiltinAdmin(db, userID)
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

func (s *UserService) isEnabledBuiltinAdmin(db tenant.TenantDatabase, userID string) (bool, error) {
	rows, err := tenant.TenantRaw[struct{ Count int64 }](db, `SELECT COUNT(*) AS count FROM users u
JOIN user_roles ur ON ur.user_id = u.id AND ur.tenant_id = u.tenant_id
JOIN roles r ON r.id = ur.role_id AND r.tenant_id = u.tenant_id
WHERE u.tenant_id = ? AND u.id = ? AND u.status = ?
  AND r.code = ? AND r.is_builtin = TRUE AND r.status = ?`,
		userID, status.ActivationEnabled, BuiltinAdminCode, status.ActivationEnabled)
	return len(rows) == 1 && rows[0].Count > 0, err
}

func (s *UserService) countEnabledBuiltinAdmins(db tenant.TenantDatabase) (int64, error) {
	rows, err := tenant.TenantRaw[struct{ Count int64 }](db, `SELECT COUNT(DISTINCT u.id) AS count
FROM users u JOIN user_roles ur ON ur.user_id = u.id AND ur.tenant_id = u.tenant_id
JOIN roles r ON r.id = ur.role_id AND r.tenant_id = u.tenant_id
WHERE u.tenant_id = ? AND u.status = ?
  AND r.code = ? AND r.is_builtin = TRUE AND r.status = ?`,
		status.ActivationEnabled, BuiltinAdminCode, status.ActivationEnabled)
	if err != nil || len(rows) != 1 {
		return 0, err
	}
	return rows[0].Count, nil
}

func replaceUserRoles(db tenant.TenantDatabase, userID string, roleIDs []string) error {
	if err := tenant.TenantExec(db, `DELETE FROM user_roles WHERE tenant_id = ? AND user_id = ?`, userID); err != nil {
		return err
	}
	for _, roleID := range roleIDs {
		if err := tenant.TenantExec(db, `WITH scoped_role AS (
  SELECT id, tenant_id FROM roles WHERE tenant_id = ? AND id = ?
)
INSERT INTO user_roles (id, tenant_id, user_id, role_id)
SELECT ?, tenant_id, ?, id FROM scoped_role`, roleID, uuid.NewString(), userID); err != nil {
			return err
		}
	}
	return nil
}

func userIDs(users []model.User) []string {
	ids := make([]string, 0, len(users))
	for _, user := range users {
		ids = append(ids, user.ID)
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

func toRoleBriefs(roles []model.Role) []UserRoleBrief {
	result := make([]UserRoleBrief, 0, len(roles))
	for _, role := range roles {
		result = append(result, UserRoleBrief{
			ID: role.ID, Code: role.Code, Name: role.Name, Status: role.Status, IsBuiltin: role.IsBuiltin,
		})
	}
	return result
}

func userView(user model.User, roles []UserRoleBrief) UserView {
	if roles == nil {
		roles = []UserRoleBrief{}
	}
	isAdmin := false
	for _, role := range roles {
		if role.Code == BuiltinAdminCode && role.IsBuiltin && role.Status == status.ActivationEnabled {
			isAdmin = true
			break
		}
	}
	return UserView{
		ID: user.ID, Username: user.Username, DisplayName: user.DisplayName, Status: user.Status,
		OrgID: user.OrgID, DeptID: user.DeptID, Roles: roles, IsAdmin: isAdmin,
		CreatedAt: user.CreatedAt, UpdatedAt: user.UpdatedAt,
	}
}

func normalizeUserError(err error, fallback string) error {
	if _, ok := errs.AsAppError(err); ok {
		return err
	}
	return errs.Wrap(errs.CodeInternal, fallback, err)
}

func setUserAudit(ctx context.Context, userID, summary string) {
	if ac := auth.MustFromContext(ctx); ac != nil {
		ac.AuditResourceType = "user"
		ac.AuditResourceID = userID
		ac.AuditSummary = summary
	}
}

// resolveLocale extracts the locale from the request context (Accept-Language header),
// falling back to the system default language.
func (s *UserService) resolveLocale(ctx context.Context) string {
	if lang := i18n.FromCtx(ctx); lang != "" {
		return lang
	}
	return i18n.DefaultLang()
}

// ── Notification helpers ──

// notifyRoleChanged writes a system.role_changed outbox event.
func (s *UserService) notifyRoleChanged(ctx context.Context, tx *gorm.DB, userID, username string, roles []model.Role) error {
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
			{"type": "user", "id": userID},
		},
		"variables": map[string]any{
			"username":  username,
			"role_name": strings.Join(roleNames, ", "),
			"action":    "assigned",
		},
		"locale":          s.resolveLocale(ctx),
		"idempotency_key": fmt.Sprintf("role_changed:%s:%d", userID, time.Now().UnixMilli()),
	}
	if _, err := s.outboxWriter.Append(ctx, tx, outbox.AppendInput{
		EventType:     "notification.requested.v1",
		AggregateType: "user",
		AggregateID:   userID,
		Payload:       payload,
	}); err != nil {
		logging.Warn(ctx, "notify role_changed outbox append failed", "user_id", userID, "error", err)
		return fmt.Errorf("append role changed notification: %w", err)
	}
	return nil
}

// notifyPasswordReset writes a user.password_reset outbox event.
func (s *UserService) notifyPasswordReset(ctx context.Context, tx *gorm.DB, userID, username string) error {
	if s.outboxWriter == nil {
		return nil
	}
	payload := map[string]any{
		"definition_key": "user.password_reset",
		"recipients": []map[string]any{
			{"type": "user", "id": userID},
		},
		"variables": map[string]any{
			"username":   username,
			"reset_link": "", // Phase 1: in-app only, no email link
			"expires_in": "30 minutes",
		},
		"locale":          s.resolveLocale(ctx),
		"idempotency_key": fmt.Sprintf("password_reset:%s:%d", userID, time.Now().UnixMilli()),
	}
	if _, err := s.outboxWriter.Append(ctx, tx, outbox.AppendInput{
		EventType:     "notification.requested.v1",
		AggregateType: "user",
		AggregateID:   userID,
		Payload:       payload,
	}); err != nil {
		logging.Warn(ctx, "notify password_reset outbox append failed", "user_id", userID, "error", err)
		return fmt.Errorf("append password reset notification: %w", err)
	}
	return nil
}
