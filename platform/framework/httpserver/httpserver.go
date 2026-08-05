package httpserver

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/yzxingtu2026/ainp/platform/framework/action"
	"github.com/yzxingtu2026/ainp/platform/framework/auth"
	"github.com/yzxingtu2026/ainp/platform/framework/buildinfo"
	"github.com/yzxingtu2026/ainp/platform/framework/codeset"
	"github.com/yzxingtu2026/ainp/platform/framework/errs"
	"github.com/yzxingtu2026/ainp/platform/framework/i18n"
	"github.com/yzxingtu2026/ainp/platform/framework/logging"
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
	Authorization   auth.AuthorizationProvider
	Actions         *action.Registry
	Menus           *menu.Registry
	CodeSets        *codeset.Registry
	DB              *gorm.DB
	OnHTTP          func(engine *gin.Engine)
}

func New(opts Options) *gin.Engine {
	if opts.Mode == "release" {
		gin.SetMode(gin.ReleaseMode)
	}
	r := gin.New()
	r.Use(
		middleware.Recover(),
		middleware.RequestID(),
		middleware.RequestLog(),
		middleware.SetupGate(opts.Installed),
		middleware.AcceptLanguage(),
		middleware.Auth(middleware.AuthOptions{
			Tokens:          opts.Tokens,
			Authorization:   opts.Authorization,
			DevTokenEnabled: opts.DevTokenEnabled,
		}),
		middleware.IdempotencyKey(),
	)

	r.GET("/healthz", func(c *gin.Context) {
		status := "ok"
		if opts.DB != nil {
			sqlDB, err := opts.DB.DB()
			if err != nil || sqlDB.Ping() != nil {
				status = "degraded"
			}
		}
		info := buildinfo.Current()
		response.OK(c, gin.H{
			"status":     status,
			"version":    info.Version,
			"commit":     info.Commit,
			"build_time": info.BuildTime,
		})
	})

	r.GET("/api/v1/menus", func(c *gin.Context) {
		if err := action.RequireAuthenticated(c.Request.Context()); err != nil {
			response.Fail(c, err)
			return
		}
		ac := auth.MustFromContext(c.Request.Context())
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

	r.GET("/api/v1/navigation/search", func(c *gin.Context) {
		if err := action.RequireAuthenticated(c.Request.Context()); err != nil {
			response.Fail(c, err)
			return
		}
		startedAt := time.Now()
		query := c.Query("query")
		limit := 0
		if rawLimit := c.Query("limit"); rawLimit != "" {
			parsed, err := strconv.Atoi(rawLimit)
			if err != nil {
				response.Fail(c, errs.NewKey(errs.CodeValidation, "navigation.invalid_limit", "navigation limit must be an integer"))
				return
			}
			limit = parsed
		}
		ac := auth.MustFromContext(c.Request.Context())
		results, semantic, err := menu.NewSearchService(opts.Menus).SearchContext(c.Request.Context(), menu.SearchOptions{
			TenantID: ac.TenantID,
			Realm:    ac.Realm, Language: ac.Language, HasPermission: ac.HasPermission,
			Query: query, Limit: limit,
		})
		if err != nil {
			response.Fail(c, errs.NewKey(errs.CodeValidation, "navigation.invalid_query", err.Error()))
			return
		}
		matchTypes := make(map[string]int)
		for _, result := range results {
			matchTypes[result.MatchType]++
		}
		logging.Info(c.Request.Context(), "navigation search completed",
			"query_length", len([]rune(query)), "result_count", len(results),
			"match_types", matchTypes, "duration_ms", time.Since(startedAt).Milliseconds())
		response.OK(c, gin.H{"items": results, "degraded": semantic.Degraded, "degraded_reason": semantic.Reason})
	})

	if opts.CodeSets != nil {
		codesetHandler := func(c *gin.Context) {
			if err := action.RequireAuthenticated(c.Request.Context()); err != nil {
				response.Fail(c, err)
				return
			}
			ac := auth.MustFromContext(c.Request.Context())
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
			handler := actionHTTPHandler(opts.Actions, a.ID)
			registerActionHTTPRoute(r, method, path, handler)
		}
		for _, alias := range opts.Actions.HTTPAliases() {
			method := strings.ToUpper(alias.HTTP.Method)
			path := alias.HTTP.Path
			handler := actionHTTPHandler(opts.Actions, alias.ID)
			registerActionHTTPRoute(r, method, path, handler)
		}
	}

	if opts.OnHTTP != nil {
		opts.OnHTTP(r)
	}

	return r
}

func actionHTTPHandler(actions *action.Registry, actionID string) gin.HandlerFunc {
	return func(c *gin.Context) {
		body, _ := io.ReadAll(c.Request.Body)
		if len(body) == 0 {
			body = []byte("{}")
		}
		ac := auth.MustFromContext(c.Request.Context())
		ctx := i18n.WithLang(c.Request.Context(), ac.Language)
		c.Request = c.Request.WithContext(ctx)

		data, err := actions.Invoke(c.Request.Context(), actionID, json.RawMessage(body))
		if err != nil {
			response.Fail(c, err)
			return
		}
		if stream, ok := data.(action.StreamResult); ok {
			writeActionStream(c, stream)
			return
		}
		response.OK(c, data)
	}
}

func registerActionHTTPRoute(r *gin.Engine, method, path string, handler gin.HandlerFunc) {
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

func writeActionStream(c *gin.Context, stream action.StreamResult) {
	if stream.Cancel != nil {
		defer stream.Cancel()
	}
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")
	c.Status(http.StatusOK)
	c.Stream(func(w io.Writer) bool {
		select {
		case <-c.Request.Context().Done():
			return false
		case event, ok := <-stream.Events:
			if !ok {
				return false
			}
			payload, err := json.Marshal(event.Data)
			if err != nil {
				return false
			}
			_, _ = fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event.Event, payload)
			// Explicit flush for real-time SSE delivery.
			if flusher, ok := w.(http.Flusher); ok {
				flusher.Flush()
			}
			return true
		}
	})
}
