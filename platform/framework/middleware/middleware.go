package middleware

import (
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/yzxingtu2026/ainp/platform/framework/auth"
	"github.com/yzxingtu2026/ainp/platform/framework/errs"
	"github.com/yzxingtu2026/ainp/platform/framework/response"
)

func RequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		rid := c.GetHeader("X-Request-ID")
		if rid == "" {
			rid = uuid.NewString()
		}
		c.Set("request_id", rid)
		c.Writer.Header().Set("X-Request-ID", rid)

		ac := &auth.Context{
			RequestID: rid,
			TraceID:   rid,
			Client:    "web",
		}
		if existing, ok := auth.FromContext(c.Request.Context()); ok && existing != nil {
			*ac = *existing
			ac.RequestID = rid
			if ac.TraceID == "" {
				ac.TraceID = rid
			}
		}
		ctx := auth.WithContext(c.Request.Context(), ac)
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	}
}

func Recover() gin.HandlerFunc {
	return gin.CustomRecovery(func(c *gin.Context, recovered any) {
		response.Fail(c, errs.Wrap(errs.CodeInternal, "panic", nil))
		c.Abort()
	})
}

// DevToken 解析开发桩：Bearer dev:<tenant_id>:<user_id>:<perm1,perm2>
func DevToken(enabled bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !enabled {
			c.Next()
			return
		}
		header := c.GetHeader("Authorization")
		if header == "" || !strings.HasPrefix(header, "Bearer ") {
			c.Next()
			return
		}
		token := strings.TrimSpace(strings.TrimPrefix(header, "Bearer "))
		if !strings.HasPrefix(token, "dev:") {
			c.Next()
			return
		}
		parts := strings.Split(token, ":")
		// dev | tenant | user | perms
		if len(parts) < 3 {
			c.Next()
			return
		}
		tenantID := parts[1]
		userID := parts[2]
		var perms []string
		if len(parts) >= 4 && parts[3] != "" {
			perms = strings.Split(parts[3], ",")
		}

		ac := auth.MustFromContext(c.Request.Context())
		ac.TenantID = tenantID
		ac.UserID = userID
		ac.Permissions = perms
		ac.Authenticated = true
		ac.Client = "web"
		ctx := auth.WithContext(c.Request.Context(), ac)
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	}
}
