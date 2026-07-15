package response

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/yzxingtu2026/ainp/platform/framework/auth"
	"github.com/yzxingtu2026/ainp/platform/framework/errs"
	"github.com/yzxingtu2026/ainp/platform/framework/i18n"
)

type Envelope struct {
	Code       int    `json:"code"`
	Message    string `json:"message"`
	MessageKey string `json:"message_key,omitempty"`
	Data       any    `json:"data"`
	RequestID  string `json:"request_id"`
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

func lang(c *gin.Context) string {
	if ac, ok := auth.FromContext(c.Request.Context()); ok && ac.Language != "" {
		return ac.Language
	}
	return i18n.DefaultLang()
}

func OK(c *gin.Context, data any) {
	l := lang(c)
	c.JSON(http.StatusOK, Envelope{
		Code:       errs.CodeOK,
		Message:    i18n.T(l, "common.ok"),
		MessageKey: "common.ok",
		Data:       data,
		RequestID:  requestID(c),
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
	case errs.CodeConflict:
		httpStatus = http.StatusConflict
	case errs.CodeInternal:
		httpStatus = http.StatusInternalServerError
	}

	l := lang(c)
	key := ae.Key()

	var msg string
	if ae.Code == errs.CodeInternal {
		// 内部错误始终使用通用文案，不泄露细节
		msg = i18n.T(l, "common.internal_error")
		key = "common.internal_error"
	} else if key != "" {
		// 优先用 i18n 解析 message_key
		msg = i18n.T(l, key)
	} else {
		// 兑底：使用 AppError.Message
		msg = ae.Message
	}

	// 校验字段 message 同步走 i18n
	var data any
	if len(ae.Fields) > 0 {
		fields := make([]errs.FieldError, len(ae.Fields))
		for i, f := range ae.Fields {
			fields[i] = f
			if f.MessageKey != "" {
				fields[i].Message = i18n.T(l, f.MessageKey)
			}
		}
		data = gin.H{"fields": fields}
	}

	c.JSON(httpStatus, Envelope{
		Code:       ae.Code,
		Message:    msg,
		MessageKey: key,
		Data:       data,
		RequestID:  requestID(c),
	})
}
