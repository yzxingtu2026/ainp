package demo

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"
	"github.com/yzxingtu2026/ainp/platform/framework/action"
	"github.com/yzxingtu2026/ainp/platform/framework/app"
	"github.com/yzxingtu2026/ainp/platform/framework/auth"
	"github.com/yzxingtu2026/ainp/platform/framework/errs"
	"github.com/yzxingtu2026/ainp/platform/framework/outbox"
	"github.com/yzxingtu2026/ainp/platform/modules/demo/jobs"
	"gorm.io/gorm"
)

const ActionJobPing = "demo.job_ping"

func registerJobPingAction(a *app.App) error {
	if a.Outbox == nil {
		return nil
	}
	writer := a.Outbox.Writer
	return a.RegisterAction(action.Action{
		ID: ActionJobPing,
		Handler: func(ctx context.Context, input json.RawMessage) (any, error) {
			ac := auth.MustFromContext(ctx)
			if !ac.Authenticated || ac.TenantID == "" {
				return nil, errs.NewKey(errs.CodeUnauthorized, "common.unauthorized", "unauthorized")
			}
			var req struct {
				Message string `json:"message"`
				Steps   int    `json:"steps"`
			}
			_ = json.Unmarshal(input, &req)
			if req.Steps <= 0 {
				req.Steps = 3
			}
			if req.Message == "" {
				req.Message = "pong"
			}
			var eventID string
			err := outbox.WithinTx(ctx, a.DB, writer, func(tx *gorm.DB, w outbox.Writer) error {
				env, err := w.Append(ctx, tx, outbox.AppendInput{
					EventType:     jobs.EventJobPingV1,
					AggregateType: "demo.job",
					AggregateID:   uuid.NewString(),
					Payload: jobs.JobPingPayload{
						Message: req.Message,
						Steps:   req.Steps,
					},
				})
				if err != nil {
					return err
				}
				eventID = env.ID
				return nil
			})
			if err != nil {
				return nil, err
			}
			return map[string]any{"event_id": eventID, "job_type": jobs.TypePing}, nil
		},
		Meta: action.Metadata{
			Capability:     "demo",
			Name:           "触发演示 Job",
			NameKey:        "action.demo.job_ping.name",
			Description:    "经 Outbox 可靠入队 demo.ping",
			DescriptionKey: "action.demo.job_ping.description",
			HTTP: &action.HTTPMeta{
				Method: "POST",
				Path:   "/api/v1/demo/jobs/ping",
			},
		},
	})
}
