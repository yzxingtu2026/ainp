package conversation

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/yzxingtu2026/ainp/platform/framework/errs"
)

type referenceEvidence struct {
	ResolverID      string    `json:"resolver_id"`
	DiscoveryToolID string    `json:"discovery_tool_id"`
	Version         string    `json:"version"`
	InvocationID    string    `json:"invocation_id"`
	ReadAt          time.Time `json:"read_at"`
}

func jsonBytes(value any) json.RawMessage {
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
