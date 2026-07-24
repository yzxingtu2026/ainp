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
		{ID: ActionMemoryList, Handler: action.JSON(s.List), Meta: memoryMeta("记忆列表", "/api/v1/ai/memories/list", &auditOff, nil, readAIMeta("查询当前用户的记忆，可按状态、类型和日期筛选", "查询我的记忆", "查询当前用户的记忆，可按状态、类型和日期筛选", "已返回当前用户记忆列表", action.InputSchemaFor[MemoryListRequest](), nil, "AI", "记忆", "列表"))},
		{ID: ActionMemoryGet, Handler: action.JSON(s.Get), Meta: memoryMeta("记忆详情", "/api/v1/ai/memories/get", &auditOff, nil, readAIMeta("按 ID 查询当前用户的记忆详情", "查询我的记忆详情", "按 ID 查询当前用户的记忆详情", "已返回记忆详情", action.InputSchemaFor[MemoryIDRequest](), nil, "AI", "记忆", "详情"))},
		{ID: ActionMemoryProfile, Handler: func(ctx context.Context, _ json.RawMessage) (any, error) { return s.Profile(ctx, nil) }, Meta: memoryMeta("当前记忆画像", "/api/v1/ai/memories/profile", &auditOff, nil, readAIMeta("查询当前用户的聚合记忆画像", "查询我的记忆画像", "查询当前用户的聚合记忆画像", "已返回当前记忆画像", action.InputSchemaFor[struct{}](), nil, "AI", "记忆", "画像"))},
		{ID: ActionMemoryConfirm, Handler: action.JSON(func(ctx context.Context, r MemoryIDRequest) (any, error) { return s.Decision(ctx, r, "activate") }), Meta: memoryMeta("确认记忆", "/api/v1/ai/memories/confirm", &auditOn, required, mutationAIMeta("确认一条待审核记忆", "确认记忆", "确认一条待审核记忆", "记忆已确认", "medium", "write", action.InputSchemaFor[MemoryIDRequest](), nil, "AI", "记忆", "确认"))},
		{ID: ActionMemoryReject, Handler: action.JSON(func(ctx context.Context, r MemoryIDRequest) (any, error) { return s.Decision(ctx, r, "reject") }), Meta: memoryMeta("拒绝记忆", "/api/v1/ai/memories/reject", &auditOn, required, mutationAIMeta("拒绝一条待审核记忆", "拒绝记忆", "拒绝一条待审核记忆", "记忆已拒绝", "medium", "write", action.InputSchemaFor[MemoryIDRequest](), nil, "AI", "记忆", "拒绝"))},
		{ID: ActionMemoryDelete, Handler: action.JSON(func(ctx context.Context, r MemoryIDRequest) (any, error) { return s.Decision(ctx, r, "revoke") }), Meta: memoryMeta("删除记忆", "/api/v1/ai/memories/delete", &auditOn, required, mutationAIMeta("撤销并删除一条当前用户记忆", "删除记忆", "撤销并删除一条当前用户记忆", "记忆已删除", "high", "destructive", action.InputSchemaFor[MemoryIDRequest](), nil, "AI", "记忆", "删除"))},
		{ID: ActionMemoryCorrect, Handler: action.JSON(s.Correct), Meta: memoryMeta("纠正记忆", "/api/v1/ai/memories/correct", &auditOn, required, mutationAIMeta("纠正当前用户的一条记忆内容", "纠正记忆", "纠正当前用户的一条记忆内容", "记忆已纠正", "high", "write", action.InputSchemaFor[MemoryCorrectRequest](), nil, "AI", "记忆", "纠正"))},
		{ID: ActionMemoryClear, Handler: action.JSON(s.Clear), Meta: memoryMeta("清空记忆", "/api/v1/ai/memories/clear", &auditOn, required, mutationAIMeta("按范围清空当前用户记忆", "清空记忆", "按范围清空当前用户记忆", "指定范围的记忆已清空", "critical", "destructive", action.InputSchemaFor[MemoryClearRequest](), nil, "AI", "记忆", "清空"))},
	}
	for _, item := range actions {
		if err := a.RegisterAction(item); err != nil {
			return err
		}
	}
	return nil
}
func memoryMeta(name, path string, audit *bool, idem *action.IdempotencyMeta, ai *action.AIMeta) action.Metadata {
	return action.Metadata{Capability: "ai", Access: action.AccessAuthenticated, Name: name, Description: name, Audit: audit, AI: ai, Idempotency: idem, HTTP: &action.HTTPMeta{Method: "POST", Path: path}}
}
