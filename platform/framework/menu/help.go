package menu

import (
	"bytes"
	"fmt"
	"io/fs"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	MaxHelpDocumentBytes = 32 * 1024
	MaxHelpUseCases      = 20
)

var helpVersionPattern = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+$`)

type HelpFrontmatter struct {
	MenuID         string   `yaml:"menu_id"`
	Version        string   `yaml:"version"`
	Locale         string   `yaml:"locale"`
	UseCases       []string `yaml:"use_cases"`
	PermissionRefs []string `yaml:"permission_refs"`
}

type HelpDocument struct {
	HelpFrontmatter
	Source  string `json:"source"`
	Content string `json:"content"`
}

type HelpRegistry struct{ documents map[string]HelpDocument }

func NewHelpRegistry() *HelpRegistry { return &HelpRegistry{documents: map[string]HelpDocument{}} }

func ParseHelpDocument(source string, data []byte) (HelpDocument, error) {
	var document HelpDocument
	if len(data) == 0 || len(data) > MaxHelpDocumentBytes {
		return document, fmt.Errorf("page help %s has invalid size", source)
	}
	text := strings.ReplaceAll(string(data), "\r\n", "\n")
	if !strings.HasPrefix(text, "---\n") {
		return document, fmt.Errorf("page help %s requires YAML frontmatter", source)
	}
	end := strings.Index(text[4:], "\n---\n")
	if end < 0 {
		return document, fmt.Errorf("page help %s has unterminated frontmatter", source)
	}
	end += 4
	decoder := yaml.NewDecoder(bytes.NewReader([]byte(text[4:end])))
	decoder.KnownFields(true)
	if err := decoder.Decode(&document.HelpFrontmatter); err != nil {
		return document, fmt.Errorf("parse page help %s: %w", source, err)
	}
	document.Source = source
	document.Content = strings.TrimSpace(text[end+5:])
	if err := validateHelpDocument(document); err != nil {
		return HelpDocument{}, fmt.Errorf("page help %s: %w", source, err)
	}
	return document, nil
}

func validateHelpDocument(document HelpDocument) error {
	if strings.TrimSpace(document.MenuID) == "" || strings.TrimSpace(document.Locale) == "" {
		return fmt.Errorf("menu_id and locale are required")
	}
	if len(document.MenuID) > 128 || len(document.Locale) > 16 || len(document.Content) > MaxHelpDocumentBytes {
		return fmt.Errorf("menu_id, locale, or content exceeds its budget")
	}
	if !helpVersionPattern.MatchString(document.Version) {
		return fmt.Errorf("version must use semantic version format")
	}
	if len(document.UseCases) == 0 || len(document.UseCases) > MaxHelpUseCases {
		return fmt.Errorf("use_cases must contain between 1 and %d entries", MaxHelpUseCases)
	}
	if !strings.HasPrefix(document.Content, "# ") {
		return fmt.Errorf("content must start with a level-one heading")
	}
	seen := map[string]bool{}
	for _, useCase := range document.UseCases {
		key := normalizeSearchText(useCase)
		if key == "" || len([]rune(useCase)) > 160 || seen[key] {
			return fmt.Errorf("use_cases must be non-empty and unique")
		}
		seen[key] = true
	}
	seen = map[string]bool{}
	for _, permission := range document.PermissionRefs {
		permission = strings.TrimSpace(permission)
		if permission == "" || len(permission) > 128 || seen[permission] {
			return fmt.Errorf("permission_refs must be non-empty, unique, and within budget")
		}
		seen[permission] = true
	}
	return nil
}

func (r *HelpRegistry) Register(document HelpDocument) error {
	if err := validateHelpDocument(document); err != nil {
		return err
	}
	key := helpKey(document.MenuID, document.Locale)
	if _, exists := r.documents[key]; exists {
		return fmt.Errorf("page help already registered: %s", key)
	}
	r.documents[key] = document
	return nil
}

func (r *HelpRegistry) Load(fsys fs.FS, pattern string) error {
	paths, err := fs.Glob(fsys, pattern)
	if err != nil {
		return err
	}
	if len(paths) == 0 {
		return fmt.Errorf("page help pattern matched no files: %s", pattern)
	}
	for _, path := range paths {
		data, err := fs.ReadFile(fsys, path)
		if err != nil {
			return err
		}
		document, err := ParseHelpDocument(filepath.ToSlash(path), data)
		if err != nil {
			return err
		}
		if err := r.Register(document); err != nil {
			return err
		}
	}
	return nil
}

func (r *HelpRegistry) ValidateMenus(registry *Registry) error {
	menus := map[string]Menu{}
	for _, item := range registry.List() {
		menus[item.ID] = item
	}
	for _, document := range r.documents {
		item, ok := menus[document.MenuID]
		if !ok || !isSearchableMenu(item) {
			return fmt.Errorf("page help references unknown or unsearchable menu: %s", document.MenuID)
		}
		for _, permission := range document.PermissionRefs {
			if permission != item.Permission {
				return fmt.Errorf("page help %s references permission outside its menu: %s", document.Source, permission)
			}
		}
	}
	return nil
}

func (r *HelpRegistry) Get(menuID, locale string) (HelpDocument, bool) {
	document, ok := r.documents[helpKey(menuID, locale)]
	if !ok && locale != "zh-CN" {
		document, ok = r.documents[helpKey(menuID, "zh-CN")]
	}
	return document, ok
}

func (r *HelpRegistry) List() []HelpDocument {
	out := make([]HelpDocument, 0, len(r.documents))
	for _, d := range r.documents {
		out = append(out, d)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].MenuID == out[j].MenuID {
			return out[i].Locale < out[j].Locale
		}
		return out[i].MenuID < out[j].MenuID
	})
	return out
}

func helpKey(menuID, locale string) string {
	return strings.TrimSpace(menuID) + "\x00" + strings.TrimSpace(locale)
}
