package tests

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/yzxingtu2026/ainp/platform/framework/aitool"
	"github.com/yzxingtu2026/ainp/platform/framework/app"
	"github.com/yzxingtu2026/ainp/platform/framework/config"
	"github.com/yzxingtu2026/ainp/platform/modules/operations"
)

type operationsDeliveryFixture struct{}

func (operationsDeliveryFixture) List(context.Context, json.RawMessage) (any, error) {
	return map[string]any{}, nil
}
func (operationsDeliveryFixture) Get(context.Context, json.RawMessage) (any, error) {
	return map[string]any{}, nil
}
func (operationsDeliveryFixture) Retry(context.Context, json.RawMessage) (any, error) {
	return map[string]any{}, nil
}

func TestOperationsActionsExposeGovernedAIContracts(t *testing.T) {
	application := app.New(&config.Config{}, nil, nil, nil)
	if err := operations.Register(application, operationsDeliveryFixture{}); err != nil {
		t.Fatal(err)
	}
	readActions := []string{
		operations.ActionJobList, operations.ActionJobGet,
		operations.ActionOutboxList, operations.ActionOutboxSummary, operations.ActionOutboxListDeliveries,
		operations.ActionNotificationDeliveryList, operations.ActionNotificationDeliveryGet,
		operations.ActionAuditLogList, operations.ActionAuditLogGet,
	}
	mutationActions := []string{
		operations.ActionJobCancel, operations.ActionJobRetry,
		operations.ActionOutboxRetry, operations.ActionNotificationDeliveryRetry,
	}
	for _, actionID := range readActions {
		assertOperationsAIContract(t, application, actionID, false)
	}
	for _, actionID := range mutationActions {
		assertOperationsAIContract(t, application, actionID, true)
		registered, _ := application.Actions.Get(actionID)
		var schema struct {
			Properties map[string]json.RawMessage `json:"properties"`
		}
		if err := json.Unmarshal(registered.Meta.AI.InputSchema, &schema); err != nil {
			t.Fatal(err)
		}
		for _, forbidden := range []string{"payload", "payload_json", "metadata", "job_type"} {
			if _, leaked := schema.Properties[forbidden]; leaked {
				t.Fatalf("operations mutation %s must not accept %s", actionID, forbidden)
			}
		}
	}
	for actionID, required := range map[string]string{
		operations.ActionJobCancel:                 "id",
		operations.ActionJobRetry:                  "id",
		operations.ActionOutboxRetry:               "event_id",
		operations.ActionNotificationDeliveryRetry: "id",
	} {
		registered, _ := application.Actions.Get(actionID)
		assertSchemaRequired(t, registered.Meta.AI.InputSchema, required)
	}
	projected, err := (&aitool.ActionProvider{Actions: application.Actions}).List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	projectedActions := make(map[string]bool, len(projected))
	for _, definition := range projected {
		projectedActions[definition.RuntimeRef] = true
	}
	for _, actionID := range append(readActions, mutationActions...) {
		if !projectedActions[actionID] {
			t.Fatalf("operations Action was not projected from Action Registry: %s", actionID)
		}
	}
}
