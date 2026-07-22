package system

import (
	"context"
	"encoding/json"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/yzxingtu2026/ainp/platform/framework/action"
	"github.com/yzxingtu2026/ainp/platform/framework/auth"
	"github.com/yzxingtu2026/ainp/platform/framework/capability"
	"github.com/yzxingtu2026/ainp/platform/framework/errs"
	"github.com/yzxingtu2026/ainp/platform/framework/i18n"
	"github.com/yzxingtu2026/ainp/platform/framework/pagination"
	"github.com/yzxingtu2026/ainp/platform/framework/status"
	"github.com/yzxingtu2026/ainp/platform/framework/tenant"
	"github.com/yzxingtu2026/ainp/platform/modules/system/model"
	"gorm.io/gorm"
)

var roleCodePattern = regexp.MustCompile(`^[a-z][a-z0-9_]{1,63}$`)

type RoleService struct {
	db           *gorm.DB
	auth         *AuthService
	actions      *action.Registry
	capabilities *capability.Registry
}

func NewRoleService(db *gorm.DB, authService *AuthService, actions *action.Registry, capabilities *capability.Registry) *RoleService {
	return &RoleService{db: db, auth: authService, actions: actions, capabilities: capabilities}
}

type roleListRequest struct {
	pagination.Request
	Keyword string                  `json:"keyword,omitempty"`
	Status  status.ActivationStatus `json:"status,omitempty" validate:"omitempty,oneof=enabled disabled"`
}

type roleIDRequest struct {
	ID string `json:"id" validate:"required"`
}

type roleCreateRequest struct {
	Code        string                  `json:"code" validate:"required,min=2,max=64"`
	Name        string                  `json:"name" validate:"required,max=128"`
	Description string                  `json:"description"`
	Status      status.ActivationStatus `json:"status" validate:"omitempty,oneof=enabled disabled"`
	Permissions []string                `json:"permissions" validate:"max=200,dive,required"`
}

type roleUpdateRequest struct {
	ID          string                  `json:"id" validate:"required"`
	Name        string                  `json:"name" validate:"required,max=128"`
	Description string                  `json:"description"`
	Status      status.ActivationStatus `json:"status" validate:"required,oneof=enabled disabled"`
}

type rolePermissionsRequest struct {
	ID          string   `json:"id" validate:"required"`
	Permissions []string `json:"permissions" validate:"max=200,dive,required"`
}

type RoleView struct {
	ID          string                  `json:"id"`
	Code        string                  `json:"code"`
	Name        string                  `json:"name"`
	Description string                  `json:"description"`
	Status      status.ActivationStatus `json:"status"`
	IsBuiltin   bool                    `json:"is_builtin"`
	UserCount   int64                   `json:"user_count"`
	Permissions []string                `json:"permissions,omitempty"`
	CreatedAt   time.Time               `json:"created_at"`
	UpdatedAt   time.Time               `json:"updated_at"`
}

type PermissionCatalogItem struct {
	Permission     string   `json:"permission"`
	Group          string   `json:"group"`
	GroupName      string   `json:"group_name"`
	GroupNameKey   string   `json:"group_name_key,omitempty"`
	ActionIDs      []string `json:"action_ids"`
	Name           string   `json:"name"`
	NameKey        string   `json:"name_key,omitempty"`
	Description    string   `json:"description,omitempty"`
	DescriptionKey string   `json:"description_key,omitempty"`
	Risk           string   `json:"risk,omitempty"`
	Audit          bool     `json:"audit"`
}

type PermissionCatalogGroup struct {
	Capability     string                  `json:"capability"`
	Name           string                  `json:"name"`
	NameKey        string                  `json:"name_key,omitempty"`
	Description    string                  `json:"description,omitempty"`
	DescriptionKey string                  `json:"description_key,omitempty"`
	Items          []PermissionCatalogItem `json:"items"`
}

func decodeRoleRequest(input json.RawMessage, target any) error {
	if err := json.Unmarshal(input, target); err != nil {
		return errs.ValidationKey("common.validation_failed", "invalid json")
	}
	return nil
}

func tenantIdentity(ctx context.Context) (string, string, error) {
	ac := auth.MustFromContext(ctx)
	if !ac.Authenticated || ac.Realm != auth.RealmTenant || ac.TenantID == "" || ac.UserID == "" {
		return "", "", errs.NewKey(errs.CodeUnauthorized, "common.unauthorized", "unauthorized")
	}
	return ac.TenantID, ac.UserID, nil
}

func validateRoleStatus(value status.ActivationStatus) bool {
	return value == status.ActivationEnabled || value == status.ActivationDisabled
}

func (s *RoleService) List(ctx context.Context, input json.RawMessage) (any, error) {
	return action.JSON(s.list)(ctx, input)
}

func (s *RoleService) list(ctx context.Context, req roleListRequest) (any, error) {
	_, _, err := tenantIdentity(ctx)
	if err != nil {
		return nil, err
	}
	tdb, err := tenant.TenantDB(ctx, s.db)
	if err != nil {
		return nil, err
	}
	keyword := strings.TrimSpace(req.Keyword)
	like := "%" + keyword + "%"
	counts, err := tenant.TenantRaw[struct{ Count int64 }](tdb, `SELECT COUNT(*) AS count FROM roles r
WHERE r.tenant_id = ? AND (? = '' OR r.code ILIKE ? OR r.name ILIKE ?)
  AND (? = '' OR r.status = ?)`, keyword, like, like, req.Status, req.Status)
	if err != nil {
		return nil, errs.Wrap(errs.CodeInternal, "count roles failed", err)
	}
	var total int64
	if len(counts) == 1 {
		total = counts[0].Count
	}
	rows, err := tenant.TenantRaw[struct {
		model.Role
		UserCount int64 `gorm:"column:user_count"`
	}](tdb, `SELECT r.*, COUNT(ur.id) AS user_count FROM roles r
LEFT JOIN user_roles ur ON ur.role_id = r.id AND ur.tenant_id = r.tenant_id
WHERE r.tenant_id = ? AND (? = '' OR r.code ILIKE ? OR r.name ILIKE ?)
  AND (? = '' OR r.status = ?)
GROUP BY r.id ORDER BY r.is_builtin DESC, r.created_at ASC, r.id ASC OFFSET ? LIMIT ?`,
		keyword, like, like, req.Status, req.Status, req.Offset(), req.PageSize)
	if err != nil {
		return nil, errs.Wrap(errs.CodeInternal, "list roles failed", err)
	}
	records := make([]RoleView, 0, len(rows))
	for _, row := range rows {
		records = append(records, roleView(row.Role, row.UserCount, nil))
	}
	return pagination.NewResult(records, total, req.Request), nil
}

func (s *RoleService) Get(ctx context.Context, input json.RawMessage) (any, error) {
	_, _, err := tenantIdentity(ctx)
	if err != nil {
		return nil, err
	}
	var req roleIDRequest
	if err := decodeRoleRequest(input, &req); err != nil {
		return nil, err
	}
	tdb, err := tenant.TenantDB(ctx, s.db)
	if err != nil {
		return nil, err
	}
	role, err := s.findRole(tdb, req.ID, false)
	if err != nil {
		return nil, err
	}
	permissions, err := s.rolePermissions(tdb, role.ID)
	if err != nil {
		return nil, err
	}
	counts, err := tenant.TenantRaw[struct{ Count int64 }](tdb,
		`SELECT COUNT(*) AS count FROM user_roles WHERE tenant_id = ? AND role_id = ?`, role.ID)
	if err != nil {
		return nil, errs.Wrap(errs.CodeInternal, "count role users failed", err)
	}
	var count int64
	if len(counts) == 1 {
		count = counts[0].Count
	}
	return roleView(*role, count, permissions), nil
}

func (s *RoleService) Create(ctx context.Context, input json.RawMessage) (any, error) {
	tenantID, _, err := tenantIdentity(ctx)
	if err != nil {
		return nil, err
	}
	var req roleCreateRequest
	if err := decodeRoleRequest(input, &req); err != nil {
		return nil, err
	}
	req.Code = strings.TrimSpace(req.Code)
	req.Name = strings.TrimSpace(req.Name)
	if !roleCodePattern.MatchString(req.Code) || req.Name == "" {
		return nil, errs.ValidationKey("common.validation_failed", "invalid role code or name")
	}
	if req.Status == "" {
		req.Status = status.ActivationEnabled
	}
	if !validateRoleStatus(req.Status) {
		return nil, errs.ValidationKey("common.validation_failed", "invalid status")
	}
	permissions, err := s.validatePermissions(req.Permissions)
	if err != nil {
		return nil, err
	}
	role := model.Role{ID: uuid.NewString(), TenantID: tenantID, Code: req.Code, Name: req.Name, Description: strings.TrimSpace(req.Description), Status: req.Status}
	setRoleAudit(ctx, role.ID, "role created")
	tdb, err := tenant.TenantDB(ctx, s.db)
	if err != nil {
		return nil, err
	}
	if err := tdb.Transaction(func(tx tenant.TenantDatabase) error {
		if err := tenant.TenantExec(tx, `WITH scoped_tenant AS (SELECT ?::text AS tenant_id)
INSERT INTO roles
(id, tenant_id, code, name, description, status, is_builtin, created_at, updated_at)
SELECT ?, tenant_id, ?, ?, ?, ?, FALSE, ?, ? FROM scoped_tenant`, role.ID, role.Code, role.Name, role.Description, role.Status, time.Now().UTC(), time.Now().UTC()); err != nil {
			if strings.Contains(strings.ToLower(err.Error()), "duplicate") {
				return errs.NewKey(errs.CodeConflict, "role.code_conflict", "role code already exists")
			}
			return err
		}
		return replaceRolePermissions(tx, role.ID, permissions)
	}); err != nil {
		if _, ok := errs.AsAppError(err); ok {
			return nil, err
		}
		return nil, errs.Wrap(errs.CodeInternal, "create role failed", err)
	}
	return roleView(role, 0, permissions), nil
}

func (s *RoleService) Update(ctx context.Context, input json.RawMessage) (any, error) {
	_, _, err := tenantIdentity(ctx)
	if err != nil {
		return nil, err
	}
	var req roleUpdateRequest
	if err := decodeRoleRequest(input, &req); err != nil {
		return nil, err
	}
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" || !validateRoleStatus(req.Status) {
		return nil, errs.ValidationKey("common.validation_failed", "invalid role name or status")
	}
	var role *model.Role
	setRoleAudit(ctx, req.ID, "role updated")
	tdb, err := tenant.TenantDB(ctx, s.db)
	if err != nil {
		return nil, err
	}
	err = tdb.Transaction(func(tx tenant.TenantDatabase) error {
		current, err := s.findRole(tx, req.ID, true)
		if err != nil {
			return err
		}
		if current.IsBuiltin && req.Status != status.ActivationEnabled {
			return errs.NewKey(errs.CodeConflict, "role.builtin_protected", "built-in role cannot be disabled")
		}
		statusChanged := current.Status != req.Status
		if err := tenant.TenantExec(tx, `WITH scoped_role AS (
  SELECT id FROM roles WHERE tenant_id = ? AND id = ?
)
UPDATE roles SET name = ?, description = ?, status = ?, updated_at = ?
WHERE id IN (SELECT id FROM scoped_role)`, current.ID, req.Name, strings.TrimSpace(req.Description), req.Status, time.Now().UTC()); err != nil {
			return err
		}
		if statusChanged {
			if err := s.auth.BumpAuthVersionForRoleScoped(tx, current.ID); err != nil {
				return err
			}
		}
		current.Name, current.Description, current.Status = req.Name, strings.TrimSpace(req.Description), req.Status
		role = current
		return nil
	})
	if err != nil {
		return nil, normalizeRoleError(err, "update role failed")
	}
	return roleView(*role, 0, nil), nil
}

func (s *RoleService) Delete(ctx context.Context, input json.RawMessage) (any, error) {
	_, _, err := tenantIdentity(ctx)
	if err != nil {
		return nil, err
	}
	var req roleIDRequest
	if err := decodeRoleRequest(input, &req); err != nil {
		return nil, err
	}
	setRoleAudit(ctx, req.ID, "role deleted")
	tdb, err := tenant.TenantDB(ctx, s.db)
	if err != nil {
		return nil, err
	}
	err = tdb.Transaction(func(tx tenant.TenantDatabase) error {
		role, err := s.findRole(tx, req.ID, true)
		if err != nil {
			return err
		}
		if role.IsBuiltin {
			return errs.NewKey(errs.CodeConflict, "role.builtin_protected", "built-in role cannot be deleted")
		}
		counts, err := tenant.TenantRaw[struct{ Count int64 }](tx,
			`SELECT COUNT(*) AS count FROM user_roles WHERE tenant_id = ? AND role_id = ?`, role.ID)
		if err != nil {
			return err
		}
		var count int64
		if len(counts) == 1 {
			count = counts[0].Count
		}
		if count > 0 {
			return errs.NewKey(errs.CodeConflict, "role.in_use", "role is assigned to users")
		}
		if err := deleteRolePermissions(tx, role.ID); err != nil {
			return err
		}
		return tenant.TenantExec(tx, `DELETE FROM roles WHERE tenant_id = ? AND id = ?`, role.ID)
	})
	if err != nil {
		return nil, normalizeRoleError(err, "delete role failed")
	}
	return map[string]any{"id": req.ID}, nil
}

func (s *RoleService) UpdatePermissions(ctx context.Context, input json.RawMessage) (any, error) {
	_, _, err := tenantIdentity(ctx)
	if err != nil {
		return nil, err
	}
	var req rolePermissionsRequest
	if err := decodeRoleRequest(input, &req); err != nil {
		return nil, err
	}
	permissions, err := s.validatePermissions(req.Permissions)
	if err != nil {
		return nil, err
	}
	setRoleAudit(ctx, req.ID, "role permissions updated")
	tdb, err := tenant.TenantDB(ctx, s.db)
	if err != nil {
		return nil, err
	}
	err = tdb.Transaction(func(tx tenant.TenantDatabase) error {
		role, err := s.findRole(tx, req.ID, true)
		if err != nil {
			return err
		}
		if role.IsBuiltin && !containsAll(permissions, adminMinimumPermissions) {
			return errs.NewKey(errs.CodeConflict, "role.admin_permissions_required", "admin role must retain management permissions")
		}
		if err := replaceRolePermissions(tx, role.ID, permissions); err != nil {
			return err
		}
		return s.auth.BumpAuthVersionForRoleScoped(tx, role.ID)
	})
	if err != nil {
		return nil, normalizeRoleError(err, "update role permissions failed")
	}
	return map[string]any{"id": req.ID, "permissions": permissions}, nil
}

func (s *RoleService) PermissionCatalog(ctx context.Context, _ json.RawMessage) (any, error) {
	if _, _, err := tenantIdentity(ctx); err != nil {
		return nil, err
	}
	groups := make(map[string]*PermissionCatalogGroup)
	items := make(map[string]map[string]*PermissionCatalogItem)
	lang := auth.MustFromContext(ctx).Language
	for _, registered := range s.actions.List() {
		if auth.NormalizeRealm(registered.Meta.Surface) != auth.RealmTenant || registered.Meta.AccessMode() != action.AccessPermission {
			continue
		}
		capabilityID := registered.Meta.Capability
		if _, ok := groups[capabilityID]; !ok {
			capabilityDef, _ := s.capabilities.Get(capabilityID)
			groups[capabilityID] = &PermissionCatalogGroup{Capability: capabilityID, Name: localizedDefinition(lang, capabilityDef.NameKey, capabilityDef.Name), NameKey: capabilityDef.NameKey, Description: localizedDefinition(lang, capabilityDef.DescriptionKey, capabilityDef.Description), DescriptionKey: capabilityDef.DescriptionKey}
			items[capabilityID] = make(map[string]*PermissionCatalogItem)
		}
		permission := registered.Meta.PermissionCode(registered.ID)
		item, ok := items[capabilityID][permission]
		if !ok {
			group := registered.Meta.PermissionGroup
			if group == "" {
				group = capabilityID + ".other"
			}
			item = &PermissionCatalogItem{
				Permission: permission, Group: group,
				GroupName:    localizedDefinition(lang, registered.Meta.PermissionGroupNameKey, registered.Meta.PermissionGroupName),
				GroupNameKey: registered.Meta.PermissionGroupNameKey,
				Name:         localizedDefinition(lang, registered.Meta.NameKey, registered.Meta.Name), NameKey: registered.Meta.NameKey,
				Description: localizedDefinition(lang, registered.Meta.DescriptionKey, registered.Meta.Description), DescriptionKey: registered.Meta.DescriptionKey,
				Audit: registered.Meta.ShouldAudit(),
			}
			if registered.Meta.AI != nil {
				item.Risk = registered.Meta.AI.Risk
			}
			items[capabilityID][permission] = item
		}
		item.ActionIDs = append(item.ActionIDs, registered.ID)
	}
	result := make([]PermissionCatalogGroup, 0, len(groups))
	for capabilityID, group := range groups {
		for _, item := range items[capabilityID] {
			sort.Strings(item.ActionIDs)
			group.Items = append(group.Items, *item)
		}
		sort.Slice(group.Items, func(i, j int) bool { return group.Items[i].Permission < group.Items[j].Permission })
		result = append(result, *group)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Capability < result[j].Capability })
	return map[string]any{"groups": result}, nil
}

func (s *RoleService) assignablePermissions() map[string]struct{} {
	allowed := make(map[string]struct{})
	for _, registered := range s.actions.List() {
		if auth.NormalizeRealm(registered.Meta.Surface) == auth.RealmTenant && registered.Meta.AccessMode() == action.AccessPermission {
			allowed[registered.Meta.PermissionCode(registered.ID)] = struct{}{}
		}
	}
	return allowed
}

func (s *RoleService) validatePermissions(values []string) ([]string, error) {
	allowed := s.assignablePermissions()
	seen := make(map[string]struct{})
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := allowed[value]; !ok {
			return nil, errs.ValidationKey("role.permission_invalid", "permission is not assignable")
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	sort.Strings(result)
	return result, nil
}

func (s *RoleService) findRole(db tenant.TenantDatabase, roleID string, lock bool) (*model.Role, error) {
	if roleID == "" {
		return nil, errs.ValidationKey("common.validation_failed", "role id is required")
	}
	sql := `SELECT * FROM roles WHERE tenant_id = ? AND id = ?`
	if lock {
		sql += ` FOR UPDATE`
	}
	rows, err := tenant.TenantRaw[model.Role](db, sql, roleID)
	if err != nil {
		return nil, err
	}
	if len(rows) != 1 {
		return nil, errs.NewKey(errs.CodeNotFound, "role.not_found", "role not found")
	}
	return &rows[0], nil
}

func (s *RoleService) rolePermissions(db tenant.TenantDatabase, roleID string) ([]string, error) {
	rows, err := tenant.TenantRaw[model.RolePermission](db, `SELECT rp.* FROM role_permissions rp
JOIN roles r ON r.id = rp.role_id WHERE r.tenant_id = ? AND r.id = ? ORDER BY rp.permission ASC`, roleID)
	if err != nil {
		return nil, errs.Wrap(errs.CodeInternal, "load role permissions failed", err)
	}
	result := make([]string, 0, len(rows))
	for _, row := range rows {
		result = append(result, row.Permission)
	}
	return result, nil
}

func deleteRolePermissions(db tenant.TenantDatabase, roleID string) error {
	return tenant.TenantExec(db, `DELETE FROM role_permissions rp USING roles r
WHERE r.tenant_id = ? AND r.id = ? AND rp.role_id = r.id`, roleID)
}

func replaceRolePermissions(db tenant.TenantDatabase, roleID string, permissions []string) error {
	if err := deleteRolePermissions(db, roleID); err != nil {
		return err
	}
	for _, permission := range permissions {
		if err := tenant.TenantExec(db, `WITH scoped_role AS (
  SELECT id FROM roles WHERE tenant_id = ? AND id = ?
)
INSERT INTO role_permissions (id, role_id, permission)
SELECT ?, id, ? FROM scoped_role`, roleID, uuid.NewString(), permission); err != nil {
			return err
		}
	}
	return nil
}

func containsAll(values, required []string) bool {
	set := make(map[string]struct{}, len(values))
	for _, value := range values {
		set[value] = struct{}{}
	}
	for _, value := range required {
		if _, ok := set[value]; !ok {
			return false
		}
	}
	return true
}

func normalizeRoleError(err error, fallback string) error {
	if _, ok := errs.AsAppError(err); ok {
		return err
	}
	return errs.Wrap(errs.CodeInternal, fallback, err)
}

func roleView(role model.Role, userCount int64, permissions []string) RoleView {
	return RoleView{ID: role.ID, Code: role.Code, Name: role.Name, Description: role.Description, Status: role.Status, IsBuiltin: role.IsBuiltin, UserCount: userCount, Permissions: permissions, CreatedAt: role.CreatedAt, UpdatedAt: role.UpdatedAt}
}

func setRoleAudit(ctx context.Context, roleID, summary string) {
	if ac := auth.MustFromContext(ctx); ac != nil {
		ac.AuditResourceType = "role"
		ac.AuditResourceID = roleID
		ac.AuditSummary = summary
	}
}

func localizedDefinition(lang, key, fallback string) string {
	if key == "" {
		return fallback
	}
	translated := i18n.T(lang, key)
	if translated == key {
		return fallback
	}
	return translated
}
