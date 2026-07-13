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
	"github.com/yzxingtu2026/ainp/platform/framework/menu"
	"github.com/yzxingtu2026/ainp/platform/framework/middleware"
	"github.com/yzxingtu2026/ainp/platform/framework/response"
	"gorm.io/gorm"
)

type Options struct {
	Mode            string
	DevTokenEnabled bool
	Actions         *action.Registry
	Menus           *menu.Registry
	DB              *gorm.DB
}

func New(opts Options) *gin.Engine {
	if opts.Mode == "release" {
		gin.SetMode(gin.ReleaseMode)
	}
	r := gin.New()
	r.Use(middleware.Recover(), middleware.RequestID(), middleware.DevToken(opts.DevTokenEnabled))

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
			response.Fail(c, errs.New(errs.CodeUnauthorized, "unauthorized"))
			return
		}
		hasPerm := func(code string) bool {
			return ac.HasPermission(code)
		}
		response.OK(c, gin.H{
			"items":   opts.Menus.TreeForUser(hasPerm),
			"buttons": opts.Menus.ButtonsForUser(hasPerm),
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
