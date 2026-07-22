// Package pagination defines transport-neutral list request and result types.
package pagination

const (
	DefaultPage     = 1
	DefaultPageSize = 20
	MaxPageSize     = 100
)

type Request struct {
	Page     int `json:"page,omitempty" validate:"min=1"`
	PageSize int `json:"page_size,omitempty" validate:"min=1,max=100"`
}

func (r *Request) Normalize() {
	if r.Page == 0 {
		r.Page = DefaultPage
	}
	if r.PageSize == 0 {
		r.PageSize = DefaultPageSize
	}
}

func (r Request) Offset() int {
	return (r.Page - 1) * r.PageSize
}

type Result[T any] struct {
	Items    []T   `json:"items"`
	Total    int64 `json:"total"`
	Page     int   `json:"page"`
	PageSize int   `json:"page_size"`
}

func NewResult[T any](items []T, total int64, request Request) Result[T] {
	if items == nil {
		items = []T{}
	}
	return Result[T]{Items: items, Total: total, Page: request.Page, PageSize: request.PageSize}
}
