package system

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/yzxingtu2026/ainp/platform/framework/aitool"
	"github.com/yzxingtu2026/ainp/platform/framework/auth"
)

type userRolesConfirmationPresenter struct {
	users *UserService
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
	user, roles, err := p.users.resolveAssignableUserRoles(ctx, arguments.ID, arguments.RoleIDs)
	if err != nil {
		return aitool.ConfirmationPresentation{}, err
	}
	roleNames := make([]string, 0, len(roles))
	for _, role := range roles {
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
