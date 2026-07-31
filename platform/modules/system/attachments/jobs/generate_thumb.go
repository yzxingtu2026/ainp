package jobs

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"strings"
	"time"

	"github.com/yzxingtu2026/ainp/platform/framework/app"
	"github.com/yzxingtu2026/ainp/platform/framework/attachment"
	"github.com/yzxingtu2026/ainp/platform/framework/auth"
	"github.com/yzxingtu2026/ainp/platform/framework/job"
	"github.com/yzxingtu2026/ainp/platform/framework/logging"
	"github.com/yzxingtu2026/ainp/platform/framework/outbox"
	"github.com/yzxingtu2026/ainp/platform/framework/status"
	"github.com/yzxingtu2026/ainp/platform/framework/storage"
	"github.com/yzxingtu2026/ainp/platform/framework/tenant"
	"github.com/yzxingtu2026/ainp/platform/modules/system/attachments/model"
)

const (
	TypeGenerateThumb           = "attachment.generate_thumb"
	EventAttachmentReadyV1      = "attachment.ready.v1"
	HandlerEnqueueGenerateThumb = "attachment.enqueue_generate_thumb"
)

// GenerateThumbPayload is the Job + Outbox DTO (never a GORM model).
type GenerateThumbPayload struct {
	AttachmentID string `json:"attachment_id"`
	ContentType  string `json:"content_type,omitempty"`
}

type thumbOperationalError struct {
	category string
	cause    error
}

func (e *thumbOperationalError) Error() string { return e.category }
func (e *thumbOperationalError) Unwrap() error { return e.cause }
func thumbOperation(category string, cause error) error {
	return &thumbOperationalError{category: category, cause: cause}
}

// Register wires attachment thumb Job + Outbox enqueue handler.
func Register(a *app.App) error {
	if a == nil || a.Jobs == nil {
		return nil
	}
	if err := a.Jobs.Register(job.Definition{
		Type: TypeGenerateThumb, SchemaVersion: 1, Queue: job.QueueFile,
		MaxAttempts: 5, Timeout: 90 * time.Second,
		Description: "Generate attachment thumb derivative",
	}, job.HandlerFunc{
		JobType: TypeGenerateThumb,
		Fn:      makeGenerateThumbHandler(a),
	}); err != nil {
		return err
	}
	if a.Outbox == nil {
		return nil
	}
	if err := a.Outbox.RegisterDefinition(outbox.Definition{
		EventType: EventAttachmentReadyV1, SchemaVersion: 1,
		Description: "Attachment original ready; enqueue thumb generation",
	}); err != nil {
		return err
	}
	return a.Outbox.RegisterHandler(outbox.HandlerFunc{
		HandlerName: HandlerEnqueueGenerateThumb,
		Types:       []string{EventAttachmentReadyV1},
		Fn: func(ctx context.Context, env outbox.Envelope) error {
			if !a.Jobs.Enabled {
				logging.Warn(ctx, "jobs disabled; skip thumb enqueue", "event_id", env.ID, "event_type", env.EventType)
				return nil
			}
			var p GenerateThumbPayload
			_ = json.Unmarshal(env.Payload, &p)
			if strings.TrimSpace(p.AttachmentID) == "" {
				logging.Warn(ctx, "thumb enqueue skipped: empty attachment_id", "event_id", env.ID)
				return nil
			}
			ctx = auth.WithContext(ctx, &auth.Context{
				TenantID: env.TenantID, Realm: auth.NormalizeRealm(auth.Realm(env.Realm)),
				UserID: env.ActorID, RequestID: env.CorrelationID,
				Authenticated: true, Client: "outbox",
			})
			_, err := a.Jobs.Enqueuer.Enqueue(ctx, job.EnqueueInput{
				Type:           TypeGenerateThumb,
				IdempotencyKey: env.ID + ":" + TypeGenerateThumb,
				CausationID:    env.ID,
				Payload:        GenerateThumbPayload{AttachmentID: p.AttachmentID, ContentType: p.ContentType},
			})
			return err
		},
	})
}

func makeGenerateThumbHandler(a *app.App) func(ctx context.Context, env job.Envelope, payload json.RawMessage, reporter job.Reporter) error {
	return func(ctx context.Context, env job.Envelope, payload json.RawMessage, reporter job.Reporter) error {
		var p GenerateThumbPayload
		if err := json.Unmarshal(payload, &p); err != nil {
			return job.Permanent(err)
		}
		if strings.TrimSpace(p.AttachmentID) == "" {
			return job.Permanent(errors.New("attachment_id required"))
		}
		if a.DB == nil || a.Storage == nil {
			return job.Permanent(errors.New("db or storage unavailable"))
		}
		_ = reporter.Progress(ctx, 5, "load")

		ctx = auth.WithContext(ctx, &auth.Context{
			TenantID: env.TenantID, Realm: auth.NormalizeRealm(env.Realm),
			UserID: env.ActorID, RequestID: env.CorrelationID,
			Authenticated: true, Client: "job",
		})
		tdb, err := tenant.TenantDB(ctx, a.DB)
		if err != nil {
			return job.Permanent(err)
		}
		attachments, _ := tenant.NewTenantRepository[model.Attachment](tdb)

		row, err := attachments.GetByID(p.AttachmentID)
		if err != nil {
			return job.Retryable(errors.New("database_read_failed"))
		}
		if row.Status != status.AttachmentReady {
			return job.Permanent(errors.New("attachment not ready"))
		}
		// Idempotent success if already ready with object.
		if row.GetThumbStatus() == status.AttachmentThumbReady && row.ThumbObjectKey != "" {
			_ = reporter.SetResultRef(ctx, row.ThumbObjectKey)
			return nil
		}
		if row.GetThumbStatus() == status.AttachmentThumbSkipped {
			return nil
		}

		if !attachment.IsThumbMediaMIME(row.ContentType) {
			now := time.Now().UTC()
			_ = attachments.Where("id = ?", row.ID).Updates(map[string]any{
				"thumb_status": status.AttachmentThumbSkipped, "thumb_error": "", "thumb_error_category": "", "thumb_retryable": nil, "thumb_updated_at": now, "updated_at": now,
			}).Error
			return nil
		}

		_ = reporter.Progress(ctx, 20, "open")
		rc, _, err := a.Storage.Open(ctx, row.ObjectKey)
		if err != nil {
			return markThumbFailed(ctx, a, row.ID, row.Size, env.Attempt, thumbOperation("storage_open_failed", err))
		}
		defer rc.Close()

		var thumb []byte
		switch {
		case attachment.IsImageMIME(row.ContentType):
			thumb, err = attachment.GenerateImageThumb(rc)
		case attachment.IsVideoMIME(row.ContentType):
			if !attachment.FFmpegAvailable() {
				now := time.Now().UTC()
				_ = attachments.Where("id = ?", row.ID).Updates(map[string]any{
					"thumb_status": status.AttachmentThumbSkipped, "thumb_error": "ffmpeg unavailable",
					"thumb_error_category": attachment.ThumbErrorVideoToolMissing, "thumb_retryable": false,
					"thumb_updated_at": now, "updated_at": now,
				}).Error
				logging.Warn(ctx, "video thumb skipped: ffmpeg unavailable", "attachment_id", row.ID)
				return nil
			}
			vctx, cancel := context.WithTimeout(ctx, attachment.VideoThumbTimeout)
			defer cancel()
			thumb, err = attachment.GenerateVideoThumb(vctx, rc, path.Ext(row.OriginalName))
			if errors.Is(err, attachment.ErrFFmpegUnavailable) {
				now := time.Now().UTC()
				_ = attachments.Where("id = ?", row.ID).Updates(map[string]any{
					"thumb_status": status.AttachmentThumbSkipped, "thumb_error": "ffmpeg unavailable",
					"thumb_error_category": attachment.ThumbErrorVideoToolMissing, "thumb_retryable": false,
					"thumb_updated_at": now, "updated_at": now,
				}).Error
				return nil
			}
		default:
			now := time.Now().UTC()
			_ = attachments.Where("id = ?", row.ID).Updates(map[string]any{
				"thumb_status": status.AttachmentThumbSkipped, "thumb_error_category": "", "thumb_retryable": nil, "thumb_updated_at": now, "updated_at": now,
			}).Error
			return nil
		}
		if err != nil {
			if ctx.Err() != nil {
				return job.Cancelled(ctx.Err())
			}
			return markThumbFailed(ctx, a, row.ID, row.Size, env.Attempt, err)
		}
		_ = reporter.Progress(ctx, 70, "put")

		thumbKey, err := storage.BuildKey(storage.KeyParts{
			TenantID: row.TenantID, Namespace: "attachments",
		})
		if err != nil {
			return markThumbFailed(ctx, a, row.ID, row.Size, env.Attempt, thumbOperation("thumb_key_failed", err))
		}
		info, err := a.Storage.Put(ctx, storage.PutRequest{
			Key: thumbKey, Body: bytes.NewReader(thumb), Size: int64(len(thumb)), ContentType: "image/jpeg",
		})
		if err != nil {
			return markThumbFailed(ctx, a, row.ID, row.Size, env.Attempt, thumbOperation("storage_put_failed", err))
		}

		// Replace previous thumb object if any (retry after partial success).
		oldKey := row.ThumbObjectKey
		now := time.Now().UTC()
		if err := attachments.Where("id = ?", row.ID).Updates(map[string]any{
			"thumb_status": status.AttachmentThumbReady, "thumb_object_key": info.Key,
			"thumb_content_type": "image/jpeg", "thumb_size": info.Size, "thumb_error": "",
			"thumb_error_category": "", "thumb_retryable": nil,
			"thumb_updated_at": now, "updated_at": now,
		}).Error; err != nil {
			_ = a.Storage.Delete(ctx, info.Key)
			return job.Retryable(errors.New("database_update_failed"))
		}
		if oldKey != "" && oldKey != info.Key {
			_ = a.Storage.Delete(ctx, oldKey)
		}
		_ = reporter.SetResultRef(ctx, info.Key)
		_ = reporter.Progress(ctx, 100, "done")
		return nil
	}
}

func ThumbJobDisposition(cause error) (category string, retryable bool, result error) {
	if errors.Is(cause, context.Canceled) || errors.Is(cause, context.DeadlineExceeded) {
		return "cancelled", false, job.Cancelled(cause)
	}
	if typed, ok := attachment.AsThumbError(cause); ok {
		category = string(typed.Category)
		return category, false, job.Permanent(errors.New(category))
	}
	var operational *thumbOperationalError
	if errors.As(cause, &operational) {
		return operational.category, true, job.Retryable(errors.New(operational.category))
	}
	return "infrastructure", true, job.Retryable(errors.New("thumbnail infrastructure failure"))
}

func markThumbFailed(ctx context.Context, a *app.App, attachmentID string, sourceSize int64, attempt int, cause error) error {
	category, retryable, disposition := ThumbJobDisposition(cause)
	if job.IsCancelled(disposition) {
		return disposition
	}
	msg := "thumbnail generation failed"
	fields := []any{"attachment_id", attachmentID, "error_category", category, "source_size", sourceSize, "attempt", attempt}
	if typed, ok := attachment.AsThumbError(cause); ok {
		msg = typed.SafeMessage()
		fields = append(fields, "width", typed.Width, "height", typed.Height, "pixel_count", typed.PixelCount, "pixel_limit", typed.PixelLimit)
	}
	now := time.Now().UTC()
	if tdb, err := tenant.TenantDB(ctx, a.DB); err == nil {
		attachments, _ := tenant.NewTenantRepository[model.Attachment](tdb)
		if updateErr := attachments.Where("id = ?", attachmentID).Updates(map[string]any{
			"thumb_status": status.AttachmentThumbFailed, "thumb_error": msg,
			"thumb_error_category": category, "thumb_retryable": retryable,
			"thumb_updated_at": now, "updated_at": now,
		}).Error; updateErr != nil {
			return job.Retryable(errors.New("database_update_failed"))
		}
	} else {
		return job.Retryable(errors.New("database_context_failed"))
	}
	logging.Info(ctx, "attachment thumb failed", fields...)
	return disposition
}

// RequeuePendingThumbs enqueues generate_thumb for ready attachments still in thumb pending/failed.
// Use after process restart or when jobs were temporarily disabled (Outbox delivery already succeeded).
func RequeuePendingThumbs(ctx context.Context, a *app.App, limit int) (int, error) {
	if a == nil || a.DB == nil || a.Jobs == nil || !a.Jobs.Enabled || a.Jobs.Enqueuer == nil {
		return 0, nil
	}
	if limit <= 0 {
		limit = 50
	}
	ac := auth.MustFromContext(ctx)
	if ac.TenantID == "" {
		return 0, fmt.Errorf("tenant context required")
	}
	tdb, err := tenant.TenantDB(ctx, a.DB)
	if err != nil {
		return 0, err
	}
	attachments, _ := tenant.NewTenantRepository[model.Attachment](tdb)
	rows, err := attachments.
		Where("status = ? AND (thumb_status = ? OR (thumb_status = ? AND thumb_retryable = TRUE))", status.AttachmentReady, status.AttachmentThumbPending, status.AttachmentThumbFailed).
		Order("updated_at ASC").List(0, limit)
	if err != nil {
		return 0, err
	}
	n := 0
	for _, row := range rows {
		if !attachment.IsThumbMediaMIME(row.ContentType) {
			continue
		}
		_, err := a.Jobs.Enqueuer.Enqueue(ctx, job.EnqueueInput{
			Type:           TypeGenerateThumb,
			IdempotencyKey: "requeue:" + row.ID + ":" + TypeGenerateThumb + ":" + row.UpdatedAt.UTC().Format(time.RFC3339Nano),
			Payload:        GenerateThumbPayload{AttachmentID: row.ID, ContentType: row.ContentType},
		})
		if err != nil {
			logging.Warn(ctx, "requeue thumb failed", "attachment_id", row.ID, "error", err.Error())
			continue
		}
		n++
	}
	return n, nil
}
