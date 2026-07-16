package middleware

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/yzxingtu2026/ainp/platform/framework/auth"
	"github.com/yzxingtu2026/ainp/platform/framework/errs"
	"github.com/yzxingtu2026/ainp/platform/framework/i18n"
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
			Realm:     auth.RealmTenant,
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

// AcceptLanguage 解析 Accept-Language 请求头，将协商后的语言写入 AuthContext.Language。
// 支持值：zh-CN、zh、en；其他值回退到 i18n.DefaultLang()。
func AcceptLanguage() gin.HandlerFunc {
	return func(c *gin.Context) {
		lang := negotiateLang(c.GetHeader("Accept-Language"))
		ac := auth.MustFromContext(c.Request.Context())
		ac.Language = lang
		ctx := auth.WithContext(c.Request.Context(), ac)
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	}
}

// negotiateLang 从 Accept-Language 头中协商出支持的语言。
// 目前仅支持 zh-CN / zh → zh-CN，其他均回退为默认语言。
func negotiateLang(header string) string {
	if header == "" {
		return i18n.DefaultLang()
	}
	// 解析形如 "zh-CN,zh;q=0.9,en;q=0.8"
	for _, part := range strings.Split(header, ",") {
		tag := strings.TrimSpace(strings.SplitN(part, ";", 2)[0])
		tag = strings.ToLower(tag)
		switch tag {
		case "zh-cn", "zh":
			return i18n.LangZhCN
		case "en", "en-us", "en-gb":
			return i18n.LangEn
		}
	}
	return i18n.DefaultLang()
}

// SetupGate 未安装时仅放行 setup / healthz 路径，其他请求返回 503。
// 已安装后透传所有请求。传入 *bool 指针以便安装完成后动态感知状态变化。
func SetupGate(installed *bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		if installed != nil && *installed {
			c.Next()
			return
		}
		path := c.Request.URL.Path
		// 白名单：healthz 和 setup 系列接口
		if path == "/healthz" || strings.HasPrefix(path, "/api/v1/setup/") {
			c.Next()
			return
		}
		c.AbortWithStatusJSON(http.StatusServiceUnavailable, gin.H{
			"code":    errs.CodeSetupRequired,
			"message": "system not installed, please complete setup wizard",
		})
	}
}

// IdempotencyKey 从 Idempotency-Key 请求头提取幂等键并写入 AuthContext。
func IdempotencyKey() gin.HandlerFunc {
	return func(c *gin.Context) {
		key := c.GetHeader("Idempotency-Key")
		if key != "" {
			ac := auth.MustFromContext(c.Request.Context())
			ac.IdempotencyKey = key
			ctx := auth.WithContext(c.Request.Context(), ac)
			c.Request = c.Request.WithContext(ctx)
		}
		c.Next()
	}
}

type AuthOptions struct {
	Tokens          *auth.TokenService
	Authorization   auth.AuthorizationProvider
	DevTokenEnabled bool
}

// Auth 解析 Authorization：正式 JWT 优先；dev: 桩仅在开关开启时可用。
func Auth(opts AuthOptions) gin.HandlerFunc {
	return func(c *gin.Context) {
		header := c.GetHeader("Authorization")
		if header == "" || !strings.HasPrefix(header, "Bearer ") {
			c.Next()
			return
		}
		token := strings.TrimSpace(strings.TrimPrefix(header, "Bearer "))
		if token == "" {
			c.Next()
			return
		}

		ac := auth.MustFromContext(c.Request.Context())

		// 正式 JWT（非 dev: 前缀）
		if !strings.HasPrefix(token, "dev:") {
			if opts.Tokens == nil || opts.Authorization == nil {
				c.Next()
				return
			}
			claims, err := opts.Tokens.Parse(token)
			if err != nil {
				// 解析失败可能是附件 transfer token（不同 audience），交由后续专用校验。
				c.Next()
				return
			}
			snapshot, err := opts.Authorization.LoadAuthorization(c.Request.Context(), auth.AuthorizationRequest{
				TenantID: claims.TenantID, UserID: claims.UserID, SessionID: claims.SessionID, Realm: claims.Realm, AuthVersion: claims.AuthVersion,
			})
			if err != nil || snapshot == nil ||
				snapshot.TenantID != claims.TenantID || snapshot.UserID != claims.UserID || snapshot.SessionID != claims.SessionID || snapshot.AuthVersion != claims.AuthVersion {
				// 会话 JWT 已验签但授权版本失效/不匹配：直接 401，避免匿名继续打到受保护 Action。
				response.Fail(c, errs.NewKey(errs.CodeUnauthorized, "common.unauthorized", "unauthorized"))
				c.Abort()
				return
			}
			ac.TenantID = snapshot.TenantID
			ac.Realm = claims.Realm
			ac.UserID = snapshot.UserID
			ac.SessionID = snapshot.SessionID
			ac.OrgID = snapshot.OrgID
			ac.DeptID = snapshot.DeptID
			ac.Roles = append([]string(nil), snapshot.Roles...)
			ac.Permissions = append([]string(nil), snapshot.Permissions...)
			ac.Authenticated = true
			ac.Client = "web"
			ctx := auth.WithContext(c.Request.Context(), ac)
			c.Request = c.Request.WithContext(ctx)
			c.Next()
			return
		}

		// 开发桩：Bearer dev:<tenant_id>:<user_id>:<perm1,perm2>
		if !opts.DevTokenEnabled {
			c.Next()
			return
		}
		parts := strings.Split(token, ":")
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
		ac.TenantID = tenantID
		ac.Realm = auth.RealmTenant
		ac.UserID = userID
		ac.Permissions = perms
		ac.Authenticated = true
		ac.Client = "web"
		ctx := auth.WithContext(c.Request.Context(), ac)
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	}
}
