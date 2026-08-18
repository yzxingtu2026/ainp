package tests

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/yzxingtu2026/ainp/platform/framework/config"
	"github.com/yzxingtu2026/ainp/platform/framework/database"
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

func TestSetupTestRedisChecksQueueCompatibility(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	server := &redisProbeServer{listener: listener}
	go server.Serve()

	cfg := &config.Config{Redis: config.RedisConfig{DialTimeout: "1s", ReadTimeout: "1s", WriteTimeout: "1s"}}
	svc := system.NewSetupService(cfg, "", &setupstate.State{}, nil)
	result, err := svc.TestRedis(context.Background(), json.RawMessage(`{"addr":"`+listener.Addr().String()+`","db":0}`))
	if err != nil {
		t.Fatalf("test redis: %v", err)
	}
	body, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		OK              bool `json:"ok"`
		Reachable       bool `json:"reachable"`
		QueueCompatible bool `json:"queue_compatible"`
		Capabilities    struct {
			Ping      bool `json:"ping"`
			Lua       bool `json:"lua"`
			String    bool `json:"string"`
			Hash      bool `json:"hash"`
			List      bool `json:"list"`
			Set       bool `json:"set"`
			SortedSet bool `json:"sorted_set"`
			PubSub    bool `json:"pub_sub"`
			Cleanup   bool `json:"cleanup"`
		} `json:"capabilities"`
	}
	if err = json.Unmarshal(body, &got); err != nil {
		t.Fatal(err)
	}
	if !got.OK || !got.Reachable || !got.QueueCompatible || !got.Capabilities.Ping || !got.Capabilities.Lua || !got.Capabilities.String || !got.Capabilities.Hash || !got.Capabilities.List || !got.Capabilities.Set || !got.Capabilities.SortedSet || !got.Capabilities.PubSub || !got.Capabilities.Cleanup {
		t.Fatalf("unexpected result: %#v", got)
	}
	if server.businessKeysTouched {
		t.Fatal("compatibility probe touched a non-isolated key")
	}
}

func TestSetupTestRedisReturnsStableFailureCodes(t *testing.T) {
	for _, tc := range []struct {
		name, failureCode string
		mode              redisProbeMode
		reachable         bool
	}{
		{name: "authentication", mode: redisProbeAuthenticationFailure, failureCode: "redis_authentication_failed"},
		{name: "lua unavailable", mode: redisProbeLuaUnavailable, failureCode: "redis_lua_unavailable", reachable: true},
		{name: "queue command incompatible", mode: redisProbeQueueIncompatible, failureCode: "redis_queue_incompatible", reachable: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			listener, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			defer listener.Close()
			server := &redisProbeServer{listener: listener, mode: tc.mode}
			go server.Serve()
			cfg := &config.Config{Redis: config.RedisConfig{DialTimeout: "1s", ReadTimeout: "1s", WriteTimeout: "1s"}}
			svc := system.NewSetupService(cfg, "", &setupstate.State{}, nil)
			result, err := svc.TestRedis(context.Background(), json.RawMessage(`{"addr":"`+listener.Addr().String()+`","db":0}`))
			if err != nil {
				t.Fatal(err)
			}
			body, _ := json.Marshal(result)
			var got struct {
				OK, Reachable, QueueCompatible bool
				FailureCode                    string `json:"failure_code"`
			}
			if err = json.Unmarshal(body, &got); err != nil {
				t.Fatal(err)
			}
			if got.OK || got.QueueCompatible || got.Reachable != tc.reachable || got.FailureCode != tc.failureCode {
				t.Fatalf("unexpected failure result: %#v", got)
			}
		})
	}
}

func TestSetupTestRedisReturnsStableConnectionFailure(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := listener.Addr().String()
	_ = listener.Close()
	cfg := &config.Config{Redis: config.RedisConfig{DialTimeout: "10ms", ReadTimeout: "10ms", WriteTimeout: "10ms"}}
	svc := system.NewSetupService(cfg, "", &setupstate.State{}, nil)
	result, err := svc.TestRedis(context.Background(), json.RawMessage(`{"addr":"`+addr+`","db":0}`))
	if err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(result)
	var got struct {
		OK, Reachable, QueueCompatible bool
		FailureCode                    string `json:"failure_code"`
	}
	if err = json.Unmarshal(body, &got); err != nil || got.OK || got.Reachable || got.QueueCompatible || got.FailureCode != "redis_connection_failed" {
		t.Fatalf("unexpected connection failure: %#v %v", got, err)
	}
}

func TestSetupCheckEnvReportsReachableButQueueIncompatible(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	go (&redisProbeServer{listener: listener, mode: redisProbeLuaUnavailable}).Serve()
	cfg := &config.Config{Redis: config.RedisConfig{Addr: listener.Addr().String(), DialTimeout: "1s", ReadTimeout: "1s", WriteTimeout: "1s"}}
	svc := system.NewSetupService(cfg, "", &setupstate.State{}, nil)
	result, err := svc.CheckEnv(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(result)
	var got struct {
		RedisReachable       bool   `json:"redis_reachable"`
		RedisQueueCompatible bool   `json:"redis_queue_compatible"`
		RedisFailureCode     string `json:"redis_failure_code"`
	}
	if err = json.Unmarshal(body, &got); err != nil || !got.RedisReachable || got.RedisQueueCompatible || got.RedisFailureCode != "redis_lua_unavailable" {
		t.Fatalf("unexpected env result: %#v %v", got, err)
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

func TestSetupInitializeReturnsStableQueueCompatibilityError(t *testing.T) {
	for _, tc := range []struct {
		name, messageKey string
		mode             redisProbeMode
	}{
		{name: "authentication", mode: redisProbeAuthenticationFailure, messageKey: "setup.redis_authentication_failed"},
		{name: "lua", mode: redisProbeLuaUnavailable, messageKey: "setup.redis_lua_unavailable"},
		{name: "commands", mode: redisProbeQueueIncompatible, messageKey: "setup.redis_queue_incompatible"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			listener, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			defer listener.Close()
			go (&redisProbeServer{listener: listener, mode: tc.mode}).Serve()
			cfg := &config.Config{Redis: config.RedisConfig{DialTimeout: "1s", ReadTimeout: "1s", WriteTimeout: "1s"}}
			svc := system.NewSetupService(cfg, "", &setupstate.State{}, nil)
			_, err = svc.Initialize(context.Background(), json.RawMessage(`{
				"tenant_name":"Tenant","username":"admin","password":"Admin1234",
				"jobs_enabled":true,"redis_addr":"`+listener.Addr().String()+`","redis_db":0
			}`))
			appErr, ok := err.(*errs.AppError)
			if !ok || appErr.MessageKey != tc.messageKey || appErr.Cause != nil {
				t.Fatalf("expected stable queue validation error, got %#v", err)
			}
		})
	}
}

func TestSetupInitializeKeepsUninstalledStateWhenStateSaveFails(t *testing.T) {
	dsn := os.Getenv("AINP_TEST_DATABASE_DSN")
	if dsn == "" {
		t.Skip("AINP_TEST_DATABASE_DSN is not set")
	}
	root, err := database.Open(dsn)
	if err != nil {
		t.Fatal(err)
	}
	schema := "ainp_setup_state_" + uuid.NewString()[:8]
	if err := root.Exec(fmt.Sprintf(`CREATE SCHEMA %q`, schema)).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = root.Exec(fmt.Sprintf(`DROP SCHEMA %q CASCADE`, schema)).Error })
	testDSN := dsn + " search_path=" + schema

	blockedParent := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(blockedParent, []byte("blocked"), 0o600); err != nil {
		t.Fatal(err)
	}
	st := &setupstate.State{}
	installedCallback := false
	svc := system.NewSetupService(
		&config.Config{Database: config.DatabaseConfig{DSN: testDSN}},
		filepath.Join(blockedParent, "state.yaml"),
		st,
		func() { installedCallback = true },
	)

	_, err = svc.Initialize(context.Background(), json.RawMessage(`{
		"tenant_name":"State Failure Tenant",
		"username":"state-failure-admin",
		"password":"Admin1234",
		"jobs_enabled":false
	}`))
	if err == nil {
		t.Fatal("expected state save failure")
	}
	if installedCallback {
		t.Fatal("failed state save must not invoke the installed callback")
	}
	status, err := svc.Status(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(status)
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		Installed bool `json:"installed"`
	}
	if err := json.Unmarshal(encoded, &got); err != nil {
		t.Fatal(err)
	}
	if got.Installed || st.Installed {
		t.Fatal("failed state save must keep setup status uninstalled")
	}
}

type redisProbeMode int

const (
	redisProbeCompatible redisProbeMode = iota
	redisProbeAuthenticationFailure
	redisProbeLuaUnavailable
	redisProbeQueueIncompatible
)

type redisProbeServer struct {
	listener            net.Listener
	mode                redisProbeMode
	businessKeysTouched bool
}

func (s *redisProbeServer) Serve() {
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			return
		}
		go s.serveConn(conn)
	}
}

func (s *redisProbeServer) serveConn(conn net.Conn) {
	defer conn.Close()
	reader := bufio.NewReader(conn)
	for {
		parts, err := readRedisCommand(reader)
		if err != nil {
			return
		}
		command := strings.ToLower(parts[0])
		switch command {
		case "hello":
			_, _ = conn.Write([]byte("%1\r\n$6\r\nserver\r\n$5\r\nredis\r\n"))
		case "ping":
			if s.mode == redisProbeAuthenticationFailure {
				_, _ = conn.Write([]byte("-NOAUTH Authentication required\r\n"))
			} else {
				_, _ = conn.Write([]byte("+PONG\r\n"))
			}
		case "evalsha":
			if s.mode == redisProbeLuaUnavailable {
				_, _ = conn.Write([]byte("-ERR This instance has Lua scripting support disabled\r\n"))
			} else {
				_, _ = conn.Write([]byte("-NOSCRIPT No matching script. Please use EVAL.\r\n"))
			}
		case "eval":
			if s.mode == redisProbeLuaUnavailable {
				_, _ = conn.Write([]byte("-ERR This instance has Lua scripting support disabled\r\n"))
			} else if len(parts) > 2 && parts[2] == "0" {
				_, _ = conn.Write([]byte(":1\r\n"))
			} else if s.mode == redisProbeQueueIncompatible {
				_, _ = conn.Write([]byte("-ERR unknown command 'ZADD'\r\n"))
			} else {
				keyCount, _ := strconv.Atoi(parts[2])
				for _, key := range parts[3 : 3+keyCount] {
					if !strings.HasPrefix(key, "ainp:queue-compatibility-probe:") {
						s.businessKeysTouched = true
					}
				}
				_, _ = conn.Write([]byte(":1\r\n"))
			}
		case "exists":
			_, _ = conn.Write([]byte(":0\r\n"))
		case "del":
			_, _ = conn.Write([]byte(":0\r\n"))
		case "subscribe":
			_, _ = conn.Write([]byte("*3\r\n$9\r\nsubscribe\r\n$" + strconv.Itoa(len(parts[1])) + "\r\n" + parts[1] + "\r\n:1\r\n"))
		case "publish":
			_, _ = conn.Write([]byte(":1\r\n"))
		default:
			_, _ = conn.Write([]byte("+OK\r\n"))
		}
	}
}

func readRedisCommand(reader *bufio.Reader) ([]string, error) {
	line, err := reader.ReadString('\n')
	if err != nil {
		return nil, err
	}
	count, err := strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(line, "*")))
	if err != nil {
		return nil, err
	}
	parts := make([]string, 0, count)
	for range count {
		if _, err = reader.ReadString('\n'); err != nil {
			return nil, err
		}
		part, readErr := reader.ReadString('\n')
		if readErr != nil {
			return nil, readErr
		}
		parts = append(parts, strings.TrimSpace(part))
	}
	return parts, nil
}
