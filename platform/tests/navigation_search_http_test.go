package tests

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/yzxingtu2026/ainp/platform/framework/errs"
	"github.com/yzxingtu2026/ainp/platform/framework/httpserver"
	"github.com/yzxingtu2026/ainp/platform/framework/menu"
)

func TestNavigationSearchAPIUsesAuthenticatedPermissions(t *testing.T) {
	registry := menu.NewRegistry()
	for _, item := range []menu.Menu{
		{ID: "system.root", Title: "系统", Type: menu.TypeDirectory},
		{ID: "system.accounts", ParentID: "system.root", Title: "用户管理", Type: menu.TypeMenu, Path: "/system/accounts", Permission: "user.list"},
		{ID: "system.roles", ParentID: "system.root", Title: "角色管理", Type: menu.TypeMenu, Path: "/system/roles", Permission: "role.list"},
	} {
		if err := registry.Register(item); err != nil {
			t.Fatal(err)
		}
	}
	installed := true
	router := httpserver.New(httpserver.Options{Installed: &installed, DevTokenEnabled: true, Menus: registry})

	request := httptest.NewRequest(http.MethodGet, "/api/v1/navigation/search?query=管理", nil)
	request.Header.Set("Authorization", "Bearer dev:tenant-1:user-1:user.list")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", response.Code, response.Body.String())
	}
	var envelope struct {
		Code int `json:"code"`
		Data struct {
			Items []menu.SearchResult `json:"items"`
		} `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.Code != 0 || len(envelope.Data.Items) != 1 || envelope.Data.Items[0].MenuID != "system.accounts" {
		t.Fatalf("unexpected response: %#v", envelope)
	}

	invalid := httptest.NewRequest(http.MethodGet, "/api/v1/navigation/search?query=x&limit=21", nil)
	invalid.Header.Set("Authorization", "Bearer dev:tenant-1:user-1:user.list")
	invalidResponse := httptest.NewRecorder()
	router.ServeHTTP(invalidResponse, invalid)
	var invalidEnvelope struct {
		Code int `json:"code"`
	}
	if err := json.Unmarshal(invalidResponse.Body.Bytes(), &invalidEnvelope); err != nil {
		t.Fatal(err)
	}
	if invalidEnvelope.Code != errs.CodeValidation {
		t.Fatalf("code = %d, want validation", invalidEnvelope.Code)
	}
}
