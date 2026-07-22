package system

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/yzxingtu2026/ainp/platform/framework/aitool"
	"github.com/yzxingtu2026/ainp/platform/framework/auth"
	"github.com/yzxingtu2026/ainp/platform/framework/tenant"
	"github.com/yzxingtu2026/ainp/platform/modules/system/model"
	"gorm.io/gorm"
)

type userRolesConfirmationPresenter struct {
	db *gorm.DB
}

func (p userRolesConfirmationPresenter) PresentConfirmation(ctx context.Context, request aitool.ConfirmationPresentationRequest) (aitool.ConfirmationPresentation, error) {
	actor, ok := auth.FromContext(ctx)
	if !ok || actor == nil || !actor.HasPermission(ActionUserRolesUpdate) {
		return aitool.ConfirmationPresentation{}, fmt.Errorf("user roles presentation is not authorized")
	}
	var arguments userRolesRequest
	if err := json.Unmarshal(request.Arguments, &arguments); err != nil {
		return aitool.ConfirmationPresentation{}, err
	}
	tdb, err := tenant.TenantDB(ctx, p.db)
	if err != nil {
		return aitool.ConfirmationPresentation{}, err
	}
	users, err := tenant.NewTenantRepository[model.User](tdb)
	if err != nil {
		return aitool.ConfirmationPresentation{}, err
	}
	user, err := users.GetByID(arguments.ID)
	if err != nil {
		return aitool.ConfirmationPresentation{}, err
	}
	roles, err := tenant.NewTenantRepository[model.Role](tdb)
	if err != nil {
		return aitool.ConfirmationPresentation{}, err
	}
	roleRows, err := roles.Find("id IN ?", arguments.RoleIDs)
	if err != nil {
		return aitool.ConfirmationPresentation{}, fmt.Errorf("resolve roles: %w", err)
	}
	if len(roleRows) != len(arguments.RoleIDs) {
		return aitool.ConfirmationPresentation{}, fmt.Errorf("resolve roles: expected %d, found %d", len(arguments.RoleIDs), len(roleRows))
	}
	roleNames := make([]string, 0, len(roleRows))
	for _, role := range roleRows {
		roleNames = append(roleNames, role.Name)
	}
	sort.Strings(roleNames)
	userName := strings.TrimSpace(user.DisplayName)
	if userName == "" {
		userName = user.Username
	}
	rolesText := strings.Join(roleNames, "、")
	if rolesText == "" {
		rolesText = "无角色"
	}
	return aitool.ConfirmationPresentation{
		Tool:    aitool.PresentationFor(request.Definition),
		Summary: fmt.Sprintf("将用户「%s」的角色调整为「%s」", userName, rolesText),
		Details: []aitool.ConfirmationPresentationDetail{
			{Label: "目标用户", Value: userName},
			{Label: "调整后的角色", Value: rolesText},
		},
	}, nil
}
