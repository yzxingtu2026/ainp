package tests

import (
	"context"
	"testing"
	"time"

	. "github.com/yzxingtu2026/ainp/platform/framework/runtimecontext"
)

func TestTimeSnapshotAndRelativeDateAreDeterministicAcrossDST(t *testing.T) {
	now := time.Date(2026, 3, 8, 6, 30, 0, 0, time.UTC)
	snapshot, err := NewTimeSnapshot(now, "America/New_York", "user_preference")
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.CurrentLocalDateTime != "2026-03-08T01:30:00-05:00" || snapshot.CurrentLocalDate != "2026-03-08" {
		t.Fatalf("snapshot=%#v", snapshot)
	}
	today, err := ResolveRelativeDate(snapshot, "today")
	if err != nil {
		t.Fatal(err)
	}
	if today.StartUTC != time.Date(2026, 3, 8, 5, 0, 0, 0, time.UTC) || today.EndUTC != time.Date(2026, 3, 9, 4, 0, 0, 0, time.UTC) {
		t.Fatalf("DST day range=%#v", today)
	}
	if today.EndUTC.Sub(today.StartUTC) != 23*time.Hour {
		t.Fatalf("DST day must be 23h, got %s", today.EndUTC.Sub(today.StartUTC))
	}
	lastWeek, err := ResolveRelativeDate(snapshot, "last_week")
	if err != nil {
		t.Fatal(err)
	}
	if lastWeek.LocalStart[:10] != "2026-02-23" || lastWeek.LocalEnd[:10] != "2026-03-02" {
		t.Fatalf("last week=%#v", lastWeek)
	}
}

func TestStoredSnapshotValidationRejectsDerivedFieldDrift(t *testing.T) {
	snapshot, _ := NewTimeSnapshot(time.Date(2026, 7, 19, 10, 0, 0, 0, time.UTC), "Asia/Shanghai", "user_preference")
	snapshot.CurrentLocalDate = "2026-07-18"
	if snapshot.Validate() == nil {
		t.Fatal("derived date drift must fail closed")
	}
}

func TestTrustedContextAccessorReturnsReadOnlyCopy(t *testing.T) {
	original := &TrustedRuntimeContext{TenantID: "tenant-a", UserID: "user-a"}
	ctx := WithTrusted(context.Background(), original)
	first, ok := FromContext(ctx)
	if !ok {
		t.Fatal("trusted context missing")
	}
	first.UserID = "changed"
	second, _ := FromContext(ctx)
	if second.UserID != "user-a" {
		t.Fatal("consumer mutated shared trusted context")
	}
}

func TestRenderTimeGreetingUsesUnifiedLocaleAndTimezoneBoundaries(t *testing.T) {
	tests := []struct {
		name, timezone, locale, body, want string
		at                                 time.Time
	}{
		{"shanghai midnight", "Asia/Shanghai", "zh-CN", "继续工作", "夜深了，继续工作", time.Date(2026, 7, 27, 16, 0, 0, 0, time.UTC)},
		{"shanghai five", "Asia/Shanghai", "zh-CN", "继续工作", "早上好，继续工作", time.Date(2026, 7, 27, 21, 0, 0, 0, time.UTC)},
		{"utc six", "UTC", "zh-CN", "继续工作", "早上好，继续工作", time.Date(2026, 7, 28, 6, 0, 0, 0, time.UTC)},
		{"utc noon", "UTC", "zh-CN", "继续工作", "中午好，继续工作", time.Date(2026, 7, 28, 12, 0, 0, 0, time.UTC)},
		{"utc fourteen", "UTC", "zh-CN", "继续工作", "下午好，继续工作", time.Date(2026, 7, 28, 14, 0, 0, 0, time.UTC)},
		{"utc eighteen", "UTC", "zh-CN", "继续工作", "晚上好，继续工作", time.Date(2026, 7, 28, 18, 0, 0, 0, time.UTC)},
		{"new york noon", "America/New_York", "en-US", "continue working", "Good afternoon, continue working", time.Date(2026, 7, 28, 16, 0, 0, 0, time.UTC)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			snapshot, err := NewTimeSnapshot(tt.at, tt.timezone, "test")
			if err != nil {
				t.Fatal(err)
			}
			if got := RenderTimeGreeting(tt.body, tt.locale, snapshot); got != tt.want {
				t.Fatalf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestNormalizeGreetingBodyRemovesLegacyPrefixes(t *testing.T) {
	tests := map[string]string{
		"晚上好，继续工作":                       "继续工作",
		"Good evening, continue working": "continue working",
		"Working late, continue working": "continue working",
	}
	for input, want := range tests {
		if got := NormalizeGreetingBody(input); got != want {
			t.Fatalf("got %q, want %q", got, want)
		}
	}
}
