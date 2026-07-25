package tests

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/yzxingtu2026/ainp/platform/framework/action"
	"github.com/yzxingtu2026/ainp/platform/framework/auth"
	"github.com/yzxingtu2026/ainp/platform/framework/diagnostic"
	"github.com/yzxingtu2026/ainp/platform/framework/logging"
)

func TestDiagnosticErrorPreservesChainAndSanitizesAttributes(t *testing.T) {
	cause := errors.New("dial failed")
	err := diagnostic.New(diagnostic.UpstreamConnect, "ai.discovery", "provider unavailable", cause, map[string]any{
		"upstream_host": "example.com",
		"authorization": "secret",
		"upstream_path": strings.Repeat("x", diagnostic.MaxAttributeLength+10),
	})
	if !errors.Is(err, cause) {
		t.Fatal("diagnostic error must preserve cause")
	}
	meta := diagnostic.Describe(err, "fallback")
	if meta.Category != diagnostic.UpstreamConnect || meta.Stage != "ai.discovery" {
		t.Fatalf("unexpected metadata: %#v", meta)
	}
	if _, ok := meta.Attributes["authorization"]; ok {
		t.Fatal("non-whitelisted attribute leaked")
	}
	if got := meta.Attributes["upstream_path"].(string); len(got) > diagnostic.MaxAttributeLength+3 {
		t.Fatalf("attribute not truncated: %d", len(got))
	}
}

func TestLoggingFileSinkConcurrentWritesAndRestartAppend(t *testing.T) {
	path := filepath.Join(t.TempDir(), "worker.jsonl")
	writeBatch := func(start, count int) {
		closer := logging.InitConfig(logging.Config{Level: "info", Outputs: []string{"file"}, Service: "worker", InstanceID: "test-1", File: logging.FileConfig{Path: path, MaxSizeMB: 1, MaxBackups: 2, MaxAgeDays: 1, Compress: true}})
		var wg sync.WaitGroup
		for i := 0; i < count; i++ {
			wg.Add(1)
			go func(sequence int) {
				defer wg.Done()
				logging.Info(context.Background(), "concurrent event", "sequence", sequence)
			}(start + i)
		}
		wg.Wait()
		if err := closer.Close(); err != nil {
			t.Fatal(err)
		}
	}
	writeBatch(0, 100)
	writeBatch(100, 25)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 125 {
		t.Fatalf("got %d records, want 125", len(lines))
	}
	seen := make(map[float64]bool, len(lines))
	for _, line := range lines {
		var record map[string]any
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			t.Fatalf("corrupt JSONL: %v", err)
		}
		seen[record["sequence"].(float64)] = true
	}
	if len(seen) != 125 {
		t.Fatalf("got %d unique records, want 125", len(seen))
	}
}

func TestLoggingSeparatesStandaloneWorkerFile(t *testing.T) {
	if got := logging.WorkerFilePath("data/logs/api.jsonl"); got != filepath.Join("data", "logs", "worker.jsonl") {
		t.Fatalf("worker path = %q", got)
	}
	if got := logging.FilePathForService("data/logs/{service}-{instance_id}.jsonl", "worker", "host/1"); got != filepath.Join("data", "logs", "worker-host_1.jsonl") {
		t.Fatalf("templated path = %q", got)
	}
}

func TestActionInternalFailureWritesCorrelatedDiagnosticLog(t *testing.T) {
	path := filepath.Join(t.TempDir(), "api.jsonl")
	closer := logging.InitConfig(logging.Config{Level: "info", Outputs: []string{"file"}, Service: "api", InstanceID: "test-1", File: logging.FileConfig{Path: path, MaxSizeMB: 1}})
	registry := action.NewRegistry(nil, nil)
	if err := registry.Register(action.Action{ID: "test.fail", Meta: action.Metadata{Capability: "test", Access: action.AccessPublic}, Handler: func(context.Context, json.RawMessage) (any, error) {
		return nil, errors.New("database password=must-not-leak")
	}}); err != nil {
		t.Fatal(err)
	}
	ctx := auth.WithContext(context.Background(), &auth.Context{RequestID: "request-123", TraceID: "trace-123", Realm: auth.RealmTenant})
	if _, err := registry.Invoke(ctx, "test.fail", nil); err == nil {
		t.Fatal("expected handler failure")
	}
	if err := closer.Close(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var record map[string]any
	if err := json.Unmarshal(data, &record); err != nil {
		t.Fatal(err)
	}
	for key, want := range map[string]any{"request_id": "request-123", "trace_id": "trace-123", "action_id": "test.fail", "error_category": "internal", "error_stage": "action.handler", "safe_summary": "internal error", "message_key": "common.internal_error"} {
		if record[key] != want {
			t.Fatalf("%s=%#v, want %#v", key, record[key], want)
		}
	}
	if record["error_code"] != float64(50000) {
		t.Fatalf("unexpected error code: %#v", record["error_code"])
	}
	if strings.Contains(string(data), "must-not-leak") {
		t.Fatal("error cause leaked credential")
	}
}

func TestLoggingFileSinkWritesJSONLWithBaseFieldsAndRedaction(t *testing.T) {
	path := filepath.Join(t.TempDir(), "api.jsonl")
	closer := logging.InitConfig(logging.Config{Level: "info", Format: "text", Outputs: []string{"file"}, Service: "api", InstanceID: "test-1", File: logging.FileConfig{Path: path, MaxSizeMB: 1}})
	logging.Info(context.Background(), "test event", "password", "secret", "authorization", "Bearer abc", "detail", strings.Repeat("x", 1044))
	if err := closer.Close(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(strings.TrimSpace(string(data)), "\n") != 0 {
		t.Fatalf("expected one JSONL record: %q", data)
	}
	var record map[string]any
	if err := json.Unmarshal(data, &record); err != nil {
		t.Fatalf("invalid JSONL: %v", err)
	}
	if record["service"] != "api" || record["instance_id"] != "test-1" {
		t.Fatalf("missing base fields: %#v", record)
	}
	if record["password"] != "***" || record["authorization"] != "***" {
		t.Fatalf("secrets not redacted: %#v", record)
	}
	if len(record["detail"].(string)) > 1027 {
		t.Fatal("value not truncated")
	}
}
