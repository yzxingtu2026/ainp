package tests

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/yzxingtu2026/ainp/platform/framework/aimodel"
	"github.com/yzxingtu2026/ainp/platform/framework/aimodelruntime"
	"github.com/yzxingtu2026/ainp/platform/framework/auth"
	"github.com/yzxingtu2026/ainp/platform/framework/database"
	tenantdata "github.com/yzxingtu2026/ainp/platform/framework/tenant"
	"github.com/yzxingtu2026/ainp/platform/modules/ai"
)

func TestCollectModelStreamPreservesTerminalCategoriesAndPartialOutput(t *testing.T) {
	tests := []struct {
		name          string
		events        []aimodelruntime.StreamEvent
		category      aimodelruntime.ErrorCategory
		wantText      string
		wantArguments string
	}{
		{name: "timeout before first token", category: aimodelruntime.ErrorTimeout},
		{
			name: "timeout after text delta",
			events: []aimodelruntime.StreamEvent{
				{Type: aimodelruntime.EventResponseStarted, Sequence: 1},
				{Type: aimodelruntime.EventOutputItemStarted, Sequence: 2, ItemID: "text-1"},
				{Type: aimodelruntime.EventOutputTextDelta, Sequence: 3, ItemID: "text-1", Delta: "partial"},
			},
			category: aimodelruntime.ErrorTimeout,
			wantText: "partial",
		},
		{
			name: "timeout during tool arguments",
			events: []aimodelruntime.StreamEvent{
				{Type: aimodelruntime.EventResponseStarted, Sequence: 1},
				{Type: aimodelruntime.EventToolCallStarted, Sequence: 2, ItemID: "tool-1", Item: &aimodelruntime.OutputItem{ID: "tool-1", Type: aimodelruntime.PartToolCall, ToolCallID: "call-1", ToolName: "lookup"}},
				{Type: aimodelruntime.EventToolArgumentsDelta, Sequence: 3, ItemID: "tool-1", Delta: `{"id":`},
			},
			category:      aimodelruntime.ErrorTimeout,
			wantArguments: `{"id":`,
		},
		{name: "categorized provider error", category: aimodelruntime.ErrorProviderUnavailable},
		{name: "user cancellation", category: aimodelruntime.ErrorCancelled},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			events := make(chan aimodelruntime.StreamEvent, len(tt.events))
			for _, event := range tt.events {
				events <- event
			}
			close(events)
			terminal := make(chan error, 1)
			terminal <- &aimodelruntime.RuntimeError{Category: tt.category, Message: "fixture terminal"}
			close(terminal)

			response, err := aimodelruntime.CollectStream(aimodelruntime.ResultStream{InvocationID: "inv-1", Events: events, Terminal: terminal}, nil)
			assertRuntimeCategory(t, err, tt.category)
			if response.InvocationID != "inv-1" {
				t.Fatalf("invocation id=%q", response.InvocationID)
			}
			if tt.wantText != "" && (len(response.Output) != 1 || response.Output[0].Text != tt.wantText) {
				t.Fatalf("partial text response=%#v", response.Output)
			}
			if tt.wantArguments != "" {
				if len(response.Output) != 1 || string(response.Output[0].Arguments) != tt.wantArguments {
					t.Fatalf("partial tool response=%#v", response.Output)
				}
				if jsonComplete(response.Output[0].Arguments) {
					t.Fatal("incomplete tool arguments became executable JSON")
				}
			}
		})
	}
}

func TestValidatedModelStreamProtocolFailures(t *testing.T) {
	tests := []struct {
		name   string
		events []aimodelruntime.StreamEvent
	}{
		{name: "missing terminal", events: []aimodelruntime.StreamEvent{{Type: aimodelruntime.EventResponseStarted, Sequence: 1}}},
		{name: "non monotonic", events: []aimodelruntime.StreamEvent{{Type: aimodelruntime.EventResponseStarted, Sequence: 1}, {Type: aimodelruntime.EventUsageUpdated, Sequence: 1}}},
		{name: "duplicate start", events: []aimodelruntime.StreamEvent{{Type: aimodelruntime.EventResponseStarted, Sequence: 1}, {Type: aimodelruntime.EventResponseStarted, Sequence: 2}}},
		{name: "item not started", events: []aimodelruntime.StreamEvent{{Type: aimodelruntime.EventResponseStarted, Sequence: 1}, {Type: aimodelruntime.EventOutputTextDelta, Sequence: 2, ItemID: "missing", Delta: "x"}}},
		{name: "terminal before start", events: []aimodelruntime.StreamEvent{{Type: aimodelruntime.EventResponseCompleted, Sequence: 1}}},
		{name: "event after terminal", events: []aimodelruntime.StreamEvent{{Type: aimodelruntime.EventResponseStarted, Sequence: 1}, {Type: aimodelruntime.EventResponseCompleted, Sequence: 2}, {Type: aimodelruntime.EventUsageUpdated, Sequence: 3}}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stream := aimodelruntime.ValidateStream(&runtimeSliceStream{events: tt.events})
			var err error
			for err == nil {
				_, err = stream.Recv(context.Background())
			}
			assertRuntimeCategory(t, err, aimodelruntime.ErrorStreamProtocol)
		})
	}
}

func TestValidatedModelStreamTerminalWinsDeadlineRace(t *testing.T) {
	source := &runtimeStepStream{steps: []runtimeStreamStep{
		{event: aimodelruntime.StreamEvent{Type: aimodelruntime.EventResponseStarted, Sequence: 1}},
		{event: aimodelruntime.StreamEvent{Type: aimodelruntime.EventResponseCompleted, Sequence: 2, FinishReason: aimodelruntime.FinishCompleted}},
		{err: context.DeadlineExceeded},
	}}
	stream := aimodelruntime.ValidateStream(source)
	if _, err := stream.Recv(context.Background()); err != nil {
		t.Fatal(err)
	}
	event, err := stream.Recv(context.Background())
	if err != nil || event.Type != aimodelruntime.EventResponseCompleted {
		t.Fatalf("terminal lost to deadline: event=%#v err=%v", event, err)
	}
	if _, err = stream.Recv(context.Background()); !errors.Is(err, io.EOF) {
		t.Fatalf("post-terminal recv=%v, want EOF", err)
	}
}

func TestCollectModelStreamFailsClosedWithoutTerminalResult(t *testing.T) {
	events := make(chan aimodelruntime.StreamEvent)
	close(events)
	_, err := aimodelruntime.CollectStream(aimodelruntime.ResultStream{Events: events}, nil)
	assertRuntimeCategory(t, err, aimodelruntime.ErrorStreamProtocol)
}

func TestCollectModelStreamCancelsProducerOnObserverFailure(t *testing.T) {
	events := make(chan aimodelruntime.StreamEvent, 3)
	events <- aimodelruntime.StreamEvent{Type: aimodelruntime.EventResponseStarted, Sequence: 1}
	events <- aimodelruntime.StreamEvent{Type: aimodelruntime.EventOutputItemStarted, Sequence: 2, ItemID: "text-1"}
	events <- aimodelruntime.StreamEvent{Type: aimodelruntime.EventOutputTextDelta, Sequence: 3, ItemID: "text-1", Delta: "x"}
	close(events)
	terminal := make(chan error, 1)
	terminal <- nil
	close(terminal)
	var cancelled atomic.Int32
	want := errors.New("observer failed")
	_, err := aimodelruntime.CollectStream(aimodelruntime.ResultStream{Events: events, Terminal: terminal, Cancel: func() { cancelled.Add(1) }}, func(aimodelruntime.StreamEvent) error { return want })
	if !errors.Is(err, want) || cancelled.Load() != 1 {
		t.Fatalf("err=%v cancel calls=%d", err, cancelled.Load())
	}
}

func TestModelStreamTerminalPersistsInvocationPostgres(t *testing.T) {
	dsn := os.Getenv("AINP_TEST_DATABASE_DSN")
	if dsn == "" {
		t.Skip("AINP_TEST_DATABASE_DSN is not set")
	}
	admin, err := database.Open(dsn)
	if err != nil {
		t.Fatal(err)
	}
	schema := "ainp_model_terminal_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if err = admin.Exec(fmt.Sprintf(`CREATE SCHEMA %q`, schema)).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = admin.Exec(fmt.Sprintf(`DROP SCHEMA %q CASCADE`, schema)).Error })
	db, err := database.Open(dsn + " search_path=" + schema)
	if err != nil {
		t.Fatal(err)
	}
	if err = ai.AutoMigrate(db); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name         string
		steps        []runtimeStreamStep
		timeout      time.Duration
		cancel       bool
		wantStatus   string
		wantFinish   aimodelruntime.FinishReason
		wantCategory aimodelruntime.ErrorCategory
		wantFirst    bool
	}{
		{name: "timeout before first token", steps: []runtimeStreamStep{{waitContext: true}}, timeout: time.Second, wantStatus: "failed", wantFinish: aimodelruntime.FinishCancelled, wantCategory: aimodelruntime.ErrorTimeout},
		{name: "timeout after text delta", steps: []runtimeStreamStep{{event: startedEvent(1)}, {event: textStartEvent(2)}, {event: textDeltaEvent(3, "partial")}, {waitContext: true}}, timeout: time.Second, wantStatus: "failed", wantFinish: aimodelruntime.FinishCancelled, wantCategory: aimodelruntime.ErrorTimeout, wantFirst: true},
		{name: "timeout during tool arguments", steps: []runtimeStreamStep{{event: startedEvent(1)}, {event: toolStartEvent(2)}, {event: aimodelruntime.StreamEvent{Type: aimodelruntime.EventToolArgumentsDelta, Sequence: 3, ItemID: "tool-1", Delta: `{"id":`}}, {waitContext: true}}, timeout: time.Second, wantStatus: "failed", wantFinish: aimodelruntime.FinishCancelled, wantCategory: aimodelruntime.ErrorTimeout, wantFirst: true},
		{name: "categorized provider error", steps: []runtimeStreamStep{{event: startedEvent(1)}, {err: &aimodelruntime.RuntimeError{Category: aimodelruntime.ErrorProviderUnavailable, Message: "fixture provider unavailable"}}}, wantStatus: "failed", wantFinish: aimodelruntime.FinishError, wantCategory: aimodelruntime.ErrorProviderUnavailable},
		{name: "user cancellation", steps: []runtimeStreamStep{{waitContext: true}}, cancel: true, wantStatus: "cancelled", wantFinish: aimodelruntime.FinishCancelled, wantCategory: aimodelruntime.ErrorCancelled},
		{name: "missing terminal EOF", steps: []runtimeStreamStep{{event: startedEvent(1)}}, wantStatus: "failed", wantFinish: aimodelruntime.FinishError, wantCategory: aimodelruntime.ErrorStreamProtocol},
		{name: "invalid event sequence", steps: []runtimeStreamStep{{event: startedEvent(1)}, {event: aimodelruntime.StreamEvent{Type: aimodelruntime.EventUsageUpdated, Sequence: 1}}}, wantStatus: "failed", wantFinish: aimodelruntime.FinishError, wantCategory: aimodelruntime.ErrorStreamProtocol},
		{name: "terminal wins deadline race", steps: []runtimeStreamStep{{event: startedEvent(1)}, {event: aimodelruntime.StreamEvent{Type: aimodelruntime.EventResponseCompleted, Sequence: 2, ResponseID: "response-1", FinishReason: aimodelruntime.FinishCompleted}}, {err: context.DeadlineExceeded}}, wantStatus: "completed", wantFinish: aimodelruntime.FinishCompleted},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			source := &runtimeStepStream{steps: tt.steps}
			registry := aimodelruntime.NewAdapterRegistry()
			if err := registry.Register("fixture", streamFixtureAdapter{source: source}); err != nil {
				t.Fatal(err)
			}
			service := ai.NewInvocationService(db, streamFixtureResolver{}, registry, nil)
			baseCtx := auth.WithContext(context.Background(), &auth.Context{Authenticated: true, TenantID: "tenant-a", PrincipalID: "user-a", SessionID: "session-a", Realm: auth.RealmTenant})
			ctx, cancel := context.WithCancel(baseCtx)
			defer cancel()
			// The runtime starts TimeoutMS after persisting the running invocation.
			// A short parent deadline also times out database preparation under CI
			// load, before the stream behavior this test exercises can begin.
			timeoutMS := int64(10000)
			if tt.timeout > 0 {
				timeoutMS = tt.timeout.Milliseconds()
			}
			value, streamErr := service.Stream(ctx, aimodelruntime.GenerateRequest{
				ModelAlias: "fixture-chat",
				Messages:   []aimodelruntime.Message{{Role: aimodelruntime.RoleUser, Content: []aimodelruntime.ContentPart{{Type: aimodelruntime.PartInputText, Text: "hello"}}}},
				TimeoutMS:  timeoutMS,
				Metadata:   map[string]string{"agent_run_id": "run-terminal"},
			})
			if streamErr != nil {
				t.Fatal(streamErr)
			}
			stream := value.(aimodelruntime.ResultStream)
			if tt.cancel {
				cancel()
			}
			_, collectErr := aimodelruntime.CollectStream(stream, nil)
			if tt.wantCategory == "" {
				if collectErr != nil {
					t.Fatalf("collect error=%v", collectErr)
				}
			} else {
				assertRuntimeCategory(t, collectErr, tt.wantCategory)
			}
			if !source.closed.Load() {
				t.Fatal("provider stream was not closed")
			}

			tdb, scopeErr := tenantdata.TenantDB(baseCtx, db)
			if scopeErr != nil {
				t.Fatal(scopeErr)
			}
			rows, queryErr := tenantdata.TenantRaw[ai.ModelInvocation](tdb, "SELECT * FROM ai_model_invocations WHERE tenant_id=? AND id=? LIMIT 1", stream.InvocationID)
			if queryErr != nil || len(rows) != 1 {
				t.Fatalf("invocation query rows=%d err=%v", len(rows), queryErr)
			}
			row := rows[0]
			if row.Status != tt.wantStatus || row.FinishReason != string(tt.wantFinish) || row.ErrorCategory != string(tt.wantCategory) {
				t.Fatalf("invocation terminal mismatch: status=%s finish=%s category=%s", row.Status, row.FinishReason, row.ErrorCategory)
			}
			if row.AgentRunID == nil || *row.AgentRunID != "run-terminal" || row.FinishedAt == nil {
				t.Fatalf("invocation linkage/finish missing: %#v", row)
			}
			if tt.wantFirst != (row.FirstTokenAt != nil) {
				t.Fatalf("first token fact=%v, want %v", row.FirstTokenAt != nil, tt.wantFirst)
			}
		})
	}
}

type runtimeStreamStep struct {
	event       aimodelruntime.StreamEvent
	err         error
	waitContext bool
}

type runtimeStepStream struct {
	steps  []runtimeStreamStep
	index  int
	closed atomic.Bool
}

func (s *runtimeStepStream) Recv(ctx context.Context) (aimodelruntime.StreamEvent, error) {
	if s.index >= len(s.steps) {
		return aimodelruntime.StreamEvent{}, io.EOF
	}
	step := s.steps[s.index]
	s.index++
	if step.waitContext {
		<-ctx.Done()
		return aimodelruntime.StreamEvent{}, ctx.Err()
	}
	return step.event, step.err
}

func (s *runtimeStepStream) Close() error {
	s.closed.Store(true)
	return nil
}

func assertRuntimeCategory(t *testing.T, err error, want aimodelruntime.ErrorCategory) {
	t.Helper()
	var runtimeErr *aimodelruntime.RuntimeError
	if !errors.As(err, &runtimeErr) || runtimeErr.Category != want {
		t.Fatalf("error=%v category=%v, want %s", err, runtimeErr, want)
	}
}

func jsonComplete(value []byte) bool {
	var target any
	return len(value) > 0 && json.Unmarshal(value, &target) == nil
}

type streamFixtureResolver struct{}

func (streamFixtureResolver) Resolve(context.Context, string, aimodel.CapabilityRequirements) (*aimodel.ResolvedModel, error) {
	return &aimodel.ResolvedModel{
		ProviderID: "provider-fixture", Protocol: "fixture", DeploymentID: "deployment-fixture", RemoteModelID: "model-fixture", ModelType: aimodel.ModelTypeChat,
		Capabilities: aimodel.Capabilities{InputModalities: []aimodel.Modality{aimodel.ModalityText}, OutputModalities: []aimodel.Modality{aimodel.ModalityText}, Features: []aimodel.Feature{aimodel.FeatureStreaming}},
	}, nil
}

type streamFixtureAdapter struct{ source aimodelruntime.EventStream }

func (streamFixtureAdapter) Generate(context.Context, aimodelruntime.AdapterRequest) (*aimodelruntime.GenerateResponse, error) {
	return nil, errors.New("fixture generate is unsupported")
}

func (a streamFixtureAdapter) Stream(context.Context, aimodelruntime.AdapterRequest) (aimodelruntime.EventStream, error) {
	return aimodelruntime.ValidateStream(a.source), nil
}

func startedEvent(sequence int64) aimodelruntime.StreamEvent {
	return aimodelruntime.StreamEvent{Type: aimodelruntime.EventResponseStarted, Sequence: sequence, ResponseID: "response-1"}
}

func textStartEvent(sequence int64) aimodelruntime.StreamEvent {
	return aimodelruntime.StreamEvent{Type: aimodelruntime.EventOutputItemStarted, Sequence: sequence, ItemID: "text-1", Item: &aimodelruntime.OutputItem{ID: "text-1", Type: aimodelruntime.PartOutputText}}
}

func textDeltaEvent(sequence int64, delta string) aimodelruntime.StreamEvent {
	return aimodelruntime.StreamEvent{Type: aimodelruntime.EventOutputTextDelta, Sequence: sequence, ItemID: "text-1", Delta: delta}
}

func toolStartEvent(sequence int64) aimodelruntime.StreamEvent {
	return aimodelruntime.StreamEvent{Type: aimodelruntime.EventToolCallStarted, Sequence: sequence, ItemID: "tool-1", Item: &aimodelruntime.OutputItem{ID: "tool-1", Type: aimodelruntime.PartToolCall, ToolCallID: "call-1", ToolName: "lookup"}}
}
