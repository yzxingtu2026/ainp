package system

import (
	"context"
	"encoding/json"
	"errors"
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
	"github.com/yzxingtu2026/ainp/platform/framework/status"
	"github.com/yzxingtu2026/ainp/platform/modules/system/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
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
	Current int                     `json:"current"`
	Size    int                     `json:"size"`
	Keyword string                  `json:"keyword"`
	Status  status.ActivationStatus `json:"status"`
}

type roleIDRequest struct {
	ID string `json:"id"`
}

type roleCreateRequest struct {
	Code        string                  `json:"code"`
	Name        string                  `json:"name"`
	Description string                  `json:"description"`
	Status      status.ActivationStatus `json:"status"`
	Permissions []string                `json:"permissions"`
}

type roleUpdateRequest struct {
	ID          string                  `json:"id"`
	Name        string                  `json:"name"`
	Description string                  `json:"description"`
	Status      status.ActivationStatus `json:"status"`
}

type rolePermissionsRequest struct {
	ID          string   `json:"id"`
	Permissions []string `json:"permissions"`
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
	tenantID, _, err := tenantIdentity(ctx)
	if err != nil {
		return nil, err
	}
	var req roleListRequest
	if err := decodeRoleRequest(input, &req); err != nil {
		return nil, err
	}
	if req.Current < 1 {
		req.Current = 1
	}
	if req.Size < 1 || req.Size > 100 {
		req.Size = 20
	}
	if req.Status != "" && !validateRoleStatus(req.Status) {
		return nil, errs.ValidationKey("common.validation_failed", "invalid status")
	}

	query := s.db.WithContext(ctx).Table("roles AS r").Where("r.tenant_id = ?", tenantID)
	if keyword := strings.TrimSpace(req.Keyword); keyword != "" {
		like := "%" + keyword + "%"
		query = query.Where("r.code ILIKE ? OR r.name ILIKE ?", like, like)
	}
	if req.Status != "" {
		query = query.Where("r.status = ?", req.Status)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, errs.Wrap(errs.CodeInternal, "count roles failed", err)
	}
	var rows []struct {
		model.Role
		UserCount int64 `gorm:"column:user_count"`
	}
	if err := query.Select("r.*, COUNT(ur.id) AS user_count").
		Joins("LEFT JOIN user_roles ur ON ur.role_id = r.id AND ur.tenant_id = r.tenant_id").
		Group("r.id").Order("r.is_builtin DESC, r.created_at ASC, r.id ASC").
		Offset((req.Current - 1) * req.Size).Limit(req.Size).Scan(&rows).Error; err != nil {
		return nil, errs.Wrap(errs.CodeInternal, "list roles failed", err)
	}
	records := make([]RoleView, 0, len(rows))
	for _, row := range rows {
		records = append(records, roleView(row.Role, row.UserCount, nil))
	}
	return map[string]any{"current": req.Current, "size": req.Size, "total": total, "records": records}, nil
}

func (s *RoleService) Get(ctx context.Context, input json.RawMessage) (any, error) {
	tenantID, _, err := tenantIdentity(ctx)
	if err != nil {
		return nil, err
	}
	var req roleIDRequest
	if err := decodeRoleRequest(input, &req); err != nil {
		return nil, err
	}
	role, err := s.findRole(ctx, s.db, tenantID, req.ID, false)
	if err != nil {
		return nil, err
	}
	permissions, err := s.rolePermissions(ctx, s.db, role.ID)
	if err != nil {
		return nil, err
	}
	var count int64
	if err := s.db.WithContext(ctx).Model(&model.UserRole{}).Where("tenant_id = ? AND role_id = ?", tenantID, role.ID).Count(&count).Error; err != nil {
		return nil, errs.Wrap(errs.CodeInternal, "count role users failed", err)
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
	if err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&role).Error; err != nil {
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
	tenantID, _, err := tenantIdentity(ctx)
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
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		current, err := s.findRole(ctx, tx, tenantID, req.ID, true)
		if err != nil {
			return err
		}
		if current.IsBuiltin && req.Status != status.ActivationEnabled {
			return errs.NewKey(errs.CodeConflict, "role.builtin_protected", "built-in role cannot be disabled")
		}
		statusChanged := current.Status != req.Status
		if err := tx.Model(current).Updates(map[string]any{"name": req.Name, "description": strings.TrimSpace(req.Description), "status": req.Status}).Error; err != nil {
			return err
		}
		if statusChanged {
			if err := s.auth.BumpAuthVersionForRole(ctx, tx, tenantID, current.ID); err != nil {
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
	tenantID, _, err := tenantIdentity(ctx)
	if err != nil {
		return nil, err
	}
	var req roleIDRequest
	if err := decodeRoleRequest(input, &req); err != nil {
		return nil, err
	}
	setRoleAudit(ctx, req.ID, "role deleted")
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		role, err := s.findRole(ctx, tx, tenantID, req.ID, true)
		if err != nil {
			return err
		}
		if role.IsBuiltin {
			return errs.NewKey(errs.CodeConflict, "role.builtin_protected", "built-in role cannot be deleted")
		}
		var count int64
		if err := tx.Model(&model.UserRole{}).Where("tenant_id = ? AND role_id = ?", tenantID, role.ID).Count(&count).Error; err != nil {
			return err
		}
		if count > 0 {
			return errs.NewKey(errs.CodeConflict, "role.in_use", "role is assigned to users")
		}
		if err := tx.Where("role_id = ?", role.ID).Delete(&model.RolePermission{}).Error; err != nil {
			return err
		}
		return tx.Delete(role).Error
	})
	if err != nil {
		return nil, normalizeRoleError(err, "delete role failed")
	}
	return map[string]any{"id": req.ID}, nil
}

func (s *RoleService) UpdatePermissions(ctx context.Context, input json.RawMessage) (any, error) {
	tenantID, _, err := tenantIdentity(ctx)
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
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		role, err := s.findRole(ctx, tx, tenantID, req.ID, true)
		if err != nil {
			return err
		}
		if role.IsBuiltin && !containsAll(permissions, adminMinimumPermissions) {
			return errs.NewKey(errs.CodeConflict, "role.admin_permissions_required", "admin role must retain management permissions")
		}
		if err := replaceRolePermissions(tx, role.ID, permissions); err != nil {
			return err
		}
		return s.auth.BumpAuthVersionForRole(ctx, tx, tenantID, role.ID)
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

func (s *RoleService) findRole(ctx context.Context, tx *gorm.DB, tenantID, roleID string, lock bool) (*model.Role, error) {
	if roleID == "" {
		return nil, errs.ValidationKey("common.validation_failed", "role id is required")
	}
	query := tx.WithContext(ctx).Where("id = ? AND tenant_id = ?", roleID, tenantID)
	if lock {
		query = query.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	var role model.Role
	if err := query.First(&role).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errs.NewKey(errs.CodeNotFound, "role.not_found", "role not found")
		}
		return nil, err
	}
	return &role, nil
}

func (s *RoleService) rolePermissions(ctx context.Context, tx *gorm.DB, roleID string) ([]string, error) {
	var rows []model.RolePermission
	if err := tx.WithContext(ctx).Where("role_id = ?", roleID).Order("permission ASC").Find(&rows).Error; err != nil {
		return nil, errs.Wrap(errs.CodeInternal, "load role permissions failed", err)
	}
	result := make([]string, 0, len(rows))
	for _, row := range rows {
		result = append(result, row.Permission)
	}
	return result, nil
}

func replaceRolePermissions(tx *gorm.DB, roleID string, permissions []string) error {
	if err := tx.Where("role_id = ?", roleID).Delete(&model.RolePermission{}).Error; err != nil {
		return err
	}
	for _, permission := range permissions {
		if err := tx.Create(&model.RolePermission{ID: uuid.NewString(), RoleID: roleID, Permission: permission}).Error; err != nil {
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
