package httpserver

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/yzxingtu2026/ainp/platform/framework/action"
	"github.com/yzxingtu2026/ainp/platform/framework/auth"
	"github.com/yzxingtu2026/ainp/platform/framework/errs"
	"github.com/yzxingtu2026/ainp/platform/framework/i18n"
	"github.com/yzxingtu2026/ainp/platform/framework/menu"
	"github.com/yzxingtu2026/ainp/platform/framework/middleware"
	"github.com/yzxingtu2026/ainp/platform/framework/response"
	"gorm.io/gorm"
)

type Options struct {
	Mode            string
	DevTokenEnabled bool
	Tokens          *auth.TokenService
	Actions         *action.Registry
	Menus           *menu.Registry
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
		items := opts.Menus.TreeForUser(hasPerm)
		for i := range items {
			if items[i].TitleKey != "" {
				items[i].Title = i18n.T(lang, items[i].TitleKey)
			}
		}
		buttons := opts.Menus.ButtonsForUser(hasPerm)
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
