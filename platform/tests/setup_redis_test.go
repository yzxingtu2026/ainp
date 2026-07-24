package tests

import (
	"bufio"
	"context"
	"encoding/json"
	"net"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/yzxingtu2026/ainp/platform/framework/config"
	"github.com/yzxingtu2026/ainp/platform/framework/errs"
	setupstate "github.com/yzxingtu2026/ainp/platform/framework/state"
	"github.com/yzxingtu2026/ainp/platform/modules/system"
)

func TestSetupStateSaveLoadAndApplyConfig(t *testing.T) {
	enabled := true
	path := filepath.Join(t.TempDir(), "state.yaml")
	want := &setupstate.State{
		Installed: true, DSN: "setup-dsn",
		Redis: setupstate.Redis{Addr: "redis.internal:6379", DB: 4},
		Jobs:  setupstate.Jobs{Enabled: &enabled, EmbeddedWorker: true},
	}
	if err := setupstate.Save(path, want); err != nil {
		t.Fatalf("save: %v", err)
	}
	got, err := setupstate.Load(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	cfg := &config.Config{}
	got.ApplyConfig(cfg)
	if cfg.Database.DSN != want.DSN || cfg.Redis.Addr != want.Redis.Addr || cfg.Redis.DB != 4 {
		t.Fatalf("unexpected applied config: %#v", cfg)
	}
	if !cfg.Jobs.IsEnabled() || !cfg.Jobs.EmbeddedWorker {
		t.Fatalf("jobs config was not applied: %#v", cfg.Jobs)
	}
}

func TestSetupStateKeepsRedisEnvironmentOverride(t *testing.T) {
	t.Setenv("REDIS_ADDR", "env.redis:6379")
	cfg := &config.Config{Redis: config.RedisConfig{Addr: "env.redis:6379", DB: 1}}
	(&setupstate.State{Redis: setupstate.Redis{Addr: "state.redis:6379", DB: 2}}).ApplyConfig(cfg)
	if cfg.Redis.Addr != "env.redis:6379" || cfg.Redis.DB != 1 {
		t.Fatalf("state overrode environment config: %#v", cfg.Redis)
	}
}

func TestSetupTestRedis(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	go serveRedisPing(listener)

	cfg := &config.Config{Redis: config.RedisConfig{DialTimeout: "1s", ReadTimeout: "1s", WriteTimeout: "1s"}}
	svc := system.NewSetupService(cfg, "", &setupstate.State{}, nil)
	result, err := svc.TestRedis(context.Background(), json.RawMessage(`{"addr":"`+listener.Addr().String()+`","db":0}`))
	if err != nil {
		t.Fatalf("test redis: %v", err)
	}
	if ok := result.(map[string]any)["ok"]; ok != true {
		t.Fatalf("unexpected result: %#v", result)
	}
}

func TestSetupInitializeRejectsEnabledJobsWhenRedisUnavailable(t *testing.T) {
	cfg := &config.Config{Redis: config.RedisConfig{DialTimeout: "10ms", ReadTimeout: "10ms", WriteTimeout: "10ms"}}
	svc := system.NewSetupService(cfg, "", &setupstate.State{}, nil)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_, err := svc.Initialize(ctx, json.RawMessage(`{
		"tenant_name":"Tenant","username":"admin","password":"Admin1234",
		"jobs_enabled":true,"redis_addr":"127.0.0.1:1","redis_db":0
	}`))
	appErr, ok := err.(*errs.AppError)
	if !ok || appErr.MessageKey != "setup.redis_unreachable" {
		t.Fatalf("expected redis validation error, got %v", err)
	}
}

func serveRedisPing(listener net.Listener) {
	conn, err := listener.Accept()
	if err != nil {
		return
	}
	defer conn.Close()
	reader := bufio.NewReader(conn)
	for {
		line, readErr := reader.ReadString('\n')
		if readErr != nil {
			return
		}
		count, parseErr := strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(line, "*")))
		if parseErr != nil {
			return
		}
		parts := make([]string, 0, count)
		for range count {
			if _, readErr = reader.ReadString('\n'); readErr != nil {
				return
			}
			part, partErr := reader.ReadString('\n')
			if partErr != nil {
				return
			}
			parts = append(parts, strings.TrimSpace(part))
		}
		switch strings.ToLower(parts[0]) {
		case "hello":
			_, _ = conn.Write([]byte("%1\r\n$6\r\nserver\r\n$5\r\nredis\r\n"))
		case "ping":
			_, _ = conn.Write([]byte("+PONG\r\n"))
		default:
			_, _ = conn.Write([]byte("+OK\r\n"))
		}
	}
}
