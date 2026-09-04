package tests

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/yzxingtu2026/ainp/platform/framework/action"
	"github.com/yzxingtu2026/ainp/platform/framework/audit"
	"github.com/yzxingtu2026/ainp/platform/framework/auth"
	"github.com/yzxingtu2026/ainp/platform/framework/mcpserver"
	"github.com/yzxingtu2026/ainp/platform/framework/permission"
)

func TestMCPToolNameIsStableAndLegal(t *testing.T) {
	first := mcpserver.MCPToolName("system.account_list")
	second := mcpserver.MCPToolName("system.account_list")
	if first != second || !strings.HasPrefix(first, "ainp_system_account_list_") {
		t.Fatalf("unstable tool name: %q %q", first, second)
	}
	if first == mcpserver.MCPToolName("system-user.list") {
		t.Fatal("normalized Action IDs must retain collision resistance")
	}
}

func TestMCPActionAuditMigrationDeclaresTrustedGrantColumns(t *testing.T) {
	body, err := os.ReadFile("../migrations/0047_mcp_action_audit_context.sql")
	if err != nil {
		t.Fatal(err)
	}
	sql := string(body)
	for _, required := range []string{"mcp_client_id", "mcp_grant_id", "idx_audit_logs_tenant_mcp_client_id", "idx_audit_logs_tenant_mcp_grant_id"} {
		if !strings.Contains(sql, required) {
			t.Fatalf("MCP audit migration missing %s", required)
		}
	}
}

func TestMCPConfirmationMigrationDeclaresOneTimeGovernance(t *testing.T) {
	body, err := os.ReadFile("../migrations/0048_mcp_confirmation_governance.sql")
	if err != nil {
		t.Fatal(err)
	}
	sql := string(body)
	for _, required := range []string{"mcp_action_confirmations", "recovery_hash", "arguments_hash", "consumed_at", "ck_mcp_confirmation_status", "idx_mcp_confirmation_owner", "idx_mcp_confirmation_grant"} {
		if !strings.Contains(sql, required) {
			t.Fatalf("MCP confirmation migration missing %s", required)
		}
	}
}

func TestMCPConfirmationResumeMigrationStoresEncryptedArguments(t *testing.T) {
	body, err := os.ReadFile("../migrations/0049_mcp_confirmation_resume.sql")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), "arguments_ciphertext") {
		t.Fatal("MCP confirmation resume migration must persist encrypted arguments")
	}
}

func TestMCPActionProjectionUsesRegistryMetadata(t *testing.T) {
	registry := action.NewRegistry(permission.NewChecker(), nil)
	registerMCPTestAction(t, registry, "demo.read", action.AccessPermission, "demo.read", "read", "low", nil)
	registerMCPTestAction(t, registry, "demo.write", action.AccessPermission, "demo.write", "write", "medium", nil)
	registerMCPTestAction(t, registry, "demo.sensitive", action.AccessPermission, "demo.sensitive", "destructive", "high", nil)

	server := mcpserver.NewActionServer(registry, mcpserver.NewService(nil, nil, "test-secret", "ainp-test", "https://api.ainp.example"))
	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "1"}, nil)
	ctx := auth.WithContext(context.Background(), &auth.Context{TenantID: "tenant-a", PrincipalID: "user-a", Realm: auth.RealmTenant, Authenticated: true, Permissions: []string{"demo.read", "demo.write", "demo.sensitive"}, Client: "mcp"})
	scopes := map[string]bool{mcpserver.ScopeRead: true, mcpserver.ScopeWrite: true, mcpserver.ScopeSensitive: true}
	ctx = mcpserver.ContextWithPrincipalForTest(ctx, scopes)
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	serverSession, err := server.Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer serverSession.Close()
	session, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	result, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.NextCursor != "" {
		t.Fatalf("a single-page directory must not return a cursor: %q", result.NextCursor)
	}
	if len(result.Tools) != 4 || result.Tools[0].Name != "ainp_confirmation_execute" {
		t.Fatalf("expected confirmation executor plus read, write and sensitive tools, got %#v", result.Tools)
	}
	for _, tool := range result.Tools {
		if tool.InputSchema == nil || tool.Description == "" || tool.Annotations == nil {
			t.Fatalf("incomplete projected metadata: %#v", tool)
		}
	}
	delete(scopes, mcpserver.ScopeWrite)
	result, err = session.ListTools(ctx, nil)
	if err != nil || len(result.Tools) != 3 || result.Tools[0].Name != "ainp_confirmation_execute" || result.Tools[1].Name != mcpserver.MCPToolName("demo.read") || result.Tools[2].Name != mcpserver.MCPToolName("demo.sensitive") {
		t.Fatalf("scope removal must hide write tools: result=%#v err=%v", result, err)
	}
}

func TestMCPActionProjectionPaginatesCompleteAuthorizedDirectory(t *testing.T) {
	registry := action.NewRegistry(permission.NewChecker(), nil)
	for index := 0; index < 140; index++ {
		registerMCPTestAction(t, registry, fmt.Sprintf("demo.page.%03d", index), action.AccessPermission, "demo.read", "read", "low", nil)
	}

	server := mcpserver.NewActionServer(registry, mcpserver.NewService(nil, nil, "test-secret", "ainp-test", "https://api.ainp.example"))
	authContext := &auth.Context{TenantID: "tenant-a", PrincipalID: "user-a", Realm: auth.RealmTenant, Authenticated: true, Permissions: []string{"demo.read"}, Client: "mcp", MCPClientID: "client-a", MCPGrantID: "grant-a", AuthVersion: 1}
	scopes := map[string]bool{mcpserver.ScopeRead: true, mcpserver.ScopeSensitive: true}
	ctx := mcpserver.ContextWithPrincipalForTest(auth.WithContext(context.Background(), authContext), scopes)
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	serverSession, err := server.Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer serverSession.Close()
	session, err := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "1"}, nil).Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()

	first, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Tools) != 100 || first.NextCursor == "" || first.Tools[0].Name != "ainp_confirmation_execute" {
		t.Fatalf("unexpected first page: tools=%d nextCursor=%q first=%q", len(first.Tools), first.NextCursor, first.Tools[0].Name)
	}
	second, err := session.ListTools(ctx, &mcp.ListToolsParams{Cursor: first.NextCursor})
	if err != nil {
		t.Fatal(err)
	}
	if len(second.Tools) != 41 || second.NextCursor != "" {
		t.Fatalf("unexpected final page: tools=%d nextCursor=%q", len(second.Tools), second.NextCursor)
	}

	want := make([]string, 0, 141)
	want = append(want, "ainp_confirmation_execute")
	for index := 0; index < 140; index++ {
		want = append(want, mcpserver.MCPToolName(fmt.Sprintf("demo.page.%03d", index)))
	}
	got := make([]string, 0, 141)
	for _, page := range [][]*mcp.Tool{first.Tools, second.Tools} {
		for _, tool := range page {
			got = append(got, tool.Name)
		}
	}
	if len(got) != len(want) {
		t.Fatalf("paginated directory length=%d want=%d", len(got), len(want))
	}
	for index := range want {
		if got[index] != want[index] {
			t.Fatalf("paginated directory[%d]=%q want=%q", index, got[index], want[index])
		}
	}

	var iterated []string
	for tool, iterateErr := range session.Tools(ctx, nil) {
		if iterateErr != nil {
			t.Fatal(iterateErr)
		}
		iterated = append(iterated, tool.Name)
	}
	if len(iterated) != len(want) {
		t.Fatalf("SDK automatic pagination discovered %d tools, want %d", len(iterated), len(want))
	}
	for index := range want {
		if iterated[index] != want[index] {
			t.Fatalf("automatic pagination[%d]=%q want=%q", index, iterated[index], want[index])
		}
	}

	assertInvalidMCPToolCursor(t, session, ctx, "not-a-valid-cursor")

	freshCursor := func() string {
		page, listErr := session.ListTools(ctx, nil)
		if listErr != nil || page.NextCursor == "" {
			t.Fatalf("load fresh cursor: result=%#v err=%v", page, listErr)
		}
		return page.NextCursor
	}

	cursor := freshCursor()
	authContext.Permissions = nil
	assertInvalidMCPToolCursor(t, session, ctx, cursor)
	authContext.Permissions = []string{"demo.read"}

	cursor = freshCursor()
	delete(scopes, mcpserver.ScopeSensitive)
	assertInvalidMCPToolCursor(t, session, ctx, cursor)
	scopes[mcpserver.ScopeSensitive] = true

	cursor = freshCursor()
	authContext.MCPGrantID = "grant-b"
	assertInvalidMCPToolCursor(t, session, ctx, cursor)
	authContext.MCPGrantID = "grant-a"

	cursor = freshCursor()
	disabled, exists := registry.Get("demo.page.139")
	if !exists {
		t.Fatal("test action is missing")
	}
	disabled.Meta.AI.ExposeTool = false
	assertInvalidMCPToolCursor(t, session, ctx, cursor)
}

func assertInvalidMCPToolCursor(t *testing.T, session *mcp.ClientSession, ctx context.Context, cursor string) {
	t.Helper()
	_, err := session.ListTools(ctx, &mcp.ListToolsParams{Cursor: cursor})
	var protocolError *jsonrpc.Error
	if !errors.As(err, &protocolError) || protocolError.Code != jsonrpc.CodeInvalidParams || protocolError.Message != "invalid cursor" {
		t.Fatalf("cursor must fail with safe Invalid params: %T %v", err, err)
	}
}

func TestMCPActionCallRechecksPermissionAndPropagatesIdempotency(t *testing.T) {
	recorder := &mcpAuditRecorder{}
	registry := action.NewRegistry(permission.NewChecker(), recorder)
	var calls int
	registerMCPTestAction(t, registry, "demo.write", action.AccessPermission, "demo.write", "write", "medium", func(ctx context.Context, _ json.RawMessage) (any, error) {
		calls++
		if auth.MustFromContext(ctx).IdempotencyKey != "retry-1" {
			t.Fatalf("idempotency key was not propagated")
		}
		return map[string]any{"ok": true}, nil
	})
	server := mcpserver.NewActionServer(registry, nil)
	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "1"}, nil)
	authContext := &auth.Context{TenantID: "tenant-a", PrincipalID: "user-a", Realm: auth.RealmTenant, Authenticated: true, Permissions: []string{"demo.write"}, Client: "mcp", MCPClientID: "client-a", MCPGrantID: "grant-a"}
	ctx := auth.WithContext(context.Background(), authContext)
	ctx = mcpserver.ContextWithPrincipalForTest(ctx, map[string]bool{mcpserver.ScopeWrite: true})
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	serverSession, err := server.Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer serverSession.Close()
	session, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: mcpserver.MCPToolName("demo.write"), Arguments: map[string]any{}, Meta: mcp.Meta{"idempotencyKey": "retry-1"}})
	if err != nil || result.IsError || calls != 1 {
		t.Fatalf("call failed: result=%#v err=%v calls=%d", result, err, calls)
	}
	if len(recorder.rows) != 1 || recorder.rows[0].ActorType != "mcp" || recorder.rows[0].MCPClientID != "client-a" || recorder.rows[0].MCPGrantID != "grant-a" {
		t.Fatalf("missing MCP audit context: %#v", recorder.rows)
	}
	authContext.Permissions = nil
	result, err = session.CallTool(ctx, &mcp.CallToolParams{Name: mcpserver.MCPToolName("demo.write"), Arguments: map[string]any{}})
	if err != nil || !result.IsError || calls != 1 {
		t.Fatalf("permission revocation must fail closed: result=%#v err=%v calls=%d", result, err, calls)
	}
	if len(recorder.rows) != 2 || recorder.rows[1].Result != "failure" || recorder.rows[1].ErrorCode == 0 {
		t.Fatalf("preflight rejection must be audited: %#v", recorder.rows)
	}
}

type mcpAuditRecorder struct{ rows []audit.Record }

func (r *mcpAuditRecorder) Write(_ context.Context, row audit.Record) error {
	r.rows = append(r.rows, row)
	return nil
}

func registerMCPTestAction(t *testing.T, registry *action.Registry, id string, access action.AccessMode, permissionCode, sideEffect, risk string, handler action.Handler) {
	t.Helper()
	if handler == nil {
		handler = func(context.Context, json.RawMessage) (any, error) { return map[string]any{"id": id}, nil }
	}
	err := registry.Register(action.Action{ID: id, Handler: handler, Meta: action.Metadata{
		Capability: "demo", Surface: auth.RealmTenant, Access: access, Permission: permissionCode, Name: id,
		AI: &action.AIMeta{ExposeTool: true, ToolTitle: "执行测试动作", Risk: risk, SideEffect: sideEffect, RequiresConfirmation: sideEffect != "read", Summary: "summary", HumanDescription: "description", ResultSummary: "done", InputSchema: json.RawMessage(`{"type":"object","additionalProperties":false,"x-ainp-input-contract":"explicit-v1"}`), OutputSchema: json.RawMessage(`{"type":"object"}`)},
	}})
	if err != nil {
		t.Fatal(err)
	}
}
