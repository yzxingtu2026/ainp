package pagecontext

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"sync"
)

const (
	SchemaVersion = "1.0"
	MaxBytes      = 8 << 10
)

var stableCode = regexp.MustCompile(`^[a-z][a-z0-9_.-]{0,63}$`)

type ResourceRef struct {
	ResourceType string `json:"resource_type" validate:"required,max=64"`
	ResourceID   string `json:"resource_id" validate:"required,max=128"`
	Revision     string `json:"revision,omitempty" validate:"max=64"`
}

type ToolHints struct {
	Capabilities []string `json:"capabilities,omitempty"`
	Tags         []string `json:"tags,omitempty"`
	Queries      []string `json:"queries,omitempty"`
}

type Envelope struct {
	SchemaVersion string         `json:"schema_version" validate:"required,max=16"`
	PageType      string         `json:"page_type" validate:"required,max=64"`
	RouteName     string         `json:"route_name" validate:"required,max=64"`
	Mode          string         `json:"mode" validate:"required,oneof=list detail create edit"`
	ResourceRefs  []ResourceRef  `json:"resource_refs,omitempty" validate:"max=8,dive"`
	UIState       map[string]any `json:"ui_state,omitempty"`
	Title         string         `json:"title,omitempty" validate:"max=128"`
	Summary       string         `json:"summary,omitempty" validate:"max=512"`
	ToolHints     ToolHints      `json:"tool_hints,omitempty"`
}

func (e Envelope) Validate() error {
	if e.SchemaVersion != SchemaVersion {
		return fmt.Errorf("unsupported page context schema")
	}
	if !stableCode.MatchString(e.PageType) || !stableCode.MatchString(e.RouteName) {
		return fmt.Errorf("invalid page context code")
	}
	if e.Mode != "list" && e.Mode != "detail" && e.Mode != "create" && e.Mode != "edit" {
		return fmt.Errorf("invalid page context mode")
	}
	if len(e.ResourceRefs) > 8 || len(e.Title) > 128 || len(e.Summary) > 512 {
		return fmt.Errorf("page context exceeds field budget")
	}
	for _, ref := range e.ResourceRefs {
		if !stableCode.MatchString(ref.ResourceType) || strings.TrimSpace(ref.ResourceID) == "" || len(ref.ResourceID) > 128 || len(ref.Revision) > 64 {
			return fmt.Errorf("invalid page context resource reference")
		}
	}
	if len(e.UIState) > 16 {
		return fmt.Errorf("page context ui state exceeds field budget")
	}
	for key, value := range e.UIState {
		if !stableCode.MatchString(key) || !validUIValue(value) {
			return fmt.Errorf("invalid page context ui state")
		}
	}
	if err := validateHints(e.ToolHints); err != nil {
		return err
	}
	body, err := json.Marshal(e)
	if err != nil || len(body) > MaxBytes {
		return fmt.Errorf("page context exceeds byte budget")
	}
	return nil
}

func validUIValue(value any) bool {
	switch v := value.(type) {
	case nil, bool, float64, float32, int, int32, int64:
		return true
	case string:
		return len(v) <= 256
	case []any:
		if len(v) > 8 {
			return false
		}
		for _, item := range v {
			if _, nested := item.([]any); nested || !validUIValue(item) {
				return false
			}
		}
		return true
	default:
		return false
	}
}

func validateHints(h ToolHints) error {
	if len(h.Capabilities) > 8 || len(h.Tags) > 8 || len(h.Queries) > 8 {
		return fmt.Errorf("page context tool hints exceed item budget")
	}
	for _, code := range append(append([]string{}, h.Capabilities...), h.Tags...) {
		if !stableCode.MatchString(code) {
			return fmt.Errorf("invalid page context tool hint")
		}
	}
	for _, query := range h.Queries {
		if strings.TrimSpace(query) == "" || len(query) > 128 {
			return fmt.Errorf("invalid page context query hint")
		}
	}
	return nil
}

type Snapshot struct {
	SchemaVersion string        `json:"schema_version"`
	PageType      string        `json:"page_type"`
	RouteName     string        `json:"route_name"`
	Mode          string        `json:"mode"`
	TrustedRefs   []ResourceRef `json:"trusted_refs,omitempty"`
	SafeSummary   string        `json:"safe_summary"`
	ToolHints     ToolHints     `json:"tool_hints,omitempty"`
	Warnings      []string      `json:"warnings,omitempty"`
}

type Resolver interface {
	Resolve(context.Context, Envelope) (Snapshot, error)
}

var ErrUnknownPageType = errors.New("unknown page context type")

type Registry struct {
	mu        sync.RWMutex
	resolvers map[string]Resolver
}

func NewRegistry() *Registry { return &Registry{resolvers: map[string]Resolver{}} }

func (r *Registry) Register(pageType string, resolver Resolver) error {
	if !stableCode.MatchString(pageType) || resolver == nil {
		return fmt.Errorf("invalid page context resolver")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.resolvers[pageType]; exists {
		return fmt.Errorf("page context resolver already registered: %s", pageType)
	}
	r.resolvers[pageType] = resolver
	return nil
}

func (r *Registry) Resolve(ctx context.Context, envelope Envelope) (Snapshot, error) {
	if err := envelope.Validate(); err != nil {
		return Snapshot{}, err
	}
	r.mu.RLock()
	resolver := r.resolvers[envelope.PageType]
	r.mu.RUnlock()
	if resolver == nil {
		return Snapshot{}, ErrUnknownPageType
	}
	snapshot, err := resolver.Resolve(ctx, envelope)
	if err != nil {
		return Snapshot{}, err
	}
	snapshot.SchemaVersion = SchemaVersion
	snapshot.PageType = envelope.PageType
	snapshot.RouteName = envelope.RouteName
	snapshot.Mode = envelope.Mode
	if err := validateSnapshot(snapshot); err != nil {
		return Snapshot{}, err
	}
	return snapshot, nil
}

func validateSnapshot(snapshot Snapshot) error {
	if len(snapshot.TrustedRefs) > 8 || len(snapshot.SafeSummary) > 1024 || len(snapshot.Warnings) > 8 {
		return fmt.Errorf("resolved page context exceeds field budget")
	}
	for _, ref := range snapshot.TrustedRefs {
		if !stableCode.MatchString(ref.ResourceType) || strings.TrimSpace(ref.ResourceID) == "" || len(ref.ResourceID) > 128 || len(ref.Revision) > 64 {
			return fmt.Errorf("invalid resolved page context reference")
		}
	}
	if err := validateHints(snapshot.ToolHints); err != nil {
		return err
	}
	for _, warning := range snapshot.Warnings {
		if !stableCode.MatchString(warning) {
			return fmt.Errorf("invalid page context warning")
		}
	}
	body, err := json.Marshal(snapshot)
	if err != nil || len(body) > MaxBytes {
		return fmt.Errorf("resolved page context exceeds byte budget")
	}
	return nil
}

func (r *Registry) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.resolvers)
}

// PageTypes returns a stable manifest of registered server-side resolvers for
// frontend-provider contract checks. Resolver implementations remain private.
func (r *Registry) PageTypes() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	pageTypes := make([]string, 0, len(r.resolvers))
	for pageType := range r.resolvers {
		pageTypes = append(pageTypes, pageType)
	}
	sort.Strings(pageTypes)
	return pageTypes
}

type snapshotKey struct{}

func WithSnapshot(ctx context.Context, snapshot *Snapshot) context.Context {
	return context.WithValue(ctx, snapshotKey{}, snapshot)
}

func FromContext(ctx context.Context) (*Snapshot, bool) {
	snapshot, ok := ctx.Value(snapshotKey{}).(*Snapshot)
	return snapshot, ok && snapshot != nil
}
