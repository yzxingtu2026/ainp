package tests

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/yzxingtu2026/ainp/platform/framework/aitool"
)

type sanitizerCredential struct {
	AccessToken   string `json:"access_token"`
	Authorization string `json:"authorization"`
}

type sanitizerItem struct {
	ID         int                 `json:"id"`
	Password   string              `json:"password"`
	Credential sanitizerCredential `json:"credential_detail"`
}

type sanitizerPayload struct {
	Items  [3]sanitizerItem         `json:"items"`
	Lookup map[string]sanitizerItem `json:"lookup"`
	Nested any                      `json:"nested"`
	Meta   *struct {
		Secret string `json:"secret"`
	} `json:"meta"`
}

func TestToolResultSanitizerNormalizesTypedAndUntypedValues(t *testing.T) {
	meta := &struct {
		Secret string `json:"secret"`
	}{Secret: "meta-secret"}
	typed := sanitizerPayload{Items: [3]sanitizerItem{
		{ID: 1, Password: "p1", Credential: sanitizerCredential{AccessToken: "t1", Authorization: "a1"}},
		{ID: 2, Password: "p2", Credential: sanitizerCredential{AccessToken: "t2", Authorization: "a2"}},
		{ID: 3, Password: "p3", Credential: sanitizerCredential{AccessToken: "t3", Authorization: "a3"}},
	}, Lookup: map[string]sanitizerItem{"primary": {ID: 4, Password: "p4", Credential: sanitizerCredential{AccessToken: "t4", Authorization: "a4"}}}, Nested: []sanitizerItem{{ID: 5, Password: "p5", Credential: sanitizerCredential{AccessToken: "t5", Authorization: "a5"}}}, Meta: meta}
	encoded, err := json.Marshal(typed)
	if err != nil {
		t.Fatal(err)
	}
	var untyped any
	if err = json.Unmarshal(encoded, &untyped); err != nil {
		t.Fatal(err)
	}
	typedSafe, typedMeta := aitool.SanitizeData(&typed, 64<<10)
	untypedSafe, untypedMeta := aitool.SanitizeData(untyped, 64<<10)
	if !reflect.DeepEqual(typedSafe, untypedSafe) || !reflect.DeepEqual(typedMeta, untypedMeta) {
		t.Fatalf("typed and untyped results differ:\n%#v\n%#v", typedSafe, untypedSafe)
	}
	root := typedSafe.(map[string]any)
	items := root["items"].([]any)
	if _, ok := items[0].(map[string]any)["credential_detail"].(map[string]any); !ok {
		t.Fatalf("redacted credential object changed type: %#v", items[0])
	}
	safeJSON, _ := json.Marshal(typedSafe)
	text := string(safeJSON)
	for _, secret := range []string{"p1", "p2", "p3", "p4", "p5", "t1", "t2", "t3", "t4", "t5", "a1", "a2", "a3", "a4", "a5", "meta-secret"} {
		if strings.Contains(text, secret) {
			t.Fatalf("sensitive value leaked: %q in %s", secret, text)
		}
	}
}

func TestToolResultSanitizerBoundsCompleteResult(t *testing.T) {
	result := &aitool.Result{
		CallID:       strings.Repeat("c", 80),
		Status:       "completed",
		Summary:      strings.Repeat("summary", 20),
		Data:         map[string]any{"items": []any{strings.Repeat("payload", 1000), strings.Repeat("payload", 1000)}},
		ResourceRefs: []string{"resource:1", "resource:2"},
	}
	const budget = 1024
	aitool.SanitizeResult(result, budget)
	encoded, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	if len(encoded) > budget {
		t.Fatalf("complete result exceeded budget: got %d, want <= %d", len(encoded), budget)
	}
	if _, ok := result.Data.(map[string]any); !ok || !result.Truncated || result.Truncation == nil {
		t.Fatalf("complete result lost type or metadata: %#v", result)
	}
}

func TestToolResultSanitizerKeepsArrayPrefixAndReportsOmissions(t *testing.T) {
	input := make([]int, 150)
	for i := range input {
		input[i] = i
	}
	safe, meta := aitool.SanitizeData(input, 64<<10)
	items, ok := safe.([]any)
	if !ok || len(items) != 100 || items[0].(json.Number).String() != "0" || items[99].(json.Number).String() != "99" {
		t.Fatalf("array prefix or type changed: %#v", safe)
	}
	assertTruncationFact(t, meta, "$", "array", 100, 50, "item_limit")
}

func TestToolResultSanitizerUsesOneEncodedByteBudget(t *testing.T) {
	input := map[string]any{}
	for i := 0; i < 24; i++ {
		input[fmt.Sprintf("field_%02d", i)] = strings.Repeat(string(rune('a'+i%20)), 180)
	}
	const budget = 900
	safe, meta := aitool.SanitizeData(input, budget)
	encoded, err := json.Marshal(struct {
		Data       any                `json:"data"`
		Truncation *aitool.Truncation `json:"truncation,omitempty"`
	}{safe, meta})
	if err != nil {
		t.Fatal(err)
	}
	if len(encoded) > budget {
		t.Fatalf("shared budget exceeded: got %d, want <= %d", len(encoded), budget)
	}
	if _, ok := safe.(map[string]any); !ok || meta == nil {
		t.Fatalf("object type or truncation metadata missing: %#v %#v", safe, meta)
	}
	firstData, firstMeta := aitool.SanitizeData(input, budget)
	secondData, secondMeta := aitool.SanitizeData(input, budget)
	first, _ := json.Marshal([]any{firstData, firstMeta})
	second, _ := json.Marshal([]any{secondData, secondMeta})
	if string(first) != string(second) {
		t.Fatalf("sanitization is not deterministic:\n%s\n%s", first, second)
	}
}

func TestToolResultSanitizerPreservesTypesAtDepthAndLargeFields(t *testing.T) {
	deep := map[string]any{"items": []any{map[string]any{"next": []any{map[string]any{"next": []any{map[string]any{"next": []any{map[string]any{"next": []any{"too-deep"}}}}}}}}}}
	safe, meta := aitool.SanitizeData(map[string]any{"deep": deep, "large": strings.Repeat("x", 10000)}, 700)
	root, ok := safe.(map[string]any)
	if !ok {
		t.Fatalf("root object changed type: %#v", safe)
	}
	if _, exists := root["deep"]; !exists {
		t.Fatalf("depth-limited field disappeared: %#v", root)
	}
	if meta == nil {
		t.Fatal("expected depth or byte truncation facts")
	}
	encoded, _ := json.Marshal(safe)
	if strings.Contains(string(encoded), `"deep":"[truncated]"`) || strings.Contains(string(encoded), `"items":"[truncated]"`) {
		t.Fatalf("container changed into a string: %s", encoded)
	}
}

func assertTruncationFact(t *testing.T, meta *aitool.Truncation, path, kind string, returned, omitted int, reason string) {
	t.Helper()
	if meta != nil {
		for _, fact := range meta.Entries {
			if fact.Path == path && fact.Kind == kind && fact.ReturnedCount == returned && fact.OmittedCount == omitted && fact.Reason == reason {
				return
			}
		}
	}
	t.Fatalf("missing truncation fact: %#v", meta)
}
