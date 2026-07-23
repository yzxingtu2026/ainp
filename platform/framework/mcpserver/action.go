package mcpserver

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/yzxingtu2026/ainp/platform/framework/action"
	"github.com/yzxingtu2026/ainp/platform/framework/aitool"
	"github.com/yzxingtu2026/ainp/platform/framework/auth"
	"github.com/yzxingtu2026/ainp/platform/framework/errs"
)

const maxProjectedTools = 100

type principalContextKey struct{}

func withPrincipal(ctx context.Context, principal *Principal) context.Context {
	return context.WithValue(ctx, principalContextKey{}, principal)
}

func principalFromContext(ctx context.Context) (*Principal, bool) {
	principal, ok := ctx.Value(principalContextKey{}).(*Principal)
	return principal, ok && principal != nil && principal.Auth != nil
}

// ContextWithPrincipalForTest builds the same trusted projection context used
// after OAuth authentication. It is exported only to support centralized tests.
func ContextWithPrincipalForTest(ctx context.Context, scopes map[string]bool) context.Context {
	return withPrincipal(ctx, &Principal{Auth: auth.MustFromContext(ctx), Scopes: scopes})
}

type projectedAction struct {
	definition aitool.Definition
	tool       *mcp.Tool
}

// NewActionServer projects the Action Registry into MCP without introducing a
// second business registration source.
func NewActionServer(actions *action.Registry, confirmations *Service) *mcp.Server {
	server := mcp.NewServer(&mcp.Implementation{Name: "AINP", Version: "1.0.0"}, &mcp.ServerOptions{PageSize: maxProjectedTools})
	if actions == nil {
		return server
	}
	projected, err := projectActions(actions)
	if err != nil {
		panic(err)
	}
	for _, item := range projected {
		item := item
		server.AddTool(item.tool, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return callProjectedAction(ctx, actions, confirmations, item, req)
		})
	}
	server.AddReceivingMiddleware(func(next mcp.MethodHandler) mcp.MethodHandler {
		return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
			if method != "tools/list" {
				return next(ctx, method, req)
			}
			principal, ok := principalFromContext(ctx)
			if !ok {
				return &mcp.ListToolsResult{Tools: []*mcp.Tool{}}, nil
			}
			tools := make([]*mcp.Tool, 0, len(projected))
			for _, item := range projected {
				if visibleToPrincipal(principal, item.definition) {
					tools = append(tools, item.tool)
				}
			}
			// The registry is bounded here; pagination is intentionally delegated
			// to a capped directory until projected Action counts exceed this guard.
			if len(tools) > maxProjectedTools {
				tools = tools[:maxProjectedTools]
			}
			return &mcp.ListToolsResult{Tools: tools}, nil
		}
	})
	return server
}

func projectActions(actions *action.Registry) ([]projectedAction, error) {
	definitions, err := (&aitool.ActionProvider{Actions: actions}).List(context.Background())
	if err != nil {
		return nil, err
	}
	sort.Slice(definitions, func(i, j int) bool { return definitions[i].RuntimeRef < definitions[j].RuntimeRef })
	seen := make(map[string]string, len(definitions))
	result := make([]projectedAction, 0, len(definitions))
	for _, definition := range definitions {
		if definition.Source != aitool.SourceAction || definition.Status != "enabled" {
			continue
		}
		registered, ok := actions.Get(definition.RuntimeRef)
		if !ok || auth.NormalizeRealm(registered.Meta.Surface) != auth.RealmTenant {
			continue
		}
		name := MCPToolName(definition.RuntimeRef)
		if previous, exists := seen[name]; exists {
			return nil, fmt.Errorf("MCP tool name collision: %s and %s", previous, definition.RuntimeRef)
		}
		seen[name] = definition.RuntimeRef
		result = append(result, projectedAction{definition: definition, tool: definitionToTool(name, definition)})
	}
	return result, nil
}

var invalidToolNamePart = regexp.MustCompile(`[^a-z0-9_-]+`)

// MCPToolName creates a readable, deterministic MCP-safe name. The hash keeps
// normalization collisions from silently replacing another Action.
func MCPToolName(actionID string) string {
	normalized := invalidToolNamePart.ReplaceAllString(strings.ToLower(strings.TrimSpace(actionID)), "_")
	normalized = strings.Trim(normalized, "_-")
	if normalized == "" {
		normalized = "action"
	}
	if len(normalized) > 80 {
		normalized = normalized[:80]
	}
	digest := sha256.Sum256([]byte(actionID))
	return "ainp_" + normalized + "_" + hex.EncodeToString(digest[:6])
}

func definitionToTool(name string, definition aitool.Definition) *mcp.Tool {
	readOnly := definition.SideEffect == aitool.SideEffectNone || definition.SideEffect == aitool.SideEffectRead
	destructive := definition.SideEffect == aitool.SideEffectDestructive
	openWorld := definition.SideEffect == aitool.SideEffectExternal || definition.SideEffect == aitool.SideEffectPaid
	return &mcp.Tool{
		Name: name, Title: definition.TitleForHuman, Description: definition.DescriptionForLLM,
		InputSchema: definition.InputSchema, OutputSchema: optionalSchema(definition.OutputSchema),
		Annotations: &mcp.ToolAnnotations{Title: definition.TitleForHuman, ReadOnlyHint: readOnly, DestructiveHint: &destructive, OpenWorldHint: &openWorld, IdempotentHint: readOnly},
	}
}

func optionalSchema(schema json.RawMessage) any {
	if len(schema) == 0 {
		return nil
	}
	var value map[string]any
	if json.Unmarshal(schema, &value) != nil || value["type"] != "object" {
		return nil
	}
	return schema
}

func requiredScope(definition aitool.Definition) string {
	if definition.Risk == aitool.RiskCritical || definition.SideEffect == aitool.SideEffectExternal || definition.SideEffect == aitool.SideEffectPaid || definition.SideEffect == aitool.SideEffectDestructive {
		return ScopeSensitive
	}
	if definition.SideEffect == aitool.SideEffectWrite {
		return ScopeWrite
	}
	return ScopeRead
}

func visibleToPrincipal(principal *Principal, definition aitool.Definition) bool {
	if principal == nil || principal.Auth == nil || !principal.Auth.Authenticated || auth.NormalizeRealm(principal.Auth.Realm) != auth.RealmTenant || definition.Status != "enabled" {
		return false
	}
	if !principal.Scopes[requiredScope(definition)] {
		return false
	}
	for _, permission := range definition.PermissionRefs {
		if !principal.Auth.HasPermission(permission) {
			return false
		}
	}
	return true
}

func callProjectedAction(ctx context.Context, actions *action.Registry, confirmations *Service, projected projectedAction, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	principal, ok := principalFromContext(ctx)
	if !ok || !visibleToPrincipal(principal, projected.definition) {
		if ok {
			actions.AuditRejected(auth.WithContext(ctx, principal.Auth), projected.definition.RuntimeRef, errs.New(errs.CodeForbidden, "tool is unavailable"))
		}
		return toolError("tool is unavailable"), nil
	}
	current, exists := actions.Get(projected.definition.RuntimeRef)
	if !exists || current.Meta.AI == nil || !current.Meta.AI.ExposeTool || auth.NormalizeRealm(current.Meta.Surface) != auth.RealmTenant {
		return toolError("tool is unavailable"), nil
	}
	input := req.Params.Arguments
	if len(input) == 0 {
		input = json.RawMessage("{}")
	}
	if requiredScope(projected.definition) == ScopeSensitive {
		if confirmations == nil {
			return toolError("tool is unavailable"), nil
		}
		recovery, _ := req.Params.Meta["confirmationToken"].(string)
		if recovery == "" {
			recovery, _ = req.Params.Meta["confirmation_token"].(string)
		}
		if strings.TrimSpace(recovery) == "" {
			pending, err := confirmations.CreateConfirmation(ctx, principal, projected.definition, projected.tool.Name, input)
			if err != nil {
				return toolError(safeActionError(err)), nil
			}
			actions.AuditDeferred(auth.WithContext(ctx, principal.Auth), projected.definition.RuntimeRef, "sensitive MCP action requires per-call confirmation")
			if req.Session != nil {
				elicited, elicitErr := req.Session.Elicit(ctx, &mcp.ElicitParams{
					Mode: "url", Message: "请在 AINP 中确认本次敏感操作", URL: pending.ConfirmationURL, ElicitationID: pending.ID,
				})
				if elicitErr == nil && elicited != nil && elicited.Action != "accept" {
					_, _ = confirmations.DecideConfirmation(ctx, principal.Auth, pending.RecoveryToken, "reject")
					return toolError("confirmation was declined"), nil
				}
			}
			return confirmationRequired(pending), nil
		}
		if err := confirmations.ConsumeConfirmation(ctx, principal, strings.TrimSpace(recovery), projected.definition, projected.tool.Name, input); err != nil {
			return toolError("confirmation is unavailable"), nil
		}
	}
	authCopy := *principal.Auth
	if key, _ := req.Params.Meta["idempotencyKey"].(string); strings.TrimSpace(key) != "" {
		authCopy.IdempotencyKey = strings.TrimSpace(key)
	} else if key, _ := req.Params.Meta["idempotency_key"].(string); strings.TrimSpace(key) != "" {
		authCopy.IdempotencyKey = strings.TrimSpace(key)
	}
	data, err := actions.Invoke(auth.WithContext(ctx, &authCopy), projected.definition.RuntimeRef, input)
	if err != nil {
		return toolError(safeActionError(err)), nil
	}
	encoded, err := json.Marshal(data)
	if err != nil {
		return toolError("tool execution failed"), nil
	}
	structured := data
	if _, ok := data.(map[string]any); !ok {
		structured = map[string]any{"result": data}
	}
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: string(encoded)}}, StructuredContent: structured}, nil
}

func confirmationRequired(pending *ConfirmationRequest) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		Content:           []mcp.Content{&mcp.TextContent{Text: "confirmation_required: open " + pending.ConfirmationURL}},
		StructuredContent: map[string]any{"status": "confirmation_required", "confirmation_url": pending.ConfirmationURL, "confirmation_token": pending.RecoveryToken, "expires_at": pending.ExpiresAt},
		IsError:           true,
	}
}

func toolError(message string) *mcp.CallToolResult {
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: message}}, IsError: true}
}

func safeActionError(err error) string {
	appErr, ok := errs.AsAppError(err)
	if !ok {
		return "tool execution failed"
	}
	switch appErr.Code {
	case errs.CodeValidation, errs.CodeConflict, errs.CodeIdempotencyKeyRequired, errs.CodeIdempotencyConflict, errs.CodeOperationInProgress, errs.CodeRevisionConflict, errs.CodeResourceLocked, errs.CodeTooManyRequests:
		return appErr.Message
	case errs.CodeUnauthorized, errs.CodeForbidden, errs.CodeNotFound, errs.CodeActionNotFound:
		return "tool is unavailable"
	default:
		return "tool execution failed"
	}
}
