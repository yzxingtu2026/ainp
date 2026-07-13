package action

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/yzxingtu2026/ainp/platform/framework/audit"
	"github.com/yzxingtu2026/ainp/platform/framework/auth"
	"github.com/yzxingtu2026/ainp/platform/framework/errs"
	"github.com/yzxingtu2026/ainp/platform/framework/logging"
	"github.com/yzxingtu2026/ainp/platform/framework/permission"
)

type Handler func(ctx context.Context, input json.RawMessage) (any, error)

type HTTPMeta struct {
	Method  string
	Path    string
	Enabled *bool
	Public  bool
}

func (h *HTTPMeta) IsEnabled() bool {
	if h == nil {
		return false
	}
	if h.Enabled == nil {
		return true
	}
	return *h.Enabled
}

type AIMeta struct {
	ExposeTool bool
	Risk       string
	Summary    string
}

type Metadata struct {
	Capability  string
	Permission  string
	Name        string
	Description string
	Audit       *bool
	HTTP        *HTTPMeta
	AI          *AIMeta
}

func (m Metadata) PermissionCode(actionID string) string {
	if m.Permission != "" {
		return m.Permission
	}
	return actionID
}

func (m Metadata) ShouldAudit() bool {
	if m.Audit == nil {
		return true
	}
	return *m.Audit
}

type Action struct {
	ID      string
	Handler Handler
	Meta    Metadata
}

type Registry struct {
	items   map[string]Action
	checker *permission.Checker
	auditor audit.Recorder
}

func NewRegistry(checker *permission.Checker, auditor audit.Recorder) *Registry {
	return &Registry{
		items:   make(map[string]Action),
		checker: checker,
		auditor: auditor,
	}
}

func (r *Registry) Register(a Action) error {
	if a.ID == "" {
		return fmt.Errorf("action id is required")
	}
	if a.Handler == nil {
		return fmt.Errorf("action handler is required: %s", a.ID)
	}
	if a.Meta.Capability == "" {
		return fmt.Errorf("action capability is required: %s", a.ID)
	}
	if _, exists := r.items[a.ID]; exists {
		return fmt.Errorf("action already registered: %s", a.ID)
	}
	r.items[a.ID] = a
	return nil
}

func (r *Registry) Get(id string) (Action, bool) {
	a, ok := r.items[id]
	return a, ok
}

func (r *Registry) List() []Action {
	out := make([]Action, 0, len(r.items))
	for _, a := range r.items {
		out = append(out, a)
	}
	return out
}

func (r *Registry) Invoke(ctx context.Context, actionID string, input json.RawMessage) (any, error) {
	a, ok := r.items[actionID]
	if !ok {
		return nil, errs.New(errs.CodeActionNotFound, "action not found")
	}

	ac := auth.MustFromContext(ctx)
	perm := a.Meta.PermissionCode(a.ID)
	public := a.Meta.HTTP != nil && a.Meta.HTTP.Public

	if !public {
		if ac == nil || !ac.Authenticated {
			err := errs.New(errs.CodeUnauthorized, "unauthorized")
			r.writeAudit(ctx, a, perm, "failure", err)
			return nil, err
		}
		if r.checker != nil && !r.checker.Allow(ac, perm) {
			err := errs.New(errs.CodeForbidden, "forbidden")
			r.writeAudit(ctx, a, perm, "failure", err)
			return nil, err
		}
	}

	start := time.Now()
	data, err := a.Handler(ctx, input)
	logging.Info(ctx, "action finished",
		"action_id", a.ID,
		"duration_ms", time.Since(start).Milliseconds(),
		"ok", err == nil,
	)
	if err != nil {
		r.writeAudit(ctx, a, perm, "failure", err)
		return nil, err
	}
	r.writeAudit(ctx, a, perm, "success", nil)
	return data, nil
}

func (r *Registry) writeAudit(ctx context.Context, a Action, perm, result string, err error) {
	if r.auditor == nil || !a.Meta.ShouldAudit() {
		return
	}
	ac := auth.MustFromContext(ctx)
	rec := audit.Record{
		OccurredAt: time.Now().UTC(),
		TenantID:   ac.TenantID,
		UserID:     ac.UserID,
		ActorType:  actorType(ac),
		ActionID:   a.ID,
		Result:     result,
		RequestID:  ac.RequestID,
		Permission: perm,
		Client:     ac.Client,
	}
	if err != nil {
		if ae, ok := errs.AsAppError(err); ok {
			rec.ErrorCode = ae.Code
			rec.Summary = ae.Message
		} else {
			rec.ErrorCode = errs.CodeInternal
			rec.Summary = "internal error"
		}
	} else {
		rec.Summary = "ok"
	}
	if werr := r.auditor.Write(ctx, rec); werr != nil {
		logging.Error(ctx, "audit write failed", werr, "action_id", a.ID)
	}
}

func actorType(ac *auth.Context) string {
	if ac == nil {
		return "system"
	}
	if ac.Client != "" {
		switch ac.Client {
		case "agent", "job", "mcp", "system":
			return ac.Client
		}
	}
	if ac.UserID == "" {
		return "system"
	}
	return "user"
}
