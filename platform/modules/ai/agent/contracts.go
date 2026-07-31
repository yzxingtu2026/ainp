package agent

import (
	"encoding/json"
	"regexp"
	"strings"

	"github.com/yzxingtu2026/ainp/platform/framework/action"
	"github.com/yzxingtu2026/ainp/platform/framework/errs"
)

var codePattern = regexp.MustCompile(`^[a-z][a-z0-9_-]{1,63}$`)

func statusOK(value string) bool { return value == "enabled" || value == "disabled" }

func marshal(value any) json.RawMessage {
	data, _ := json.Marshal(value)
	return data
}

func dbError(err error, resource string) error {
	if err == nil {
		return nil
	}
	message := strings.ToLower(err.Error())
	if strings.Contains(message, "duplicate") || strings.Contains(message, "unique") {
		return errs.New(errs.CodeConflict, resource+" already exists")
	}
	return errs.Wrap(errs.CodeInternal, resource+" failed", err)
}

type IDRequest = action.IDRequest

const (
	ActionCreate             = "ai.agent_create"
	ActionUpdate             = "ai.agent_update"
	ActionDuplicate          = "ai.agent_duplicate"
	ActionStatus             = "ai.agent_status_update"
	ActionVersionCreate      = "ai.agent_version_create"
	ActionVersionPublish     = "ai.agent_version_publish"
	ActionPreviewMaterialize = "ai.agent_preview_materialize"
	ActionToolBind           = "ai.agent_tool_bind"
	ActionToolUnbind         = "ai.agent_tool_unbind"
	ActionBuiltinReconcile   = "ai.agent_builtin_reconcile"
)
