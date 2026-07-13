package langs

import "github.com/yzxingtu2026/ainp/platform/framework/i18n"

func init() {
	i18n.Register(i18n.LangEn, map[string]string{
		// 通用
		"common.ok":                "ok",
		"common.validation_failed": "validation failed",
		"common.unauthorized":      "unauthorized",
		"common.forbidden":         "forbidden",
		"common.not_found":         "resource not found",
		"common.internal_error":    "internal error",
		"common.field_required":    "required",

		// 租户 / 账号
		"auth.invalid_credentials": "invalid username or password",
		"auth.tenant_disabled":     "tenant is disabled",
		"auth.user_disabled":       "user is disabled",

		// Action
		"action.not_found": "action not registered",
	})
}
