// Package i18n 提供全后端多语言翻译支持。
//
// 翻译文件为 owner-scoped YAML 资源；正式运行时使用不可变 fs.FS，显式开发源可 reload。
// 键命名规范：`domain.reason`，小写点分（如 auth.invalid_credentials）。
package i18n

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
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

// Source is one immutable, owner-scoped translation filesystem.
type Source struct {
	Owner string
	FS    fs.FS
}

// Asset is safe runtime diagnostic metadata and never includes translations.
type Asset struct {
	Owner  string
	Key    string
	Type   string
	Digest string
	Locale string
}

var ownerPattern = regexp.MustCompile(`^[a-z][a-z0-9]*(?:[._-][a-z0-9]+)*$`)

var (
	defaultLang = LangZhCN
	dir         string // 翻译文件目录，LoadDir 时记录，Reload 复用
	sources     []Source

	mu           sync.RWMutex
	translations = map[string]map[string]string{}
	assets       []Asset
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
	if strings.TrimSpace(d) == "" {
		return fmt.Errorf("i18n: directory is required")
	}
	loaded, manifest, err := loadSources(Source{Owner: "development", FS: os.DirFS(d)})
	if err != nil {
		return err
	}
	mu.Lock()
	dir = d
	sources = []Source{{Owner: "development", FS: os.DirFS(d)}}
	translations = loaded
	assets = manifest
	mu.Unlock()
	return nil
}

// Reload re-reads explicitly trusted development sources and atomically swaps them.
func Reload() error {
	return reload()
}

// LoadSources atomically replaces translations from immutable owner sources.
func LoadSources(input ...Source) error {
	loaded, manifest, err := loadSources(input...)
	if err != nil {
		return err
	}
	mu.Lock()
	dir = ""
	sources = nil
	translations = loaded
	assets = manifest
	mu.Unlock()
	return nil
}

// LoadReloadableSources loads trusted development sources and enables Reload.
func LoadReloadableSources(input ...Source) error {
	loaded, manifest, err := loadSources(input...)
	if err != nil {
		return err
	}
	mu.Lock()
	dir = ""
	sources = append([]Source(nil), input...)
	translations = loaded
	assets = manifest
	mu.Unlock()
	return nil
}

// Manifest returns safe, deterministic metadata for the loaded translations.
func Manifest() []Asset {
	mu.RLock()
	defer mu.RUnlock()
	return append([]Asset(nil), assets...)
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
	loadedSources := append([]Source(nil), sources...)
	mu.RUnlock()
	if len(loadedSources) == 0 {
		return fmt.Errorf("i18n: loaded resources are immutable; configure an explicit development source to reload")
	}

	newTrans, manifest, err := loadSources(loadedSources...)
	if err != nil {
		return err
	}

	// 原子替换
	mu.Lock()
	translations = newTrans
	assets = manifest
	mu.Unlock()
	return nil
}

func loadSources(sources ...Source) (map[string]map[string]string, []Asset, error) {
	loaded := map[string]map[string]string{}
	manifest := make([]Asset, 0)
	owners := map[string]struct{}{}
	keyOwners := map[string]string{}
	for _, source := range sources {
		owner := strings.TrimSpace(source.Owner)
		if !ownerPattern.MatchString(owner) {
			return nil, nil, fmt.Errorf("i18n: invalid owner %q", owner)
		}
		if source.FS == nil {
			return nil, nil, fmt.Errorf("i18n: owner %q filesystem is required", owner)
		}
		if _, exists := owners[owner]; exists {
			return nil, nil, fmt.Errorf("i18n: duplicate owner %q", owner)
		}
		owners[owner] = struct{}{}
		assetCount := len(manifest)
		err := fs.WalkDir(source.FS, ".", func(path string, entry fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if entry.IsDir() || !strings.EqualFold(filepath.Ext(entry.Name()), ".yaml") {
				return nil
			}
			locale := strings.TrimSuffix(entry.Name(), filepath.Ext(entry.Name()))
			data, err := fs.ReadFile(source.FS, path)
			if err != nil {
				return fmt.Errorf("read %s: %w", path, err)
			}
			var messages map[string]string
			if err := yaml.Unmarshal(data, &messages); err != nil {
				return fmt.Errorf("parse %s: %w", path, err)
			}
			if loaded[locale] == nil {
				loaded[locale] = map[string]string{}
			}
			for key, value := range messages {
				if strings.TrimSpace(key) == "" || strings.TrimSpace(value) == "" {
					return fmt.Errorf("owner %q locale %q has empty key or value", owner, locale)
				}
				if owner != "ainp" && owner != "development" && !strings.HasPrefix(key, owner+".") {
					return fmt.Errorf("owner %q key %q must use prefix %q", owner, key, owner+".")
				}
				if _, exists := loaded[locale][key]; exists {
					return fmt.Errorf("owner %q conflicts on locale %q key %q", owner, locale, key)
				}
				if existingOwner, exists := keyOwners[key]; exists && existingOwner != owner {
					return fmt.Errorf("owner %q conflicts with owner %q on key %q", owner, existingOwner, key)
				}
				loaded[locale][key] = value
				keyOwners[key] = owner
				digest := fmt.Sprintf("sha256:%x", sha256.Sum256([]byte(value)))
				manifest = append(manifest, Asset{Owner: owner, Key: key, Type: "i18n", Digest: digest, Locale: locale})
			}
			return nil
		})
		if err != nil {
			return nil, nil, fmt.Errorf("i18n: load owner %q: %w", owner, err)
		}
		if len(manifest) == assetCount {
			return nil, nil, fmt.Errorf("i18n: owner %q has no YAML translations", owner)
		}
	}
	if len(loaded) == 0 {
		return nil, nil, fmt.Errorf("i18n: no YAML resources found")
	}
	for key, owner := range keyOwners {
		for locale, messages := range loaded {
			if _, exists := messages[key]; !exists {
				return nil, nil, fmt.Errorf("i18n: owner %q key %q is missing locale %q", owner, key, locale)
			}
		}
	}
	sort.Slice(manifest, func(i, j int) bool {
		if manifest[i].Owner != manifest[j].Owner {
			return manifest[i].Owner < manifest[j].Owner
		}
		if manifest[i].Locale != manifest[j].Locale {
			return manifest[i].Locale < manifest[j].Locale
		}
		return manifest[i].Key < manifest[j].Key
	})
	return loaded, manifest, nil
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
