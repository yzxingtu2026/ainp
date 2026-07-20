package ai

import (
	"testing"

	"github.com/yzxingtu2026/ainp/platform/framework/aimodel"
)

func TestRuntimeModelLimitsIssueRequiresContextWindow(t *testing.T) {
	issue := runtimeModelLimitsIssue(&aimodel.ResolvedModel{DeploymentID: "deployment-a"})
	if issue == nil || issue.Code != "model.context_window_missing" || issue.Resource != "deployment-a" {
		t.Fatalf("unexpected issue: %#v", issue)
	}
	if issue = runtimeModelLimitsIssue(&aimodel.ResolvedModel{Limits: aimodel.Limits{ContextWindow: 32768}}); issue != nil {
		t.Fatalf("configured context window should be healthy: %#v", issue)
	}
}
