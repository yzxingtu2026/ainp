package tests

import (
	"os"
	"strings"
	"testing"
)

func TestGlobalAiAssistantUsesOneControllerAndConversationSurface(t *testing.T) {
	controller := readFrontendContract(t, "views/ai/workspace/use-workspace.ts")
	workspace := readFrontendContract(t, "views/ai/workspace/index.vue")
	panel := readFrontendContract(t, "layouts/modules/global-ai-assistant/index.vue")
	surface := readFrontendContract(t, "components/ai/ai-conversation-surface.vue")

	for _, expected := range []string{"sharedController ||= createAiConversationController()", "startAIAgentRun(", "subscribeAgentRun(", "capturePageContext()", "function dispose()", "function reset()"} {
		if !strings.Contains(controller, expected) {
			t.Fatalf("shared AI controller missing %q", expected)
		}
	}
	if !strings.Contains(workspace, `<AiConversationSurface density="workspace"`) || !strings.Contains(panel, `<AiConversationSurface density="sidebar"`) {
		t.Fatal("Workspace and global sidebar must use the shared conversation surface")
	}
	for _, forbidden := range []string{"startAIAgentRun(", "subscribeAgentRun(", "createAIMessage(", "capturePageContext()"} {
		if strings.Contains(panel, forbidden) || strings.Contains(surface, forbidden) {
			t.Fatalf("sidebar or surface duplicates controller behavior %q", forbidden)
		}
	}
	for _, forbidden := range []string{"page_context", "resource_id", "tool_id", "capability", "querySelector", "document.body"} {
		if strings.Contains(strings.ToLower(panel), forbidden) || strings.Contains(strings.ToLower(surface), forbidden) {
			t.Fatalf("global assistant exposes or scrapes forbidden context %q", forbidden)
		}
	}
}

func TestGlobalAiAssistantShellLifecycleAndLaunchers(t *testing.T) {
	header := readFrontendContract(t, "layouts/modules/global-header/index.vue")
	launcher := readFrontendContract(t, "layouts/modules/ai-workspace-launcher/index.vue")
	base := readFrontendContract(t, "layouts/base-layout/index.vue")
	panel := readFrontendContract(t, "layouts/modules/global-ai-assistant/index.vue")
	store := readFrontendContract(t, "store/modules/ai-assistant/index.ts")
	authStore := readFrontendContract(t, "store/modules/auth/index.ts")

	if !strings.Contains(header, `<AiWorkspaceLauncher location="header"`) || strings.Contains(header, `themeStore.layout.mode === 'horizontal'`) {
		t.Fatal("AI header launcher must be available in every layout mode")
	}
	if !strings.Contains(launcher, "aiAssistantStore.toggle()") || !strings.Contains(launcher, "router.push(launcherRoute.value.path)") {
		t.Fatal("Header must toggle the sidebar while Sider opens Workspace")
	}
	if !strings.Contains(base, "<GlobalAiAssistant />") {
		t.Fatal("global AI assistant must be mounted at the application shell")
	}
	for _, expected := range []string{`<Teleport to="body">`, "z-index: 10000", "isolation: isolate", "background: rgb(var(--container-bg-color))"} {
		if !strings.Contains(panel, expected) {
			t.Fatalf("global AI assistant must stay above shell stacking contexts with an opaque surface: missing %q", expected)
		}
	}
	for _, expected := range []string{"controller.reset()", "returnFocus", "view.value = 'conversation'"} {
		if !strings.Contains(store, expected) {
			t.Fatalf("assistant shell store missing lifecycle behavior %q", expected)
		}
	}
	if !strings.Contains(authStore, "aiAssistantStore.resetStore()") {
		t.Fatal("logout and authentication reset must clear the global assistant")
	}
}

func TestGlobalAiAssistantRestoresOnlyCanonicalConversationReference(t *testing.T) {
	controller := readFrontendContract(t, "views/ai/workspace/use-workspace.ts")
	for _, expected := range []string{"sessionStorage.setItem(activeConversationStorageKey, id)", "fetchAIMessages(", "fetchAIAgentRuns(", "selectConversation(previous)"} {
		if !strings.Contains(controller, expected) {
			t.Fatalf("refresh recovery missing %q", expected)
		}
	}
	for _, forbidden := range []string{"sessionStorage.setItem('message", "sessionStorage.setItem('draft", "localStorage.setItem"} {
		if strings.Contains(controller, forbidden) {
			t.Fatalf("assistant persisted sensitive client state %q", forbidden)
		}
	}
}

func TestGlobalAiAssistantRuntimeActionsAreAuthenticatedSelfService(t *testing.T) {
	register, err := os.ReadFile("../modules/ai/register.go")
	if err != nil {
		t.Fatal(err)
	}
	runtimeRegister, err := os.ReadFile("../modules/ai/agent_runtime_register.go")
	if err != nil {
		t.Fatal(err)
	}
	registerSource := string(register)
	runtimeSource := string(runtimeRegister)
	for _, actionID := range []string{
		"ActionAgentSlotGet",
		"ActionConversationList", "ActionConversationGet", "ActionConversationCreate", "ActionConversationCreateForSlot",
		"ActionConversationUpdate", "ActionConversationArchive", "ActionConversationRestore",
		"ActionMessageList", "ActionMessageGet",
	} {
		if !strings.Contains(registerSource, actionID) {
			t.Fatalf("authenticated assistant action set missing %s", actionID)
		}
	}
	if !strings.Contains(registerSource, "Capability: \"ai\", Access: action.AccessAuthenticated, Name: \"提交用户消息\"") {
		t.Fatal("message create must be authenticated self-service")
	}
	if strings.Contains(registerSource, `Component: "ai/workspace", Permission: ActionConversationList`) {
		t.Fatal("global AI workspace route must not require a role permission")
	}
	for _, expected := range []string{
		"ID: ActionAgentRunStart", "Access: action.AccessAuthenticated",
		"func runActionMeta", "Capability: \"ai\", Access: action.AccessAuthenticated",
	} {
		if !strings.Contains(runtimeSource, expected) {
			t.Fatalf("agent runtime self-service contract missing %q", expected)
		}
	}
}

func readFrontendContract(t *testing.T, path string) string {
	t.Helper()
	body, err := os.ReadFile("../../frontend/src/" + path)
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}
