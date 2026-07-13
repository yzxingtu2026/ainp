// Package i18n 提供全后端多语言翻译支持。
//
// 翻译文件为独立 YAML，运行时从磁盘加载，AI Agent 新增语言后触发 reload 即可生效。
// 键命名规范：`domain.reason`，小写点分（如 auth.invalid_credentials）。
package i18n

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"
)

const (
	LangZhCN = "zh-CN"
	LangEn   = "en"
)

type contextKey struct{}

var (
	defaultLang = LangZhCN
	dir         string // 翻译文件目录，LoadDir 时记录，Reload 复用

	mu           sync.RWMutex
	translations = map[string]map[string]string{}
)

// ── 配置 ──

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

// ── 加载 / 重载 ──

// LoadDir 从指定目录加载所有 *.yaml 翻译文件。
// 文件名（去掉扩展名）即为语言标识，如 zh-CN.yaml → "zh-CN"。
func LoadDir(d string) error {
	mu.Lock()
	dir = d
	mu.Unlock()
	return reload()
}

// Reload 重新从磁盘读取翻译文件，原子替换内存翻译表。
// AI Agent 新增或修改语言文件后调用此方法即可生效。
func Reload() error {
	return reload()
}

// AvailableLangs 返回已加载的语言列表。
func AvailableLangs() []string {
	mu.RLock()
	defer mu.RUnlock()
	out := make([]string, 0, len(translations))
	for lang := range translations {
		out = append(out, lang)
	}
	sort.Strings(out)
	return out
}

// KeyCount 返回已加载的翻译 key 总数（取各语言最大值）。
func KeyCount() int {
	mu.RLock()
	defer mu.RUnlock()
	max := 0
	for _, msgs := range translations {
		if len(msgs) > max {
			max = len(msgs)
		}
	}
	return max
}

func reload() error {
	mu.RLock()
	d := dir
	mu.RUnlock()

	if d == "" {
		return fmt.Errorf("i18n: directory not set, call LoadDir first")
	}

	entries, err := os.ReadDir(d)
	if err != nil {
		return fmt.Errorf("i18n: read dir %s: %w", d, err)
	}

	newTrans := map[string]map[string]string{}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".yaml") {
			continue
		}
		lang := strings.TrimSuffix(entry.Name(), ".yaml")
		path := filepath.Join(d, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("i18n: read %s: %w", path, err)
		}
		var msgs map[string]string
		if err := yaml.Unmarshal(data, &msgs); err != nil {
			return fmt.Errorf("i18n: parse %s: %w", path, err)
		}
		newTrans[lang] = msgs
	}

	// 原子替换
	mu.Lock()
	translations = newTrans
	mu.Unlock()
	return nil
}

// ── 注册（保留兼容旧调用） ──

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

// ── 查找 ──

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

	if lang != defaultLang {
		if msgs, ok := translations[defaultLang]; ok {
			if v, ok := msgs[key]; ok {
				return v
			}
		}
	}

	return key
}

// ── context 工具 ──

// WithLang 将语言标识写入 context，供业务 handler 使用。
func WithLang(ctx context.Context, lang string) context.Context {
	return context.WithValue(ctx, contextKey{}, lang)
}

// FromCtx 从 context 中读取语言标识；空字符串表示未设置。
func FromCtx(ctx context.Context) string {
	if v, ok := ctx.Value(contextKey{}).(string); ok {
		return v
	}
	return ""
}
