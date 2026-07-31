package tests

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/yzxingtu2026/ainp/platform/framework/action"
	"github.com/yzxingtu2026/ainp/platform/framework/auth"
	"github.com/yzxingtu2026/ainp/platform/framework/errs"
	"github.com/yzxingtu2026/ainp/platform/framework/i18n"
	"github.com/yzxingtu2026/ainp/platform/framework/permission"
	"github.com/yzxingtu2026/ainp/platform/framework/validation"
)

type validationLifecycleRequest struct {
	Name string `json:"name" validate:"required,min=3"`
}

var validationLifecycleOrder []string

func (r *validationLifecycleRequest) Normalize() {
	r.Name = strings.TrimSpace(r.Name)
	validationLifecycleOrder = append(validationLifecycleOrder, "normalize")
}

func (r *validationLifecycleRequest) Validate(context.Context) error {
	validationLifecycleOrder = append(validationLifecycleOrder, "validate")
	return nil
}

func TestValidationUsesJSONPathsAndCollectsFields(t *testing.T) {
	type profile struct {
		Email string `json:"email" validate:"required,email"`
	}
	type request struct {
		Name    string   `json:"name" validate:"required,min=3"`
		Roles   []string `json:"role_ids" validate:"required,dive,uuid"`
		Profile profile  `json:"profile" validate:"required"`
	}
	err := validation.Struct(request{Roles: []string{"bad"}})
	appErr, ok := errs.AsAppError(err)
	if !ok || appErr.Code != errs.CodeValidation {
		t.Fatalf("unexpected error: %#v", err)
	}
	want := map[string]string{"name": "validation.required", "role_ids[0]": "validation.uuid", "profile.email": "validation.required"}
	for _, field := range appErr.Fields {
		if want[field.Field] != field.MessageKey {
			t.Fatalf("unexpected field error: %#v", field)
		}
		delete(want, field.Field)
	}
	if len(want) != 0 {
		t.Fatalf("missing field errors: %#v", want)
	}
}

func TestActionJSONLifecycleAndStrictDecode(t *testing.T) {
	validationLifecycleOrder = nil
	handler := action.JSON(func(ctx context.Context, req validationLifecycleRequest) (any, error) {
		validationLifecycleOrder = append(validationLifecycleOrder, "handler")
		return req.Name, nil
	})
	result, err := handler(context.Background(), json.RawMessage(`{"name":" alice "}`))
	if err != nil || result != "alice" || strings.Join(validationLifecycleOrder, ",") != "normalize,validate,handler" {
		t.Fatalf("unexpected typed handler result: result=%v err=%v order=%v", result, err, validationLifecycleOrder)
	}
	if _, err := handler(context.Background(), json.RawMessage(`{"name":"alice","extra":true}`)); errorCode(err) != errs.CodeValidation {
		t.Fatalf("unknown field must fail: %v", err)
	} else if appErr, _ := errs.AsAppError(err); appErr.MessageKey != "common.validation_failed" || len(appErr.Fields) != 1 || appErr.Fields[0].MessageKey != "validation.unknown_field" {
		t.Fatalf("unexpected unknown field protocol: %#v", appErr)
	}
	if _, err := handler(context.Background(), json.RawMessage(`{"name":1}`)); errorCode(err) != errs.CodeValidation {
		t.Fatalf("type mismatch must fail: %v", err)
	}
	if _, err := handler(context.Background(), json.RawMessage(`{"name":"alice"}{"name":"bob"}`)); errorCode(err) != errs.CodeValidation {
		t.Fatalf("multiple JSON values must fail: %v", err)
	}
}

func TestValidationCommonRulesAndTranslations(t *testing.T) {
	type request struct {
		Email    string `json:"email" validate:"email"`
		Code     string `json:"code" validate:"len=3"`
		Title    string `json:"title" validate:"min=3"`
		Items    []int  `json:"items" validate:"max=1"`
		Status   string `json:"status" validate:"oneof=enabled disabled"`
		Locale   string `json:"locale" validate:"locale"`
		Timezone string `json:"timezone" validate:"timezone"`
		URL      string `json:"url" validate:"url"`
	}
	err := validation.Struct(request{Email: "bad", Code: "xx", Title: "x", Items: []int{1, 2}, Status: "other", Locale: "fr", Timezone: "Mars/Olympus", URL: "://"})
	appErr, ok := errs.AsAppError(err)
	if !ok || len(appErr.Fields) != 8 {
		t.Fatalf("expected all common rule errors, got %#v", err)
	}
	want := map[string]string{
		"email": "validation.email", "code": "validation.length", "title": "validation.min", "items": "validation.max", "status": "validation.one_of",
		"locale": "validation.locale", "timezone": "validation.timezone", "url": "validation.url",
	}
	for _, field := range appErr.Fields {
		if want[field.Field] != field.MessageKey {
			t.Fatalf("unexpected common rule error: %#v", field)
		}
	}
	if err := i18n.LoadDir("../i18n"); err != nil {
		t.Fatal(err)
	}
	for _, key := range want {
		if i18n.T(i18n.LangZhCN, key) == key || i18n.T(i18n.LangEn, key) == key {
			t.Fatalf("missing translation for %s", key)
		}
	}
}

func TestActionJSONStopsBeforeHandlerAndPermissionRunsFirst(t *testing.T) {
	called := false
	typed := action.JSON(func(context.Context, validationLifecycleRequest) (any, error) {
		called = true
		return nil, nil
	})
	registry := action.NewRegistry(permission.NewChecker(), nil)
	if err := registry.Register(action.Action{ID: "validation.run", Handler: typed, Meta: action.Metadata{Capability: "test", AI: action.ExcludeFromAI(action.AIExcludePendingSafetyReview)}}); err != nil {
		t.Fatal(err)
	}
	withoutPermission := auth.WithContext(context.Background(), &auth.Context{Realm: auth.RealmTenant, Authenticated: true})
	if _, err := registry.Invoke(withoutPermission, "validation.run", json.RawMessage(`{}`)); errorCode(err) != errs.CodeForbidden || called {
		t.Fatalf("permission must reject before validation: err=%v called=%v", err, called)
	}
	withPermission := auth.WithContext(context.Background(), &auth.Context{Realm: auth.RealmTenant, Authenticated: true, Permissions: []string{"validation.run"}})
	if _, err := registry.Invoke(withPermission, "validation.run", json.RawMessage(`{}`)); errorCode(err) != errs.CodeValidation || called {
		t.Fatalf("validation must reject before handler: err=%v called=%v", err, called)
	}
}
