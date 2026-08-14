package tests

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/yzxingtu2026/ainp/platform/framework/action"
	"github.com/yzxingtu2026/ainp/platform/framework/aitool"
)

type atomicBatchTestItem struct {
	Code string `json:"code" validate:"required,min=1,max=64"`
}

type atomicBatchTestRequest struct {
	Items []atomicBatchTestItem `json:"items" validate:"required,min=1,max=100,dive"`
}

type atomicBatchTestResponse struct {
	ID string `json:"id"`
}

type atomicBatchTestResult struct {
	CreatedCount int                       `json:"created_count"`
	Items        []atomicBatchTestResponse `json:"items"`
}

func TestValidateAtomicAIBatchContract(t *testing.T) {
	metadata := validAtomicBatchMetadata()
	if err := action.ValidateAIContract("orders.batch_create", metadata); err != nil {
		t.Fatalf("valid atomic contract rejected: %v", err)
	}
	registry := action.NewRegistry(nil, nil)
	if err := registry.Register(action.Action{ID: "orders.batch_create", Handler: func(context.Context, json.RawMessage) (any, error) { return nil, nil }, Meta: metadata}); err != nil {
		t.Fatalf("registry rejected valid atomic contract: %v", err)
	}
	definition, err := (&aitool.ActionProvider{Actions: registry}).Get(context.Background(), "action:orders.batch_create")
	if err != nil || definition.Operation != action.AIOperationCreate || definition.Batch == nil || definition.Batch.ItemsPath != "items" {
		t.Fatalf("AI create contract projection failed: %#v %v", definition, err)
	}
	toolRegistry := aitool.NewRegistry()
	if err = toolRegistry.RegisterProvider(&aitool.ActionProvider{Actions: registry}, &aitool.ActionExecutor{Actions: registry}); err != nil {
		t.Fatal(err)
	}
	if err = toolRegistry.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	projected, ok := toolRegistry.Get("action:orders.batch_create")
	if !ok || projected.SchemaHash == "" {
		t.Fatalf("projected atomic contract has no schema hash: %#v", projected)
	}
	legacyRegistry := action.NewRegistry(nil, nil)
	legacy := validAtomicBatchMetadata()
	legacy.AI.Operation = ""
	legacy.AI.Batch = nil
	if err = legacyRegistry.Register(action.Action{ID: "orders.batch_create", Handler: func(context.Context, json.RawMessage) (any, error) { return nil, nil }, Meta: legacy}); err != nil {
		t.Fatal(err)
	}
	legacyTools := aitool.NewRegistry()
	if err = legacyTools.RegisterProvider(&aitool.ActionProvider{Actions: legacyRegistry}, &aitool.ActionExecutor{Actions: legacyRegistry}); err != nil {
		t.Fatal(err)
	}
	if err = legacyTools.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	legacyDefinition, _ := legacyTools.Get("action:orders.batch_create")
	if legacyDefinition.SchemaHash == projected.SchemaHash {
		t.Fatal("operation and batch metadata must participate in the tool schema hash")
	}
}

func TestValidateAtomicAIBatchContractFailsClosed(t *testing.T) {
	validInput := action.InputSchemaFor[atomicBatchTestRequest]()
	tests := []struct {
		name   string
		mutate func(*action.Metadata)
		want   string
	}{
		{name: "missing batch", mutate: func(m *action.Metadata) { m.AI.Batch = nil }, want: "requires a batch contract"},
		{name: "unknown operation", mutate: func(m *action.Metadata) { m.AI.Operation = "update" }, want: "unsupported AI operation"},
		{name: "unknown mode", mutate: func(m *action.Metadata) { m.AI.Batch.Mode = "best_effort" }, want: "unsupported AI batch mode"},
		{name: "invalid path", mutate: func(m *action.Metadata) { m.AI.Batch.ItemsPath = "Items" }, want: "items_path is invalid"},
		{name: "missing path", mutate: func(m *action.Metadata) { m.AI.Batch.ItemsPath = "entries" }, want: "does not exist"},
		{name: "not required", mutate: func(m *action.Metadata) {
			m.AI.InputSchema = replaceSchema(t, validInput, func(s map[string]any) { s["required"] = []any{} })
		}, want: "must be required"},
		{name: "bare array", mutate: func(m *action.Metadata) {
			m.AI.InputSchema = json.RawMessage(`{"type":"array","items":{"type":"object","additionalProperties":false},"x-ainp-input-contract":"explicit-v1"}`)
		}, want: "must be a closed object"},
		{name: "open root", mutate: func(m *action.Metadata) {
			m.AI.InputSchema = replaceSchema(t, validInput, func(s map[string]any) { s["additionalProperties"] = true })
		}, want: "must be a closed object"},
		{name: "missing min", mutate: func(m *action.Metadata) {
			m.AI.InputSchema = replaceItemsSchema(t, validInput, func(s map[string]any) { delete(s, "minItems") })
		}, want: "minItems"},
		{name: "empty allowed", mutate: func(m *action.Metadata) {
			m.AI.InputSchema = replaceItemsSchema(t, validInput, func(s map[string]any) { s["minItems"] = float64(0) })
		}, want: "minItems"},
		{name: "missing max", mutate: func(m *action.Metadata) {
			m.AI.InputSchema = replaceItemsSchema(t, validInput, func(s map[string]any) { delete(s, "maxItems") })
		}, want: "finite maxItems"},
		{name: "over platform max", mutate: func(m *action.Metadata) {
			m.AI.InputSchema = replaceItemsSchema(t, validInput, func(s map[string]any) { s["maxItems"] = float64(action.AIMaxAtomicBatchItems + 1) })
		}, want: "finite maxItems"},
		{name: "open item", mutate: func(m *action.Metadata) {
			m.AI.InputSchema = replaceItemsSchema(t, validInput, func(s map[string]any) { s["items"].(map[string]any)["additionalProperties"] = true })
		}, want: "closed object"},
		{name: "no confirmation", mutate: func(m *action.Metadata) { m.AI.RequiresConfirmation = false; m.AI.Risk = "low" }, want: "must require confirmation"},
		{name: "wrong side effect", mutate: func(m *action.Metadata) { m.AI.SideEffect = "external" }, want: "side_effect=write"},
		{name: "optional idempotency", mutate: func(m *action.Metadata) { m.Idempotency.Mode = "optional" }, want: "mode=required"},
		{name: "missing output", mutate: func(m *action.Metadata) { m.AI.OutputSchema = nil }, want: "output schema must be a closed object"},
		{name: "unstable output", mutate: func(m *action.Metadata) { m.AI.OutputSchema = action.SchemaFor[atomicBatchTestResponse]() }, want: "created_count"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			metadata := validAtomicBatchMetadata()
			test.mutate(&metadata)
			err := action.ValidateAIContract("orders.batch_create", metadata)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want substring %q", err, test.want)
			}
		})
	}
}

func TestValidateControlledAIBatchModes(t *testing.T) {
	for _, mode := range []action.AIBatchMode{action.AIBatchPreviewCommit, action.AIBatchAsyncJob} {
		metadata := validAtomicBatchMetadata()
		metadata.AI.Batch = &action.AIBatchMeta{Mode: mode}
		if err := action.ValidateAIContract("orders.create", metadata); err != nil {
			t.Fatalf("mode %q rejected: %v", mode, err)
		}
	}
	for _, reason := range []action.AIBatchReason{action.AIBatchReasonSingletonResource, action.AIBatchReasonDomainSingleItem} {
		metadata := validAtomicBatchMetadata()
		metadata.AI.Batch = &action.AIBatchMeta{Mode: action.AIBatchSingleOnly, Reason: reason}
		if err := action.ValidateAIContract("orders.create", metadata); err != nil {
			t.Fatalf("single_only reason %q rejected: %v", reason, err)
		}
	}
	metadata := validAtomicBatchMetadata()
	metadata.AI.Batch = &action.AIBatchMeta{Mode: action.AIBatchSingleOnly, Reason: "temporary"}
	if err := action.ValidateAIContract("orders.create", metadata); err == nil || !strings.Contains(err.Error(), "controlled reason") {
		t.Fatalf("arbitrary single_only reason must fail: %v", err)
	}
}

func TestAIContractInventoryReportsLegacyWithoutNameInference(t *testing.T) {
	registry := action.NewRegistry(nil, nil)
	handler := func(context.Context, json.RawMessage) (any, error) { return nil, nil }
	legacy := validAtomicBatchMetadata()
	legacy.AI.Operation = ""
	legacy.AI.Batch = nil
	legacy.AI.SideEffect = "read"
	legacy.AI.Risk = "low"
	legacy.AI.RequiresConfirmation = false
	legacy.Idempotency = nil
	if err := registry.Register(action.Action{ID: "orders.legacy_create_name_only", Handler: handler, Meta: legacy}); err != nil {
		t.Fatal(err)
	}
	if err := registry.Register(action.Action{ID: "orders.batch_create", Handler: handler, Meta: validAtomicBatchMetadata()}); err != nil {
		t.Fatal(err)
	}
	inventory := registry.AIContractInventory()
	if len(inventory) != 2 || inventory[0].ActionID != "orders.batch_create" || inventory[0].Legacy || inventory[0].BatchMode != action.AIBatchAtomic || inventory[1].ActionID != "orders.legacy_create_name_only" || !inventory[1].Legacy {
		t.Fatalf("unexpected AI contract inventory: %#v", inventory)
	}
}

func TestAtomicBatchToolSchemaRejectsInvalidCollections(t *testing.T) {
	schema := action.ToolInputSchema(action.InputSchemaFor[atomicBatchTestRequest]())
	for _, arguments := range []json.RawMessage{
		json.RawMessage(`{"items":[{"code":"B1"}]}`),
		json.RawMessage(`{"items":[{"code":"B1"},{"code":"B2"}]}`),
	} {
		if err := aitool.ValidateArguments(schema, arguments); err != nil {
			t.Fatalf("valid arguments rejected: %s: %v", arguments, err)
		}
	}
	overLimit := make([]atomicBatchTestItem, 101)
	for index := range overLimit {
		overLimit[index].Code = "B"
	}
	overLimitJSON, err := json.Marshal(atomicBatchTestRequest{Items: overLimit})
	if err != nil {
		t.Fatal(err)
	}
	for _, arguments := range []json.RawMessage{
		json.RawMessage(`{"items":[]}`),
		overLimitJSON,
		json.RawMessage(`{"items":[{"code":"B1","tenant_id":"other"}]}`),
	} {
		if err := aitool.ValidateArguments(schema, arguments); err == nil {
			t.Fatalf("invalid arguments accepted: %s", arguments)
		}
	}
}

func TestSummarizeBatchForConfirmationUsesStableBudget(t *testing.T) {
	items := make([]string, 12)
	for index := range items {
		items[index] = fmt.Sprintf("item-%d", index+1)
	}
	summary := aitool.SummarizeBatchForConfirmation(items, 50)
	if summary.Total != 12 || summary.Omitted != 2 || !reflect.DeepEqual(summary.Items, items[:aitool.ConfirmationBatchItemLimit]) {
		t.Fatalf("unexpected batch summary: %#v", summary)
	}
	summary.Items[0] = "changed"
	if items[0] == "changed" {
		t.Fatal("batch summary must not alias presenter input")
	}
}

func validAtomicBatchMetadata() action.Metadata {
	return action.Metadata{
		Capability: "orders",
		Name:       "Create orders",
		AI: &action.AIMeta{
			ExposeTool: true, Operation: action.AIOperationCreate, Batch: &action.AIBatchMeta{Mode: action.AIBatchAtomic, ItemsPath: "items"},
			ToolTitle: "Create orders", Summary: "Create orders", HumanDescription: "Create orders", ResultSummary: "Orders created",
			Risk: "high", SideEffect: "write", RequiresConfirmation: true,
			InputSchema: action.InputSchemaFor[atomicBatchTestRequest](), OutputSchema: action.SchemaFor[atomicBatchTestResult](),
		},
		Idempotency: &action.IdempotencyMeta{Mode: "required"},
	}
}

func replaceItemsSchema(t *testing.T, schema json.RawMessage, mutate func(map[string]any)) json.RawMessage {
	t.Helper()
	return replaceSchema(t, schema, func(root map[string]any) {
		properties := root["properties"].(map[string]any)
		mutate(properties["items"].(map[string]any))
	})
}

func replaceSchema(t *testing.T, schema json.RawMessage, mutate func(map[string]any)) json.RawMessage {
	t.Helper()
	var value map[string]any
	if err := json.Unmarshal(schema, &value); err != nil {
		t.Fatal(err)
	}
	mutate(value)
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return encoded
}
