package knowledge

import (
	"encoding/json"
	"strings"

	"github.com/yzxingtu2026/ainp/platform/framework/errs"
)

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
