package ai

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/yzxingtu2026/ainp/platform/framework/action"
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
