package tests

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"log/slog"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/yzxingtu2026/ainp/platform/framework/aiagentruntime"
	"github.com/yzxingtu2026/ainp/platform/framework/aiconversation"
	"github.com/yzxingtu2026/ainp/platform/framework/aimodel"
	"github.com/yzxingtu2026/ainp/platform/framework/aimodelruntime"
	"github.com/yzxingtu2026/ainp/platform/framework/aitool"
	"github.com/yzxingtu2026/ainp/platform/framework/attachment"
	"github.com/yzxingtu2026/ainp/platform/framework/auth"
	"github.com/yzxingtu2026/ainp/platform/framework/database"
	"github.com/yzxingtu2026/ainp/platform/framework/errs"
	"github.com/yzxingtu2026/ainp/platform/framework/logging"
	"github.com/yzxingtu2026/ainp/platform/framework/prompt"
	"github.com/yzxingtu2026/ainp/platform/framework/runtimecontext"
	"github.com/yzxingtu2026/ainp/platform/framework/storage"
	tenantdata "github.com/yzxingtu2026/ainp/platform/framework/tenant"
	"github.com/yzxingtu2026/ainp/platform/modules/ai"
	"github.com/yzxingtu2026/ainp/platform/modules/ai/experience/pagecapture"
	"github.com/yzxingtu2026/ainp/platform/modules/ai/tooling"
	"github.com/yzxingtu2026/ainp/platform/modules/system"
	"github.com/yzxingtu2026/ainp/platform/modules/system/attachments"
)

type stubVisionModelResolver struct {
	visionAliases map[string]bool
}

func (r stubVisionModelResolver) Resolve(_ context.Context, alias string, req aimodel.CapabilityRequirements) (*aimodel.ResolvedModel, error) {
	wantsVision := slices.Contains(req.Features, aimodel.FeatureVision) || slices.Contains(req.InputModalities, aimodel.ModalityImage)
	if wantsVision && !r.visionAliases[alias] {
		return nil, errs.NewKey(errs.CodeValidation, "ai.model.vision_not_supported", "model does not support vision")
	}
	return &aimodel.ResolvedModel{
		ProviderID:    "provider-1",
		DeploymentID:  "deployment-" + alias,
		Protocol:      aimodel.ProtocolOpenAI,
		RemoteModelID: "gpt-test",
		Capabilities: aimodel.Capabilities{
			InputModalities:  []aimodel.Modality{aimodel.ModalityText, aimodel.ModalityImage},
			OutputModalities: []aimodel.Modality{aimodel.ModalityText},
			Features:         []aimodel.Feature{aimodel.FeatureStreaming, aimodel.FeatureToolCalling, aimodel.FeatureVision},
		},
		Limits: aimodel.Limits{ContextWindow: 128000, MaxOutputTokens: 4096},
	}, nil
}

type scriptedStreamAdapter struct {
	calls     int
	requests  []aimodelruntime.GenerateRequest
	responses [][]aimodelruntime.StreamEvent
}

type sliceEventStream struct {
	events []aimodelruntime.StreamEvent
	index  int
}

func (s *sliceEventStream) Recv(context.Context) (aimodelruntime.StreamEvent, error) {
	if s.index >= len(s.events) {
		return aimodelruntime.StreamEvent{}, os.ErrClosed
	}
	ev := s.events[s.index]
	s.index++
	return ev, nil
}
func (*sliceEventStream) Close() error { return nil }

func (a *scriptedStreamAdapter) Generate(_ context.Context, in aimodelruntime.AdapterRequest) (*aimodelruntime.GenerateResponse, error) {
	a.requests = append(a.requests, in.Request)
	return &aimodelruntime.GenerateResponse{
		ID:           "gen-1",
		FinishReason: aimodelruntime.FinishCompleted,
		Output:       []aimodelruntime.OutputItem{{ID: "item-1", Type: aimodelruntime.PartOutputText, Text: "safe fallback"}},
	}, nil
}

func (a *scriptedStreamAdapter) Stream(_ context.Context, in aimodelruntime.AdapterRequest) (aimodelruntime.EventStream, error) {
	a.requests = append(a.requests, in.Request)
	idx := a.calls
	a.calls++
	if idx >= len(a.responses) {
		idx = len(a.responses) - 1
	}
	return &sliceEventStream{events: a.responses[idx]}, nil
}

type stubRuntimeContextProvider struct{}

func (stubRuntimeContextProvider) Build(_ context.Context, req runtimecontext.BuildRequest) (*runtimecontext.TrustedRuntimeContext, error) {
	now := time.Date(2026, 11, 13, 6, 0, 0, 0, time.UTC)
	if req.TimeSnapshot != nil {
		return &runtimecontext.TrustedRuntimeContext{
			ContextVersion: "1.0",
			PolicyVersion:  "1.0",
			RunID:          req.RunID,
			AgentID:        req.AgentID,
			Locale:         "zh-CN",
			LocaleSource:   "system",
			Time:           *req.TimeSnapshot,
		}, nil
	}
	return &runtimecontext.TrustedRuntimeContext{
		ContextVersion: "1.0",
		PolicyVersion:  "1.0",
		RunID:          req.RunID,
		AgentID:        req.AgentID,
		Locale:         "zh-CN",
		LocaleSource:   "system",
		Time: runtimecontext.TimeSnapshot{
			EvaluatedAtUTC:       now,
			Timezone:             "Asia/Shanghai",
			TimezoneSource:       "system",
			CurrentLocalDateTime: "2026-11-13T14:00:00+08:00",
			CurrentLocalDate:     "2026-11-13",
			Weekday:              "Friday",
		},
	}, nil
}

type staticAuthProvider struct{}

func (staticAuthProvider) LoadAuthorization(_ context.Context, req auth.AuthorizationRequest) (*auth.AuthorizationSnapshot, error) {
	return &auth.AuthorizationSnapshot{
		TenantID:      req.TenantID,
		Principal:     req.Principal,
		PrincipalID:   req.PrincipalID,
		SessionID:     req.SessionID,
		AuthVersion:   req.AuthVersion,
		Audience:      req.Audience,
		ClientSurface: req.ClientSurface,
	}, nil
}

func makeSamplePNG(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 16, 12))
	for y := 0; y < 12; y++ {
		for x := 0; x < 16; x++ {
			img.Set(x, y, color.RGBA{R: 40, G: 120, B: 200, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestPageCaptureSlotProjectionAndContractRequiredBinding(t *testing.T) {
	if err := prompt.LoadDir("../prompts"); err != nil {
		t.Fatal(err)
	}
	contracts, err := ai.NewBuiltinAgentContractRegistry()
	if err != nil {
		t.Fatal(err)
	}
	contract, ok := contracts.Get("system.operations", "1.0.0")
	if !ok {
		t.Fatal("system.operations contract missing")
	}
	if !slices.Contains(contract.RequiredToolSelectors, pagecapture.ToolID) {
		t.Fatalf("expected %s in RequiredToolSelectors, got %#v", pagecapture.ToolID, contract.RequiredToolSelectors)
	}

	resolver := stubVisionModelResolver{visionAliases: map[string]bool{"vision-chat": true}}
	slot := pagecapture.New(resolver, func(_ context.Context, runID string) (string, error) {
		if runID == "run-vision" {
			return "vision-chat", nil
		}
		return "text-only", nil
	})
	def := slot.Definition()
	if def.BindingMode != aitool.BindingForbidden || def.SideEffect != aitool.SideEffectRead || def.Risk != aitool.RiskLow || def.RequiresConfirmation {
		t.Fatalf("unexpected page capture definition governance: %#v", def)
	}
	meta := slot.Metadata()
	if !meta.ClientFulfillment || meta.MaxCallsPerRun != pagecapture.DefaultMaxCallsPerRun || meta.FulfillmentTimeoutSecond != pagecapture.DefaultTimeoutSeconds {
		t.Fatalf("unexpected page capture metadata: %#v", meta)
	}

	registry := aitool.NewRegistry()
	if err := registry.RegisterSlot(slot); err != nil {
		t.Fatal(err)
	}
	if err := registry.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	if registry.IsBindable(pagecapture.ToolID) {
		t.Fatal("builtin:page.capture must be non-bindable / non-removable contract tool")
	}

	webActor := &auth.Context{
		Authenticated: true,
		TenantID:      "tenant-a",
		PrincipalID:   "user-a",
		Principal:     auth.PrincipalRef{Kind: auth.PrincipalKindAccount, ID: "user-a"},
		ClientSurface: auth.TenantConsoleSurface,
		Realm:         auth.RealmTenant,
		SessionID:     "sess-1",
		AuthVersion:   1,
	}
	webScope := aitool.RuntimeScope{
		TenantID:      "tenant-a",
		PrincipalID:   "user-a",
		PrincipalKind: string(auth.PrincipalKindAccount),
		ClientSurface: auth.TenantConsoleSurface,
		Realm:         string(auth.RealmTenant),
		SessionID:     "sess-1",
		AuthVersion:   1,
		AgentRunID:    "run-vision",
	}

	// Vision model on web console surface -> projected.
	ctxVision := aimodelruntime.WithModelAlias(auth.WithContext(context.Background(), webActor), "vision-chat")
	if _, visible, projErr := slot.Project(ctxVision, webScope, def); projErr != nil || !visible {
		t.Fatalf("expected vision model on web console to project, visible=%v err=%v", visible, projErr)
	}

	// Text-only model on web console surface -> NOT projected (fail-closed).
	ctxText := aimodelruntime.WithModelAlias(auth.WithContext(context.Background(), webActor), "text-only")
	if _, visible, projErr := slot.Project(ctxText, webScope, def); projErr != nil || visible {
		t.Fatalf("expected text-only model to hide page.capture, visible=%v err=%v", visible, projErr)
	}

	// Mini-program / mobile client surface -> NOT projected even with vision model.
	miniActor := *webActor
	miniActor.ClientSurface = "hoa.miniapp"
	miniScope := webScope
	miniScope.ClientSurface = "hoa.miniapp"
	ctxMini := aimodelruntime.WithModelAlias(auth.WithContext(context.Background(), &miniActor), "vision-chat")
	if def.VisibleToActor(&miniActor) {
		t.Fatal("miniapp actor must not pass Definition.VisibleToActor")
	}
	if _, visible, projErr := slot.Project(ctxMini, miniScope, def); projErr != nil || visible {
		t.Fatalf("expected miniapp surface to hide page.capture, visible=%v err=%v", visible, projErr)
	}
}

func TestPageCaptureFulfillmentLoopIdempotencyTimeoutIsolationBudgetAndRedactionPostgres(t *testing.T) {
	dsn := os.Getenv("AINP_TEST_DATABASE_DSN")
	if dsn == "" {
		t.Skip("AINP_TEST_DATABASE_DSN is not set")
	}
	if err := prompt.LoadDir("../prompts"); err != nil {
		t.Fatal(err)
	}
	admin, err := database.Open(dsn)
	if err != nil {
		t.Fatal(err)
	}
	schema := "ainp_page_capture_" + strings.ReplaceAll(uuid.NewString()[:8], "-", "")
	if err := admin.Exec(fmt.Sprintf(`CREATE SCHEMA %q`, schema)).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = admin.Exec(fmt.Sprintf(`DROP SCHEMA %q CASCADE`, schema)).Error })
	db, err := database.Open(dsn + " search_path=" + schema)
	if err != nil {
		t.Fatal(err)
	}
	if err := system.AutoMigrate(db); err != nil {
		t.Fatal(err)
	}
	if err := ai.AutoMigrate(db); err != nil {
		t.Fatal(err)
	}
	migrationSQL, err := os.ReadFile("../migrations/0070_page_capture_requests.sql")
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(string(migrationSQL)).Error; err != nil {
		t.Fatalf("0070 migration failed: %v", err)
	}
	// Idempotent rerun of 0070 migration.
	if err := db.Exec(string(migrationSQL)).Error; err != nil {
		t.Fatalf("0070 migration idempotency failed: %v", err)
	}

	var logBuf bytes.Buffer
	prevLogger := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&logBuf, nil)))
	t.Cleanup(func() { slog.SetDefault(prevLogger) })

	storeDir := t.TempDir()
	objStore, err := storage.NewLocalObjectStore(storage.LocalConfig{RootDir: storeDir}, nil)
	if err != nil {
		t.Fatal(err)
	}
	transferTokens, err := attachment.NewTransferTokenService("0123456789abcdef0123456789abcdef", "ainp-test")
	if err != nil {
		t.Fatal(err)
	}
	policies := attachment.NewPolicyRegistry()
	if err := policies.Register(ai.NewPageCaptureAttachmentPolicy(db)); err != nil {
		t.Fatal(err)
	}
	attSvc := attachments.NewAttachmentService(db, objStore, transferTokens, policies, nil)

	contracts, err := ai.NewBuiltinAgentContractRegistry()
	if err != nil {
		t.Fatal(err)
	}
	resolver := stubVisionModelResolver{visionAliases: map[string]bool{"vision-chat": true}}
	toolRegistry := aitool.NewRegistry()
	signer, err := aitool.NewHandleSigner("0123456789abcdef0123456789abcdef")
	if err != nil {
		t.Fatal(err)
	}
	tools := ai.NewToolService(db, toolRegistry, signer, nil)

	adapters := aimodelruntime.NewAdapterRegistry()
	streamAdapter := &scriptedStreamAdapter{
		responses: [][]aimodelruntime.StreamEvent{
			{
				{Type: aimodelruntime.EventResponseStarted, Sequence: 1, ResponseID: "r1"},
				{Type: aimodelruntime.EventToolCallStarted, Sequence: 2, ResponseID: "r1", ItemID: "tc-item-1", Item: &aimodelruntime.OutputItem{ID: "tc-item-1", Type: aimodelruntime.PartToolCall, ToolCallID: "call-cap-1", ToolName: "builtin__page_capture"}},
				{Type: aimodelruntime.EventToolCallCompleted, Sequence: 3, ResponseID: "r1", ItemID: "tc-item-1", Item: &aimodelruntime.OutputItem{ID: "tc-item-1", Type: aimodelruntime.PartToolCall, Arguments: json.RawMessage(`{"reason":"check validation errors"}`)}},
				{Type: aimodelruntime.EventOutputItemCompleted, Sequence: 4, ResponseID: "r1", ItemID: "tc-item-1"},
				{Type: aimodelruntime.EventResponseCompleted, Sequence: 5, ResponseID: "r1", FinishReason: aimodelruntime.FinishToolCall},
			},
			{
				{Type: aimodelruntime.EventResponseStarted, Sequence: 1, ResponseID: "r2"},
				{Type: aimodelruntime.EventOutputItemStarted, Sequence: 2, ResponseID: "r2", ItemID: "msg-1", Item: &aimodelruntime.OutputItem{ID: "msg-1", Type: aimodelruntime.PartOutputText}},
				{Type: aimodelruntime.EventOutputTextDelta, Sequence: 3, ResponseID: "r2", ItemID: "msg-1", Delta: "我看到页面上有必填项红字提示，请先填写名称。"},
				{Type: aimodelruntime.EventOutputItemCompleted, Sequence: 4, ResponseID: "r2", ItemID: "msg-1"},
				{Type: aimodelruntime.EventResponseCompleted, Sequence: 5, ResponseID: "r2", FinishReason: aimodelruntime.FinishCompleted},
			},
		},
	}
	if err := adapters.Register(aimodel.ProtocolOpenAI, streamAdapter); err != nil {
		t.Fatal(err)
	}
	invocations := ai.NewInvocationService(db, resolver, adapters, nil)
	invocations.SetToolProjector(tools)
	invocations.SetAssetResolver(attSvc)

	conversations := ai.NewConversationService(db, attSvc)
	compactions := ai.NewContextCompactionService(db, conversations, resolver, invocations, toolRegistry)
	runtimeSvc := ai.NewAgentRuntimeService(db, conversations, invocations, tools, compactions, nil, staticAuthProvider{}, nil, stubRuntimeContextProvider{})
	runtimeSvc.SetBudgetProvider(ai.NewRuntimeBudgetProvider(nil))
	runtimeSvc.SetAgentContracts(contracts)
	runtimeSvc.SetPageCaptureDependencies(attSvc, attSvc)

	if err := tooling.RegisterBuiltins(toolRegistry, pagecapture.New(resolver, runtimeSvc.LookupRunModelAlias)); err != nil {
		t.Fatal(err)
	}
	metaDefs := []aitool.Definition{
		{ID: "meta:tool.search", Source: aitool.SourceMeta, Name: "tool.search", TitleForHuman: "工具搜索", DescriptionForLLM: "search", DescriptionForHuman: "search", Capability: "ai", InputSchema: json.RawMessage(`{"type":"object"}`), Risk: aitool.RiskLow, SideEffect: aitool.SideEffectRead, BindingMode: aitool.BindingOptional, LoadingMode: aitool.LoadingDirect, BindingGroup: "tool_discovery", RuntimeRef: "tool.search", SchemaVersion: 1, Status: "enabled"},
		{ID: "meta:tool.load", Source: aitool.SourceMeta, Name: "tool.load", TitleForHuman: "工具加载", DescriptionForLLM: "load", DescriptionForHuman: "load", Capability: "ai", InputSchema: json.RawMessage(`{"type":"object"}`), Risk: aitool.RiskLow, SideEffect: aitool.SideEffectRead, BindingMode: aitool.BindingOptional, LoadingMode: aitool.LoadingDirect, BindingGroup: "tool_discovery", RuntimeRef: "tool.load", SchemaVersion: 1, Status: "enabled"},
	}
	if err := toolRegistry.RegisterProvider(&aitool.StaticProvider{ProviderSource: aitool.SourceMeta, Definitions: metaDefs}, aitool.ExecuteFunc(func(context.Context, aitool.LoadedTool, json.RawMessage) (*aitool.Result, error) {
		return &aitool.Result{Status: "completed", Summary: "ok"}, nil
	})); err != nil {
		t.Fatal(err)
	}
	if err := toolRegistry.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}

	agentSvc := ai.NewAgentService(db, contracts, resolver, toolRegistry)
	fixtureDB, err := tenantdata.MigrationDB(db)
	if err != nil {
		t.Fatal(err)
	}
	if err := fixtureDB.Exec(`INSERT INTO tenants (id, code, name, status, created_at, updated_at) VALUES ('tenant-a', 'ta', 'Tenant A', 'enabled', now(), now()), ('tenant-b', 'tb', 'Tenant B', 'enabled', now(), now())`).Error; err != nil {
		t.Fatal(err)
	}
	if err := agentSvc.ReconcileBuiltinForTenant(context.Background(), db, "tenant-a", "user-a"); err != nil {
		t.Fatal(err)
	}

	ownerCtx := auth.WithContext(context.Background(), &auth.Context{
		Authenticated:  true,
		TenantID:       "tenant-a",
		PrincipalID:    "user-a",
		Principal:      auth.PrincipalRef{Kind: auth.PrincipalKindAccount, ID: "user-a"},
		ClientSurface:  auth.TenantConsoleSurface,
		Audience:       auth.TenantAudience,
		Realm:          auth.RealmTenant,
		SessionID:      "sess-owner",
		AuthVersion:    1,
		Permissions:    []string{ai.ActionAgentVersionPublish, ai.ActionAgentStatus},
		IdempotencyKey: "idem-setup",
	})
	tdbA, err := tenantdata.TenantDB(ownerCtx, db)
	if err != nil {
		t.Fatal(err)
	}
	agentsA, err := tenantdata.TenantRaw[ai.Agent](tdbA, `SELECT * FROM ai_agents WHERE tenant_id=? LIMIT 1`)
	if err != nil || len(agentsA) != 1 {
		t.Fatalf("builtin agent missing: %v %#v", err, agentsA)
	}
	builtinAgent := agentsA[0]

	// Check diagnostics for text-only vs vision model.
	if _, err := agentSvc.PublishVersion(ownerCtx, ai.AgentVersionCreateRequest{
		AgentID:    builtinAgent.ID,
		ModelAlias: "text-only",
		Revision:   builtinAgent.Revision,
	}); err != nil {
		t.Fatal(err)
	}
	diagVal, err := agentSvc.Diagnostics(ownerCtx, ai.IDRequest{ID: builtinAgent.ID})
	if err != nil {
		t.Fatal(err)
	}
	diagText := diagVal.(ai.AgentDiagnostics)
	if !diagText.Healthy || !slices.ContainsFunc(diagText.Warnings, func(w ai.AgentDiagnosticIssue) bool {
		return w.Code == "tool.vision_required" && w.Resource == pagecapture.ToolID
	}) {
		t.Fatalf("expected non-blocking tool.vision_required diagnostic warning for text-only model, got %#v", diagText)
	}

	// Publish vision version and enable agent.
	verVal, err := agentSvc.PublishVersion(ownerCtx, ai.AgentVersionCreateRequest{
		AgentID:    builtinAgent.ID,
		ModelAlias: "vision-chat",
		Revision:   builtinAgent.Revision + 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	visionVersion := verVal.(ai.AgentVersion)
	if _, err := agentSvc.Status(ownerCtx, ai.AgentStatusRequest{ID: builtinAgent.ID, Status: "enabled", Revision: builtinAgent.Revision + 2}); err != nil {
		t.Fatal(err)
	}

	convVal, err := conversations.Create(ownerCtx, ai.ConversationCreateRequest{
		AgentID:        builtinAgent.ID,
		AgentVersionID: visionVersion.ID,
		Title:          "Page Capture Guide",
	})
	if err != nil {
		t.Fatal(err)
	}
	conv := convVal.(ai.Conversation)

	msgCtx := *auth.MustFromContext(ownerCtx)
	msgCtx.IdempotencyKey = "msg-1"
	msgVal, err := conversations.CreateMessage(auth.WithContext(context.Background(), &msgCtx), ai.MessageCreateRequest{
		ConversationID: conv.ID,
		Role:           "user",
		Content:        []aimodelruntime.ContentPart{{Type: aimodelruntime.PartInputText, Text: "请引导我操作当前页面"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	userMsg := msgVal.(ai.ConversationMessage)

	runCtx := *auth.MustFromContext(ownerCtx)
	runCtx.IdempotencyKey = "run-1"
	runVal, err := runtimeSvc.Start(auth.WithContext(context.Background(), &runCtx), ai.AgentRunStartRequest{
		ConversationID:   conv.ID,
		TriggerMessageID: userMsg.ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	startedRun := runVal.(ai.AgentRun)

	// Step 1: Execute Run -> calls builtin__page_capture -> emits run.capture_requested.v1 and pauses Run.
	if err := runtimeSvc.Execute(ownerCtx, startedRun.ID, "worker-1"); err != nil {
		t.Fatalf("first execute failed: %v", err)
	}
	runsAfterPause, err := tenantdata.TenantRaw[ai.AgentRun](tdbA, `SELECT * FROM ai_agent_runs WHERE tenant_id=? AND id=? LIMIT 1`, startedRun.ID)
	if err != nil || len(runsAfterPause) != 1 || runsAfterPause[0].Status != string(aiagentruntime.RunPaused) {
		t.Fatalf("expected run to pause waiting for page capture, got %#v err=%v", runsAfterPause, err)
	}
	eventsVal, err := runtimeSvc.Events(ownerCtx, ai.AgentRunEventsRequest{ID: startedRun.ID})
	if err != nil {
		t.Fatal(err)
	}
	eventsItems := eventsVal.(map[string]any)["items"].([]ai.AgentRunEvent)
	var captureRequestID string
	for _, ev := range eventsItems {
		if ev.Type == aiagentruntime.RunEventCaptureRequest {
			var payload map[string]any
			_ = json.Unmarshal(ev.Payload, &payload)
			captureRequestID, _ = payload["capture_request_id"].(string)
		}
	}
	if captureRequestID == "" {
		t.Fatalf("expected %s event with capture_request_id, events=%#v", aiagentruntime.RunEventCaptureRequest, eventsItems)
	}

	// Upload captured PNG via standard prepare + CompleteUpload.
	pngBytes := makeSamplePNG(t)
	prepInput, _ := json.Marshal(map[string]any{
		"original_name": "page-capture.png",
		"content_type":  "image/png",
		"size":          len(pngBytes),
	})
	prepVal, err := attSvc.UploadPrepare(ownerCtx, prepInput)
	if err != nil {
		t.Fatal(err)
	}
	prepView := prepVal.(attachments.TransferPrepareView)
	claims, err := attSvc.ParseTransferToken(prepView.Token)
	if err != nil {
		t.Fatal(err)
	}
	completedAtt, err := attSvc.CompleteUpload(ownerCtx, claims, bytes.NewReader(pngBytes))
	if err != nil {
		t.Fatal(err)
	}

	// Security boundary: cross-tenant, non-owner, and forged request ID must fail-closed.
	otherUserCtx := auth.WithContext(context.Background(), &auth.Context{
		Authenticated: true, TenantID: "tenant-a", PrincipalID: "user-other",
		Principal:     auth.PrincipalRef{Kind: auth.PrincipalKindAccount, ID: "user-other"},
		ClientSurface: auth.TenantConsoleSurface, Audience: auth.TenantAudience,
		Realm: auth.RealmTenant, SessionID: "sess-other", AuthVersion: 1,
	})
	if _, err := runtimeSvc.SubmitPageCapture(otherUserCtx, ai.PageCaptureSubmitRequest{
		CaptureRequestID: captureRequestID, AttachmentID: completedAtt.ID, AttachmentRevision: completedAtt.SHA256,
	}); !isAppErrorCode(err, errs.CodeNotFound) {
		t.Fatalf("non-owner submit must fail-closed with not found, got %v", err)
	}
	otherTenantCtx := auth.WithContext(context.Background(), &auth.Context{
		Authenticated: true, TenantID: "tenant-b", PrincipalID: "user-a",
		Principal:     auth.PrincipalRef{Kind: auth.PrincipalKindAccount, ID: "user-a"},
		ClientSurface: auth.TenantConsoleSurface, Audience: auth.TenantAudience,
		Realm: auth.RealmTenant, SessionID: "sess-owner", AuthVersion: 1,
	})
	if _, err := runtimeSvc.SubmitPageCapture(otherTenantCtx, ai.PageCaptureSubmitRequest{
		CaptureRequestID: captureRequestID, AttachmentID: completedAtt.ID, AttachmentRevision: completedAtt.SHA256,
	}); !isAppErrorCode(err, errs.CodeNotFound) {
		t.Fatalf("cross-tenant submit must fail-closed with not found, got %v", err)
	}
	if _, err := runtimeSvc.SubmitPageCapture(ownerCtx, ai.PageCaptureSubmitRequest{
		CaptureRequestID: "forged-capture-id", AttachmentID: completedAtt.ID, AttachmentRevision: completedAtt.SHA256,
	}); !isAppErrorCode(err, errs.CodeNotFound) {
		t.Fatalf("forged capture request submit must fail-closed, got %v", err)
	}

	// Owner submits page capture -> binds ai.page_capture resource and requeues Run.
	submitVal, err := runtimeSvc.SubmitPageCapture(ownerCtx, ai.PageCaptureSubmitRequest{
		CaptureRequestID:   captureRequestID,
		Status:             "completed",
		AttachmentID:       completedAtt.ID,
		AttachmentRevision: completedAtt.SHA256,
	})
	if err != nil {
		t.Fatalf("owner submit failed: %v", err)
	}
	submitResp := submitVal.(ai.PageCaptureSubmitResponse)
	if submitResp.Status != "submitted" {
		t.Fatalf("unexpected submit response: %#v", submitResp)
	}

	// Duplicate submit with same attachment is idempotent no-op.
	dupVal, err := runtimeSvc.SubmitPageCapture(ownerCtx, ai.PageCaptureSubmitRequest{
		CaptureRequestID:   captureRequestID,
		Status:             "completed",
		AttachmentID:       completedAtt.ID,
		AttachmentRevision: completedAtt.SHA256,
	})
	if err != nil || dupVal.(ai.PageCaptureSubmitResponse).CaptureRequestID != captureRequestID {
		t.Fatalf("duplicate submit must be idempotent, val=%#v err=%v", dupVal, err)
	}

	// Attachment AccessPolicy: owner can download, other user in same tenant cannot.
	dlReq, _ := json.Marshal(map[string]any{"id": completedAtt.ID})
	if _, err := attSvc.DownloadPrepare(ownerCtx, dlReq); err != nil {
		t.Fatalf("owner download prepare failed: %v", err)
	}
	if _, err := attSvc.DownloadPrepare(otherUserCtx, dlReq); err == nil {
		t.Fatal("non-owner must not read ai.page_capture attachment")
	}

	// Step 2: Execute resumed Run -> consumes capture ToolResult, projects input_image to model, completes Run.
	if err := runtimeSvc.Execute(ownerCtx, startedRun.ID, "worker-2"); err != nil {
		t.Fatalf("resumed execute failed: %v", err)
	}
	finalRuns, err := tenantdata.TenantRaw[ai.AgentRun](tdbA, `SELECT * FROM ai_agent_runs WHERE tenant_id=? AND id=? LIMIT 1`, startedRun.ID)
	if err != nil || len(finalRuns) != 1 || finalRuns[0].Status != string(aiagentruntime.RunCompleted) {
		t.Fatalf("expected completed run, got %#v err=%v", finalRuns, err)
	}

	// Verify second model request received projected input_image with resolved PNG bytes.
	if len(streamAdapter.requests) != 2 {
		t.Fatalf("expected 2 model stream calls, got %d", len(streamAdapter.requests))
	}
	foundProjectedImage := false
	for _, m := range streamAdapter.requests[1].Messages {
		for _, p := range m.Content {
			if p.Type == aimodelruntime.PartInputImage && p.ImageAttachmentID == completedAtt.ID && p.AttachmentRevision == completedAtt.SHA256 {
				foundProjectedImage = true
			}
		}
	}
	if !foundProjectedImage {
		t.Fatalf("second model call did not receive projected page capture input_image: %#v", streamAdapter.requests[1].Messages)
	}

	// Verify canonical ToolResult only stores attachment ID, revision, and safe status (no object_key, URL, token, or base64).
	msgsVal, err := conversations.ListMessages(ownerCtx, ai.MessageListRequest{ConversationID: conv.ID, Limit: 50})
	if err != nil {
		t.Fatal(err)
	}
	msgItems := msgsVal.(map[string]any)["items"].([]ai.ConversationMessage)
	foundToolResult := false
	for _, item := range msgItems {
		raw := string(item.Content)
		for _, forbidden := range []string{completedAtt.ObjectKey, prepView.Token, "/api/v1/attachments/transfer/", "iVBORw0KGgo"} {
			if forbidden != "" && strings.Contains(raw, forbidden) {
				t.Fatalf("canonical message leaked forbidden field %q: %s", forbidden, raw)
			}
		}
		if item.Role == "tool" && strings.Contains(raw, completedAtt.ID) && strings.Contains(raw, `"capture_status":"ready"`) {
			foundToolResult = true
		}
	}
	if !foundToolResult {
		t.Fatalf("canonical tool result missing safe attachment reference: %#v", msgItems)
	}

	// Verify logs do not leak object_key, transfer token, or base64 image content.
	logging.Info(ownerCtx, "page capture verification completed", "run_id", startedRun.ID)
	logs := logBuf.String()
	for _, forbidden := range []string{completedAtt.ObjectKey, prepView.Token, "iVBORw0KGgo"} {
		if forbidden != "" && strings.Contains(logs, forbidden) {
			t.Fatalf("structured logs leaked sensitive media artifact %q: %s", forbidden, logs)
		}
	}

	// Verify timeout fail-closed behavior and per-run MaxCallsPerRun=3 budget.
	streamAdapter.calls = 0
	streamAdapter.responses = [][]aimodelruntime.StreamEvent{
		{
			{Type: aimodelruntime.EventResponseStarted, Sequence: 1, ResponseID: "rt1"},
			{Type: aimodelruntime.EventToolCallStarted, Sequence: 2, ResponseID: "rt1", ItemID: "tc-t1", Item: &aimodelruntime.OutputItem{ID: "tc-t1", Type: aimodelruntime.PartToolCall, ToolCallID: "call-timeout-1", ToolName: "builtin__page_capture"}},
			{Type: aimodelruntime.EventToolCallCompleted, Sequence: 3, ResponseID: "rt1", ItemID: "tc-t1", Item: &aimodelruntime.OutputItem{ID: "tc-t1", Type: aimodelruntime.PartToolCall, Arguments: json.RawMessage(`{}`)}},
			{Type: aimodelruntime.EventOutputItemCompleted, Sequence: 4, ResponseID: "rt1", ItemID: "tc-t1"},
			{Type: aimodelruntime.EventResponseCompleted, Sequence: 5, ResponseID: "rt1", FinishReason: aimodelruntime.FinishToolCall},
		},
		{
			{Type: aimodelruntime.EventResponseStarted, Sequence: 1, ResponseID: "rt2"},
			{Type: aimodelruntime.EventOutputItemStarted, Sequence: 2, ResponseID: "rt2", ItemID: "m-t2", Item: &aimodelruntime.OutputItem{ID: "m-t2", Type: aimodelruntime.PartOutputText}},
			{Type: aimodelruntime.EventOutputTextDelta, Sequence: 3, ResponseID: "rt2", ItemID: "m-t2", Delta: "页面捕获超时，请直接描述页面状态。"},
			{Type: aimodelruntime.EventOutputItemCompleted, Sequence: 4, ResponseID: "rt2", ItemID: "m-t2"},
			{Type: aimodelruntime.EventResponseCompleted, Sequence: 5, ResponseID: "rt2", FinishReason: aimodelruntime.FinishCompleted},
		},
	}
	msgCtx2 := *auth.MustFromContext(ownerCtx)
	msgCtx2.IdempotencyKey = "msg-timeout"
	msgVal2, err := conversations.CreateMessage(auth.WithContext(context.Background(), &msgCtx2), ai.MessageCreateRequest{
		ConversationID: conv.ID,
		Role:           "user",
		Content:        []aimodelruntime.ContentPart{{Type: aimodelruntime.PartInputText, Text: "再看一眼当前页面"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	runCtx2 := *auth.MustFromContext(ownerCtx)
	runCtx2.IdempotencyKey = "run-timeout"
	runVal2, err := runtimeSvc.Start(auth.WithContext(context.Background(), &runCtx2), ai.AgentRunStartRequest{
		ConversationID:   conv.ID,
		TriggerMessageID: msgVal2.(ai.ConversationMessage).ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	timeoutRun := runVal2.(ai.AgentRun)
	if err := runtimeSvc.Execute(ownerCtx, timeoutRun.ID, "worker-timeout-1"); err != nil {
		t.Fatal(err)
	}
	// Expire the pending capture request in DB and trigger WakeExpiredPageCapture + Execute.
	expDB, _ := tenantdata.MigrationDB(db)
	if err := expDB.Exec(`UPDATE ai_page_capture_requests SET expires_at = now() - interval '5 seconds' WHERE run_id = ?`, timeoutRun.ID).Error; err != nil {
		t.Fatal(err)
	}
	woke, err := runtimeSvc.WakeExpiredPageCapture(ownerCtx, timeoutRun.ID)
	if err != nil || !woke {
		t.Fatalf("expected WakeExpiredPageCapture to wake run, woke=%v err=%v", woke, err)
	}
	if err := runtimeSvc.Execute(ownerCtx, timeoutRun.ID, "worker-timeout-2"); err != nil {
		t.Fatal(err)
	}
	capturesAfterTimeout, err := tenantdata.TenantRaw[ai.PageCaptureRequest](tdbA, `SELECT * FROM ai_page_capture_requests WHERE tenant_id=? AND run_id=? LIMIT 1`, timeoutRun.ID)
	if err != nil || len(capturesAfterTimeout) != 1 || capturesAfterTimeout[0].Status != "expired" || capturesAfterTimeout[0].ErrorCategory != "capture_timeout" {
		t.Fatalf("expected expired capture_timeout state, got %#v err=%v", capturesAfterTimeout, err)
	}

	// Verify MaxCallsPerRun=3 budget enforcement in ToolService.
	budgetRunID := "run-budget-check"
	budgetScope := tooling.ToolScope{
		ID:               "scope-budget-check",
		TenantID:         "tenant-a",
		PrincipalID:      "user-a",
		PrincipalKind:    string(auth.PrincipalKindAccount),
		ClientSurface:    auth.TenantConsoleSurface,
		Audience:         auth.TenantAudience,
		Realm:            string(auth.RealmTenant),
		SessionID:        "sess-owner",
		AuthVersion:      1,
		AgentRunID:       &budgetRunID,
		AllowedToolsJSON: json.RawMessage(`["builtin:page.capture"]`),
		LoadedToolsJSON:  json.RawMessage(`[]`),
		ExpiresAt:        time.Now().UTC().Add(10 * time.Minute),
	}
	scopeDB, _ := tenantdata.MigrationDB(db)
	if err := scopeDB.Create(&budgetScope).Error; err != nil {
		t.Fatal(err)
	}
	budgetCtx := aimodelruntime.WithModelAlias(ownerCtx, "vision-chat")
	loadVal, err := tools.Load(budgetCtx, tooling.ToolLoadRequest{ScopeID: budgetScope.ID, ToolIDs: []string{pagecapture.ToolID}})
	if err != nil {
		t.Fatal(err)
	}
	handle := loadVal.(tooling.ToolLoadResponse).Tools[0].Handle
	for i := 1; i <= pagecapture.DefaultMaxCallsPerRun; i++ {
		execVal, execErr := tools.Execute(budgetCtx, tooling.ToolExecuteRequest{
			Handle:     handle,
			ToolCallID: fmt.Sprintf("budget-call-%d", i),
			Arguments:  json.RawMessage(`{}`),
		})
		if execErr != nil || execVal.(*aitool.Result).Status != aitool.ResultStatusClientFulfillmentRequired {
			t.Fatalf("call %d should succeed as client_fulfillment_required, got %#v err=%v", i, execVal, execErr)
		}
	}
	fourthVal, err := tools.Execute(budgetCtx, tooling.ToolExecuteRequest{
		Handle:     handle,
		ToolCallID: "budget-call-4",
		Arguments:  json.RawMessage(`{}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	fourthResult := fourthVal.(*aitool.Result)
	if fourthResult.Status != aitool.ResultStatusFailed || fourthResult.ErrorCategory != "page_capture_budget_exhausted" {
		t.Fatalf("4th call in same run must fail with page_capture_budget_exhausted, got %#v", fourthResult)
	}

	// Verify ContextAssembler projects capture ToolResult into native image working set.
	assembled, err := conversations.Assemble(ownerCtx, aiconversation.AssemblyRequest{
		ConversationID:   conv.ID,
		TriggerMessageID: userMsg.ID,
		MaxTokens:        32000,
		MaxBytes:         256000,
		MaxNativeImages:  4,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.ContainsFunc(assembled.AttachmentManifest.Attachments, func(ref aiconversation.AttachmentReference) bool {
		return ref.AttachmentID == completedAtt.ID && ref.ContentKind == "image"
	}) {
		t.Fatalf("assembled manifest missing captured image reference: %#v", assembled.AttachmentManifest)
	}
}
