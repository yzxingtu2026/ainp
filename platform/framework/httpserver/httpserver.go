package httpserver

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/yzxingtu2026/ainp/platform/framework/action"
	"github.com/yzxingtu2026/ainp/platform/framework/auth"
	"github.com/yzxingtu2026/ainp/platform/framework/codeset"
	"github.com/yzxingtu2026/ainp/platform/framework/errs"
	"github.com/yzxingtu2026/ainp/platform/framework/i18n"
	"github.com/yzxingtu2026/ainp/platform/framework/menu"
	"github.com/yzxingtu2026/ainp/platform/framework/middleware"
	"github.com/yzxingtu2026/ainp/platform/framework/response"
	"gorm.io/gorm"
)

type Options struct {
	Mode            string
	Installed       *bool
	DevTokenEnabled bool
	Tokens          *auth.TokenService
	Actions         *action.Registry
	Menus           *menu.Registry
	CodeSets        *codeset.Registry
	DB              *gorm.DB
}

func New(opts Options) *gin.Engine {
	if opts.Mode == "release" {
		gin.SetMode(gin.ReleaseMode)
	}
	r := gin.New()
	r.Use(
		middleware.Recover(),
		middleware.RequestID(),
		middleware.SetupGate(opts.Installed),
		middleware.AcceptLanguage(),
		middleware.Auth(middleware.AuthOptions{
			Tokens:          opts.Tokens,
			DevTokenEnabled: opts.DevTokenEnabled,
		}),
	)

	r.GET("/healthz", func(c *gin.Context) {
		status := "ok"
		if opts.DB != nil {
			sqlDB, err := opts.DB.DB()
			if err != nil || sqlDB.Ping() != nil {
				status = "degraded"
			}
		}
		response.OK(c, gin.H{"status": status})
	})

	r.GET("/api/v1/menus", func(c *gin.Context) {
		ac, ok := auth.FromContext(c.Request.Context())
		if !ok || ac == nil || !ac.Authenticated {
			response.Fail(c, errs.NewKey(errs.CodeUnauthorized, "common.unauthorized", "unauthorized"))
			return
		}
		hasPerm := func(code string) bool {
			return ac.HasPermission(code)
		}
		lang := ac.Language

		// 菜单标题按语言本地化
		items := opts.Menus.TreeForUser(ac.Realm, hasPerm)
		for i := range items {
			if items[i].TitleKey != "" {
				items[i].Title = i18n.T(lang, items[i].TitleKey)
			}
		}
		buttons := opts.Menus.ButtonsForUser(ac.Realm, hasPerm)
		for i := range buttons {
			if buttons[i].TitleKey != "" {
				buttons[i].Title = i18n.T(lang, buttons[i].TitleKey)
			}
		}

		response.OK(c, gin.H{
			"items":   items,
			"buttons": buttons,
		})
	})

	if opts.CodeSets != nil {
		codesetHandler := func(c *gin.Context) {
			ac, ok := auth.FromContext(c.Request.Context())
			if !ok || ac == nil || !ac.Authenticated {
				response.Fail(c, errs.NewKey(errs.CodeUnauthorized, "common.unauthorized", "unauthorized"))
				return
			}
			version := opts.CodeSets.Version()
			etag := `"` + version + `"`
			if c.GetHeader("If-None-Match") == etag {
				c.Status(http.StatusNotModified)
				return
			}
			c.Header("ETag", etag)
			sets := opts.CodeSets.List(ac.Realm, ac.HasPermission)
			requested := c.QueryArray("key")
			if len(requested) == 0 {
				requested = c.QueryArray("key[]")
			}
			if key := c.Param("key"); key != "" {
				requested = []string{key}
			}
			if len(requested) > 0 {
				allowed := make(map[string]struct{}, len(requested))
				for _, key := range requested {
					allowed[key] = struct{}{}
				}
				filtered := sets[:0]
				for _, set := range sets {
					if _, ok := allowed[set.Key]; ok {
						filtered = append(filtered, set)
					}
				}
				sets = filtered
				if len(sets) == 0 && c.Param("key") != "" {
					response.Fail(c, errs.NewKey(errs.CodeCodeSetNotFound, "codeset.not_found", "codeset not found"))
					return
				}
			}
			for i := range sets {
				sets[i] = codeset.Localize(sets[i], ac.Language)
			}
			response.OK(c, gin.H{"version": version, "items": sets})
		}
		r.GET("/api/v1/codesets", codesetHandler)
		r.GET("/api/v1/codesets/:key", codesetHandler)
	}

	if opts.Actions != nil {
		for _, a := range opts.Actions.List() {
			if a.Meta.HTTP == nil || !a.Meta.HTTP.IsEnabled() {
				continue
			}
			method := strings.ToUpper(a.Meta.HTTP.Method)
			path := a.Meta.HTTP.Path
			actionID := a.ID
			handler := func(c *gin.Context) {
				body, _ := io.ReadAll(c.Request.Body)
				if len(body) == 0 {
					body = []byte("{}")
				}
				// 将语言写入 context，供业务 handler 通过 i18n.FromCtx 获取
				ac := auth.MustFromContext(c.Request.Context())
				ctx := i18n.WithLang(c.Request.Context(), ac.Language)
				c.Request = c.Request.WithContext(ctx)

				data, err := opts.Actions.Invoke(c.Request.Context(), actionID, json.RawMessage(body))
				if err != nil {
					response.Fail(c, err)
					return
				}
				response.OK(c, data)
			}
			switch method {
			case http.MethodGet:
				r.GET(path, handler)
			case http.MethodPost:
				r.POST(path, handler)
			case http.MethodPut:
				r.PUT(path, handler)
			case http.MethodPatch:
				r.PATCH(path, handler)
			case http.MethodDelete:
				r.DELETE(path, handler)
			}
		}
	}

	return r
}
