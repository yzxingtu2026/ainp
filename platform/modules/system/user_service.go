package system

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/yzxingtu2026/ainp/platform/framework/auth"
	"github.com/yzxingtu2026/ainp/platform/framework/errs"
	"github.com/yzxingtu2026/ainp/platform/framework/i18n"
	"github.com/yzxingtu2026/ainp/platform/framework/logging"
	"github.com/yzxingtu2026/ainp/platform/framework/outbox"
	"github.com/yzxingtu2026/ainp/platform/framework/status"
	"github.com/yzxingtu2026/ainp/platform/modules/system/model"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
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
	Current int                     `json:"current"`
	Size    int                     `json:"size"`
	Keyword string                  `json:"keyword"`
	Status  status.ActivationStatus `json:"status"`
	RoleID  string                  `json:"role_id"`
}

type userIDRequest struct {
	ID string `json:"id"`
}

type userCreateRequest struct {
	Username    string                  `json:"username"`
	DisplayName string                  `json:"display_name"`
	Password    string                  `json:"password"`
	Status      status.ActivationStatus `json:"status"`
	RoleIDs     []string                `json:"role_ids"`
	OrgID       string                  `json:"org_id"`
	DeptID      string                  `json:"dept_id"`
}

type userUpdateRequest struct {
	ID          string `json:"id"`
	DisplayName string `json:"display_name"`
	OrgID       string `json:"org_id"`
	DeptID      string `json:"dept_id"`
}

type userStatusRequest struct {
	ID     string                  `json:"id"`
	Status status.ActivationStatus `json:"status"`
}

type userRolesRequest struct {
	ID      string   `json:"id"`
	RoleIDs []string `json:"role_ids"`
}

type userPasswordRequest struct {
	ID       string `json:"id"`
	Password string `json:"password"`
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
	tenantID, _, err := tenantIdentity(ctx)
	if err != nil {
		return nil, err
	}
	var req userListRequest
	if err := decodeUserRequest(input, &req); err != nil {
		return nil, err
	}
	if req.Current < 1 {
		req.Current = 1
	}
	if req.Size < 1 || req.Size > 100 {
		req.Size = 20
	}
	if req.Status != "" && !validateUserStatus(req.Status) {
		return nil, errs.ValidationKey("common.validation_failed", "invalid status")
	}

	query := s.db.WithContext(ctx).Table("users AS u").Where("u.tenant_id = ?", tenantID)
	if keyword := strings.TrimSpace(req.Keyword); keyword != "" {
		like := "%" + keyword + "%"
		query = query.Where("u.username ILIKE ? OR u.display_name ILIKE ?", like, like)
	}
	if req.Status != "" {
		query = query.Where("u.status = ?", req.Status)
	}
	if roleID := strings.TrimSpace(req.RoleID); roleID != "" {
		query = query.Where(
			"EXISTS (SELECT 1 FROM user_roles ur WHERE ur.user_id = u.id AND ur.tenant_id = u.tenant_id AND ur.role_id = ?)",
			roleID,
		)
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, errs.Wrap(errs.CodeInternal, "count users failed", err)
	}

	var users []model.User
	if err := query.Select("u.*").Order("u.created_at ASC, u.id ASC").
		Offset((req.Current - 1) * req.Size).Limit(req.Size).Scan(&users).Error; err != nil {
		return nil, errs.Wrap(errs.CodeInternal, "list users failed", err)
	}

	roleMap, err := s.loadUserRoles(ctx, s.db, tenantID, userIDs(users))
	if err != nil {
		return nil, err
	}
	records := make([]UserView, 0, len(users))
	for _, user := range users {
		records = append(records, userView(user, roleMap[user.ID]))
	}
	return map[string]any{"current": req.Current, "size": req.Size, "total": total, "records": records}, nil
}

func (s *UserService) Get(ctx context.Context, input json.RawMessage) (any, error) {
	tenantID, _, err := tenantIdentity(ctx)
	if err != nil {
		return nil, err
	}
	var req userIDRequest
	if err := decodeUserRequest(input, &req); err != nil {
		return nil, err
	}
	user, err := s.findUser(ctx, s.db, tenantID, req.ID, false)
	if err != nil {
		return nil, err
	}
	roleMap, err := s.loadUserRoles(ctx, s.db, tenantID, []string{user.ID})
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
		roles, err = s.validateAssignableRoles(ctx, tx, tenantID, req.RoleIDs)
		if err != nil {
			return err
		}
		if err := tx.Create(&user).Error; err != nil {
			if strings.Contains(strings.ToLower(err.Error()), "duplicate") {
				return errs.NewKey(errs.CodeConflict, "user.username_conflict", "username already exists")
			}
			return err
		}
		return replaceUserRoles(tx, tenantID, user.ID, roleIDs(roles))
	}); err != nil {
		return nil, normalizeUserError(err, "create user failed")
	}
	return userView(user, toRoleBriefs(roles)), nil
}

func (s *UserService) Update(ctx context.Context, input json.RawMessage) (any, error) {
	tenantID, _, err := tenantIdentity(ctx)
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
		current, err := s.findUser(ctx, tx, tenantID, req.ID, true)
		if err != nil {
			return err
		}
		if err := tx.Model(current).Updates(map[string]any{
			"display_name": req.DisplayName, "org_id": req.OrgID, "dept_id": req.DeptID,
		}).Error; err != nil {
			return err
		}
		current.DisplayName, current.OrgID, current.DeptID = req.DisplayName, req.OrgID, req.DeptID
		user = current
		return nil
	})
	if err != nil {
		return nil, normalizeUserError(err, "update user failed")
	}
	roleMap, err := s.loadUserRoles(ctx, s.db, tenantID, []string{user.ID})
	if err != nil {
		return nil, err
	}
	return userView(*user, roleMap[user.ID]), nil
}

func (s *UserService) UpdateStatus(ctx context.Context, input json.RawMessage) (any, error) {
	tenantID, actorID, err := tenantIdentity(ctx)
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
		user, err := s.findUser(ctx, tx, tenantID, req.ID, true)
		if err != nil {
			return err
		}
		if user.Status == req.Status {
			return nil
		}
		if req.Status == status.ActivationDisabled {
			if err := s.protectLastAdmin(ctx, tx, tenantID, user.ID, false); err != nil {
				return err
			}
		}
		if err := tx.Model(user).Update("status", req.Status).Error; err != nil {
			return err
		}
		return s.auth.BumpAuthVersion(ctx, tx, tenantID, user.ID)
	})
	if err != nil {
		return nil, normalizeUserError(err, "update user status failed")
	}
	return map[string]any{"id": req.ID, "status": req.Status}, nil
}

func (s *UserService) UpdateRoles(ctx context.Context, input json.RawMessage) (any, error) {
	tenantID, _, err := tenantIdentity(ctx)
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
		user, err := s.findUser(ctx, tx, tenantID, req.ID, true)
		if err != nil {
			return err
		}
		targetUsername = user.Username
		roles, err = s.validateAssignableRoles(ctx, tx, tenantID, req.RoleIDs)
		if err != nil {
			return err
		}
		remainsAdmin := user.Status == status.ActivationEnabled && hasBuiltinAdmin(roles)
		if err := s.protectLastAdmin(ctx, tx, tenantID, user.ID, remainsAdmin); err != nil {
			return err
		}
		if err := replaceUserRoles(tx, tenantID, user.ID, roleIDs(roles)); err != nil {
			return err
		}
		if err := s.auth.BumpAuthVersion(ctx, tx, tenantID, user.ID); err != nil {
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
	tenantID, _, err := tenantIdentity(ctx)
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
		user, err := s.findUser(ctx, tx, tenantID, req.ID, true)
		if err != nil {
			return err
		}
		targetUsername = user.Username
		if err := tx.Model(user).Update("password_hash", string(hash)).Error; err != nil {
			return err
		}
		if err := s.auth.BumpAuthVersion(ctx, tx, tenantID, user.ID); err != nil {
			return err
		}
		return s.notifyPasswordReset(ctx, tx, user.ID, targetUsername)
	})
	if err != nil {
		return nil, normalizeUserError(err, "reset user password failed")
	}
	return map[string]any{"id": req.ID}, nil
}

func (s *UserService) findUser(ctx context.Context, tx *gorm.DB, tenantID, userID string, lock bool) (*model.User, error) {
	if userID == "" {
		return nil, errs.ValidationKey("common.validation_failed", "user id is required")
	}
	query := tx.WithContext(ctx).Where("id = ? AND tenant_id = ?", userID, tenantID)
	if lock {
		query = query.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	var user model.User
	if err := query.First(&user).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errs.NewKey(errs.CodeNotFound, "user.not_found", "user not found")
		}
		return nil, err
	}
	return &user, nil
}

func (s *UserService) validateAssignableRoles(ctx context.Context, tx *gorm.DB, tenantID string, values []string) ([]model.Role, error) {
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
	var roles []model.Role
	if err := tx.WithContext(ctx).
		Where("tenant_id = ? AND id IN ? AND status = ?", tenantID, ids, status.ActivationEnabled).
		Find(&roles).Error; err != nil {
		return nil, err
	}
	if len(roles) != len(ids) {
		return nil, errs.ValidationKey("user.role_invalid", "role is invalid or not assignable")
	}
	sort.Slice(roles, func(i, j int) bool { return roles[i].Code < roles[j].Code })
	return roles, nil
}

func (s *UserService) loadUserRoles(ctx context.Context, tx *gorm.DB, tenantID string, ids []string) (map[string][]UserRoleBrief, error) {
	result := make(map[string][]UserRoleBrief, len(ids))
	if len(ids) == 0 {
		return result, nil
	}
	var rows []struct {
		UserID    string                  `gorm:"column:user_id"`
		ID        string                  `gorm:"column:id"`
		Code      string                  `gorm:"column:code"`
		Name      string                  `gorm:"column:name"`
		Status    status.ActivationStatus `gorm:"column:status"`
		IsBuiltin bool                    `gorm:"column:is_builtin"`
	}
	if err := tx.WithContext(ctx).Table("user_roles AS ur").
		Select("ur.user_id, r.id, r.code, r.name, r.status, r.is_builtin").
		Joins("JOIN roles r ON r.id = ur.role_id AND r.tenant_id = ur.tenant_id").
		Where("ur.tenant_id = ? AND ur.user_id IN ?", tenantID, ids).
		Order("r.is_builtin DESC, r.code ASC").
		Scan(&rows).Error; err != nil {
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
func (s *UserService) protectLastAdmin(ctx context.Context, tx *gorm.DB, tenantID, userID string, remainsAdmin bool) error {
	isAdmin, err := s.isEnabledBuiltinAdmin(ctx, tx, tenantID, userID)
	if err != nil {
		return err
	}
	if !isAdmin || remainsAdmin {
		return nil
	}
	count, err := s.countEnabledBuiltinAdmins(ctx, tx, tenantID)
	if err != nil {
		return err
	}
	if count <= 1 {
		return errs.NewKey(errs.CodeConflict, "user.last_admin_required", "tenant must retain at least one enabled admin")
	}
	return nil
}

func (s *UserService) isEnabledBuiltinAdmin(ctx context.Context, tx *gorm.DB, tenantID, userID string) (bool, error) {
	var count int64
	err := tx.WithContext(ctx).Table("users AS u").
		Joins("JOIN user_roles ur ON ur.user_id = u.id AND ur.tenant_id = u.tenant_id").
		Joins("JOIN roles r ON r.id = ur.role_id AND r.tenant_id = u.tenant_id").
		Where("u.id = ? AND u.tenant_id = ? AND u.status = ?", userID, tenantID, status.ActivationEnabled).
		Where("r.code = ? AND r.is_builtin = ? AND r.status = ?", BuiltinAdminCode, true, status.ActivationEnabled).
		Count(&count).Error
	return count > 0, err
}

func (s *UserService) countEnabledBuiltinAdmins(ctx context.Context, tx *gorm.DB, tenantID string) (int64, error) {
	var count int64
	err := tx.WithContext(ctx).Raw(`
		SELECT COUNT(DISTINCT u.id)
		FROM users u
		INNER JOIN user_roles ur ON ur.user_id = u.id AND ur.tenant_id = u.tenant_id
		INNER JOIN roles r ON r.id = ur.role_id AND r.tenant_id = u.tenant_id
		WHERE u.tenant_id = ? AND u.status = ?
		  AND r.code = ? AND r.is_builtin = TRUE AND r.status = ?
	`, tenantID, status.ActivationEnabled, BuiltinAdminCode, status.ActivationEnabled).Scan(&count).Error
	return count, err
}

func replaceUserRoles(tx *gorm.DB, tenantID, userID string, roleIDs []string) error {
	if err := tx.Where("tenant_id = ? AND user_id = ?", tenantID, userID).Delete(&model.UserRole{}).Error; err != nil {
		return err
	}
	for _, roleID := range roleIDs {
		if err := tx.Create(&model.UserRole{ID: uuid.NewString(), TenantID: tenantID, UserID: userID, RoleID: roleID}).Error; err != nil {
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
