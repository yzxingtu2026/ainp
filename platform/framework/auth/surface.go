package auth

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

const TenantConsoleSurface = "ainp.tenant-console"

type SessionOverflowPolicy string

const (
	SessionOverflowReject       SessionOverflowPolicy = "reject"
	SessionOverflowRevokeOldest SessionOverflowPolicy = "revoke_oldest"
)

var surfaceIDPattern = regexp.MustCompile(`^[a-z][a-z0-9]*(?:[.-][a-z][a-z0-9-]*)+$`)

type ClientSurface struct {
	ID                string
	Audience          string
	PathPrefix        string
	PrincipalKinds    []PrincipalKind
	Methods           []string
	AbsoluteTTL       time.Duration
	IdleTTL           time.Duration
	MaxActiveSessions int
	OverflowPolicy    SessionOverflowPolicy
}

func BuiltinTenantConsoleSurface() ClientSurface {
	return ClientSurface{ID: TenantConsoleSurface, Audience: TenantAudience, PathPrefix: "/api/", PrincipalKinds: []PrincipalKind{PrincipalKindAccount}, Methods: []string{"password"}}
}

func (s ClientSurface) AllowsKind(kind PrincipalKind) bool {
	for _, allowed := range s.PrincipalKinds {
		if allowed == kind {
			return true
		}
	}
	return false
}

func (s ClientSurface) AllowsMethod(method string) bool {
	for _, allowed := range s.Methods {
		if allowed == method {
			return true
		}
	}
	return false
}

type ClientSurfaceRegistry struct {
	mu     sync.RWMutex
	items  map[string]ClientSurface
	sealed bool
}

func NewClientSurfaceRegistry() *ClientSurfaceRegistry {
	return &ClientSurfaceRegistry{items: make(map[string]ClientSurface)}
}

func (r *ClientSurfaceRegistry) Register(surface ClientSurface) error {
	surface.ID = strings.TrimSpace(surface.ID)
	surface.Audience = strings.TrimSpace(surface.Audience)
	surface.PathPrefix = strings.TrimSpace(surface.PathPrefix)
	if !surfaceIDPattern.MatchString(surface.ID) || surface.Audience == "" || !strings.HasPrefix(surface.PathPrefix, "/") || len(surface.PrincipalKinds) == 0 || len(surface.Methods) == 0 {
		return fmt.Errorf("auth: invalid client surface %q", surface.ID)
	}
	if surface.AbsoluteTTL < 0 || surface.IdleTTL < 0 || surface.MaxActiveSessions < 0 {
		return fmt.Errorf("auth: surface %s has invalid session policy", surface.ID)
	}
	if surface.OverflowPolicy != "" && surface.OverflowPolicy != SessionOverflowReject && surface.OverflowPolicy != SessionOverflowRevokeOldest {
		return fmt.Errorf("auth: surface %s has invalid overflow policy %q", surface.ID, surface.OverflowPolicy)
	}
	if surface.MaxActiveSessions > 0 && surface.OverflowPolicy == "" {
		surface.OverflowPolicy = SessionOverflowReject
	}
	for _, kind := range surface.PrincipalKinds {
		if !kind.Valid() {
			return fmt.Errorf("auth: surface %s has invalid principal kind %q", surface.ID, kind)
		}
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.sealed {
		return fmt.Errorf("auth: client surface registry is sealed")
	}
	for _, existing := range r.items {
		if existing.Audience == surface.Audience {
			return fmt.Errorf("auth: duplicate client surface audience %q", surface.Audience)
		}
		if existing.PathPrefix == surface.PathPrefix {
			return fmt.Errorf("auth: duplicate client surface prefix %q", surface.PathPrefix)
		}
	}
	if _, exists := r.items[surface.ID]; exists {
		return fmt.Errorf("auth: client surface already registered: %s", surface.ID)
	}
	r.items[surface.ID] = surface
	return nil
}

func (r *ClientSurfaceRegistry) Get(id string) (ClientSurface, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	v, ok := r.items[id]
	return v, ok
}

func (r *ClientSurfaceRegistry) ResolvePath(path string) (ClientSurface, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var matched ClientSurface
	for _, surface := range r.items {
		if strings.HasPrefix(path, surface.PathPrefix) && len(surface.PathPrefix) > len(matched.PathPrefix) {
			matched = surface
		}
	}
	return matched, matched.ID != ""
}

func (r *ClientSurfaceRegistry) List() []ClientSurface {
	r.mu.RLock()
	defer r.mu.RUnlock()
	items := make([]ClientSurface, 0, len(r.items))
	for _, item := range r.items {
		items = append(items, item)
	}
	sort.Slice(items, func(i, j int) bool { return items[i].ID < items[j].ID })
	return items
}

func (r *ClientSurfaceRegistry) Validate(principals *PrincipalRegistry) error {
	for _, surface := range r.List() {
		for _, kind := range surface.PrincipalKinds {
			if _, ok := principals.Get(kind); !ok {
				return fmt.Errorf("auth: surface %s references unregistered principal kind %s", surface.ID, kind)
			}
		}
	}
	return nil
}

func (r *ClientSurfaceRegistry) Seal() { r.mu.Lock(); r.sealed = true; r.mu.Unlock() }
