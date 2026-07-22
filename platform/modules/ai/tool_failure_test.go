package ai

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/yzxingtu2026/ainp/platform/framework/action"
	"github.com/yzxingtu2026/ainp/platform/framework/aitool"
	"github.com/yzxingtu2026/ainp/platform/framework/errs"
	"github.com/yzxingtu2026/ainp/platform/framework/pagination"
)

type toolListArguments struct {
	pagination.Request
	Keyword string `json:"keyword,omitempty"`
	Status  string `json:"status,omitempty" validate:"omitempty,oneof=enabled disabled"`
	RoleID  string `json:"role_id,omitempty"`
}

func TestValidateToolArgumentsAcceptsOmittedOptionalListFilters(t *testing.T) {
	schema := action.SchemaFor[toolListArguments]()
	arguments := json.RawMessage(`{"status":"enabled","keyword":""}`)
	if err := validateToolArguments(schema, arguments); err != nil {
		t.Fatalf("screenshot arguments should satisfy list schema: %v; schema=%s", err, schema)
	}
}

func TestValidateToolArgumentsReturnsCategorizedValidationError(t *testing.T) {
	err := validateToolArguments(json.RawMessage(`{"type":"object","properties":{},"required":["id"],"additionalProperties":false}`), json.RawMessage(`{}`))
	if category := toolExecutionErrorCategory(err); category != "invalid_arguments" {
		t.Fatalf("unexpected validation category: %q", category)
	}
}

func TestToolExecutionErrorCategory(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want string
	}{
		{name: "authorization", err: errs.New(errs.CodeForbidden, "forbidden"), want: "authorization_failed"},
		{name: "changed", err: errs.New(errs.CodeRevisionConflict, "changed"), want: "authorization_changed"},
		{name: "unavailable", err: errs.New(errs.CodeNotFound, "missing"), want: "tool_unavailable"},
		{name: "wrapped validation", err: fmt.Errorf("wrapped: %w", errs.Validation("invalid")), want: "invalid_arguments"},
		{name: "unknown", err: fmt.Errorf("boom"), want: "execution_failed"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := toolExecutionErrorCategory(tt.err); got != tt.want {
				t.Fatalf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestInvalidArgumentsIsTheOnlyRecoverableToolError(t *testing.T) {
	for _, tt := range []struct {
		err         error
		recoverable bool
	}{
		{err: errs.Validation("invalid"), recoverable: true},
		{err: errs.New(errs.CodeForbidden, "forbidden"), recoverable: false},
		{err: errs.New(errs.CodeConflict, "changed"), recoverable: false},
		{err: fmt.Errorf("failed"), recoverable: false},
	} {
		result, got := toolExecutionFailure("call-1", tt.err)
		if got != tt.recoverable {
			t.Fatalf("error %v recoverable=%v, want %v", tt.err, got, tt.recoverable)
		}
		if result.CallID != "call-1" || result.Status != "failed" || result.ErrorCategory == "" {
			t.Fatalf("invalid failed result: %#v", result)
		}
		if got && result.Summary != "tool arguments are invalid; correct them to match the input schema and retry" {
			t.Fatalf("recoverable failure must guide model retry: %#v", result)
		}
	}
}

func TestToolNotLoadedFailureGuidesProgressiveLoad(t *testing.T) {
	result, recoverable := toolExecutionFailure("call-load", &toolRuntimeError{category: "tool_not_loaded", summary: "tool is not loaded; call meta__tool_load first", recoverable: true})
	if !recoverable || result.Status != "failed" || result.ErrorCategory != "tool_not_loaded" {
		t.Fatalf("unexpected progressive load failure: %#v recoverable=%v", result, recoverable)
	}
	if result.Summary != "tool is not loaded; call meta__tool_load first" {
		t.Fatalf("missing load guidance: %#v", result)
	}
}

func TestUnknownToolFailureLetsLaterDiscoveryCallsRun(t *testing.T) {
	result, recoverable := toolExecutionFailure("call-unknown", &toolRuntimeError{category: "tool_unavailable", summary: "search and load before retrying", recoverable: true})
	if !recoverable || result.ErrorCategory != "tool_unavailable" || result.Summary != "search and load before retrying" {
		t.Fatalf("unexpected unavailable tool result: %#v recoverable=%v", result, recoverable)
	}
}

func TestResolveRegistryToolIDAcceptsModelNameAndStableID(t *testing.T) {
	definitions := []aitool.Definition{{ID: "action:system.user_update"}}
	for _, name := range []string{"action__system_user_update", "action:system.user_update"} {
		if got := resolveRegistryToolID(definitions, name); got != definitions[0].ID {
			t.Fatalf("resolve %q got %q", name, got)
		}
	}
	if got := resolveRegistryToolID(definitions, "action__missing"); got != "" {
		t.Fatalf("unknown tool resolved as %q", got)
	}
}

func TestRegistryIDToolNameIsRecoverableButNotExecutableAlias(t *testing.T) {
	result, recoverable := toolExecutionFailure("call-id", &toolRuntimeError{category: "tool_name_invalid", summary: "call the projected model tool name", recoverable: true})
	if !recoverable || result.ErrorCategory != "tool_name_invalid" {
		t.Fatalf("unexpected registry ID failure: %#v recoverable=%v", result, recoverable)
	}
}

func TestSyntheticDeferredToolFailurePairsPendingCall(t *testing.T) {
	result := syntheticToolFailure("call-deferred", "deferred_for_confirmation", "retry after confirmation")
	if result.CallID != "call-deferred" || result.Status != "failed" || result.ErrorCategory != "deferred_for_confirmation" || result.Summary != "retry after confirmation" {
		t.Fatalf("unexpected deferred result: %#v", result)
	}
}

func TestAgentToolLoadResultOmitsDefinitionsForRepeatedLoads(t *testing.T) {
	definition := aitool.Definition{ID: "action:system.user_list"}
	first := agentToolLoadResult(ToolLoadResponse{ScopeID: "scope-1", Tools: []aitool.LoadedTool{{Definition: definition}}, NewlyLoadedToolIDs: []string{definition.ID}, AlreadyLoadedToolIDs: []string{}})
	if len(first.Tools) != 1 || first.Tools[0].ID != definition.ID {
		t.Fatalf("first load must return the new definition: %#v", first)
	}
	repeated := agentToolLoadResult(ToolLoadResponse{ScopeID: "scope-1", Tools: []aitool.LoadedTool{{Definition: definition}}, NewlyLoadedToolIDs: []string{}, AlreadyLoadedToolIDs: []string{definition.ID}})
	if len(repeated.Tools) != 0 || len(repeated.AlreadyLoadedToolIDs) != 1 || repeated.AlreadyLoadedToolIDs[0] != definition.ID {
		t.Fatalf("repeated load must return compact state: %#v", repeated)
	}
}

func TestStepLimitReservesConfirmationContinuationOnlyWhenNeeded(t *testing.T) {
	for _, tt := range []struct {
		name                string
		configured, current int
		resumed             bool
		want                int
	}{
		{name: "ordinary run", configured: 24, current: 24, resumed: false, want: 24},
		{name: "confirmation has room", configured: 24, current: 10, resumed: true, want: 24},
		{name: "confirmation needs room", configured: 24, current: 22, resumed: true, want: 26},
		{name: "confirmation exhausted", configured: 24, current: 24, resumed: true, want: 28},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := stepLimitAfterConfirmation(tt.configured, tt.current, tt.resumed); got != tt.want {
				t.Fatalf("got %d, want %d", got, tt.want)
			}
		})
	}
}

func TestReactTurnModeAllowsOneCompletionTurnForPendingToolResults(t *testing.T) {
	for _, tt := range []struct {
		name                        string
		current, limit              int
		toolResultsPending          bool
		wantAllowed, wantCompletion bool
	}{
		{name: "within budget", current: 10, limit: 12, wantAllowed: true},
		{name: "exhausted without result", current: 12, limit: 12},
		{name: "tool result at boundary", current: 12, limit: 12, toolResultsPending: true, wantAllowed: true, wantCompletion: true},
		{name: "tool result beyond boundary", current: 13, limit: 12, toolResultsPending: true, wantAllowed: true, wantCompletion: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			allowed, completion := reactTurnMode(tt.current, tt.limit, tt.toolResultsPending)
			if allowed != tt.wantAllowed || completion != tt.wantCompletion {
				t.Fatalf("got (%t, %t), want (%t, %t)", allowed, completion, tt.wantAllowed, tt.wantCompletion)
			}
		})
	}
}
