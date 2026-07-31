package showcase

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/yzxingtu2026/ainp/platform/framework/action"
	"github.com/yzxingtu2026/ainp/platform/framework/app"
	"github.com/yzxingtu2026/ainp/platform/framework/attachment"
	"github.com/yzxingtu2026/ainp/platform/framework/auth"
	"github.com/yzxingtu2026/ainp/platform/framework/capability"
	"github.com/yzxingtu2026/ainp/platform/framework/errs"
	"github.com/yzxingtu2026/ainp/platform/framework/i18n"
	"github.com/yzxingtu2026/ainp/platform/framework/menu"
	"github.com/yzxingtu2026/ainp/platform/framework/statemachine"
	"github.com/yzxingtu2026/ainp/platform/modules/demo/jobs"
)

func Register(a *app.App) error {
	if err := a.RegisterCapability(capability.Capability{
		ID:             "demo",
		Name:           "演示",
		NameKey:        "capability.demo.name",
		Description:    "Phase 1 骨架演示能力",
		DescriptionKey: "capability.demo.description",
		Module:         "demo",
		Order:          100,
	}); err != nil {
		return err
	}

	if err := a.RegisterAction(action.Action{
		ID: "demo.hello",
		Handler: func(ctx context.Context, input json.RawMessage) (any, error) {
			ac := auth.MustFromContext(ctx)
			var req struct {
				Name string `json:"name"`
			}
			if len(input) > 0 {
				_ = json.Unmarshal(input, &req)
			}
			if req.Name == "" {
				req.Name = "AINP"
			}
			lang := i18n.FromCtx(ctx)
			greeting := i18n.T(lang, "demo.hello_greeting")
			return map[string]any{
				"message":   greeting + " " + req.Name,
				"tenant_id": ac.TenantID,
				"user_id":   ac.UserID,
			}, nil
		},
		Meta: action.Metadata{
			Capability:     "demo",
			Name:           "Hello",
			NameKey:        "action.demo.hello.name",
			Description:    "演示需鉴权的 Action",
			DescriptionKey: "action.demo.hello.description",
			HTTP: &action.HTTPMeta{
				Method: "POST",
				Path:   "/api/v1/demo/hello",
			},
		},
	}); err != nil {
		return err
	}

	if err := a.RegisterAction(action.Action{
		ID: "demo.echo",
		Handler: func(ctx context.Context, input json.RawMessage) (any, error) {
			if len(input) == 0 {
				return nil, errs.Validation("body required")
			}
			var payload any
			if err := json.Unmarshal(input, &payload); err != nil {
				return nil, errs.Validation("invalid json")
			}
			return map[string]any{"echo": payload}, nil
		},
		Meta: action.Metadata{
			Capability: "demo",
			Name:       "Echo",
			HTTP: &action.HTTPMeta{
				Method: "POST",
				Path:   "/api/v1/demo/echo",
			},
		},
	}); err != nil {
		return err
	}

	if err := a.RegisterAction(action.Action{
		ID: ActionComplete,
		Handler: func(_ context.Context, input json.RawMessage) (any, error) {
			var req struct {
				From TaskStatus `json:"from"`
			}
			if err := json.Unmarshal(input, &req); err != nil || req.From != TaskPending {
				return nil, errs.ValidationKey("common.validation_failed", "task must be pending")
			}
			return map[string]TaskStatus{"from": req.From, "status": TaskCompleted}, nil
		},
		Meta: action.Metadata{
			Capability: "demo", Name: "完成演示任务", NameKey: "action.demo.complete.name",
			Description: "通过明确 Action 完成演示任务", DescriptionKey: "action.demo.complete.description",
			HTTP: &action.HTTPMeta{Method: "POST", Path: "/api/v1/demo/tasks/complete"},
		},
	}); err != nil {
		return err
	}

	if a.AttachmentPolicies != nil {
		if err := a.AttachmentPolicies.Register(attachment.FuncPolicy{
			Type: "demo.task",
			Fn: func(_ context.Context, tenantID, userID, resourceID, _ string) error {
				if tenantID == "" || userID == "" || resourceID == "" {
					return fmt.Errorf("demo.task attachment access requires identity")
				}
				return nil
			},
		}); err != nil {
			return err
		}
	}

	if err := a.RegisterCodeSet(taskStatusCodeSet()); err != nil {
		return err
	}
	if err := a.RegisterStateMachine(statemachine.StateMachine{
		Key: StateMachineTask, CodeSetKey: CodeSetTaskStatus, Terminal: []string{string(TaskCompleted)},
		Transitions: []statemachine.Transition{{From: string(TaskPending), To: string(TaskCompleted), Action: ActionComplete}},
	}); err != nil {
		return err
	}

	if err := a.RegisterMenu(menu.Menu{
		ID:     "demo.root",
		Title:  "演示",
		Icon:   "mdi:flask-outline",
		Type:   menu.TypeDirectory,
		Module: "demo",
		Order:  100,
	}); err != nil {
		return err
	}

	if err := a.RegisterMenu(menu.Menu{
		ID:         "demo.hello_page",
		ParentID:   "demo.root",
		Title:      "Hello",
		Icon:       "mdi:hand-wave-outline",
		Type:       menu.TypeMenu,
		Path:       "/demo/hello",
		Component:  "demo/hello",
		Permission: "demo.hello",
		BindAction: "demo.hello",
		Module:     "demo",
		Order:      1,
	}); err != nil {
		return err
	}

	if err := a.RegisterMenu(menu.Menu{
		ID:         "demo.hello_btn",
		ParentID:   "demo.hello_page",
		Title:      "打个招呼",
		Type:       menu.TypeButton,
		Permission: "demo.hello",
		BindAction: "demo.hello",
		HideInMenu: true,
		Module:     "demo",
		Order:      1,
	}); err != nil {
		return err
	}

	if err := a.RegisterMenu(menu.Menu{
		ID:         "demo.attachments_page",
		ParentID:   "demo.root",
		Title:      "附件联调",
		TitleKey:   "menu.demo.attachments.title",
		Icon:       "mdi:paperclip",
		Type:       menu.TypeMenu,
		Path:       "/demo/attachments",
		Component:  "demo/attachments",
		Permission: "system.attachment_list_by_resource",
		Module:     "demo",
		Order:      2,
	}); err != nil {
		return err
	}

	if err := registerOutboxDemo(a); err != nil {
		return err
	}
	if err := registerJobPingAction(a); err != nil {
		return err
	}

	// Register revision doc demo actions (idempotency + optimistic locking)
	revisionDocSvc := NewRevisionDocService(a.DB)
	if err := RegisterRevisionDocActions(a, revisionDocSvc); err != nil {
		return err
	}

	return jobs.Register(a)
}
