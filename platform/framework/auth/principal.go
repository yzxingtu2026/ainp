package auth

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"sync"
)

type PrincipalKind string

const (
	PrincipalKindAccount PrincipalKind = "ainp.account"
	PrincipalKindSystem  PrincipalKind = "ainp.system"
)

var principalKindPattern = regexp.MustCompile(`^[a-z][a-z0-9]*(?:[.-][a-z][a-z0-9]*)+$`)

func (k PrincipalKind) Valid() bool { return principalKindPattern.MatchString(string(k)) }

type PrincipalRef struct {
	Kind PrincipalKind `json:"kind"`
	ID   string        `json:"id"`
}

func (r PrincipalRef) Valid() bool { return r.Kind.Valid() && strings.TrimSpace(r.ID) != "" }

type PrincipalSnapshot struct {
	Ref         PrincipalRef
	TenantID    string
	AuthVersion uint64
	Active      bool
	OrgID       string
	DeptID      string
	Roles       []string
	Permissions []string
}

type PrincipalProvider interface {
	Kind() PrincipalKind
	LoadAuthorization(context.Context, AuthorizationRequest) (*AuthorizationSnapshot, error)
}

type PrincipalRegistry struct {
	mu        sync.RWMutex
	providers map[PrincipalKind]PrincipalProvider
	sealed    bool
}

func NewPrincipalRegistry() *PrincipalRegistry {
	return &PrincipalRegistry{providers: make(map[PrincipalKind]PrincipalProvider)}
}

func (r *PrincipalRegistry) Register(provider PrincipalProvider) error {
	if provider == nil {
		return fmt.Errorf("auth: principal provider is required")
	}
	kind := provider.Kind()
	if !kind.Valid() {
		return fmt.Errorf("auth: invalid principal kind %q", kind)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.sealed {
		return fmt.Errorf("auth: principal registry is sealed")
	}
	if _, exists := r.providers[kind]; exists {
		return fmt.Errorf("auth: principal kind already registered: %s", kind)
	}
	r.providers[kind] = provider
	return nil
}

func (r *PrincipalRegistry) Get(kind PrincipalKind) (PrincipalProvider, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	provider, ok := r.providers[kind]
	return provider, ok
}

func (r *PrincipalRegistry) Kinds() []PrincipalKind {
	r.mu.RLock()
	defer r.mu.RUnlock()
	kinds := make([]PrincipalKind, 0, len(r.providers))
	for kind := range r.providers {
		kinds = append(kinds, kind)
	}
	sort.Slice(kinds, func(i, j int) bool { return kinds[i] < kinds[j] })
	return kinds
}

func (r *PrincipalRegistry) Seal() { r.mu.Lock(); r.sealed = true; r.mu.Unlock() }

func (r *PrincipalRegistry) LoadAuthorization(ctx context.Context, req AuthorizationRequest) (*AuthorizationSnapshot, error) {
	principal := req.PrincipalRef()
	provider, ok := r.Get(principal.Kind)
	if !ok {
		return nil, ErrAuthorizationInvalid
	}
	req.Principal = principal
	req.PrincipalID = principal.ID
	snapshot, err := provider.LoadAuthorization(ctx, req)
	if err != nil || snapshot == nil {
		return snapshot, err
	}
	if !snapshot.Principal.Valid() {
		snapshot.Principal = PrincipalRef{Kind: principal.Kind, ID: snapshot.PrincipalID}
	}
	if snapshot.Principal != principal {
		return nil, ErrAuthorizationInvalid
	}
	return snapshot, nil
}
