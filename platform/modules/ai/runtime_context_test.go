package ai

import (
	"strings"
	"testing"
	"time"

	"github.com/yzxingtu2026/ainp/platform/framework/aimodelruntime"
	"github.com/yzxingtu2026/ainp/platform/framework/runtimecontext"
)

func TestTrustedRuntimeContextIsTemporaryAndNearUserTail(t *testing.T) {
	trusted := &runtimecontext.TrustedRuntimeContext{Username: "alice", DisplayName: "Alice", PreferredAddress: "老板", PreferredAddressSource: "user_preference", Locale: "zh-CN", LocaleSource: "user_preference", ContextVersion: runtimecontext.ContextVersion, PolicyVersion: runtimecontext.PolicyVersion, Time: runtimecontext.TimeSnapshot{EvaluatedAtUTC: time.Date(2026, 7, 19, 10, 0, 0, 0, time.UTC), Timezone: "Asia/Shanghai", TimezoneSource: "user_preference", CurrentLocalDateTime: "2026-07-19T18:00:00+08:00", CurrentLocalDate: "2026-07-19", Weekday: "Sunday"}}
	messages := []aimodelruntime.Message{{Role: aimodelruntime.RoleUser, Content: []aimodelruntime.ContentPart{{Type: aimodelruntime.PartInputText, Text: "hello"}}}}
	out := injectTrustedRuntimeContext(messages, trusted)
	if len(out) != 2 || out[0].Role != aimodelruntime.RoleDeveloper || out[1].Role != aimodelruntime.RoleUser {
		t.Fatalf("unexpected prompt order: %#v", out)
	}
	text := out[0].Content[0].Text
	for _, expected := range []string{"TRUSTED RUNTIME CONTEXT", "UNTRUSTED USER MEMORY", "2026-07-19T18:00:00+08:00", "老板"} {
		if !strings.Contains(text, expected) {
			t.Fatalf("context section missing %q: %s", expected, text)
		}
	}
	if strings.Contains(text, "tenant_id") || strings.Contains(text, "user_id") {
		t.Fatal("internal identity IDs must not be exposed to model prompt")
	}
}
