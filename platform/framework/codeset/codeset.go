package codeset

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"sync"

	"github.com/yzxingtu2026/ainp/platform/framework/auth"
	"github.com/yzxingtu2026/ainp/platform/framework/i18n"
)

type Semantic string

const (
	SemanticDefault Semantic = "default"
	SemanticInfo    Semantic = "info"
	SemanticSuccess Semantic = "success"
	SemanticWarning Semantic = "warning"
	SemanticDanger  Semantic = "danger"
	SemanticMuted   Semantic = "muted"
)

var (
	keyPattern   = regexp.MustCompile(`^[a-z][a-z0-9_]*(\.[a-z][a-z0-9_]*)+$`)
	valuePattern = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)
	iconPattern  = regexp.MustCompile(`^[a-z0-9-]+:[a-z0-9][a-z0-9-]*$`)
)

func (s Semantic) Valid() bool {
	switch s {
	case SemanticDefault, SemanticInfo, SemanticSuccess, SemanticWarning, SemanticDanger, SemanticMuted:
		return true
	default:
		return false
	}
}

type CodeItem struct {
	Value          string   `json:"value"`
	Label          string   `json:"label,omitempty"`
	LabelKey       string   `json:"label_key"`
	Description    string   `json:"description,omitempty"`
	DescriptionKey string   `json:"description_key,omitempty"`
	Semantic       Semantic `json:"semantic"`
	Icon           string   `json:"icon,omitempty"`
	LocalIcon      string   `json:"local_icon,omitempty"`
	Order          int      `json:"order"`
	Deprecated     bool     `json:"deprecated"`
}

type CodeSet struct {
	Key            string     `json:"key"`
	Name           string     `json:"name,omitempty"`
	NameKey        string     `json:"name_key"`
	Description    string     `json:"description,omitempty"`
	DescriptionKey string     `json:"description_key,omitempty"`
	Items          []CodeItem `json:"items"`
	Order          int        `json:"order"`
	Surface        auth.Realm `json:"surface"`
	Permission     string     `json:"-"`
}

type Registry struct {
	mu      sync.RWMutex
	items   map[string]CodeSet
	version string
}

func NewRegistry() *Registry { return &Registry{items: make(map[string]CodeSet)} }

func (r *Registry) Register(set CodeSet) error {
	if err := Validate(set); err != nil {
		return err
	}
	set.Surface = auth.NormalizeRealm(set.Surface)
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.items[set.Key]; exists {
		return fmt.Errorf("codeset already registered: %s", set.Key)
	}
	r.items[set.Key] = clone(set)
	r.refreshVersion()
	return nil
}

func Validate(set CodeSet) error {
	if !keyPattern.MatchString(set.Key) {
		return fmt.Errorf("invalid codeset key: %s", set.Key)
	}
	if set.NameKey == "" {
		return fmt.Errorf("codeset name_key is required: %s", set.Key)
	}
	if set.Order < 0 {
		return fmt.Errorf("codeset order must be non-negative: %s", set.Key)
	}
	if surface := auth.NormalizeRealm(set.Surface); !surface.Valid() {
		return fmt.Errorf("invalid codeset surface: %s", set.Surface)
	}
	seen := make(map[string]struct{}, len(set.Items))
	for _, item := range set.Items {
		if !valuePattern.MatchString(item.Value) {
			return fmt.Errorf("invalid codeset value %q in %s", item.Value, set.Key)
		}
		if _, ok := seen[item.Value]; ok {
			return fmt.Errorf("duplicate codeset value %q in %s", item.Value, set.Key)
		}
		seen[item.Value] = struct{}{}
		if item.LabelKey == "" {
			return fmt.Errorf("codeset item label_key is required: %s.%s", set.Key, item.Value)
		}
		if !item.Semantic.Valid() {
			return fmt.Errorf("invalid semantic %q in %s.%s", item.Semantic, set.Key, item.Value)
		}
		if item.Icon != "" && !iconPattern.MatchString(item.Icon) {
			return fmt.Errorf("invalid Iconify icon %q in %s.%s", item.Icon, set.Key, item.Value)
		}
		if item.Order < 0 {
			return fmt.Errorf("codeset item order must be non-negative: %s.%s", set.Key, item.Value)
		}
	}
	return nil
}

func (r *Registry) Get(key string) (CodeSet, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	set, ok := r.items[key]
	return clone(set), ok
}

func (r *Registry) List(realm auth.Realm, hasPermission func(string) bool) []CodeSet {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]CodeSet, 0, len(r.items))
	for _, set := range r.items {
		if auth.NormalizeRealm(set.Surface) != auth.NormalizeRealm(realm) {
			continue
		}
		if set.Permission != "" && (hasPermission == nil || !hasPermission(set.Permission)) {
			continue
		}
		out = append(out, clone(set))
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Order == out[j].Order {
			return out[i].Key < out[j].Key
		}
		return out[i].Order < out[j].Order
	})
	return out
}

func Localize(set CodeSet, lang string) CodeSet {
	set.Name = i18n.T(lang, set.NameKey)
	if set.DescriptionKey != "" {
		set.Description = i18n.T(lang, set.DescriptionKey)
	}
	for i := range set.Items {
		set.Items[i].Label = i18n.T(lang, set.Items[i].LabelKey)
		if set.Items[i].DescriptionKey != "" {
			set.Items[i].Description = i18n.T(lang, set.Items[i].DescriptionKey)
		}
	}
	return set
}

func (r *Registry) Version() string { r.mu.RLock(); defer r.mu.RUnlock(); return r.version }

func (r *Registry) refreshVersion() {
	keys := make([]string, 0, len(r.items))
	for key := range r.items {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	var b strings.Builder
	for _, key := range keys {
		fmt.Fprintf(&b, "%s:%#v;", key, r.items[key])
	}
	sum := sha256.Sum256([]byte(b.String()))
	r.version = hex.EncodeToString(sum[:8])
}

func clone(set CodeSet) CodeSet { set.Items = append([]CodeItem(nil), set.Items...); return set }
