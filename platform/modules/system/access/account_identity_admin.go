package access

import (
	"context"
	"errors"

	"github.com/yzxingtu2026/ainp/platform/framework/action"
	"github.com/yzxingtu2026/ainp/platform/framework/app"
	"github.com/yzxingtu2026/ainp/platform/framework/errs"
	"github.com/yzxingtu2026/ainp/platform/framework/platformresource"
)

type AccountIdentityAdminActionService struct {
	resources platformresource.AccountIdentityAdministrationService
}

func NewAccountIdentityAdminActionService(resources platformresource.AccountIdentityAdministrationService) *AccountIdentityAdminActionService {
	return &AccountIdentityAdminActionService{resources: resources}
}

func (s *AccountIdentityAdminActionService) List(ctx context.Context, req platformresource.AccountIdentityAdminListRequest) (any, error) {
	identities, err := s.resources.ListForAccount(ctx, req)
	if err != nil {
		return nil, accountIdentityAdminError(err)
	}
	return identities, nil
}

func (s *AccountIdentityAdminActionService) Bind(ctx context.Context, req platformresource.AccountIdentityAdminBindRequest) (any, error) {
	identity, err := s.resources.BindForAccount(ctx, req)
	if err != nil {
		return nil, accountIdentityAdminError(err)
	}
	return identity, nil
}

func (s *AccountIdentityAdminActionService) Unbind(ctx context.Context, req platformresource.AccountIdentityAdminUnbindRequest) (any, error) {
	if err := s.resources.UnbindForAccount(ctx, req); err != nil {
		return nil, accountIdentityAdminError(err)
	}
	return map[string]any{"account_id": req.AccountID, "identity_id": req.IdentityID}, nil
}

func RegisterAccountIdentityAdminActions(a *app.App, svc *AccountIdentityAdminActionService) error {
	auditOn := true
	definitions := []struct {
		id          string
		path        string
		name        string
		description string
		handler     action.Handler
		ai          *action.AIMeta
	}{
		{ActionAccountIdentityList, "/api/v1/system/accounts/identities/list", "外部身份列表", "查询指定账号已绑定的脱敏外部身份", action.JSON(svc.List), readAIMeta("查询指定后台账号已绑定的脱敏外部身份", "查询账号外部身份", "查询指定后台账号已绑定的脱敏外部身份", "已返回账号外部身份", action.InputSchemaFor[platformresource.AccountIdentityAdminListRequest](), action.SchemaFor[[]platformresource.ManagedAccountIdentity](), "账号", "外部身份", "列表")},
		{ActionAccountIdentityBind, "/api/v1/system/accounts/identities/bind", "绑定外部身份", "为指定账号预置外部身份绑定", action.JSON(svc.Bind), action.ExcludeFromAI(action.AIExcludeSensitiveCredential)},
		{ActionAccountIdentityUnbind, "/api/v1/system/accounts/identities/unbind", "解绑外部身份", "解除指定账号的外部身份绑定", action.JSON(svc.Unbind), destructiveAIMeta("解除指定后台账号的外部身份绑定，解绑后该身份立即无法登录", "解绑账号外部身份", "解除指定后台账号的外部身份绑定", "账号外部身份已解绑", action.InputSchemaFor[platformresource.AccountIdentityAdminUnbindRequest](), action.SchemaFor[map[string]string](), "账号", "外部身份", "解绑")},
	}
	for _, definition := range definitions {
		if err := a.RegisterAction(action.Action{
			ID: definition.id, Handler: definition.handler,
			Meta: action.Metadata{
				Capability: "system", Permission: ActionAccountIdentityManage,
				PermissionGroup: "system.account_management", PermissionGroupName: "账号管理", PermissionGroupNameKey: "permission_group.system.account_management",
				Name: definition.name, NameKey: "action." + definition.id + ".name",
				Description: definition.description, DescriptionKey: "action." + definition.id + ".description",
				Audit: &auditOn, AI: definition.ai, HTTP: &action.HTTPMeta{Method: "POST", Path: definition.path},
			},
		}); err != nil {
			return err
		}
	}
	return nil
}

func accountIdentityAdminError(err error) error {
	switch {
	case errors.Is(err, platformresource.ErrInvalid):
		return errs.ValidationKey("account_identity.invalid", "invalid account identity request")
	case errors.Is(err, platformresource.ErrConflict):
		return errs.NewKey(errs.CodeConflict, "account_identity.conflict", "account identity already belongs to an account or provider is already bound")
	case errors.Is(err, platformresource.ErrDisabled):
		return errs.NewKey(errs.CodeConflict, "account_identity.account_disabled", "target account is disabled")
	case errors.Is(err, platformresource.ErrNotFound):
		return errs.NewKey(errs.CodeNotFound, "account_identity.not_found", "account or identity not found")
	case errors.Is(err, platformresource.ErrForbidden):
		return errs.NewKey(errs.CodeForbidden, "common.forbidden", "forbidden")
	default:
		return errs.Wrap(errs.CodeInternal, "manage account identity failed", err)
	}
}
