package system

import (
	"bytes"
	"context"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"net/http"
	"strings"

	"github.com/yzxingtu2026/ainp/platform/framework/aimodelruntime"
	"github.com/yzxingtu2026/ainp/platform/framework/status"
	_ "golang.org/x/image/webp"
)

const aiImageMaxBytes int64 = 10 * 1024 * 1024
const aiImageMaxDimension = 8192
const aiImageMaxPixels = 25_000_000

var aiImageMIMEs = map[string]bool{"image/jpeg": true, "image/png": true, "image/gif": true, "image/webp": true}

func (s *AttachmentService) Resolve(ctx context.Context, req aimodelruntime.AssetResolveRequest) (*aimodelruntime.ResolvedImage, error) {
	if req.Purpose != "model_input" || strings.TrimSpace(req.AttachmentID) == "" {
		return nil, fmt.Errorf("invalid asset resolve request")
	}
	tenantID, userID, err := tenantIdentity(ctx)
	if err != nil {
		return nil, err
	}
	row, err := s.getOwned(ctx, tenantID, req.AttachmentID)
	if err != nil {
		return nil, err
	}
	if row.Status != status.AttachmentReady {
		return nil, fmt.Errorf("attachment is not ready")
	}
	if err := s.authorizeDownload(ctx, tenantID, userID, row); err != nil {
		return nil, err
	}
	if row.Size <= 0 || row.Size > aiImageMaxBytes {
		return nil, fmt.Errorf("image size exceeds model input limit")
	}
	rc, _, err := s.store.Open(ctx, row.ObjectKey)
	if err != nil {
		return nil, mapAttachmentError(err)
	}
	defer rc.Close()
	data, err := io.ReadAll(io.LimitReader(rc, aiImageMaxBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read model input image: %w", err)
	}
	if int64(len(data)) != row.Size || int64(len(data)) > aiImageMaxBytes {
		return nil, fmt.Errorf("image size is invalid")
	}
	mime := strings.ToLower(strings.TrimSpace(strings.Split(http.DetectContentType(data), ";")[0]))
	declared := strings.ToLower(strings.TrimSpace(strings.Split(row.ContentType, ";")[0]))
	if !aiImageMIMEs[mime] || mime != declared {
		return nil, fmt.Errorf("image content type is invalid")
	}
	config, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("invalid image content")
	}
	if !formatMatchesMIME(format, mime) || config.Width < 1 || config.Height < 1 || config.Width > aiImageMaxDimension || config.Height > aiImageMaxDimension || int64(config.Width)*int64(config.Height) > aiImageMaxPixels {
		return nil, fmt.Errorf("image dimensions exceed model input limit")
	}
	return &aimodelruntime.ResolvedImage{AttachmentID: row.ID, Revision: row.SHA256, MIME: mime, Size: int64(len(data)), Width: config.Width, Height: config.Height, SHA256: row.SHA256, Data: data}, nil
}

func formatMatchesMIME(format, mime string) bool {
	return (format == "jpeg" && mime == "image/jpeg") || (format == "png" && mime == "image/png") || (format == "gif" && mime == "image/gif") || (format == "webp" && mime == "image/webp")
}
