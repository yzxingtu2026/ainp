package jobs

import (
	"context"

	"github.com/yzxingtu2026/ainp/platform/framework/app"
	attachmentjobs "github.com/yzxingtu2026/ainp/platform/modules/system/attachments/jobs"
)

const (
	TypeGenerateThumb           = attachmentjobs.TypeGenerateThumb
	EventAttachmentReadyV1      = attachmentjobs.EventAttachmentReadyV1
	HandlerEnqueueGenerateThumb = attachmentjobs.HandlerEnqueueGenerateThumb
)

type GenerateThumbPayload = attachmentjobs.GenerateThumbPayload

func Register(a *app.App) error { return attachmentjobs.Register(a) }

func ThumbJobDisposition(cause error) (category string, retryable bool, result error) {
	return attachmentjobs.ThumbJobDisposition(cause)
}

func RequeuePendingThumbs(ctx context.Context, a *app.App, limit int) (int, error) {
	return attachmentjobs.RequeuePendingThumbs(ctx, a, limit)
}
