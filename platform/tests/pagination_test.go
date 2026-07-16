package tests

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/yzxingtu2026/ainp/platform/framework/action"
	"github.com/yzxingtu2026/ainp/platform/framework/errs"
	"github.com/yzxingtu2026/ainp/platform/framework/pagination"
)

type paginationTestRequest struct {
	pagination.Request
	Keyword string `json:"keyword"`
}

func TestPaginationRequestDefaultsAndLimits(t *testing.T) {
	handler := action.JSON(func(_ context.Context, request paginationTestRequest) (any, error) {
		return request, nil
	})
	result, err := handler(context.Background(), json.RawMessage(`{"keyword":"demo"}`))
	if err != nil {
		t.Fatal(err)
	}
	request := result.(paginationTestRequest)
	if request.Page != pagination.DefaultPage || request.PageSize != pagination.DefaultPageSize || request.Offset() != 0 {
		t.Fatalf("unexpected defaults: %#v", request)
	}
	if _, err := handler(context.Background(), json.RawMessage(`{"page":1,"page_size":101}`)); errorCode(err) != errs.CodeValidation {
		t.Fatalf("page_size over limit must fail: %v", err)
	}
	if _, err := handler(context.Background(), json.RawMessage(`{"page":-1,"page_size":20}`)); errorCode(err) != errs.CodeValidation {
		t.Fatalf("negative page must fail: %v", err)
	}
	if _, err := handler(context.Background(), json.RawMessage(`{"page":1,"page_size":20,"sort":"created_at desc"}`)); errorCode(err) != errs.CodeValidation {
		t.Fatalf("undeclared sort must be rejected: %v", err)
	}
}

func TestPaginationResultUsesStableJSONFields(t *testing.T) {
	result := pagination.NewResult([]string{"a"}, 1, pagination.Request{Page: 2, PageSize: 10})
	data, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != `{"items":["a"],"total":1,"page":2,"page_size":10}` {
		t.Fatalf("unexpected result JSON: %s", data)
	}
	empty, _ := json.Marshal(pagination.NewResult[string](nil, 0, pagination.Request{Page: 1, PageSize: 20}))
	if string(empty) != `{"items":[],"total":0,"page":1,"page_size":20}` {
		t.Fatalf("nil items must serialize as empty array: %s", empty)
	}
}
