package system

import (
	"regexp"
	"unicode/utf8"

	"github.com/yzxingtu2026/ainp/platform/framework/errs"
)

const (
	passwordMinLen = 8
	passwordMaxLen = 72 // bcrypt 输入上限
)

var (
	passwordHasLetter = regexp.MustCompile(`[A-Za-z]`)
	passwordHasDigit  = regexp.MustCompile(`[0-9]`)
)

// ValidatePassword 统一密码强度校验（创建 / 重置 / 安装向导共用）。
// 规则：长度 8–72，至少包含一个字母和一个数字；后端为最终边界。
func ValidatePassword(password string) error {
	length := utf8.RuneCountInString(password)
	if length < passwordMinLen || length > passwordMaxLen {
		return errs.ValidationKey("user.password_weak", "password does not meet strength requirements",
			errs.FieldError{Field: "password", Message: "weak password", MessageKey: "user.password_weak"},
		)
	}
	if !passwordHasLetter.MatchString(password) || !passwordHasDigit.MatchString(password) {
		return errs.ValidationKey("user.password_weak", "password does not meet strength requirements",
			errs.FieldError{Field: "password", Message: "weak password", MessageKey: "user.password_weak"},
		)
	}
	return nil
}
