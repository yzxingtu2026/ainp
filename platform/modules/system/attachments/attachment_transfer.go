package attachments

import (
	"context"
	"errors"
	"io"
	"math"
	"mime"
	"mime/multipart"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/yzxingtu2026/ainp/platform/framework/attachment"
	"github.com/yzxingtu2026/ainp/platform/framework/auth"
	"github.com/yzxingtu2026/ainp/platform/framework/errs"
	"github.com/yzxingtu2026/ainp/platform/framework/i18n"
	"github.com/yzxingtu2026/ainp/platform/framework/response"
	"github.com/yzxingtu2026/ainp/platform/modules/system/attachments/model"
)

const (
	multipartFileFieldName       = "file"
	multipartMaxBoundaryBytes    = 70
	multipartMaxOverheadBytes    = int64(64 << 10)
	multipartMaxMetadataBytes    = int64(32 << 10)
	multipartMaxPartHeaderBytes  = 16 << 10
	multipartMaxPartHeaderValues = 32
)

var errMultipartMetadataLimit = errors.New("multipart metadata limit exceeded")

// RegisterAttachmentTransferRoutes mounts binary upload/download endpoints.
// Permission decisions happen in prepare Actions / domain service; tokens authorize the transfer only.
func RegisterAttachmentTransferRoutes(r gin.IRoutes, svc *AttachmentService) {
	if svc == nil {
		return
	}
	r.PUT("/api/v1/attachments/transfer/:id", func(c *gin.Context) { handleAttachmentRawUpload(c, svc) })
	r.POST("/api/v1/attachments/transfer/:id", func(c *gin.Context) { handleAttachmentMultipartUpload(c, svc) })
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
		TenantID: claims.TenantID, PrincipalID: claims.PrincipalID,
		Realm: auth.RealmTenant, Authenticated: true, Language: i18n.FromCtx(c.Request.Context()),
	}
	ctx := auth.WithContext(c.Request.Context(), ac)
	c.Request = c.Request.WithContext(ctx)
	return ctx
}

func handleAttachmentRawUpload(c *gin.Context, svc *AttachmentService) {
	claims, ok := attachmentUploadClaims(c, svc, attachment.TransferTransportRaw)
	if !ok {
		return
	}
	defer c.Request.Body.Close()
	ctx := withTransferAuth(c, claims)
	row, err := svc.CompleteUpload(ctx, claims, c.Request.Body)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, gin.H{"attachment": toAttachmentView(*row)})
}

func handleAttachmentMultipartUpload(c *gin.Context, svc *AttachmentService) {
	claims, ok := attachmentUploadClaims(c, svc, attachment.TransferTransportMultipart)
	if !ok {
		return
	}
	if claims.Size <= 0 || claims.Size > math.MaxInt64-multipartMaxOverheadBytes {
		response.Fail(c, errs.NewKey(errs.CodeForbidden, "attachment.token_invalid", "invalid transfer token size"))
		return
	}
	boundary, err := multipartBoundary(c.GetHeader("Content-Type"))
	if err != nil {
		response.Fail(c, err)
		return
	}
	maxBody := claims.Size + multipartMaxOverheadBytes
	if c.Request.ContentLength > maxBody {
		response.Fail(c, errs.ValidationKey("attachment.multipart_limit_exceeded", "multipart body exceeds limit"))
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxBody)
	defer c.Request.Body.Close()
	budget := &multipartMetadataReader{r: c.Request.Body, remaining: multipartMaxMetadataBytes, active: true}
	uploadBody := &multipartUploadReader{reader: multipart.NewReader(budget, boundary), budget: budget}
	ctx := withTransferAuth(c, claims)
	row, err := svc.CompleteUpload(ctx, claims, uploadBody)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, gin.H{"attachment": toAttachmentView(*row)})
}

func attachmentUploadClaims(c *gin.Context, svc *AttachmentService, transport string) (*attachment.TransferClaims, bool) {
	token := transferTokenFromRequest(c)
	if token == "" {
		response.Fail(c, errs.NewKey(errs.CodeUnauthorized, "attachment.token_invalid", "missing transfer token"))
		return nil, false
	}
	claims, err := svc.ParseTransferToken(token)
	if err != nil {
		response.Fail(c, err)
		return nil, false
	}
	if claims.AttachmentID != c.Param("id") {
		response.Fail(c, errs.NewKey(errs.CodeForbidden, "attachment.token_invalid", "attachment id mismatch"))
		return nil, false
	}
	if claims.Op != attachment.OpUpload || claims.EffectiveUploadTransport() != transport {
		response.Fail(c, errs.NewKey(errs.CodeForbidden, "attachment.transport_mismatch", "upload transport does not match token"))
		return nil, false
	}
	return claims, true
}

func multipartBoundary(contentType string) (string, error) {
	mediaType, params, err := mime.ParseMediaType(strings.TrimSpace(contentType))
	if err != nil || !strings.EqualFold(mediaType, "multipart/form-data") {
		return "", errs.ValidationKey("attachment.multipart_invalid", "content type must be multipart/form-data")
	}
	boundary := params["boundary"]
	if !validMultipartBoundary(boundary) {
		return "", errs.ValidationKey("attachment.multipart_invalid", "invalid multipart boundary")
	}
	return boundary, nil
}

func validMultipartBoundary(boundary string) bool {
	if len(boundary) == 0 || len(boundary) > multipartMaxBoundaryBytes || boundary[len(boundary)-1] == ' ' {
		return false
	}
	for i := 0; i < len(boundary); i++ {
		ch := boundary[i]
		if ch >= 'a' && ch <= 'z' || ch >= 'A' && ch <= 'Z' || ch >= '0' && ch <= '9' {
			continue
		}
		if strings.ContainsRune("'()+_,-./:=? ", rune(ch)) {
			continue
		}
		return false
	}
	return true
}

type multipartMetadataReader struct {
	r         io.Reader
	remaining int64
	active    bool
}

func (r *multipartMetadataReader) Read(p []byte) (int, error) {
	if r.active {
		if r.remaining <= 0 {
			return 0, errMultipartMetadataLimit
		}
		if int64(len(p)) > r.remaining {
			p = p[:r.remaining]
		}
	}
	n, err := r.r.Read(p)
	if r.active {
		r.remaining -= int64(n)
	}
	return n, err
}

type multipartUploadReader struct {
	reader    *multipart.Reader
	budget    *multipartMetadataReader
	part      *multipart.Part
	validated bool
	uploadErr error
}

func (r *multipartUploadReader) Read(p []byte) (int, error) {
	if r.uploadErr != nil {
		return 0, r.uploadErr
	}
	if r.part == nil {
		if err := r.openFilePart(); err != nil {
			return 0, err
		}
	}
	if r.validated {
		return 0, io.EOF
	}
	n, err := r.part.Read(p)
	if !errors.Is(err, io.EOF) {
		if err != nil {
			return n, r.fail(mapMultipartReadError(err))
		}
		return n, nil
	}
	_ = r.part.Close()
	r.budget.active = true
	next, nextErr := r.reader.NextRawPart()
	if next != nil {
		_ = next.Close()
		return n, r.fail(errs.ValidationKey("attachment.multipart_invalid", "multipart must contain exactly one file part"))
	}
	if !errors.Is(nextErr, io.EOF) {
		return n, r.fail(mapMultipartReadError(nextErr))
	}
	r.validated = true
	return n, io.EOF
}

func (r *multipartUploadReader) UploadError() error { return r.uploadErr }

func (r *multipartUploadReader) openFilePart() error {
	part, err := r.reader.NextRawPart()
	if errors.Is(err, io.EOF) {
		return r.fail(errs.ValidationKey("attachment.multipart_invalid", "multipart file part is required"))
	}
	if err != nil {
		return r.fail(mapMultipartReadError(err))
	}
	if !validMultipartPartHeaders(part) {
		_ = part.Close()
		return r.fail(errs.ValidationKey("attachment.multipart_limit_exceeded", "multipart part headers exceed limit"))
	}
	partType := strings.ToLower(strings.TrimSpace(part.Header.Get("Content-Type")))
	if strings.HasPrefix(partType, "multipart/") {
		_ = part.Close()
		return r.fail(errs.ValidationKey("attachment.multipart_invalid", "nested multipart is not allowed"))
	}
	if part.Header.Get("Content-Transfer-Encoding") != "" {
		_ = part.Close()
		return r.fail(errs.ValidationKey("attachment.multipart_invalid", "multipart content transfer encoding is not allowed"))
	}
	if part.FormName() != multipartFileFieldName || part.FileName() == "" {
		_ = part.Close()
		return r.fail(errs.ValidationKey("attachment.multipart_invalid", "multipart must contain one file field named file"))
	}
	r.part = part
	r.budget.active = false
	return nil
}

func (r *multipartUploadReader) fail(err error) error {
	if err == nil {
		err = errs.ValidationKey("attachment.multipart_invalid", "invalid multipart upload")
	}
	r.uploadErr = err
	return err
}

func validMultipartPartHeaders(part *multipart.Part) bool {
	values := 0
	bytes := 0
	for key, items := range part.Header {
		values += len(items)
		bytes += len(key)
		for _, item := range items {
			bytes += len(item)
		}
	}
	return values <= multipartMaxPartHeaderValues && bytes <= multipartMaxPartHeaderBytes
}

func mapMultipartReadError(err error) error {
	if err == nil {
		return nil
	}
	var maxBytesErr *http.MaxBytesError
	if errors.As(err, &maxBytesErr) || errors.Is(err, errMultipartMetadataLimit) {
		return errs.ValidationKey("attachment.multipart_limit_exceeded", "multipart body exceeds limit")
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return errs.ValidationKey("attachment.upload_cancelled", "attachment upload cancelled")
	}
	return errs.ValidationKey("attachment.multipart_invalid", "invalid multipart upload")
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
	// DownloadPrepare signs the selected variant size. Parse the range from that
	// immutable claim so the transfer opens the object only once; the service
	// still compares it with current metadata before returning any bytes.
	offset, length, ok := parseBytesRange(rangeHeader, claims.Size)
	if !ok {
		c.Header("Content-Range", "bytes */"+strconv.FormatInt(claims.Size, 10))
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
