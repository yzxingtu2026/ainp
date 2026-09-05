package mcpserver

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/yzxingtu2026/ainp/platform/framework/action"
	"github.com/yzxingtu2026/ainp/platform/framework/aitool"
	"github.com/yzxingtu2026/ainp/platform/framework/auth"
	"github.com/yzxingtu2026/ainp/platform/framework/errs"
)

const (
	maxProjectedTools      = 100
	confirmationResumeTool = "ainp_confirmation_execute"
	toolCursorVersion      = 1
	toolCursorLifetime     = 10 * time.Minute
)

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

type toolCursorPayload struct {
	Version   int    `json:"v"`
	IssuedAt  int64  `json:"iat"`
	Directory string `json:"directory"`
	After     string `json:"after"`
}

type toolCursorCodec struct {
	key []byte
	now func() time.Time
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
	byActionID := make(map[string]projectedAction, len(projected))
	for _, item := range projected {
		item := item
		byActionID[item.definition.RuntimeRef] = item
		server.AddTool(item.tool, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return callProjectedAction(ctx, actions, confirmations, item, req)
		})
	}
	resumeTool := confirmationExecutionTool()
	cursors := newToolCursorCodec()
	server.AddTool(resumeTool, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return executeConfirmedAction(ctx, actions, confirmations, byActionID, req)
	})
	server.AddReceivingMiddleware(func(next mcp.MethodHandler) mcp.MethodHandler {
		return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
			if method != "tools/list" {
				return next(ctx, method, req)
			}
			principal, ok := principalFromContext(ctx)
			if !ok {
				return &mcp.ListToolsResult{Tools: []*mcp.Tool{}}, nil
			}
			params, _ := req.GetParams().(*mcp.ListToolsParams)
			cursor := ""
			if params != nil {
				cursor = params.Cursor
			}
			tools := visibleToolDirectory(actions, confirmations, principal, projected, resumeTool)
			return paginateToolDirectory(tools, principal, cursor, cursors)
		}
	})
	return server
}

func visibleToolDirectory(actions *action.Registry, confirmations *Service, principal *Principal, projected []projectedAction, resumeTool *mcp.Tool) []*mcp.Tool {
	tools := make([]*mcp.Tool, 0, len(projected)+1)
	if confirmations != nil && principal.Scopes[ScopeSensitive] {
		tools = append(tools, resumeTool)
	}
	for _, item := range projected {
		current, exists := actions.Get(item.definition.RuntimeRef)
		if !exists || current.Meta.AI == nil || !current.Meta.AI.ExposeTool || auth.NormalizeRealm(current.Meta.Surface) != auth.RealmTenant {
			continue
		}
		if visibleToPrincipal(principal, item.definition) {
			tools = append(tools, item.tool)
		}
	}
	return tools
}

func paginateToolDirectory(tools []*mcp.Tool, principal *Principal, cursor string, cursors toolCursorCodec) (*mcp.ListToolsResult, error) {
	start := 0
	directory := toolDirectoryFingerprint(principal, tools)
	if cursor != "" {
		payload, err := cursors.decode(cursor)
		if err != nil || payload.Directory != directory {
			return nil, invalidToolCursorError()
		}
		start = toolIndex(tools, payload.After)
		if start < 0 {
			return nil, invalidToolCursorError()
		}
		start++
	}
	end := min(start+maxProjectedTools, len(tools))
	page := append([]*mcp.Tool(nil), tools[start:end]...)
	result := &mcp.ListToolsResult{Tools: page}
	if end < len(tools) {
		nextCursor, err := cursors.encode(toolCursorPayload{
			Version: toolCursorVersion, IssuedAt: cursors.now().Unix(), Directory: directory, After: tools[end-1].Name,
		})
		if err != nil {
			return nil, err
		}
		result.NextCursor = nextCursor
	}
	return result, nil
}

func newToolCursorCodec() toolCursorCodec {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		panic(fmt.Errorf("create MCP tool cursor key: %w", err))
	}
	return toolCursorCodec{key: key, now: time.Now}
}

func (c toolCursorCodec) encode(payload toolCursorPayload) (string, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("encode MCP tool cursor: %w", err)
	}
	mac := hmac.New(sha256.New, c.key)
	_, _ = mac.Write(body)
	return base64.RawURLEncoding.EncodeToString(body) + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil)), nil
}

func (c toolCursorCodec) decode(cursor string) (toolCursorPayload, error) {
	var payload toolCursorPayload
	parts := strings.Split(cursor, ".")
	if len(parts) != 2 || len(cursor) > 2048 {
		return payload, fmt.Errorf("invalid cursor encoding")
	}
	body, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return payload, err
	}
	signature, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return payload, err
	}
	mac := hmac.New(sha256.New, c.key)
	_, _ = mac.Write(body)
	if !hmac.Equal(signature, mac.Sum(nil)) {
		return payload, fmt.Errorf("invalid cursor signature")
	}
	if err = json.Unmarshal(body, &payload); err != nil {
		return payload, err
	}
	now := c.now()
	issuedAt := time.Unix(payload.IssuedAt, 0)
	if payload.Version != toolCursorVersion || payload.Directory == "" || payload.After == "" || issuedAt.After(now) || now.Sub(issuedAt) > toolCursorLifetime {
		return toolCursorPayload{}, fmt.Errorf("invalid or expired cursor")
	}
	return payload, nil
}

func toolDirectoryFingerprint(principal *Principal, tools []*mcp.Tool) string {
	type directoryBinding struct {
		TenantID      string   `json:"tenant_id"`
		PrincipalKind string   `json:"principal_kind"`
		PrincipalID   string   `json:"principal_id"`
		AuthVersion   uint64   `json:"auth_version"`
		ClientID      string   `json:"client_id"`
		GrantID       string   `json:"grant_id"`
		Scopes        []string `json:"scopes"`
		Tools         []string `json:"tools"`
	}
	binding := directoryBinding{
		TenantID: principal.Auth.TenantID, PrincipalKind: string(principal.Auth.Principal.Kind), PrincipalID: principal.Auth.PrincipalID,
		AuthVersion: principal.Auth.AuthVersion, ClientID: principal.Auth.MCPClientID, GrantID: principal.Auth.MCPGrantID,
	}
	for scope, enabled := range principal.Scopes {
		if enabled {
			binding.Scopes = append(binding.Scopes, scope)
		}
	}
	sort.Strings(binding.Scopes)
	for _, tool := range tools {
		binding.Tools = append(binding.Tools, tool.Name)
	}
	body, _ := json.Marshal(binding)
	digest := sha256.Sum256(body)
	return hex.EncodeToString(digest[:])
}

func toolIndex(tools []*mcp.Tool, name string) int {
	for index, tool := range tools {
		if tool.Name == name {
			return index
		}
	}
	return -1
}

func invalidToolCursorError() error {
	return &jsonrpc.Error{Code: jsonrpc.CodeInvalidParams, Message: "invalid cursor"}
}

func confirmationExecutionTool() *mcp.Tool {
	readOnly, destructive, openWorld, idempotent := false, true, false, true
	return &mcp.Tool{
		Name: confirmationResumeTool, Title: "执行已批准的 AINP 操作",
		Description: "执行用户已在 AINP 网页批准的单次敏感操作。只接受确认 ID，不接受 Action ID 或业务参数。",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"confirmation_id":{"type":"string","description":"敏感操作首次调用返回的确认 ID"}},"required":["confirmation_id"],"additionalProperties":false}`),
		Annotations: &mcp.ToolAnnotations{Title: "执行已批准的 AINP 操作", ReadOnlyHint: readOnly, DestructiveHint: &destructive, OpenWorldHint: &openWorld, IdempotentHint: idempotent},
	}
}

func executeConfirmedAction(ctx context.Context, actions *action.Registry, confirmations *Service, projected map[string]projectedAction, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	principal, ok := principalFromContext(ctx)
	if !ok || confirmations == nil || !principal.Scopes[ScopeSensitive] {
		return toolError("confirmation is unavailable"), nil
	}
	var input struct {
		ConfirmationID string `json:"confirmation_id"`
	}
	if err := json.Unmarshal(req.Params.Arguments, &input); err != nil || strings.TrimSpace(input.ConfirmationID) == "" {
		return toolError("confirmation_id is required"), nil
	}
	row, err := confirmations.ConfirmationForResume(ctx, principal, strings.TrimSpace(input.ConfirmationID))
	if err != nil {
		return toolError("confirmation is unavailable"), nil
	}
	item, exists := projected[row.ActionID]
	if !exists || item.tool.Name != row.ToolName || !visibleToPrincipal(principal, item.definition) {
		return toolError("confirmation is unavailable"), nil
	}
	current, exists := actions.Get(item.definition.RuntimeRef)
	if !exists || current.Meta.AI == nil || !current.Meta.AI.ExposeTool || auth.NormalizeRealm(current.Meta.Surface) != auth.RealmTenant {
		return toolError("confirmation is unavailable"), nil
	}
	arguments, err := confirmations.ConsumeStoredConfirmation(ctx, principal, row.ID, item.definition, item.tool.Name)
	if err != nil {
		return toolError("confirmation is unavailable"), nil
	}
	authCopy := *principal.Auth
	authCopy.IdempotencyKey = "mcp-confirmation:" + row.ID
	data, err := actions.Invoke(auth.WithContext(ctx, &authCopy), item.definition.RuntimeRef, arguments)
	if err != nil {
		return toolError(safeActionError(err)), nil
	}
	return successfulToolResult(data)
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
	return definition.VisibleToActor(principal.Auth)
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
	if !exists || current.Meta.AI == nil || !current.Meta.AI.ExposeTool || auth.NormalizeRealm(current.Meta.Surface) != auth.RealmTenant || !action.AllowsActor(principal.Auth, current.Meta, current.ID) {
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
	return successfulToolResult(data)
}

func successfulToolResult(data any) (*mcp.CallToolResult, error) {
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
		StructuredContent: map[string]any{"status": "confirmation_required", "confirmation_id": pending.ID, "confirmation_url": pending.ConfirmationURL, "confirmation_token": pending.RecoveryToken, "expires_at": pending.ExpiresAt, "resume_tool": confirmationResumeTool, "resume_arguments": map[string]any{"confirmation_id": pending.ID}},
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
