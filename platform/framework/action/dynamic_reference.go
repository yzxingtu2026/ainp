package action

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/yzxingtu2026/ainp/platform/framework/errs"
)

const (
	ReferenceRequired = "reference_required"
	InvalidReference  = "invalid_reference"
	ReferenceStale    = "reference_stale"
)

// DynamicReference declares that string values at Field must come from a
// governed resolver. Candidates remain outside the stable JSON Schema.
type DynamicReference struct {
	Field            string `json:"field"`
	ResolverID       string `json:"resolver_id"`
	DiscoveryAction  string `json:"discovery_action"`
	RequireEvidence  bool   `json:"require_evidence"`
	MaxInvalidValues int    `json:"max_invalid_values,omitempty"`
	MaxInvalidBytes  int    `json:"max_invalid_bytes,omitempty"`
}

type ReferenceSnapshot struct {
	Values  []string
	Version string
}

type ReferenceResolver interface {
	Resolve(context.Context) (ReferenceSnapshot, error)
}

type ReferenceResolverFunc func(context.Context) (ReferenceSnapshot, error)

func (f ReferenceResolverFunc) Resolve(ctx context.Context) (ReferenceSnapshot, error) { return f(ctx) }

func (r *Registry) RegisterReferenceResolver(id string, resolver ReferenceResolver) error {
	id = strings.TrimSpace(id)
	if id == "" || resolver == nil {
		return fmt.Errorf("reference resolver id and implementation are required")
	}
	if _, exists := r.resolvers[id]; exists {
		return fmt.Errorf("reference resolver already registered: %s", id)
	}
	r.resolvers[id] = resolver
	return nil
}

func (r *Registry) ResolveReference(ctx context.Context, id string) (ReferenceSnapshot, error) {
	resolver, ok := r.resolvers[id]
	if !ok {
		return ReferenceSnapshot{}, fmt.Errorf("reference resolver not found: %s", id)
	}
	snapshot, err := resolver.Resolve(ctx)
	if err != nil {
		return ReferenceSnapshot{}, err
	}
	snapshot.Values = normalizedReferenceValues(snapshot.Values)
	if snapshot.Version == "" {
		h := sha256.Sum256([]byte(strings.Join(snapshot.Values, "\x00")))
		snapshot.Version = hex.EncodeToString(h[:])
	}
	return snapshot, nil
}

func (r *Registry) ValidateDynamicReferences() error {
	for _, registered := range r.items {
		if registered.Meta.AI == nil {
			continue
		}
		for _, ref := range registered.Meta.AI.DynamicReferences {
			if _, ok := r.resolvers[ref.ResolverID]; !ok {
				return fmt.Errorf("action %s: unknown reference resolver %s", registered.ID, ref.ResolverID)
			}
			discovery, ok := r.items[ref.DiscoveryAction]
			if !ok || discovery.Meta.AI == nil || !discovery.Meta.AI.ExposeTool || discovery.Meta.AI.SideEffect != "read" || discovery.Meta.AI.RequiresConfirmation {
				return fmt.Errorf("action %s: discovery action %s must be an exposed, confirmation-free read action", registered.ID, ref.DiscoveryAction)
			}
			if len(discovery.Meta.AI.OutputSchema) == 0 || !json.Valid(discovery.Meta.AI.OutputSchema) {
				return fmt.Errorf("action %s: discovery action %s requires a valid output contract", registered.ID, ref.DiscoveryAction)
			}
			if discovery.Meta.Surface != registered.Meta.Surface {
				return fmt.Errorf("action %s: discovery action crosses realm", registered.ID)
			}
		}
	}
	visiting, visited := map[string]bool{}, map[string]bool{}
	var visit func(string) error
	visit = func(id string) error {
		if visiting[id] {
			return fmt.Errorf("action %s: circular dynamic reference dependency", id)
		}
		if visited[id] {
			return nil
		}
		visiting[id] = true
		registered := r.items[id]
		if registered.Meta.AI != nil {
			for _, ref := range registered.Meta.AI.DynamicReferences {
				if err := visit(ref.DiscoveryAction); err != nil {
					return err
				}
			}
		}
		visiting[id], visited[id] = false, true
		return nil
	}
	for id := range r.items {
		if err := visit(id); err != nil {
			return err
		}
	}
	return nil
}

func validateDynamicReferenceShapes(a Action) error {
	if a.Meta.AI == nil {
		return nil
	}
	seen := map[string]bool{}
	for _, ref := range a.Meta.AI.DynamicReferences {
		if ref.Field == "" || ref.ResolverID == "" || ref.DiscoveryAction == "" {
			return fmt.Errorf("action %s: dynamic reference fields are required", a.ID)
		}
		if seen[ref.Field] {
			return fmt.Errorf("action %s: duplicate dynamic reference field %s", a.ID, ref.Field)
		}
		seen[ref.Field] = true
		if !schemaHasStringReference(a.Meta.AI.InputSchema, ref.Field) {
			return fmt.Errorf("action %s: dynamic reference field %s is missing or not string-compatible", a.ID, ref.Field)
		}
	}
	return nil
}

func schemaHasStringReference(raw json.RawMessage, path string) bool {
	var node map[string]any
	if json.Unmarshal(raw, &node) != nil {
		return false
	}
	parts := strings.Split(strings.TrimSuffix(path, "[]"), ".")
	for _, part := range parts {
		props, _ := node["properties"].(map[string]any)
		next, _ := props[part].(map[string]any)
		if next == nil {
			return false
		}
		node = next
	}
	if strings.HasSuffix(path, "[]") {
		items, _ := node["items"].(map[string]any)
		return node["type"] == "array" && items["type"] == "string"
	}
	return node["type"] == "string"
}

func ExtractReferenceValues(args json.RawMessage, path string) ([]string, error) {
	var value any
	if err := json.Unmarshal(args, &value); err != nil {
		return nil, err
	}
	for _, part := range strings.Split(strings.TrimSuffix(path, "[]"), ".") {
		object, ok := value.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("reference field not found")
		}
		value, ok = object[part]
		if !ok {
			return nil, fmt.Errorf("reference field not found")
		}
	}
	if strings.HasSuffix(path, "[]") {
		items, ok := value.([]any)
		if !ok {
			return nil, fmt.Errorf("reference field must be an array")
		}
		out := make([]string, 0, len(items))
		for _, item := range items {
			s, ok := item.(string)
			if !ok {
				return nil, fmt.Errorf("reference value must be a string")
			}
			out = append(out, s)
		}
		return normalizedReferenceValues(out), nil
	}
	s, ok := value.(string)
	if !ok {
		return nil, fmt.Errorf("reference value must be a string")
	}
	return normalizedReferenceValues([]string{s}), nil
}

func InvalidReferenceValues(values, candidates []string) []string {
	allowed := map[string]bool{}
	for _, value := range candidates {
		allowed[value] = true
	}
	invalid := []string{}
	for _, value := range normalizedReferenceValues(values) {
		if !allowed[value] {
			invalid = append(invalid, value)
		}
	}
	return invalid
}

func (r *Registry) validateActionReferences(ctx context.Context, registered Action, input json.RawMessage) error {
	if registered.Meta.AI == nil {
		return nil
	}
	for _, ref := range registered.Meta.AI.DynamicReferences {
		values, err := ExtractReferenceValues(input, ref.Field)
		if err != nil {
			return errs.Validation("invalid dynamic reference field")
		}
		snapshot, err := r.ResolveReference(ctx, ref.ResolverID)
		if err != nil {
			return err
		}
		invalid := InvalidReferenceValues(values, snapshot.Values)
		if len(invalid) == 0 {
			continue
		}
		maxValues, maxBytes := ref.MaxInvalidValues, ref.MaxInvalidBytes
		if maxValues <= 0 {
			maxValues = 8
		}
		if maxBytes <= 0 {
			maxBytes = 512
		}
		count, size, truncated := len(invalid), 0, false
		limited := make([]string, 0, len(invalid))
		for _, value := range invalid {
			if len(limited) >= maxValues || size+len(value) > maxBytes {
				truncated = true
				break
			}
			limited = append(limited, value)
			size += len(value)
		}
		return errs.WithSafeDetails(errs.ValidationKey("reference.invalid", "dynamic reference is invalid"), errs.SafeDetails{Field: ref.Field, Reason: InvalidReference, InvalidValues: limited, InvalidCount: count, ValuesTruncated: truncated, RefreshAction: ref.DiscoveryAction, RefreshRecommended: true})
	}
	return nil
}

func normalizedReferenceValues(values []string) []string {
	set := map[string]bool{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			set[value] = true
		}
	}
	out := make([]string, 0, len(set))
	for value := range set {
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}
