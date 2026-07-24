package tests

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/yzxingtu2026/ainp/platform/framework/buildinfo"
	"github.com/yzxingtu2026/ainp/platform/framework/httpserver"
	"github.com/yzxingtu2026/ainp/platform/framework/response"
)

func TestHealthzIncludesBuildInfo(t *testing.T) {
	info := buildinfo.Current()
	if info.Version != "dev" || info.Commit != "unknown" || info.BuildTime != "unknown" {
		t.Fatalf("unexpected development build info: %#v", info)
	}

	installed := true
	router := httpserver.New(httpserver.Options{Installed: &installed})
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/healthz", nil))

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	var envelope response.Envelope
	if err := json.Unmarshal(recorder.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	data, ok := envelope.Data.(map[string]any)
	if !ok {
		t.Fatalf("unexpected health data: %#v", envelope.Data)
	}
	if data["status"] != "ok" || data["version"] != "dev" || data["commit"] != "unknown" || data["build_time"] != "unknown" {
		t.Fatalf("unexpected health data: %#v", data)
	}
}
