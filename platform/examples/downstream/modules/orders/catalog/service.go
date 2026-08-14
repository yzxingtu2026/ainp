package catalog

import (
	"context"
	"encoding/json"

	"github.com/yzxingtu2026/ainp/platform/framework/job"
)

type Service struct{}

func (Service) List(context.Context, json.RawMessage) (any, error) {
	return []string{}, nil
}

func (Service) BatchCreate(_ context.Context, request orderBatchCreateRequest) (any, error) {
	items := make([]orderCreateResponse, len(request.Items))
	for index, item := range request.Items {
		items[index] = orderCreateResponse{Reference: item.Reference}
	}
	return orderBatchCreateResponse{CreatedCount: len(items), Items: items}, nil
}

func (Service) Sync(context.Context, job.Envelope, json.RawMessage, job.Reporter) error {
	return nil
}
