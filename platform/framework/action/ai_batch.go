package action

import (
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"sort"
	"strings"
)

type AIOperation string
type AIBatchMode string
type AIBatchReason string

const (
	AIOperationCreate AIOperation = "create"

	AIBatchAtomic        AIBatchMode = "atomic"
	AIBatchPreviewCommit AIBatchMode = "preview_commit"
	AIBatchAsyncJob      AIBatchMode = "async_job"
	AIBatchSingleOnly    AIBatchMode = "single_only"

	AIBatchReasonSingletonResource AIBatchReason = "singleton-resource"
	AIBatchReasonDomainSingleItem  AIBatchReason = "domain-single-item"

	// AIMaxAtomicBatchItems is the platform safety ceiling. Domains may declare
	// a smaller finite maxItems limit in their request DTO.
	AIMaxAtomicBatchItems = 100
)

type AIBatchMeta struct {
	Mode      AIBatchMode   `json:"mode"`
	ItemsPath string        `json:"items_path,omitempty"`
	Reason    AIBatchReason `json:"reason,omitempty"`
}

type AIContractInventoryItem struct {
	ActionID  string      `json:"action_id"`
	Operation AIOperation `json:"operation,omitempty"`
	BatchMode AIBatchMode `json:"batch_mode,omitempty"`
	Legacy    bool        `json:"legacy"`
}

// AIContractInventory derives a reviewable migration report from the live
// Registry without inferring operation semantics from Action identifiers.
func (r *Registry) AIContractInventory() []AIContractInventoryItem {
	items := make([]AIContractInventoryItem, 0)
	for actionID, registered := range r.items {
		if registered.Meta.AI == nil || !registered.Meta.AI.ExposeTool {
			continue
		}
		item := AIContractInventoryItem{ActionID: actionID, Operation: registered.Meta.AI.Operation, Legacy: registered.Meta.AI.Operation == ""}
		if registered.Meta.AI.Batch != nil {
			item.BatchMode = registered.Meta.AI.Batch.Mode
		}
		items = append(items, item)
	}
	sort.Slice(items, func(i, j int) bool { return items[i].ActionID < items[j].ActionID })
	return items
}

var aiBatchItemsPathPattern = regexp.MustCompile(`^[a-z][a-z0-9_]*(\.[a-z][a-z0-9_]*)*$`)

func validateAICreateContract(actionID string, metadata Metadata) error {
	meta := metadata.AI
	if meta == nil {
		return nil
	}
	if !meta.ExposeTool {
		if meta.Operation != "" || meta.Batch != nil {
			return fmt.Errorf("action %s: excluded AI action must not declare an operation or batch contract", actionID)
		}
		return nil
	}
	if meta.Operation == "" {
		if meta.Batch != nil {
			return fmt.Errorf("action %s: AI batch contract requires an operation", actionID)
		}
		return nil
	}
	if meta.Operation != AIOperationCreate {
		return fmt.Errorf("action %s: unsupported AI operation %q", actionID, meta.Operation)
	}
	if meta.Batch == nil {
		return fmt.Errorf("action %s: AI create operation requires a batch contract", actionID)
	}

	switch meta.Batch.Mode {
	case AIBatchAtomic:
		if meta.Batch.Reason != "" {
			return fmt.Errorf("action %s: atomic AI batch contract must not declare a reason", actionID)
		}
		return validateAtomicAIBatch(actionID, metadata)
	case AIBatchPreviewCommit, AIBatchAsyncJob:
		if strings.TrimSpace(meta.Batch.ItemsPath) != "" || meta.Batch.Reason != "" {
			return fmt.Errorf("action %s: AI batch mode %q must not declare items_path or reason", actionID, meta.Batch.Mode)
		}
		return nil
	case AIBatchSingleOnly:
		if strings.TrimSpace(meta.Batch.ItemsPath) != "" {
			return fmt.Errorf("action %s: single_only AI batch contract must not declare items_path", actionID)
		}
		if meta.Batch.Reason != AIBatchReasonSingletonResource && meta.Batch.Reason != AIBatchReasonDomainSingleItem {
			return fmt.Errorf("action %s: single_only AI batch contract requires a controlled reason", actionID)
		}
		return nil
	default:
		return fmt.Errorf("action %s: unsupported AI batch mode %q", actionID, meta.Batch.Mode)
	}
}

func validateAtomicAIBatch(actionID string, metadata Metadata) error {
	meta := metadata.AI
	if meta.SideEffect != "write" {
		return fmt.Errorf("action %s: atomic AI create action must declare side_effect=write", actionID)
	}
	if !meta.RequiresConfirmation {
		return fmt.Errorf("action %s: atomic AI create action must require confirmation", actionID)
	}
	if metadata.IdempotencyMode() != "required" {
		return fmt.Errorf("action %s: atomic AI create action requires idempotency mode=required", actionID)
	}
	path := strings.TrimSpace(meta.Batch.ItemsPath)
	if !aiBatchItemsPathPattern.MatchString(path) {
		return fmt.Errorf("action %s: atomic AI batch items_path is invalid", actionID)
	}

	input, err := decodeSchemaObject(meta.InputSchema)
	if err != nil || schemaType(input) != "object" || input["additionalProperties"] != false {
		return fmt.Errorf("action %s: atomic AI batch input schema must be a closed object", actionID)
	}
	items, err := schemaAtRequiredPath(input, strings.Split(path, "."))
	if err != nil {
		return fmt.Errorf("action %s: atomic AI batch items_path %q: %w", actionID, path, err)
	}
	if schemaType(items) != "array" {
		return fmt.Errorf("action %s: atomic AI batch items_path %q must be an array", actionID, path)
	}
	minItems, ok := schemaInteger(items["minItems"])
	if !ok || minItems < 1 {
		return fmt.Errorf("action %s: atomic AI batch items requires minItems >= 1", actionID)
	}
	maxItems, ok := schemaInteger(items["maxItems"])
	if !ok || maxItems < minItems || maxItems > AIMaxAtomicBatchItems {
		return fmt.Errorf("action %s: atomic AI batch items requires finite maxItems between minItems and %d", actionID, AIMaxAtomicBatchItems)
	}
	itemSchema, ok := items["items"].(map[string]any)
	if !ok || schemaType(itemSchema) != "object" || itemSchema["additionalProperties"] != false {
		return fmt.Errorf("action %s: atomic AI batch items must contain closed object schemas", actionID)
	}

	output, err := decodeSchemaObject(meta.OutputSchema)
	if err != nil || schemaType(output) != "object" || output["additionalProperties"] != false {
		return fmt.Errorf("action %s: atomic AI batch output schema must be a closed object", actionID)
	}
	properties, _ := output["properties"].(map[string]any)
	createdCount, _ := properties["created_count"].(map[string]any)
	resultItems, _ := properties["items"].(map[string]any)
	resultItem, _ := resultItems["items"].(map[string]any)
	if schemaType(createdCount) != "integer" || schemaType(resultItems) != "array" || schemaType(resultItem) != "object" {
		return fmt.Errorf("action %s: atomic AI batch output schema requires integer created_count and object-array items", actionID)
	}
	return nil
}

func decodeSchemaObject(schema json.RawMessage) (map[string]any, error) {
	if len(schema) == 0 {
		return nil, fmt.Errorf("schema is required")
	}
	var value map[string]any
	if err := json.Unmarshal(schema, &value); err != nil {
		return nil, err
	}
	return value, nil
}

func schemaAtRequiredPath(root map[string]any, path []string) (map[string]any, error) {
	current := root
	for index, segment := range path {
		properties, _ := current["properties"].(map[string]any)
		next, ok := properties[segment].(map[string]any)
		if !ok {
			return nil, fmt.Errorf("segment %q does not exist", strings.Join(path[:index+1], "."))
		}
		if !schemaRequires(current, segment) {
			return nil, fmt.Errorf("segment %q must be required", strings.Join(path[:index+1], "."))
		}
		current = next
	}
	return current, nil
}

func schemaRequires(schema map[string]any, field string) bool {
	required, _ := schema["required"].([]any)
	for _, item := range required {
		if item == field {
			return true
		}
	}
	return false
}

func schemaType(schema map[string]any) string {
	value, _ := schema["type"].(string)
	return value
}

func schemaInteger(value any) (int, bool) {
	number, ok := value.(float64)
	if !ok || math.IsInf(number, 0) || math.IsNaN(number) || number != math.Trunc(number) {
		return 0, false
	}
	return int(number), true
}
