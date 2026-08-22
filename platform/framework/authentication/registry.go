package authentication

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/yzxingtu2026/ainp/platform/framework/auth"
)

type Capabilities struct {
	Challenge bool   `json:"challenge"`
	Redirect  bool   `json:"redirect"`
	LabelKey  string `json:"label_key,omitempty"`
}

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

type Registry struct{ items map[string]Authenticator }

func NewRegistry() *Registry { return &Registry{items: make(map[string]Authenticator)} }

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
	if _, exists := r.items[method]; exists {
		return fmt.Errorf("authentication: method already registered: %s", method)
	}
	r.items[method] = provider
	return nil
}

func (r *Registry) Get(method string) (Authenticator, bool) {
	provider, ok := r.items[strings.TrimSpace(method)]
	return provider, ok
}

type Method struct {
	Method       string       `json:"method"`
	Capabilities Capabilities `json:"capabilities"`
}

func (r *Registry) Methods() []Method {
	methods := make([]Method, 0, len(r.items))
	for method, provider := range r.items {
		methods = append(methods, Method{Method: method, Capabilities: provider.Capabilities()})
	}
	sort.Slice(methods, func(i, j int) bool { return methods[i].Method < methods[j].Method })
	return methods
}
