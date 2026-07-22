package ai

import (
	"context"
	"encoding/json"

	"github.com/yzxingtu2026/ainp/platform/framework/action"
	"github.com/yzxingtu2026/ainp/platform/framework/app"
)

const (
	ActionMemoryList    = "ai.memory_list"
	ActionMemoryGet     = "ai.memory_get"
	ActionMemoryProfile = "ai.memory_profile"
	ActionMemoryConfirm = "ai.memory_confirm"
	ActionMemoryReject  = "ai.memory_reject"
	ActionMemoryCorrect = "ai.memory_correct"
	ActionMemoryDelete  = "ai.memory_delete"
	ActionMemoryClear   = "ai.memory_clear"
)

func RegisterMemoryManagement(a *app.App, s *MemoryManagementService) error {
	auditOn, auditOff := true, false
	required := &action.IdempotencyMeta{Mode: "required"}
	actions := []action.Action{
		{ID: ActionMemoryList, Handler: action.JSON(s.List), Meta: memoryMeta("记忆列表", "/api/v1/ai/memories/list", &auditOff, nil)},
		{ID: ActionMemoryGet, Handler: action.JSON(s.Get), Meta: memoryMeta("记忆详情", "/api/v1/ai/memories/get", &auditOff, nil)},
		{ID: ActionMemoryProfile, Handler: func(ctx context.Context, _ json.RawMessage) (any, error) { return s.Profile(ctx, nil) }, Meta: memoryMeta("当前记忆画像", "/api/v1/ai/memories/profile", &auditOff, nil)},
		{ID: ActionMemoryConfirm, Handler: action.JSON(func(ctx context.Context, r MemoryIDRequest) (any, error) { return s.Decision(ctx, r, "activate") }), Meta: memoryMeta("确认记忆", "/api/v1/ai/memories/confirm", &auditOn, required)},
		{ID: ActionMemoryReject, Handler: action.JSON(func(ctx context.Context, r MemoryIDRequest) (any, error) { return s.Decision(ctx, r, "reject") }), Meta: memoryMeta("拒绝记忆", "/api/v1/ai/memories/reject", &auditOn, required)},
		{ID: ActionMemoryDelete, Handler: action.JSON(func(ctx context.Context, r MemoryIDRequest) (any, error) { return s.Decision(ctx, r, "revoke") }), Meta: memoryMeta("删除记忆", "/api/v1/ai/memories/delete", &auditOn, required)},
		{ID: ActionMemoryCorrect, Handler: action.JSON(s.Correct), Meta: memoryMeta("纠正记忆", "/api/v1/ai/memories/correct", &auditOn, required)},
		{ID: ActionMemoryClear, Handler: action.JSON(s.Clear), Meta: memoryMeta("清空记忆", "/api/v1/ai/memories/clear", &auditOn, required)},
	}
	for _, item := range actions {
		if err := a.RegisterAction(item); err != nil {
			return err
		}
	}
	return nil
}
func memoryMeta(name, path string, audit *bool, idem *action.IdempotencyMeta) action.Metadata {
	return action.Metadata{Capability: "ai", Access: action.AccessAuthenticated, Name: name, Description: name, Audit: audit, AI: action.ExcludeFromAI(action.AIExcludePendingSafetyReview), Idempotency: idem, HTTP: &action.HTTPMeta{Method: "POST", Path: path}}
}
