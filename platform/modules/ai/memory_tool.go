package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/yzxingtu2026/ainp/platform/framework/aimemory"
	"github.com/yzxingtu2026/ainp/platform/framework/aitool"
	"github.com/yzxingtu2026/ainp/platform/framework/runtimecontext"
)

type MemoryTool struct{ retriever *MemoryRetrievalService }

func NewMemoryTool(r *MemoryRetrievalService) *MemoryTool { return &MemoryTool{retriever: r} }
func (t *MemoryTool) Project(ctx context.Context, scope aitool.RuntimeScope, d aitool.Definition) (aitool.Definition, bool, error) {
	trusted, ok := runtimecontext.FromContext(ctx)
	if !ok || scope.AgentRunID == "" || trusted.RunID != scope.AgentRunID || trusted.UserID != scope.UserID {
		return d, false, nil
	}
	return d, true, nil
}
func (t *MemoryTool) Execute(ctx context.Context, _ aitool.LoadedTool, args json.RawMessage) (*aitool.Result, error) {
	trusted, ok := runtimecontext.FromContext(ctx)
	if !ok {
		return &aitool.Result{Status: "failed", Summary: "trusted runtime context unavailable", ErrorCategory: "runtime_context_missing"}, nil
	}
	var input aitool.MemorySearchInput
	if json.Unmarshal(args, &input) != nil || strings.TrimSpace(input.Query) == "" {
		return &aitool.Result{Status: "failed", Summary: "memory search input is invalid", ErrorCategory: "invalid_request"}, nil
	}
	types := []aimemory.ItemType{}
	for _, value := range input.Types {
		typ := aimemory.ItemType(value)
		if !aimemory.ValidItemType(typ) {
			return &aitool.Result{Status: "failed", Summary: "memory type is invalid", ErrorCategory: "invalid_request"}, nil
		}
		types = append(types, typ)
	}
	from, err := parseMemoryDate(input.DateFrom, trusted.Time, false)
	if err != nil {
		return memoryInvalidDate(), nil
	}
	to, err := parseMemoryDate(input.DateTo, trusted.Time, true)
	if err != nil {
		return memoryInvalidDate(), nil
	}
	response, err := t.retriever.Retrieve(ctx, MemoryRecallRequest{Query: input.Query, AgentID: trusted.AgentID, Types: types, DateFrom: from, DateTo: to, Limit: input.Limit, MaxTokens: 4000, NormalOnly: true})
	if err != nil {
		return nil, err
	}
	refs := make([]string, len(response.Items))
	for i, item := range response.Items {
		refs[i] = fmt.Sprintf("memory:%s@%d", item.ID, item.Provenance.ItemRevision)
	}
	return &aitool.Result{Status: "completed", Summary: fmt.Sprintf("returned %d memories", len(response.Items)), Data: response, ResourceRefs: refs}, nil
}
func parseMemoryDate(value string, snapshot runtimecontext.TimeSnapshot, end bool) (*time.Time, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, nil
	}
	if relative, err := runtimecontext.ResolveRelativeDate(snapshot, value); err == nil {
		if end {
			return &relative.EndUTC, nil
		}
		return &relative.StartUTC, nil
	}
	loc, err := time.LoadLocation(snapshot.Timezone)
	if err != nil {
		return nil, err
	}
	date, err := time.ParseInLocation("2006-01-02", value, loc)
	if err != nil {
		return nil, err
	}
	if end {
		date = date.AddDate(0, 0, 1)
	}
	utc := date.UTC()
	return &utc, nil
}
func memoryInvalidDate() *aitool.Result {
	return &aitool.Result{Status: "failed", Summary: "memory date must use YYYY-MM-DD or a supported relative date", ErrorCategory: "invalid_request"}
}
