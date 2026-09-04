package tests

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/yzxingtu2026/ainp/platform/framework/action"
	"github.com/yzxingtu2026/ainp/platform/framework/aitool"
	"github.com/yzxingtu2026/ainp/platform/framework/app"
	"github.com/yzxingtu2026/ainp/platform/framework/auth"
	"github.com/yzxingtu2026/ainp/platform/framework/database"
	"github.com/yzxingtu2026/ainp/platform/framework/errs"
	"github.com/yzxingtu2026/ainp/platform/framework/httpserver"
	"github.com/yzxingtu2026/ainp/platform/host"
	"github.com/yzxingtu2026/ainp/platform/modules/ai"
	"github.com/yzxingtu2026/ainp/platform/modules/system"
)

const (
	testCommunityKind     auth.PrincipalKind = "hoa.resident"
	testCommunitySurface                     = "hoa.community"
	testCommunityAudience                    = "hoa:community"
	testCommunityPrefix                      = "/api/v1/community/"
)

func TestAllowsActorRequiresAccessSurfaceEvenWithoutPermission(t *testing.T) {
	meta := action.Metadata{
		Surface:        auth.RealmTenant,
		Access:         action.AccessAuthenticated,
		PrincipalKinds: []auth.PrincipalKind{auth.PrincipalKindAccount},
		ClientSurfaces: []string{auth.TenantConsoleSurface},
	}
	account := &auth.Context{Authenticated: true, Realm: auth.RealmTenant, Principal: auth.PrincipalRef{Kind: auth.PrincipalKindAccount, ID: "a1"}, ClientSurface: auth.TenantConsoleSurface}
	community := &auth.Context{Authenticated: true, Realm: auth.RealmTenant, Principal: auth.PrincipalRef{Kind: testCommunityKind, ID: "r1"}, ClientSurface: testCommunitySurface}
	if !action.AllowsActor(account, meta, "ai.conversation_create_for_slot") {
		t.Fatal("account tenant-console must remain allowed")
	}
	if action.AllowsActor(community, meta, "ai.conversation_create_for_slot") {
		t.Fatal("authenticated community actor must not see account-only action")
	}
}

func TestProjectRuntimeSurfacesRewritesPathsAndKeepsCanonicalIdentity(t *testing.T) {
	registry, surfaces := runtimeSurfaceFixture(t)
	if err := action.ProjectRuntimeSurfaces(registry, surfaces); err != nil {
		t.Fatal(err)
	}
	create, ok := registry.Get(ai.ActionConversationCreateForSlot)
	if !ok || !hasPrincipalKind(create.Meta.PrincipalKinds, testCommunityKind) || !hasClientSurface(create.Meta.ClientSurfaces, testCommunitySurface) {
		t.Fatalf("projected action missing community access: %#v", create.Meta)
	}
	if create.Meta.HTTP == nil || create.Meta.HTTP.Path != "/api/v1/ai/conversations/create-for-slot" {
		t.Fatalf("canonical path changed: %#v", create.Meta.HTTP)
	}
	want := "/api/v1/community/ai/conversations/create-for-slot"
	found := false
	for _, route := range registry.HTTPRoutes() {
		if route.ActionID == ai.ActionConversationCreateForSlot && route.HTTP != nil && route.HTTP.Path == want {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("projected HTTP path missing: %s routes=%#v", want, registry.HTTPRoutes())
	}
	agent, _ := registry.Get(ai.ActionAgentList)
	if hasPrincipalKind(agent.Meta.PrincipalKinds, testCommunityKind) || hasClientSurface(agent.Meta.ClientSurfaces, testCommunitySurface) {
		t.Fatal("management action must not be projected to community surface")
	}
}

func TestProjectRuntimeSurfacesRejectsPermissionActions(t *testing.T) {
	registry := action.NewRegistry(nil, nil)
	surfaces := auth.NewClientSurfaceRegistry()
	if err := surfaces.Register(auth.BuiltinTenantConsoleSurface()); err != nil {
		t.Fatal(err)
	}
	handler := func(context.Context, json.RawMessage) (any, error) { return nil, nil }
	for _, id := range action.RuntimeSurfaceActionIDs() {
		access := action.AccessAuthenticated
		if id == ai.ActionConversationCreateForSlot {
			access = action.AccessPermission
		}
		if err := registry.Register(action.Action{ID: id, Handler: handler, Meta: action.Metadata{Capability: "demo", Access: access, HTTP: &action.HTTPMeta{Method: "POST", Path: "/api/v1/ai/" + id}}}); err != nil {
			t.Fatal(err)
		}
	}
	if err := action.ProjectRuntimeSurfaces(registry, surfaces); err == nil {
		t.Fatal("permission-mode runtime action must fail closed")
	}
}

func TestRuntimeSurfaceHTTPRejectsCrossAudienceAndKeepsCanonicalPath(t *testing.T) {
	registry, surfaces := runtimeSurfaceFixture(t)
	if err := action.ProjectRuntimeSurfaces(registry, surfaces); err != nil {
		t.Fatal(err)
	}

	tokens, err := auth.NewTokenService("runtime-surface-secret", "ainp-test", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	accountToken, _, err := tokens.Issue("tenant-1", "account-1", "session-account", 1)
	if err != nil {
		t.Fatal(err)
	}
	communityToken, _, err := tokens.IssueFor("tenant-1", auth.PrincipalRef{Kind: testCommunityKind, ID: "resident-1"}, testCommunitySurface, testCommunityAudience, "session-community", 1)
	if err != nil {
		t.Fatal(err)
	}
	provider := &surfaceAuthProvider{
		account: &auth.AuthorizationSnapshot{
			TenantID: "tenant-1", Principal: auth.PrincipalRef{Kind: auth.PrincipalKindAccount, ID: "account-1"},
			PrincipalID: "account-1", SessionID: "session-account", AuthVersion: 1,
			Audience: auth.TenantAudience, ClientSurface: auth.TenantConsoleSurface,
		},
		community: &auth.AuthorizationSnapshot{
			TenantID: "tenant-1", Principal: auth.PrincipalRef{Kind: testCommunityKind, ID: "resident-1"},
			PrincipalID: "resident-1", SessionID: "session-community", AuthVersion: 1,
			Audience: testCommunityAudience, ClientSurface: testCommunitySurface,
		},
	}
	installed := true
	router := httpserver.New(httpserver.Options{
		Mode: "test", Installed: &installed, Tokens: tokens, Authorization: provider,
		ClientSurfaces: surfaces, Actions: registry,
	})

	canonical := "/api/v1/ai/conversations/create-for-slot"
	projected := "/api/v1/community/ai/conversations/create-for-slot"
	management := "/api/v1/community/ai/agents/list"

	if code := postAction(router, canonical, accountToken); code != http.StatusOK {
		t.Fatalf("account canonical path status=%d", code)
	}
	if code := postAction(router, canonical, communityToken); code != http.StatusUnauthorized {
		t.Fatalf("community token on canonical AI path must 401, got %d", code)
	}
	if code := postAction(router, projected, accountToken); code != http.StatusUnauthorized {
		t.Fatalf("account token on community path must 401, got %d", code)
	}
	if code := postAction(router, projected, communityToken); code != http.StatusOK {
		t.Fatalf("community projected path status=%d", code)
	}
	if code := postAction(router, management, communityToken); code != http.StatusNotFound {
		t.Fatalf("unprojected management path must 404, got %d", code)
	}
}

func TestActionProviderListKeepsCatalogWhileSearchFiltersAccessSurface(t *testing.T) {
	registry := action.NewRegistry(nil, nil)
	staff := exposedAction("staff.notice_list", []auth.PrincipalKind{auth.PrincipalKindAccount}, []string{auth.TenantConsoleSurface}, []string{"staff.notice_list"})
	community := exposedAction("community.notice_list", []auth.PrincipalKind{testCommunityKind}, []string{testCommunitySurface}, nil)
	if err := registry.Register(staff); err != nil {
		t.Fatal(err)
	}
	if err := registry.Register(community); err != nil {
		t.Fatal(err)
	}
	listed, err := (&aitool.ActionProvider{Actions: registry}).List(context.Background())
	if err != nil || len(listed) != 2 {
		t.Fatalf("catalog must still list all exposed tools: %#v %v", listed, err)
	}
	account := &auth.Context{Authenticated: true, Realm: auth.RealmTenant, Principal: auth.PrincipalRef{Kind: auth.PrincipalKindAccount, ID: "a1"}, ClientSurface: auth.TenantConsoleSurface, Permissions: []string{"staff.notice_list"}}
	resident := &auth.Context{Authenticated: true, Realm: auth.RealmTenant, Principal: auth.PrincipalRef{Kind: testCommunityKind, ID: "r1"}, ClientSurface: testCommunitySurface}
	var staffDef, communityDef aitool.Definition
	for _, item := range listed {
		switch item.RuntimeRef {
		case "staff.notice_list":
			staffDef = item
		case "community.notice_list":
			communityDef = item
		}
	}
	if !staffDef.VisibleToActor(account) || communityDef.VisibleToActor(account) {
		t.Fatal("operations actor must not see community-only action")
	}
	if staffDef.VisibleToActor(resident) || !communityDef.VisibleToActor(resident) {
		t.Fatal("community actor must not see staff permission action")
	}
	if _, err := registry.Invoke(auth.WithContext(context.Background(), resident), "staff.notice_list", json.RawMessage(`{}`)); errorCode(err) != errs.CodeForbidden {
		t.Fatalf("community invoke of staff action: %v", err)
	}
}

func TestConversationOwnerUsesPrincipalIDWithoutAccountUser(t *testing.T) {
	ac := action.NormalizeActor(&auth.Context{Authenticated: true, TenantID: "t1", PrincipalID: "resident-1", Principal: auth.PrincipalRef{Kind: testCommunityKind, ID: "resident-1"}, ClientSurface: testCommunitySurface, Realm: auth.RealmTenant})
	if ac.PrincipalID != "resident-1" || ac.Principal.ID != "resident-1" {
		t.Fatalf("community conversation owner must be principal id: %#v", ac)
	}
}

func runtimeSurfaceFixture(t *testing.T) (*action.Registry, *auth.ClientSurfaceRegistry) {
	t.Helper()
	registry := action.NewRegistry(nil, nil)
	handler := func(ctx context.Context, _ json.RawMessage) (any, error) {
		return map[string]any{"ok": true}, nil
	}
	paths := map[string]string{
		ai.ActionConversationList:              "/api/v1/ai/conversations/list",
		ai.ActionConversationGet:               "/api/v1/ai/conversations/get",
		ai.ActionConversationCreateForSlot:     "/api/v1/ai/conversations/create-for-slot",
		ai.ActionConversationUpdate:            "/api/v1/ai/conversations/update",
		ai.ActionConversationArchive:           "/api/v1/ai/conversations/archive",
		ai.ActionConversationRestore:           "/api/v1/ai/conversations/restore",
		ai.ActionMessageList:                   "/api/v1/ai/messages/list",
		ai.ActionMessageGet:                    "/api/v1/ai/messages/get",
		ai.ActionMessageCreate:                 "/api/v1/ai/messages/create",
		ai.ActionAgentRunStart:                 "/api/v1/ai/agent-runs/start",
		ai.ActionAgentRunGet:                   "/api/v1/ai/agent-runs/get",
		ai.ActionAgentRunList:                  "/api/v1/ai/agent-runs/list",
		ai.ActionAgentRunCancel:                "/api/v1/ai/agent-runs/cancel",
		ai.ActionAgentRunResume:                "/api/v1/ai/agent-runs/resume",
		ai.ActionAgentRunRetry:                 "/api/v1/ai/agent-runs/retry",
		ai.ActionAgentRunEvents:                "/api/v1/ai/agent-runs/events",
		ai.ActionToolConfirmationList:          "/api/v1/ai/tool-confirmations/list",
		ai.ActionToolConfirmationApprove:       "/api/v1/ai/tool-confirmations/approve",
		ai.ActionToolConfirmationReject:        "/api/v1/ai/tool-confirmations/reject",
		ai.ActionAgentSlotGet:                  "/api/v1/ai/agent-slots/get",
		ai.ActionAgentSlotPreferenceGet:        "/api/v1/ai/agent-slot-preferences/get",
		ai.ActionAgentSlotPreferenceUpdate:     "/api/v1/ai/agent-slot-preferences/update",
		ai.ActionAgentSlotPreferenceReset:      "/api/v1/ai/agent-slot-preferences/reset",
		system.ActionAttachmentMineList:        "/api/v1/system/attachments/mine/list",
		system.ActionAttachmentUploadPrepare:   "/api/v1/system/attachments/upload/prepare",
		system.ActionAttachmentDownloadPrepare: "/api/v1/system/attachments/download/prepare",
		system.ActionAttachmentPolicyGet:       "/api/v1/system/attachments/policy",
		system.ActionAttachmentContentInspect:  "/api/v1/system/attachments/content/inspect",
	}
	for _, id := range action.RuntimeSurfaceActionIDs() {
		path := paths[id]
		if path == "" {
			t.Fatalf("missing fixture path for %s", id)
		}
		if err := registry.Register(action.Action{ID: id, Handler: handler, Meta: action.Metadata{Capability: "demo", Access: action.AccessAuthenticated, HTTP: &action.HTTPMeta{Method: "POST", Path: path}}}); err != nil {
			t.Fatal(err)
		}
	}
	if err := registry.Register(action.Action{ID: ai.ActionAgentList, Handler: handler, Meta: action.Metadata{Capability: "demo", Access: action.AccessPermission, HTTP: &action.HTTPMeta{Method: "POST", Path: "/api/v1/ai/agents/list"}}}); err != nil {
		t.Fatal(err)
	}
	surfaces := auth.NewClientSurfaceRegistry()
	if err := surfaces.Register(auth.BuiltinTenantConsoleSurface()); err != nil {
		t.Fatal(err)
	}
	if err := surfaces.Register(auth.ClientSurface{
		ID: testCommunitySurface, Audience: testCommunityAudience, PathPrefix: testCommunityPrefix,
		PrincipalKinds: []auth.PrincipalKind{testCommunityKind}, Methods: []string{"password"},
	}); err != nil {
		t.Fatal(err)
	}
	return registry, surfaces
}

func exposedAction(id string, kinds []auth.PrincipalKind, surfaces, permissions []string) action.Action {
	permission := ""
	access := action.AccessAuthenticated
	if len(permissions) > 0 {
		access = action.AccessPermission
		permission = permissions[0]
	}
	return action.Action{
		ID: id,
		Handler: func(context.Context, json.RawMessage) (any, error) {
			return map[string]any{"ok": true}, nil
		},
		Meta: action.Metadata{
			Capability: "demo", Access: access, Permission: permission,
			PrincipalKinds: kinds, ClientSurfaces: surfaces,
			AI: &action.AIMeta{
				ExposeTool: true, ToolTitle: "查询通知", Summary: "查询通知", HumanDescription: "查询通知", ResultSummary: "已返回通知",
				Risk: "low", SideEffect: "read", InputSchema: action.InputSchemaFor[struct{}](),
			},
		},
	}
}

func postAction(router http.Handler, path, token string) int {
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{}`))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)
	return resp.Code
}

func hasPrincipalKind(items []auth.PrincipalKind, want auth.PrincipalKind) bool {
	for _, item := range items {
		if item == want {
			return true
		}
	}
	return false
}

func hasClientSurface(items []string, want string) bool {
	for _, item := range items {
		if item == want {
			return true
		}
	}
	return false
}

type surfaceAuthProvider struct {
	account   *auth.AuthorizationSnapshot
	community *auth.AuthorizationSnapshot
}

func (p *surfaceAuthProvider) LoadAuthorization(_ context.Context, req auth.AuthorizationRequest) (*auth.AuthorizationSnapshot, error) {
	ref := req.PrincipalRef()
	switch {
	case ref.Kind == auth.PrincipalKindAccount && ref.ID == "account-1":
		return p.account, nil
	case ref.Kind == testCommunityKind && ref.ID == "resident-1":
		return p.community, nil
	default:
		return nil, auth.ErrAuthorizationInvalid
	}
}

func TestRuntimeSurfaceWhitelistCoversConversationControlPlane(t *testing.T) {
	ids := strings.Join(action.RuntimeSurfaceActionIDs(), ",")
	for _, want := range []string{
		ai.ActionConversationCreateForSlot, ai.ActionMessageCreate,
		ai.ActionAgentRunStart, ai.ActionAgentRunEvents, ai.ActionAgentRunCancel,
		ai.ActionToolConfirmationList, ai.ActionToolConfirmationApprove, ai.ActionToolConfirmationReject,
		system.ActionAttachmentMineList,
	} {
		if !strings.Contains(","+ids+",", ","+want+",") {
			t.Fatalf("runtime surface whitelist missing %s", want)
		}
	}
	for _, blocked := range []string{ai.ActionAgentList, ai.ActionAgentRunSubscribe, ai.ActionConversationCreate} {
		if strings.Contains(","+ids+",", ","+blocked+",") {
			t.Fatalf("management or SSE action leaked into runtime whitelist: %s", blocked)
		}
	}
}

func TestHostProjectsRuntimeSurfaceOntoRegisteredClient(t *testing.T) {
	dsn := os.Getenv("AINP_TEST_DATABASE_DSN")
	if dsn == "" {
		t.Skip("AINP_TEST_DATABASE_DSN is not set")
	}
	root, err := database.Open(dsn)
	if err != nil {
		t.Fatal(err)
	}
	previousDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(".."); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(previousDir) })
	schema := "ainp_runtime_surface_" + strings.ReplaceAll(uuid.NewString()[:8], "-", "")
	if err := root.Exec(fmt.Sprintf(`CREATE SCHEMA %q`, schema)).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = root.Exec(fmt.Sprintf(`DROP SCHEMA %q CASCADE`, schema)).Error })
	platform, err := host.OpenInstalled(context.Background(), hostTestConfig(dsn+" search_path="+schema, t.TempDir()), host.WithModules(testCommunityRuntimeModule{}))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		platform.App.Outbox = nil
		_ = platform.Close()
	})
	create, ok := platform.App.Actions.Get(ai.ActionConversationCreateForSlot)
	if !ok || !hasPrincipalKind(create.Meta.PrincipalKinds, testCommunityKind) || !hasClientSurface(create.Meta.ClientSurfaces, testCommunitySurface) {
		t.Fatalf("host did not expand runtime action access: %#v", create.Meta)
	}
	agent, _ := platform.App.Actions.Get(ai.ActionAgentList)
	if hasClientSurface(agent.Meta.ClientSurfaces, testCommunitySurface) {
		t.Fatal("host projected management action onto community surface")
	}
	found := false
	for _, route := range platform.App.Actions.HTTPRoutes() {
		if route.ActionID == ai.ActionConversationCreateForSlot && route.HTTP != nil && route.HTTP.Path == "/api/v1/community/ai/conversations/create-for-slot" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("host missing community create-for-slot route")
	}
	slot, ok := platform.App.AgentSlots.Current(ai.WorkspaceDefaultSlot)
	if !ok || slot.AllowsActor(&auth.Context{Authenticated: true, Principal: auth.PrincipalRef{Kind: testCommunityKind, ID: "r1"}, ClientSurface: testCommunitySurface}) {
		t.Fatal("workspace.default must stay closed to community actors")
	}
}

type testCommunityRuntimeModule struct{}

func (testCommunityRuntimeModule) ID() string { return "hoa.community-client" }
func (testCommunityRuntimeModule) Register(*app.App) error {
	return nil
}
func (testCommunityRuntimeModule) Prepare(application *app.App) error {
	if err := application.RegisterPrincipalProvider(testCommunityPrincipal{}); err != nil {
		return err
	}
	return application.RegisterClientSurface(auth.ClientSurface{
		ID: testCommunitySurface, Audience: testCommunityAudience, PathPrefix: testCommunityPrefix,
		PrincipalKinds: []auth.PrincipalKind{testCommunityKind}, Methods: []string{"password"},
	})
}

type testCommunityPrincipal struct{}

func (testCommunityPrincipal) Kind() auth.PrincipalKind { return testCommunityKind }
func (testCommunityPrincipal) LoadAuthorization(context.Context, auth.AuthorizationRequest) (*auth.AuthorizationSnapshot, error) {
	return nil, auth.ErrAuthorizationInvalid
}
