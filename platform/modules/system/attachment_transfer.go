package system

import (
	"context"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/yzxingtu2026/ainp/platform/framework/attachment"
	"github.com/yzxingtu2026/ainp/platform/framework/auth"
	"github.com/yzxingtu2026/ainp/platform/framework/errs"
	"github.com/yzxingtu2026/ainp/platform/framework/i18n"
	"github.com/yzxingtu2026/ainp/platform/framework/response"
	"github.com/yzxingtu2026/ainp/platform/modules/system/model"
)

// RegisterAttachmentTransferRoutes mounts binary upload/download endpoints.
// Permission decisions happen in prepare Actions / domain service; tokens authorize the transfer only.
func RegisterAttachmentTransferRoutes(r gin.IRoutes, svc *AttachmentService) {
	if svc == nil {
		return
	}
	r.PUT("/api/v1/attachments/transfer/:id", func(c *gin.Context) { handleAttachmentUpload(c, svc) })
	r.GET("/api/v1/attachments/transfer/:id", func(c *gin.Context) { handleAttachmentDownload(c, svc) })
	r.HEAD("/api/v1/attachments/transfer/:id", func(c *gin.Context) { handleAttachmentDownload(c, svc) })
}

func transferTokenFromRequest(c *gin.Context) string {
	if q := c.Query("token"); q != "" {
		return q
	}
	h := c.GetHeader("Authorization")
	if len(h) > 7 && strings.EqualFold(h[:7], "bearer ") {
		return strings.TrimSpace(h[7:])
	}
	return c.GetHeader("X-Attachment-Token")
}

func withTransferAuth(c *gin.Context, claims *attachment.TransferClaims) context.Context {
	ac := &auth.Context{
		TenantID: claims.TenantID, UserID: claims.UserID,
		Realm: auth.RealmTenant, Authenticated: true, Language: i18n.FromCtx(c.Request.Context()),
	}
	ctx := auth.WithContext(c.Request.Context(), ac)
	c.Request = c.Request.WithContext(ctx)
	return ctx
}

func handleAttachmentUpload(c *gin.Context, svc *AttachmentService) {
	token := transferTokenFromRequest(c)
	if token == "" {
		response.Fail(c, errs.NewKey(errs.CodeUnauthorized, "attachment.token_invalid", "missing transfer token"))
		return
	}
	claims, err := svc.ParseTransferToken(token)
	if err != nil {
		response.Fail(c, err)
		return
	}
	if claims.AttachmentID != c.Param("id") {
		response.Fail(c, errs.NewKey(errs.CodeForbidden, "attachment.token_invalid", "attachment id mismatch"))
		return
	}
	ctx := withTransferAuth(c, claims)
	row, err := svc.CompleteUpload(ctx, claims, c.Request.Body)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, gin.H{"attachment": toAttachmentView(*row)})
}

func handleAttachmentDownload(c *gin.Context, svc *AttachmentService) {
	token := transferTokenFromRequest(c)
	if token == "" {
		response.Fail(c, errs.NewKey(errs.CodeUnauthorized, "attachment.token_invalid", "missing transfer token"))
		return
	}
	claims, err := svc.ParseTransferToken(token)
	if err != nil {
		response.Fail(c, err)
		return
	}
	if claims.AttachmentID != c.Param("id") {
		response.Fail(c, errs.NewKey(errs.CodeForbidden, "attachment.token_invalid", "attachment id mismatch"))
		return
	}
	ctx := withTransferAuth(c, claims)

	rangeHeader := c.GetHeader("Range")
	if rangeHeader != "" && strings.HasPrefix(rangeHeader, "bytes=") {
		writeAttachmentRange(c, svc, ctx, claims, rangeHeader)
		return
	}

	rc, att, err := svc.OpenDownload(ctx, claims)
	if err != nil {
		response.Fail(c, err)
		return
	}
	defer rc.Close()
	writeAttachmentHeaders(c, att, att.Size)
	if c.Request.Method == http.MethodHead {
		c.Status(http.StatusOK)
		return
	}
	c.Status(http.StatusOK)
	_, _ = io.Copy(c.Writer, rc)
}

func writeAttachmentRange(c *gin.Context, svc *AttachmentService, ctx context.Context, claims *attachment.TransferClaims, rangeHeader string) {
	rcFull, att, err := svc.OpenDownload(ctx, claims)
	if err != nil {
		response.Fail(c, err)
		return
	}
	_ = rcFull.Close()
	offset, length, ok := parseBytesRange(rangeHeader, att.Size)
	if !ok {
		c.Header("Content-Range", "bytes */"+strconv.FormatInt(att.Size, 10))
		c.Status(http.StatusRequestedRangeNotSatisfiable)
		return
	}
	rc, att, err := svc.OpenDownloadRange(ctx, claims, offset, length)
	if err != nil {
		response.Fail(c, err)
		return
	}
	defer rc.Close()
	c.Header("Content-Range", formatContentRange(offset, length, att.Size))
	c.Header("Accept-Ranges", "bytes")
	c.Header("Content-Length", strconv.FormatInt(length, 10))
	c.Header("Content-Type", att.ContentType)
	c.Header("Content-Disposition", ContentDisposition(att.OriginalName))
	if att.SHA256 != "" {
		c.Header("ETag", `"`+att.SHA256+`"`)
	}
	if c.Request.Method == http.MethodHead {
		c.Status(http.StatusPartialContent)
		return
	}
	c.Status(http.StatusPartialContent)
	_, _ = io.Copy(c.Writer, rc)
}

func writeAttachmentHeaders(c *gin.Context, att *model.Attachment, length int64) {
	c.Header("Accept-Ranges", "bytes")
	c.Header("Content-Length", strconv.FormatInt(length, 10))
	c.Header("Content-Type", att.ContentType)
	c.Header("Content-Disposition", ContentDisposition(att.OriginalName))
	if att.SHA256 != "" {
		c.Header("ETag", `"`+att.SHA256+`"`)
	}
}

func parseBytesRange(header string, size int64) (offset, length int64, ok bool) {
	spec := strings.TrimPrefix(header, "bytes=")
	parts := strings.Split(spec, "-")
	if len(parts) != 2 {
		return 0, 0, false
	}
	if parts[0] == "" {
		// suffix: bytes=-N
		n, err := strconv.ParseInt(parts[1], 10, 64)
		if err != nil || n <= 0 || size < 0 {
			return 0, 0, false
		}
		if size >= 0 && n > size {
			n = size
		}
		return size - n, n, true
	}
	start, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil || start < 0 {
		return 0, 0, false
	}
	if parts[1] == "" {
		if size < 0 {
			return 0, 0, false
		}
		if start >= size {
			return 0, 0, false
		}
		return start, size - start, true
	}
	end, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil || end < start {
		return 0, 0, false
	}
	if size >= 0 && start >= size {
		return 0, 0, false
	}
	if size >= 0 && end >= size {
		end = size - 1
	}
	return start, end - start + 1, true
}

func formatContentRange(offset, length, size int64) string {
	return "bytes " + strconv.FormatInt(offset, 10) + "-" + strconv.FormatInt(offset+length-1, 10) + "/" + strconv.FormatInt(size, 10)
}
