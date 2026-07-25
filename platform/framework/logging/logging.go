package logging

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"regexp"
	"strings"
	"sync"

	"github.com/yzxingtu2026/ainp/platform/framework/auth"
	"gopkg.in/natefinch/lumberjack.v2"
)

var defaultLogger *slog.Logger = slog.Default()

type FileConfig struct {
	Path       string
	MaxSizeMB  int
	MaxBackups int
	MaxAgeDays int
	Compress   bool
}

type Config struct {
	Level      string
	Format     string
	Outputs    []string
	File       FileConfig
	Service    string
	InstanceID string
}

func Init(level, format string) {
	_ = InitConfig(Config{Level: level, Format: format, Outputs: []string{"stdout"}})
}

func InitConfig(cfg Config) io.Closer {
	var lvl slog.Level
	switch strings.ToLower(cfg.Level) {
	case "debug":
		lvl = slog.LevelDebug
	case "warn":
		lvl = slog.LevelWarn
	case "error":
		lvl = slog.LevelError
	default:
		lvl = slog.LevelInfo
	}

	opts := &slog.HandlerOptions{Level: lvl, ReplaceAttr: sanitizeAttr}
	outputs := cfg.Outputs
	if len(outputs) == 0 {
		outputs = []string{"stdout"}
	}
	handlers := make([]slog.Handler, 0, len(outputs))
	closers := multiCloser{}
	for _, output := range outputs {
		switch strings.ToLower(strings.TrimSpace(output)) {
		case "stdout":
			if strings.EqualFold(cfg.Format, "json") {
				handlers = append(handlers, slog.NewJSONHandler(os.Stdout, opts))
			} else {
				handlers = append(handlers, slog.NewTextHandler(os.Stdout, opts))
			}
		case "file":
			if cfg.File.Path == "" {
				fmt.Fprintln(os.Stderr, "logging file sink disabled: path is empty")
				continue
			}
			writer := &lumberjack.Logger{Filename: cfg.File.Path, MaxSize: positive(cfg.File.MaxSizeMB, 100), MaxBackups: positive(cfg.File.MaxBackups, 14), MaxAge: positive(cfg.File.MaxAgeDays, 14), Compress: cfg.File.Compress}
			if err := probeWriter(writer); err != nil {
				fmt.Fprintf(os.Stderr, "logging file sink degraded path=%q error=%v\n", cfg.File.Path, err)
				_ = writer.Close()
				continue
			}
			handlers = append(handlers, slog.NewJSONHandler(writer, opts))
			closers = append(closers, writer)
		}
	}
	if len(handlers) == 0 {
		handlers = append(handlers, slog.NewJSONHandler(os.Stdout, opts))
	}
	handler := slog.Handler(handlers[0])
	if len(handlers) > 1 {
		handler = fanoutHandler{handlers: handlers}
	}
	defaultLogger = slog.New(handler)
	base := []any{}
	if cfg.Service != "" {
		base = append(base, "service", cfg.Service)
	}
	if cfg.InstanceID != "" {
		base = append(base, "instance_id", cfg.InstanceID)
	}
	if len(base) > 0 {
		defaultLogger = defaultLogger.With(base...)
	}
	slog.SetDefault(defaultLogger)
	return closers
}

func positive(value, fallback int) int {
	if value > 0 {
		return value
	}
	return fallback
}

const maxLogValueLength = 1024

var (
	credentialPattern = regexp.MustCompile(`(?i)(password|authorization|cookie|api[_-]?key|token|dsn|connection_string)(\s*[:=]\s*)([^\s,;]+)`)
	urlQueryPattern   = regexp.MustCompile(`(https?://[^\s?]+)\?[^\s]+`)
)

func sanitizeAttr(_ []string, attr slog.Attr) slog.Attr {
	key := strings.ToLower(strings.ReplaceAll(attr.Key, "-", "_"))
	for _, fragment := range []string{"password", "authorization", "cookie", "api_key", "apikey", "token", "dsn", "connection_string"} {
		if strings.Contains(key, fragment) {
			attr.Value = slog.StringValue("***")
			return attr
		}
	}
	if attr.Value.Kind() == slog.KindString {
		value := redactText(attr.Value.String())
		if len(value) > maxLogValueLength {
			value = value[:maxLogValueLength] + "..."
		}
		attr.Value = slog.StringValue(value)
	} else if attr.Value.Kind() == slog.KindAny {
		if err, ok := attr.Value.Any().(error); ok {
			attr.Value = slog.StringValue(redactText(err.Error()))
		}
	}
	return attr
}

func redactText(value string) string {
	value = credentialPattern.ReplaceAllString(value, "$1$2***")
	return urlQueryPattern.ReplaceAllString(value, "$1?[redacted]")
}
func probeWriter(writer io.Writer) error { _, err := writer.Write(nil); return err }

type multiCloser []io.Closer

func (m multiCloser) Close() error {
	var first error
	for _, c := range m {
		if err := c.Close(); err != nil && first == nil {
			first = err
		}
	}
	return first
}

type fanoutHandler struct{ handlers []slog.Handler }

func (h fanoutHandler) Enabled(ctx context.Context, level slog.Level) bool {
	for _, item := range h.handlers {
		if item.Enabled(ctx, level) {
			return true
		}
	}
	return false
}
func (h fanoutHandler) Handle(ctx context.Context, record slog.Record) error {
	var wg sync.WaitGroup
	errs := make(chan error, len(h.handlers))
	for _, item := range h.handlers {
		if !item.Enabled(ctx, record.Level) {
			continue
		}
		wg.Add(1)
		go func(handler slog.Handler) {
			defer wg.Done()
			if err := handler.Handle(ctx, record.Clone()); err != nil {
				errs <- err
			}
		}(item)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		fmt.Fprintf(os.Stderr, "logging sink write degraded: %v\n", err)
	}
	return nil
}
func (h fanoutHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	result := make([]slog.Handler, len(h.handlers))
	for i, item := range h.handlers {
		result[i] = item.WithAttrs(attrs)
	}
	return fanoutHandler{handlers: result}
}
func (h fanoutHandler) WithGroup(name string) slog.Handler {
	result := make([]slog.Handler, len(h.handlers))
	for i, item := range h.handlers {
		result[i] = item.WithGroup(name)
	}
	return fanoutHandler{handlers: result}
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
