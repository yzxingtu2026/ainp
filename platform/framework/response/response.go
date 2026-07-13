package response

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/yzxingtu2026/ainp/platform/framework/auth"
	"github.com/yzxingtu2026/ainp/platform/framework/errs"
)

type Envelope struct {
	Code      int    `json:"code"`
	Message   string `json:"message"`
	Data      any    `json:"data"`
	RequestID string `json:"request_id"`
}

func requestID(c *gin.Context) string {
	if ac, ok := auth.FromContext(c.Request.Context()); ok && ac.RequestID != "" {
		return ac.RequestID
	}
	if v, ok := c.Get("request_id"); ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

func OK(c *gin.Context, data any) {
	c.JSON(http.StatusOK, Envelope{
		Code:      errs.CodeOK,
		Message:   "ok",
		Data:      data,
		RequestID: requestID(c),
	})
}

func Fail(c *gin.Context, err error) {
	ae, ok := errs.AsAppError(err)
	if !ok {
		ae = errs.Wrap(errs.CodeInternal, "internal error", err)
	}

	httpStatus := http.StatusOK
	switch ae.Code {
	case errs.CodeUnauthorized:
		httpStatus = http.StatusUnauthorized
	case errs.CodeForbidden, errs.CodeTenantDisabled:
		httpStatus = http.StatusForbidden
	case errs.CodeInternal:
		httpStatus = http.StatusInternalServerError
	}

	var data any
	if len(ae.Fields) > 0 {
		data = gin.H{"fields": ae.Fields}
	}

	msg := ae.Message
	if ae.Code == errs.CodeInternal {
		msg = "internal error"
	}

	c.JSON(httpStatus, Envelope{
		Code:      ae.Code,
		Message:   msg,
		Data:      data,
		RequestID: requestID(c),
	})
}
