package action

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/yzxingtu2026/ainp/platform/framework/audit"
	"github.com/yzxingtu2026/ainp/platform/framework/auth"
	"github.com/yzxingtu2026/ainp/platform/framework/errs"
	"github.com/yzxingtu2026/ainp/platform/framework/logging"
	"github.com/yzxingtu2026/ainp/platform/framework/permission"
)

type Handler func(ctx context.Context, input json.RawMessage) (any, error)

type AccessMode string

const (
	AccessPublic        AccessMode = "public"
	AccessAuthenticated AccessMode = "authenticated"
	AccessPermission    AccessMode = "permission"
)

type HTTPMeta struct {
	Method  string
	Path    string
	Enabled *bool
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
	ExposeTool           bool
	ExclusionReason      string
	Risk                 string
	SideEffect           string
	RequiresConfirmation bool
	Summary              string
	HumanDescription     string
	Tags                 []string
	ResultSummary        string
	InputSchema          json.RawMessage
	OutputSchema         json.RawMessage
}

const (
	AIExcludePendingSafetyReview  = "pending-safety-review"
	AIExcludePublicAuthentication = "public-authentication-entry"
	AIExcludeSetupOperation       = "installation-operation"
	AIExcludeInternalRuntime      = "internal-runtime-operation"
	AIExcludeSensitiveCredential  = "sensitive-credential-boundary"
)

func ExcludeFromAI(reason string) *AIMeta {
	return &AIMeta{ExclusionReason: reason}
}

// IdempotencyMeta 控制 Action 的幂等策略。
type IdempotencyMeta struct {
	Mode                string        // "none" | "optional" | "required"
	TTL                 time.Duration // 幂等记录保留时间，默认 24h
	InProgressTimeout   time.Duration // processing 超时，默认 30s
	CacheBusinessErrors bool          // 是否缓存业务错误响应
}

type Metadata struct {
	Capability             string
	Access                 AccessMode
	PermissionGroup        string
	PermissionGroupName    string
	PermissionGroupNameKey string
	Surface                auth.Realm
	Permission             string
	Name                   string // 兜底文案
	NameKey                string // i18n key，如 action.system.auth_login.name
	Description            string
	DescriptionKey         string // i18n key，如 action.system.auth_login.description
	Audit                  *bool
	HTTP                   *HTTPMeta
	AI                     *AIMeta
	Idempotency            *IdempotencyMeta
}

func (m Metadata) AccessMode() AccessMode {
	if m.Access == "" {
		return AccessPermission
	}
	return m.Access
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

// IdempotencyMode 返回幂等模式，未声明时默认 "none"。
func (m Metadata) IdempotencyMode() string {
	if m.Idempotency == nil {
		return "none"
	}
	return m.Idempotency.Mode
}

func validateIdempotency(meta *IdempotencyMeta) error {
	if meta == nil {
		return nil
	}
	switch meta.Mode {
	case "none":
		if meta.TTL != 0 || meta.InProgressTimeout != 0 {
			return fmt.Errorf("idempotency mode=none must not set TTL or InProgressTimeout")
		}
	case "optional", "required":
		// defaults are fine
	default:
		return fmt.Errorf("idempotency mode must be none/optional/required, got %q", meta.Mode)
	}
	if meta.TTL < 0 {
		return fmt.Errorf("idempotency TTL must be >= 0")
	}
	if meta.InProgressTimeout < 0 {
		return fmt.Errorf("idempotency InProgressTimeout must be >= 0")
	}
	return nil
}

type Action struct {
	ID      string
	Handler Handler
	Meta    Metadata
}

type Registry struct {
	items               map[string]Action
	checker             *permission.Checker
	auditor             audit.Recorder
	idempotencyExecutor IdempotencyExecutor
}

// IdempotencyExecutor is the interface for idempotent execution.
type IdempotencyExecutor interface {
	Execute(ctx context.Context, actionID string, input json.RawMessage, handler Handler, mode string, cacheBusinessErrors bool, ttl, inProgressTimeout time.Duration) (any, bool, error)
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
	a.Meta.Surface = auth.NormalizeRealm(a.Meta.Surface)
	if !a.Meta.Surface.Valid() {
		return fmt.Errorf("invalid action surface: %s", a.Meta.Surface)
	}
	a.Meta.Access = a.Meta.AccessMode()
	switch a.Meta.Access {
	case AccessPublic, AccessAuthenticated:
		if a.Meta.Permission != "" {
			return fmt.Errorf("action %s: %s access must not declare permission", a.ID, a.Meta.Access)
		}
	case AccessPermission:
	default:
		return fmt.Errorf("action %s: invalid access mode %q", a.ID, a.Meta.Access)
	}
	if err := validateIdempotency(a.Meta.Idempotency); err != nil {
		return fmt.Errorf("action %s: %w", a.ID, err)
	}
	if a.Meta.AI != nil {
		if err := ValidateAIClassification(a.ID, a.Meta.AI); err != nil {
			return err
		}
	}
	if _, exists := r.items[a.ID]; exists {
		return fmt.Errorf("action already registered: %s", a.ID)
	}
	r.items[a.ID] = a
	return nil
}

func ValidateAIClassification(actionID string, meta *AIMeta) error {
	if meta == nil {
		return fmt.Errorf("action %s: AI classification is required", actionID)
	}
	reason := strings.TrimSpace(meta.ExclusionReason)
	if meta.ExposeTool {
		if reason != "" {
			return fmt.Errorf("action %s: exposed AI action must not declare an exclusion reason", actionID)
		}
		return validateAIMeta(actionID, meta)
	}
	if reason == "" {
		return fmt.Errorf("action %s: excluded AI action requires an exclusion reason", actionID)
	}
	return nil
}

func validateAIMeta(actionID string, meta *AIMeta) error {
	if meta.Summary == "" || meta.HumanDescription == "" {
		return fmt.Errorf("action %s: AI summary and human description are required", actionID)
	}
	if strings.TrimSpace(meta.ResultSummary) == "" {
		return fmt.Errorf("action %s: AI result summary is required", actionID)
	}
	switch meta.Risk {
	case "low", "medium", "high", "critical":
	default:
		return fmt.Errorf("action %s: invalid AI risk", actionID)
	}
	switch meta.SideEffect {
	case "none", "read", "write", "external", "paid", "destructive":
	default:
		return fmt.Errorf("action %s: invalid AI side effect", actionID)
	}
	if (meta.Risk == "high" || meta.Risk == "critical" || meta.SideEffect == "write" || meta.SideEffect == "external" || meta.SideEffect == "paid" || meta.SideEffect == "destructive") && !meta.RequiresConfirmation {
		return fmt.Errorf("action %s: high-risk and side-effecting AI actions must require confirmation", actionID)
	}
	if len(meta.InputSchema) == 0 || !json.Valid(meta.InputSchema) {
		return fmt.Errorf("action %s: valid AI input schema is required", actionID)
	}
	if len(meta.OutputSchema) > 0 && !json.Valid(meta.OutputSchema) {
		return fmt.Errorf("action %s: AI output schema is invalid", actionID)
	}
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

func (r *Registry) SetIdempotencyExecutor(executor IdempotencyExecutor) {
	r.idempotencyExecutor = executor
}

func (r *Registry) Invoke(ctx context.Context, actionID string, input json.RawMessage) (any, error) {
	a, ok := r.items[actionID]
	if !ok {
		return nil, errs.New(errs.CodeActionNotFound, "action not found")
	}

	ac := auth.MustFromContext(ctx)
	if auth.NormalizeRealm(ac.Realm) != a.Meta.Surface {
		err := errs.New(errs.CodeForbidden, "action is not available in current realm")
		r.writeAudit(ctx, a, a.Meta.PermissionCode(a.ID), "failure", err)
		return nil, err
	}
	perm := a.Meta.PermissionCode(a.ID)
	if a.Meta.AccessMode() != AccessPublic {
		if err := RequireAuthenticated(ctx); err != nil {
			r.writeAudit(ctx, a, perm, "failure", err)
			return nil, err
		}
		if a.Meta.AccessMode() == AccessPermission && r.checker != nil && !r.checker.Allow(ac, perm) {
			err := errs.New(errs.CodeForbidden, "forbidden")
			r.writeAudit(ctx, a, perm, "failure", err)
			return nil, err
		}
	}

	start := time.Now()

	var data any
	var isReplay bool

	// Idempotency execution path
	if a.Meta.Idempotency != nil && a.Meta.Idempotency.Mode != "none" && r.idempotencyExecutor != nil {
		ttl := a.Meta.Idempotency.TTL
		if ttl == 0 {
			ttl = 24 * time.Hour
		}
		ipt := a.Meta.Idempotency.InProgressTimeout
		if ipt == 0 {
			ipt = 30 * time.Second
		}
		var execErr error
		data, isReplay, execErr = r.idempotencyExecutor.Execute(ctx, a.ID, input, a.Handler, a.Meta.Idempotency.Mode, a.Meta.Idempotency.CacheBusinessErrors, ttl, ipt)
		logging.Info(ctx, "action finished",
			"action_id", a.ID,
			"duration_ms", time.Since(start).Milliseconds(),
			"ok", execErr == nil,
			"replay", isReplay,
		)
		if execErr != nil {
			r.writeAuditWithReplay(ctx, a, perm, "failure", execErr, isReplay)
			return nil, execErr
		}
		r.writeAuditWithReplay(ctx, a, perm, "success", nil, isReplay)
		return data, nil
	}

	// Normal execution path
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

// RequireAuthenticated is the shared access policy for authenticated-only endpoints.
func RequireAuthenticated(ctx context.Context) error {
	ac, ok := auth.FromContext(ctx)
	if !ok || ac == nil || !ac.Authenticated {
		return errs.New(errs.CodeUnauthorized, "unauthorized")
	}
	return nil
}

func (r *Registry) writeAudit(ctx context.Context, a Action, perm, result string, err error) {
	r.writeAuditWithReplay(ctx, a, perm, result, err, false)
}

func (r *Registry) writeAuditWithReplay(ctx context.Context, a Action, perm, result string, err error, replay bool) {
	if r.auditor == nil || !a.Meta.ShouldAudit() {
		return
	}
	ac := auth.MustFromContext(ctx)
	rec := audit.Record{
		OccurredAt:       time.Now().UTC(),
		TenantID:         ac.TenantID,
		Realm:            string(auth.NormalizeRealm(ac.Realm)),
		UserID:           ac.UserID,
		ActorType:        actorType(ac),
		ActionID:         a.ID,
		Result:           result,
		RequestID:        ac.RequestID,
		TraceID:          optionalAuditValue(ac.TraceID),
		AgentRunID:       optionalAuditValue(ac.AgentRunID),
		ToolInvocationID: optionalAuditValue(ac.ToolInvocationID),
		Permission:       perm,
		Client:           ac.Client,
		ResourceType:     ac.AuditResourceType,
		ResourceID:       ac.AuditResourceID,
		Replay:           replay,
	}
	if replay && err == nil {
		rec.Summary = "replay"
	} else if err != nil {
		if ae, ok := errs.AsAppError(err); ok {
			rec.ErrorCode = ae.Code
			rec.Summary = ae.Message
		} else {
			rec.ErrorCode = errs.CodeInternal
			rec.Summary = "internal error"
		}
	} else {
		rec.Summary = ac.AuditSummary
		if rec.Summary == "" {
			rec.Summary = "ok"
		}
	}
	if werr := r.auditor.Write(ctx, rec); werr != nil {
		logging.Error(ctx, "audit write failed", werr, "action_id", a.ID)
	}
}

func optionalAuditValue(value string) *string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return &value
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
