package authentication

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/yzxingtu2026/ainp/platform/framework/auth"
)

type Capabilities struct {
	Challenge bool   `json:"challenge"`
	Redirect  bool   `json:"redirect"`
	Label     string `json:"label,omitempty"`
	LabelKey  string `json:"label_key,omitempty"`
	Icon      string `json:"icon,omitempty"`
	Order     int    `json:"order,omitempty"`
}

var (
	labelKeyPattern = regexp.MustCompile(`^[a-z][a-z0-9]*(?:[._-][a-z0-9]+)*$`)
	iconPattern     = regexp.MustCompile(`^[a-z0-9]+:[a-z0-9][a-z0-9-]*$`)
)

type BeginRequest struct {
	Client  string
	Payload json.RawMessage
}

type Challenge struct {
	ID        string         `json:"challenge_id"`
	ExpiresIn int            `json:"expires_in"`
	Data      map[string]any `json:"data,omitempty"`
}

type CompleteRequest struct {
	ChallengeID string
	Client      string
	Payload     json.RawMessage
}

type VerifiedIdentity struct {
	Provider             string
	Subject              string
	NormalizedIdentifier string
	TenantID             string
	Principal            auth.PrincipalRef
	AuthVersion          uint64
}

type Authenticator interface {
	Method() string
	Capabilities() Capabilities
	Begin(context.Context, BeginRequest) (*Challenge, error)
	Complete(context.Context, CompleteRequest) (*VerifiedIdentity, error)
}

// AvailabilityChecker is an optional request-context-aware boundary for
// authenticators backed by runtime configuration such as tenant integrations.
// Returning false hides the method and blocks Begin/Complete consistently.
type AvailabilityChecker interface {
	Available(context.Context) bool
}

type registryItem struct {
	provider Authenticator
	method   Method
}

type Registry struct{ items map[string]registryItem }

func NewRegistry() *Registry { return &Registry{items: make(map[string]registryItem)} }

func (r *Registry) Register(provider Authenticator) error {
	if provider == nil {
		return fmt.Errorf("authentication: authenticator is required")
	}
	method := strings.TrimSpace(provider.Method())
	if method == "" {
		return fmt.Errorf("authentication: method is required")
	}
	if method != strings.ToLower(method) || strings.ContainsAny(method, " /:") {
		return fmt.Errorf("authentication: invalid method %q", method)
	}
	capabilities := provider.Capabilities()
	if capabilities.Redirect && !capabilities.Challenge {
		return fmt.Errorf("authentication: redirect method %q must support challenge", method)
	}
	capabilities.Label = strings.TrimSpace(capabilities.Label)
	capabilities.LabelKey = strings.TrimSpace(capabilities.LabelKey)
	capabilities.Icon = strings.TrimSpace(capabilities.Icon)
	if len(capabilities.Label) > 80 || strings.ContainsAny(capabilities.Label, "\r\n\t") {
		return fmt.Errorf("authentication: invalid label for method %q", method)
	}
	if capabilities.LabelKey != "" && !labelKeyPattern.MatchString(capabilities.LabelKey) {
		return fmt.Errorf("authentication: invalid label key for method %q", method)
	}
	if capabilities.Icon != "" && !iconPattern.MatchString(capabilities.Icon) {
		return fmt.Errorf("authentication: invalid icon for method %q", method)
	}
	if capabilities.Order < 0 || capabilities.Order > 10000 {
		return fmt.Errorf("authentication: invalid order for method %q", method)
	}
	if _, exists := r.items[method]; exists {
		return fmt.Errorf("authentication: method already registered: %s", method)
	}
	r.items[method] = registryItem{provider: provider, method: Method{Method: method, Capabilities: capabilities}}
	return nil
}

// Available evaluates the optional runtime boundary and fails closed if a
// downstream checker panics. Provider-specific failures are never exposed.
func Available(ctx context.Context, provider Authenticator) (available bool) {
	checker, ok := provider.(AvailabilityChecker)
	if !ok {
		return true
	}
	defer func() {
		if recover() != nil {
			available = false
		}
	}()
	return checker.Available(ctx)
}

func (r *Registry) Get(method string) (Authenticator, bool) {
	item, ok := r.items[strings.TrimSpace(method)]
	return item.provider, ok
}

type Method struct {
	Method       string       `json:"method"`
	Capabilities Capabilities `json:"capabilities"`
}

func (r *Registry) Methods() []Method {
	methods := make([]Method, 0, len(r.items))
	for _, item := range r.items {
		methods = append(methods, item.method)
	}
	sort.Slice(methods, func(i, j int) bool {
		left, right := methods[i].Capabilities.Order, methods[j].Capabilities.Order
		if left != right {
			return left < right
		}
		return methods[i].Method < methods[j].Method
	})
	return methods
}
