package attachment

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// AccessPolicy decides whether a user may access attachments bound to a resource.
// Unregistered resource types are denied by default.
type AccessPolicy interface {
	ResourceType() string
	CanAccess(ctx context.Context, tenantID, userID, resourceID, field string) error
}

// PolicyRegistry holds code-registered attachment access policies.
type PolicyRegistry struct {
	mu       sync.RWMutex
	policies map[string]AccessPolicy
}

func NewPolicyRegistry() *PolicyRegistry {
	return &PolicyRegistry{policies: make(map[string]AccessPolicy)}
}

func (r *PolicyRegistry) Register(policy AccessPolicy) error {
	if policy == nil {
		return fmt.Errorf("attachment access policy is nil")
	}
	rt := policy.ResourceType()
	if rt == "" {
		return fmt.Errorf("attachment access policy resource_type required")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.policies[rt]; ok {
		return fmt.Errorf("duplicate attachment access policy: %s", rt)
	}
	r.policies[rt] = policy
	return nil
}

func (r *PolicyRegistry) Get(resourceType string) (AccessPolicy, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	p, ok := r.policies[resourceType]
	return p, ok
}

func (r *PolicyRegistry) Allow(ctx context.Context, resourceType, tenantID, userID, resourceID, field string) error {
	p, ok := r.Get(resourceType)
	if !ok {
		return fmt.Errorf("attachment resource_type not registered: %s", resourceType)
	}
	return p.CanAccess(ctx, tenantID, userID, resourceID, field)
}

// FuncPolicy adapts a function into an AccessPolicy for demos and tests.
type FuncPolicy struct {
	Type string
	Fn   func(ctx context.Context, tenantID, userID, resourceID, field string) error
}

func (p FuncPolicy) ResourceType() string { return p.Type }

func (p FuncPolicy) CanAccess(ctx context.Context, tenantID, userID, resourceID, field string) error {
	if p.Fn == nil {
		return fmt.Errorf("attachment access policy fn is nil")
	}
	return p.Fn(ctx, tenantID, userID, resourceID, field)
}

// UploadPolicy is the code-default (and later settings-overridable) upload constraints.
type UploadPolicy struct {
	MaxSizeBytes   int64
	AllowedMIME    []string
	AllowedExt     []string
	TempRetain     time.Duration
	TransferTTL    time.Duration
	MaxUploadsOnce int
}

func DefaultUploadPolicy() UploadPolicy {
	return UploadPolicy{
		MaxSizeBytes: 20 << 20, // 20 MiB
		AllowedMIME: []string{
			"image/png", "image/jpeg", "image/gif", "image/webp",
			"video/mp4", "video/webm", "video/quicktime",
			"application/pdf", "text/plain", "text/markdown",
			"application/vnd.openxmlformats-officedocument.wordprocessingml.document",
			"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
			"application/vnd.openxmlformats-officedocument.presentationml.presentation",
			"application/zip", "application/octet-stream",
		},
		AllowedExt: []string{
			".png", ".jpg", ".jpeg", ".gif", ".webp",
			".mp4", ".webm", ".mov",
			".pdf", ".txt", ".md", ".docx", ".xlsx", ".pptx", ".zip",
		},
		TempRetain:     24 * time.Hour,
		TransferTTL:    15 * time.Minute,
		MaxUploadsOnce: 10,
	}
}

// PolicyProvider resolves effective upload policy for a tenant.
type PolicyProvider interface {
	UploadPolicy(ctx context.Context, tenantID string) (UploadPolicy, error)
}

// StaticPolicyProvider always returns code defaults (Phase 1; #13 override later).
type StaticPolicyProvider struct {
	Policy UploadPolicy
}

func (p StaticPolicyProvider) UploadPolicy(context.Context, string) (UploadPolicy, error) {
	pol := p.Policy
	if pol.MaxSizeBytes <= 0 {
		pol = DefaultUploadPolicy()
	}
	return pol, nil
}
