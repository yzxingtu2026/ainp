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
