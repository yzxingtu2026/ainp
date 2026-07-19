package runtimecontext

import (
	"context"
	"fmt"
	"time"
)

const (
	ContextVersion = "runtime-context.v1"
	PolicyVersion  = "trusted-identity-time.v1"
)

type Clock interface{ Now() time.Time }
type ClockFunc func() time.Time

func (f ClockFunc) Now() time.Time { return f() }

type TimeSnapshot struct {
	EvaluatedAtUTC       time.Time `json:"evaluated_at_utc"`
	Timezone             string    `json:"timezone"`
	TimezoneSource       string    `json:"timezone_source"`
	CurrentLocalDateTime string    `json:"current_local_datetime"`
	CurrentLocalDate     string    `json:"current_local_date"`
	Weekday              string    `json:"weekday"`
}

type SnapshotRefs struct {
	RunID          string    `json:"run_id"`
	ContextVersion string    `json:"context_version"`
	PolicyVersion  string    `json:"policy_version"`
	Timezone       string    `json:"timezone"`
	TimezoneSource string    `json:"timezone_source"`
	EvaluatedAtUTC time.Time `json:"evaluated_at_utc"`
}

func NewTimeSnapshot(now time.Time, timezone, source string) (TimeSnapshot, error) {
	location, err := time.LoadLocation(timezone)
	if err != nil {
		return TimeSnapshot{}, fmt.Errorf("invalid IANA timezone %q: %w", timezone, err)
	}
	utc := now.UTC()
	local := utc.In(location)
	return TimeSnapshot{EvaluatedAtUTC: utc, Timezone: timezone, TimezoneSource: source,
		CurrentLocalDateTime: local.Format(time.RFC3339), CurrentLocalDate: local.Format("2006-01-02"), Weekday: local.Weekday().String()}, nil
}

func (s TimeSnapshot) Validate() error {
	if s.EvaluatedAtUTC.IsZero() || s.EvaluatedAtUTC.Location() != time.UTC {
		return fmt.Errorf("evaluated_at_utc must be non-zero UTC")
	}
	want, err := NewTimeSnapshot(s.EvaluatedAtUTC, s.Timezone, s.TimezoneSource)
	if err != nil {
		return err
	}
	if want.CurrentLocalDateTime != s.CurrentLocalDateTime || want.CurrentLocalDate != s.CurrentLocalDate || want.Weekday != s.Weekday {
		return fmt.Errorf("time snapshot derived fields mismatch")
	}
	return nil
}

type TrustedRuntimeContext struct {
	TenantID               string       `json:"tenant_id"`
	UserID                 string       `json:"user_id"`
	Username               string       `json:"username"`
	DisplayName            string       `json:"display_name"`
	PreferredAddress       string       `json:"preferred_address"`
	PreferredAddressSource string       `json:"preferred_address_source"`
	Locale                 string       `json:"locale"`
	LocaleSource           string       `json:"locale_source"`
	AutoMemoryEnabled      bool         `json:"auto_memory_enabled"`
	ConversationID         string       `json:"conversation_id"`
	ConversationCreatedAt  time.Time    `json:"conversation_created_at"`
	AgentID                string       `json:"agent_id"`
	AgentVersionID         string       `json:"agent_version_id"`
	RunID                  string       `json:"run_id"`
	Time                   TimeSnapshot `json:"time"`
	ContextVersion         string       `json:"context_version"`
	PolicyVersion          string       `json:"policy_version"`
}

type BuildRequest struct {
	TenantID              string
	UserID                string
	ConversationID        string
	ConversationCreatedAt time.Time
	AgentID               string
	AgentVersionID        string
	RunID                 string
	TimeSnapshot          *TimeSnapshot
}

type Provider interface {
	Build(context.Context, BuildRequest) (*TrustedRuntimeContext, error)
}

type trustedContextKey struct{}

func WithTrusted(ctx context.Context, value *TrustedRuntimeContext) context.Context {
	if value == nil {
		return ctx
	}
	copyValue := *value
	return context.WithValue(ctx, trustedContextKey{}, &copyValue)
}

func FromContext(ctx context.Context) (*TrustedRuntimeContext, bool) {
	value, ok := ctx.Value(trustedContextKey{}).(*TrustedRuntimeContext)
	if !ok || value == nil {
		return nil, false
	}
	copyValue := *value
	return &copyValue, true
}

type DateRange struct {
	StartUTC   time.Time `json:"start_utc"`
	EndUTC     time.Time `json:"end_utc"`
	LocalStart string    `json:"local_start"`
	LocalEnd   string    `json:"local_end"`
}

func ResolveRelativeDate(snapshot TimeSnapshot, value string) (DateRange, error) {
	if err := snapshot.Validate(); err != nil {
		return DateRange{}, err
	}
	loc, _ := time.LoadLocation(snapshot.Timezone)
	base := snapshot.EvaluatedAtUTC.In(loc)
	day := time.Date(base.Year(), base.Month(), base.Day(), 0, 0, 0, 0, loc)
	end := day.AddDate(0, 0, 1)
	switch value {
	case "today":
	case "yesterday":
		day = day.AddDate(0, 0, -1)
		end = day.AddDate(0, 0, 1)
	case "this_week", "last_week":
		daysSinceMonday := (int(day.Weekday()) + 6) % 7
		day = day.AddDate(0, 0, -daysSinceMonday)
		if value == "last_week" {
			day = day.AddDate(0, 0, -7)
		}
		end = day.AddDate(0, 0, 7)
	default:
		return DateRange{}, fmt.Errorf("unsupported relative date %q", value)
	}
	return DateRange{StartUTC: day.UTC(), EndUTC: end.UTC(), LocalStart: day.Format(time.RFC3339), LocalEnd: end.Format(time.RFC3339)}, nil
}
