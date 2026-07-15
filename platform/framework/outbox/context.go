package outbox

import (
	"context"
	"fmt"
	"strings"

	"github.com/yzxingtu2026/ainp/platform/framework/auth"
)

// eventContextKey marks an explicitly constructed EventContext for Job/system callers.
type eventContextKey struct{}

// EventContext carries trusted identity for non-HTTP producers (jobs, system tasks).
// Callers must not invent arbitrary tenants; use NewTenantEventContext / NewPlatformEventContext.
type EventContext struct {
	TenantID      string
	Realm         auth.Realm
	ActorID       string
	CorrelationID string
	CausationID   string
	Client        string // e.g. job|system
}

// WithEventContext attaches an explicit EventContext (takes precedence over AuthContext fields
// when resolving writer context, while still allowing AuthContext for logging).
func WithEventContext(ctx context.Context, ec EventContext) context.Context {
	return context.WithValue(ctx, eventContextKey{}, ec)
}

func EventContextFrom(ctx context.Context) (EventContext, bool) {
	ec, ok := ctx.Value(eventContextKey{}).(EventContext)
	return ec, ok
}

// NewTenantEventContext builds a fail-closed tenant event context for Job/system callers.
func NewTenantEventContext(tenantID, actorID, correlationID string) (EventContext, error) {
	tenantID = strings.TrimSpace(tenantID)
	if tenantID == "" {
		return EventContext{}, fmt.Errorf("outbox: tenant_id is required")
	}
	return EventContext{
		TenantID:      tenantID,
		Realm:         auth.RealmTenant,
		ActorID:       actorID,
		CorrelationID: correlationID,
		Client:        "job",
	}, nil
}

// NewPlatformEventContext builds a platform-realm event context (Phase 1: limited use).
func NewPlatformEventContext(actorID, correlationID string) EventContext {
	return EventContext{
		Realm:         auth.RealmPlatform,
		ActorID:       actorID,
		CorrelationID: correlationID,
		Client:        "system",
	}
}

type resolvedContext struct {
	TenantID      string
	Realm         auth.Realm
	ActorID       string
	CorrelationID string
	CausationID   string
}

func resolveEventContext(ctx context.Context) (resolvedContext, error) {
	if ec, ok := EventContextFrom(ctx); ok {
		realm := auth.NormalizeRealm(ec.Realm)
		if realm == auth.RealmTenant && strings.TrimSpace(ec.TenantID) == "" {
			return resolvedContext{}, fmt.Errorf("outbox: tenant_id is required for tenant realm")
		}
		return resolvedContext{
			TenantID:      strings.TrimSpace(ec.TenantID),
			Realm:         realm,
			ActorID:       firstNonEmpty(ec.ActorID),
			CorrelationID: firstNonEmpty(ec.CorrelationID),
			CausationID:   firstNonEmpty(ec.CausationID),
		}, nil
	}
	ac := auth.MustFromContext(ctx)
	realm := auth.NormalizeRealm(ac.Realm)
	if realm == auth.RealmTenant && strings.TrimSpace(ac.TenantID) == "" {
		return resolvedContext{}, fmt.Errorf("outbox: tenant_id missing from AuthContext")
	}
	return resolvedContext{
		TenantID:      strings.TrimSpace(ac.TenantID),
		Realm:         realm,
		ActorID:       strings.TrimSpace(ac.UserID),
		CorrelationID: firstNonEmpty(ac.RequestID, ac.TraceID),
	}, nil
}
