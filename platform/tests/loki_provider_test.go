package tests

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/yzxingtu2026/ainp/platform/framework/config"
	"github.com/yzxingtu2026/ainp/platform/framework/logcenter"
	"github.com/yzxingtu2026/ainp/platform/framework/logcenter/loki"
)

func TestLokiProviderBuildsRestrictedQueryAndMapsEntries(t *testing.T) {
	t.Setenv("TEST_LOKI_USER", "reader")
	t.Setenv("TEST_LOKI_PASS", "secret")
	t.Setenv("TEST_LOKI_ORG", "ops")
	timestamp := time.Now().UTC().Truncate(time.Nanosecond)
	var capturedQuery string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/ready" {
			w.WriteHeader(http.StatusOK)
			return
		}
		if user, password, ok := r.BasicAuth(); !ok || user != "reader" || password != "secret" {
			t.Errorf("missing Loki basic auth")
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if r.Header.Get("X-Scope-OrgID") != "ops" {
			t.Errorf("missing Loki tenant header")
		}
		capturedQuery = r.URL.Query().Get("query")
		line := `{"level":"error","message":"boom","request_id":"req-1","action_id":"demo.run","password":"must-redact"}`
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "success", "data": map[string]any{"resultType": "streams", "result": []any{map[string]any{"stream": map[string]string{"service": "api", "component": "action"}, "values": [][]string{{strconv.FormatInt(timestamp.UnixNano(), 10), line}}}}}})
	}))
	defer server.Close()

	cfg := lokiTestConfig(server.URL)
	cfg.UsernameEnv, cfg.PasswordEnv, cfg.TenantHeaderEnv = "TEST_LOKI_USER", "TEST_LOKI_PASS", "TEST_LOKI_ORG"
	application, _, err := loki.NewProviders(cfg)
	if err != nil {
		t.Fatal(err)
	}
	query := logcenter.Query{StartAt: timestamp.Add(-time.Minute), EndAt: timestamp.Add(time.Minute), TenantID: `tenant"} |= "escape`, Level: "error", RequestID: "req-1", Page: 1, PageSize: 20}

	page, err := application.Query(context.Background(), query)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 1 || page.Items[0].TenantID != query.TenantID || page.Items[0].Message != "boom" || page.Items[0].Service != "api" {
		t.Fatalf("unexpected Loki mapping: %#v", page)
	}
	if !strings.HasPrefix(capturedQuery, `{process="application",tenant_id="tenant\"} |= \"escape"} | json`) || !strings.Contains(capturedQuery, `| level="error"`) || !strings.Contains(capturedQuery, `| request_id="req-1"`) {
		t.Fatalf("unsafe or incomplete LogQL: %s", capturedQuery)
	}
	if strings.Contains(capturedQuery, "must-redact") {
		t.Fatalf("log content leaked into query: %s", capturedQuery)
	}
	detail, err := application.Get(context.Background(), logcenter.Ref{ID: page.Items[0].ID, TenantID: query.TenantID})
	if err != nil || detail.ID != page.Items[0].ID {
		t.Fatalf("Loki detail lookup failed: %#v %v", detail, err)
	}
	if err := application.Health(context.Background()); err != nil {
		t.Fatalf("health failed: %v", err)
	}
}

func TestLokiProviderMapsTimeoutAndHTTPFailures(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	cfg := lokiTestConfig(server.URL)
	cfg.Timeout = "10ms"
	application, _, err := loki.NewProviders(cfg)
	if err != nil {
		t.Fatal(err)
	}
	_, err = application.Query(context.Background(), logcenter.Query{StartAt: time.Now().Add(-time.Minute), EndAt: time.Now(), TenantID: "t1", Page: 1, PageSize: 20})
	if !errors.Is(err, logcenter.ErrTimeout) {
		t.Fatalf("timeout mapping=%v", err)
	}

	forbidden := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusForbidden) }))
	defer forbidden.Close()
	cfg = lokiTestConfig(forbidden.URL)
	application, _, err = loki.NewProviders(cfg)
	if err != nil {
		t.Fatal(err)
	}
	_, err = application.Query(context.Background(), logcenter.Query{StartAt: time.Now().Add(-time.Minute), EndAt: time.Now(), TenantID: "t1", Page: 1, PageSize: 20})
	if !errors.Is(err, logcenter.ErrForbidden) {
		t.Fatalf("forbidden mapping=%v", err)
	}
}

func TestLokiProviderRejectsInvalidEndpointAndLabels(t *testing.T) {
	cfg := lokiTestConfig("file:///tmp/loki")
	if _, _, err := loki.NewProviders(cfg); err == nil {
		t.Fatal("non-http endpoint must fail")
	}
	cfg = lokiTestConfig("http://localhost:3100")
	cfg.TenantLabel = `tenant}bad`
	if _, _, err := loki.NewProviders(cfg); err == nil {
		t.Fatal("invalid label must fail")
	}
}

func lokiTestConfig(endpoint string) config.LokiConfig {
	return config.LokiConfig{Enabled: true, Endpoint: endpoint, Timeout: "1s", TenantLabel: "tenant_id", SourceLabel: "process", ApplicationValue: "application", WorkerValue: "worker", ServiceLabel: "service", ComponentLabel: "component"}
}
