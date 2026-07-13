package permission

import "github.com/yzxingtu2026/ainp/platform/framework/auth"

// Checker 校验 permission 字符串（默认即 action.id）。
type Checker struct{}

func NewChecker() *Checker {
	return &Checker{}
}

func (c *Checker) Allow(ac *auth.Context, permission string) bool {
	if permission == "" {
		return true
	}
	if ac == nil || !ac.Authenticated {
		return false
	}
	if ac.IsSystemOperator {
		return true
	}
	return ac.HasPermission(permission)
}
