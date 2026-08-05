package access

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/yzxingtu2026/ainp/platform/framework/aitool"
	"github.com/yzxingtu2026/ainp/platform/framework/auth"
)

type AccountRolesConfirmationPresenter struct {
	accounts *AccountService
}

func (p AccountRolesConfirmationPresenter) PresentConfirmation(ctx context.Context, request aitool.ConfirmationPresentationRequest) (aitool.ConfirmationPresentation, error) {
	actor, ok := auth.FromContext(ctx)
	if !ok || actor == nil || !actor.HasPermission(ActionAccountRolesUpdate) {
		return aitool.ConfirmationPresentation{}, fmt.Errorf("account roles presentation is not authorized")
	}
	var arguments accountRolesRequest
	if err := json.Unmarshal(request.Arguments, &arguments); err != nil {
		return aitool.ConfirmationPresentation{}, err
	}
	account, roles, err := p.accounts.resolveAssignableAccountRoles(ctx, arguments.ID, arguments.RoleIDs)
	if err != nil {
		return aitool.ConfirmationPresentation{}, err
	}
	roleNames := make([]string, 0, len(roles))
	for _, role := range roles {
		roleNames = append(roleNames, role.Name)
	}
	sort.Strings(roleNames)
	accountName := strings.TrimSpace(account.DisplayName)
	if accountName == "" {
		accountName = account.Username
	}
	rolesText := strings.Join(roleNames, "、")
	if rolesText == "" {
		rolesText = "无角色"
	}
	return aitool.ConfirmationPresentation{
		Tool:    aitool.PresentationFor(request.Definition),
		Summary: fmt.Sprintf("将账号「%s」的角色调整为「%s」", accountName, rolesText),
		Details: []aitool.ConfirmationPresentationDetail{
			{Label: "目标账号", Value: accountName},
			{Label: "调整后的角色", Value: rolesText},
		},
	}, nil
}

func NewAccountRolesConfirmationPresenter(accounts *AccountService) AccountRolesConfirmationPresenter {
	return AccountRolesConfirmationPresenter{accounts: accounts}
}
