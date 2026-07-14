package logging

import (
	"context"
	"log/slog"
	"os"
	"strings"

	"github.com/yzxingtu2026/ainp/platform/framework/auth"
)

var defaultLogger *slog.Logger = slog.Default()

func Init(level, format string) {
	var lvl slog.Level
	switch strings.ToLower(level) {
	case "debug":
		lvl = slog.LevelDebug
	case "warn":
		lvl = slog.LevelWarn
	case "error":
		lvl = slog.LevelError
	default:
		lvl = slog.LevelInfo
	}

	opts := &slog.HandlerOptions{Level: lvl}
	var handler slog.Handler
	if strings.EqualFold(format, "json") {
		handler = slog.NewJSONHandler(os.Stdout, opts)
	} else {
		handler = slog.NewTextHandler(os.Stdout, opts)
	}
	defaultLogger = slog.New(handler)
	slog.SetDefault(defaultLogger)
}

func withContext(ctx context.Context) *slog.Logger {
	logger := defaultLogger
	if ac, ok := auth.FromContext(ctx); ok {
		attrs := []any{}
		if ac.RequestID != "" {
			attrs = append(attrs, "request_id", ac.RequestID)
		}
		if ac.TraceID != "" {
			attrs = append(attrs, "trace_id", ac.TraceID)
		}
		if ac.TenantID != "" {
			attrs = append(attrs, "tenant_id", ac.TenantID)
		}
		if ac.Realm != "" {
			attrs = append(attrs, "realm", ac.Realm)
		}
		if ac.UserID != "" {
			attrs = append(attrs, "user_id", ac.UserID)
		}
		if len(attrs) > 0 {
			logger = logger.With(attrs...)
		}
	}
	return logger
}

func Info(ctx context.Context, msg string, args ...any) {
	withContext(ctx).Info(msg, args...)
}

func Warn(ctx context.Context, msg string, args ...any) {
	withContext(ctx).Warn(msg, args...)
}

func Error(ctx context.Context, msg string, err error, args ...any) {
	all := append([]any{"err", err}, args...)
	withContext(ctx).Error(msg, all...)
}

func Debug(ctx context.Context, msg string, args ...any) {
	withContext(ctx).Debug(msg, args...)
}
