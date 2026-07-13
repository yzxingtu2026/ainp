// Package i18n 提供 API 错误文案的多语言查找与回退。
//
// 键命名规范：`domain.reason`，小写点分（如 auth.invalid_credentials）。
// 翻译文案集中在 langs/ 子包注册，业务代码只引用 key。
package i18n

import "sync"

const (
	LangZhCN = "zh-CN"
	LangEn   = "en"
)

var (
	defaultLang = LangZhCN

	mu           sync.RWMutex
	translations = map[string]map[string]string{}
)

// SetDefaultLang 设置缺省回退语言；中间件未协商到语言时使用。
func SetDefaultLang(lang string) {
	mu.Lock()
	defer mu.Unlock()
	defaultLang = lang
}

// DefaultLang 返回当前默认语言。
func DefaultLang() string {
	mu.RLock()
	defer mu.RUnlock()
	return defaultLang
}

// Register 批量注册某语言的翻译条目。已存在的 key 会被覆盖。
func Register(lang string, msgs map[string]string) {
	mu.Lock()
	defer mu.Unlock()
	if translations[lang] == nil {
		translations[lang] = make(map[string]string, len(msgs))
	}
	for k, v := range msgs {
		translations[lang][k] = v
	}
}

// T 按语言查找 message key 对应的文案。
// 查找顺序：lang → defaultLang → key 本身。
func T(lang, key string) string {
	mu.RLock()
	defer mu.RUnlock()

	if lang != "" {
		if msgs, ok := translations[lang]; ok {
			if v, ok := msgs[key]; ok {
				return v
			}
		}
	}

	// 回退默认语言
	if lang != defaultLang {
		if msgs, ok := translations[defaultLang]; ok {
			if v, ok := msgs[key]; ok {
				return v
			}
		}
	}

	return key
}
