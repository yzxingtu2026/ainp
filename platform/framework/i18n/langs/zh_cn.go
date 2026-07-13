package langs

import "github.com/yzxingtu2026/ainp/platform/framework/i18n"

func init() {
	i18n.Register(i18n.LangZhCN, map[string]string{
		// 通用
		"common.ok":                "成功",
		"common.validation_failed": "参数校验失败",
		"common.unauthorized":      "未认证",
		"common.forbidden":         "无权限",
		"common.not_found":         "资源不存在",
		"common.internal_error":    "系统内部错误",
		"common.field_required":    "必填",

		// 租户 / 账号
		"auth.invalid_credentials": "账号或密码错误",
		"auth.tenant_disabled":     "租户已停用",
		"auth.user_disabled":       "用户已停用",

		// Action
		"action.not_found": "Action 未注册",
	})
}
