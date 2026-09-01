package tests

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/yzxingtu2026/ainp/platform/framework/aimodel"
	"github.com/yzxingtu2026/ainp/platform/framework/aimodelruntime"
	"github.com/yzxingtu2026/ainp/platform/framework/auth"
	"github.com/yzxingtu2026/ainp/platform/framework/config"
	"github.com/yzxingtu2026/ainp/platform/framework/database"
	"github.com/yzxingtu2026/ainp/platform/framework/errs"
	"github.com/yzxingtu2026/ainp/platform/framework/outbox"
	tenantdata "github.com/yzxingtu2026/ainp/platform/framework/tenant"
	"github.com/yzxingtu2026/ainp/platform/modules/ai"
)

func TestPGVectorDatabaseValueKeepsAIRootCompatibility(t *testing.T) {
	var legacy ai.PGVector = database.PGVector{0.25, -1.5, 3}
	value, err := legacy.Value()
	if err != nil || value != "[0.25,-1.5,3]" {
		t.Fatalf("unexpected pgvector value: %v %v", value, err)
	}
	var decoded database.PGVector
	if err = decoded.Scan([]byte("[0.25,-1.5,3]")); err != nil {
		t.Fatal(err)
	}
	if len(decoded) != len(legacy) || decoded[0] != legacy[0] || decoded[1] != legacy[1] || decoded[2] != legacy[2] {
		t.Fatalf("pgvector round trip mismatch: %#v", decoded)
	}
}

func TestEmbeddingAdapterRegistryFailsClosed(t *testing.T) {
	r := aimodelruntime.NewEmbeddingAdapterRegistry()
	if _, ok := r.Get("missing"); ok {
		t.Fatal("unknown protocol must not resolve")
	}
	if err := r.Register(aimodel.ProtocolOpenAI, aimodelruntime.NewFakeEmbeddingAdapter(8)); err != nil {
		t.Fatal(err)
	}
	if err := r.Register(aimodel.ProtocolOpenAI, aimodelruntime.NewFakeEmbeddingAdapter(8)); err == nil {
		t.Fatal("duplicate protocol must fail")
	}
}

func TestEmbeddingRequestValidation(t *testing.T) {
	req := aimodelruntime.EmbeddingRequest{
		ModelAlias: "embed-default",
		Purpose:    aimodelruntime.EmbeddingPurposeToolIndex,
		Inputs:     []aimodelruntime.EmbeddingItem{{ID: "a", Text: "hello"}},
	}
	req.Normalize()
	if err := req.Validate(context.Background()); err != nil {
		t.Fatal(err)
	}
	req.Inputs = append(req.Inputs, aimodelruntime.EmbeddingItem{ID: "a", Text: "dup"})
	if err := req.Validate(context.Background()); err == nil {
		t.Fatal("duplicate item id must fail")
	}
	req.Inputs = []aimodelruntime.EmbeddingItem{{ID: "b", Text: "   "}}
	if err := req.Validate(context.Background()); err == nil {
		t.Fatal("blank text must fail")
	}
	req.Inputs = []aimodelruntime.EmbeddingItem{{ID: "c", Text: strings.Repeat("x", aimodelruntime.MaxEmbeddingItemBytes+1)}}
	if err := req.Validate(context.Background()); err == nil {
		t.Fatal("oversized item must fail before provider call")
	}
}

func TestFakeEmbeddingAdapterDeterministicAndOrdered(t *testing.T) {
	adapter := aimodelruntime.NewFakeEmbeddingAdapter(4)
	in := aimodelruntime.EmbeddingAdapterRequest{
		Request: aimodelruntime.EmbeddingRequest{
			ModelAlias: "embed",
			Purpose:    aimodelruntime.EmbeddingPurposeOther,
			Inputs: []aimodelruntime.EmbeddingItem{
				{ID: "i1", Text: "alpha"},
				{ID: "i2", Text: "beta"},
			},
		},
		Model: &aimodel.ResolvedModel{Protocol: "fake", DeploymentID: "d1", RemoteModelID: "fake-embed"},
	}
	first, err := adapter.Embed(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	second, err := adapter.Embed(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	if first.Dimensions != 4 || len(first.Vectors) != 2 || first.Vectors[0].ID != "i1" || first.Vectors[1].Index != 1 {
		t.Fatalf("unexpected response: %#v", first)
	}
	if fmt.Sprint(first.Vectors[0].Vector) != fmt.Sprint(second.Vectors[0].Vector) {
		t.Fatal("fake adapter must be deterministic")
	}
}

func TestOpenAIEmbeddingsFixtureMappingAndOrder(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/embeddings" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer secret" {
			t.Fatal("missing auth header")
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body["model"] != "text-embedding-3-small" {
			t.Fatalf("unexpected model: %#v", body["model"])
		}
		input, _ := body["input"].([]any)
		if len(input) != 2 || input[0] != "one" || input[1] != "two" {
			t.Fatalf("unexpected input: %#v", body["input"])
		}
		if body["dimensions"] != float64(3) {
			t.Fatalf("unexpected dimensions: %#v", body["dimensions"])
		}
		_, _ = io.WriteString(w, `{"id":"emb_1","data":[{"index":1,"embedding":[0.4,0.5,0.6]},{"index":0,"embedding":[0.1,0.2,0.3]}],"usage":{"prompt_tokens":5,"total_tokens":5}}`)
	}))
	defer server.Close()
	dims := 3
	adapter := aimodelruntime.NewOpenAIEmbeddingAdapter(server.Client())
	resp, err := adapter.Embed(context.Background(), aimodelruntime.EmbeddingAdapterRequest{
		Request: aimodelruntime.EmbeddingRequest{
			ModelAlias: "embed",
			Dimensions: &dims,
			Inputs:     []aimodelruntime.EmbeddingItem{{ID: "a", Text: "one"}, {ID: "b", Text: "two"}},
		},
		Model: &aimodel.ResolvedModel{Protocol: aimodel.ProtocolOpenAI, BaseURL: server.URL, Credential: "secret", DeploymentID: "dep", RemoteModelID: "text-embedding-3-small"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Dimensions != 3 || resp.Vectors[0].ID != "a" || resp.Vectors[0].Vector[0] != 0.1 || resp.Vectors[1].Vector[0] != 0.4 {
		t.Fatalf("order/dimensions mismatch: %#v", resp)
	}
	if resp.Usage.TotalTokens == nil || *resp.Usage.TotalTokens != 5 || !resp.Usage.Reported {
		t.Fatalf("unexpected usage: %#v", resp.Usage)
	}
	if resp.SubBatchCount != 1 {
		t.Fatalf("unexpected sub batches: %d", resp.SubBatchCount)
	}
}

func TestOpenAICompatibleEmbeddingRequiresEndpoint(t *testing.T) {
	adapter := aimodelruntime.NewOpenAICompatibleEmbeddingAdapter(http.DefaultClient)
	_, err := adapter.Embed(context.Background(), aimodelruntime.EmbeddingAdapterRequest{
		Request: aimodelruntime.EmbeddingRequest{ModelAlias: "embed", Inputs: []aimodelruntime.EmbeddingItem{{ID: "a", Text: "x"}}},
		Model:   &aimodel.ResolvedModel{Protocol: aimodel.ProtocolOpenAICompatible, Credential: "secret", RemoteModelID: "local-embed"},
	})
	if err == nil || !strings.Contains(err.Error(), "endpoint is required") {
		t.Fatalf("compatible adapter must require endpoint, got %v", err)
	}
}

func TestOpenAICustomEndpointUsesCompatibleBatchLimit(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		var body struct {
			Input []string `json:"input"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if len(body.Input) > aimodelruntime.OpenAICompatibleBatchItems {
			t.Fatalf("compatible batch contains %d items", len(body.Input))
		}
		items := make([]string, 0, len(body.Input))
		for index := range body.Input {
			items = append(items, fmt.Sprintf(`{"index":%d,"embedding":[%d]}`, index, (calls-1)*aimodelruntime.OpenAICompatibleBatchItems+index))
		}
		_, _ = io.WriteString(w, `{"data":[`+strings.Join(items, ",")+`]}`)
	}))
	defer server.Close()
	inputs := make([]aimodelruntime.EmbeddingItem, aimodelruntime.OpenAICompatibleBatchItems+1)
	for index := range inputs {
		inputs[index] = aimodelruntime.EmbeddingItem{ID: fmt.Sprintf("item-%d", index), Text: "text"}
	}
	response, err := aimodelruntime.NewOpenAIEmbeddingAdapter(server.Client()).Embed(context.Background(), aimodelruntime.EmbeddingAdapterRequest{
		Request: aimodelruntime.EmbeddingRequest{ModelAlias: "embed", Inputs: inputs},
		Model:   &aimodel.ResolvedModel{BaseURL: server.URL, Credential: "secret", DeploymentID: "dep", RemoteModelID: "model"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if calls != 2 || response.SubBatchCount != 2 || response.Vectors[len(response.Vectors)-1].ID != "item-10" {
		t.Fatalf("unexpected compatible sub-batches: calls=%d response=%#v", calls, response)
	}
}

func TestOpenAIEmbeddingRejectsInvalidVectorsAndMismatch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"data":[{"index":0,"embedding":[1,2]},{"index":1,"embedding":[3]}]}`)
	}))
	defer server.Close()
	_, err := aimodelruntime.NewOpenAIEmbeddingAdapter(server.Client()).Embed(context.Background(), aimodelruntime.EmbeddingAdapterRequest{
		Request: aimodelruntime.EmbeddingRequest{ModelAlias: "embed", Inputs: []aimodelruntime.EmbeddingItem{{ID: "a", Text: "one"}, {ID: "b", Text: "two"}}},
		Model:   &aimodel.ResolvedModel{BaseURL: server.URL, Credential: "secret", RemoteModelID: "m"},
	})
	if err == nil {
		t.Fatal("dimension mismatch must fail")
	}
	var re *aimodelruntime.RuntimeError
	if !asRuntimeError(err, &re) || re.Category != aimodelruntime.ErrorInvalidVector {
		t.Fatalf("expected invalid_vector, got %v", err)
	}
}

func TestOpenAIEmbeddingRejectsNaN(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, fmt.Sprintf(`{"data":[{"index":0,"embedding":[%g,1]}]}`, math.NaN()))
	}))
	defer server.Close()
	_, err := aimodelruntime.NewOpenAIEmbeddingAdapter(server.Client()).Embed(context.Background(), aimodelruntime.EmbeddingAdapterRequest{
		Request: aimodelruntime.EmbeddingRequest{ModelAlias: "embed", Inputs: []aimodelruntime.EmbeddingItem{{ID: "a", Text: "one"}}},
		Model:   &aimodel.ResolvedModel{BaseURL: server.URL, Credential: "secret", RemoteModelID: "m"},
	})
	if err == nil {
		t.Fatal("NaN must fail")
	}
}

func TestOpenAIEmbeddingSubBatchPreservesOrderAndFailsClosed(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		var body struct {
			Input []string `json:"input"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if calls == 2 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		items := make([]string, 0, len(body.Input))
		for i := range body.Input {
			items = append(items, fmt.Sprintf(`{"index":%d,"embedding":[%d.0]}`, i, calls*10+i))
		}
		_, _ = io.WriteString(w, `{"data":[`+strings.Join(items, ",")+`],"usage":{"prompt_tokens":1,"total_tokens":1}}`)
	}))
	defer server.Close()
	adapter := aimodelruntime.NewOpenAIEmbeddingAdapter(server.Client())
	adapter.MaxBatchItems = 1
	_, err := adapter.Embed(context.Background(), aimodelruntime.EmbeddingAdapterRequest{
		Request: aimodelruntime.EmbeddingRequest{ModelAlias: "embed", Inputs: []aimodelruntime.EmbeddingItem{{ID: "a", Text: "one"}, {ID: "b", Text: "two"}}},
		Model:   &aimodel.ResolvedModel{BaseURL: server.URL, Credential: "secret", RemoteModelID: "m"},
	})
	if err == nil {
		t.Fatal("sub-batch failure must fail the whole logical request")
	}
	if calls != 2 {
		t.Fatalf("expected two provider calls, got %d", calls)
	}

	calls = 0
	okServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		var body struct {
			Input []string `json:"input"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		_, _ = io.WriteString(w, fmt.Sprintf(`{"data":[{"index":0,"embedding":[%d.0]}],"usage":{"prompt_tokens":2,"total_tokens":2}}`, calls))
	}))
	defer okServer.Close()
	adapter = aimodelruntime.NewOpenAIEmbeddingAdapter(okServer.Client())
	adapter.MaxBatchItems = 1
	resp, err := adapter.Embed(context.Background(), aimodelruntime.EmbeddingAdapterRequest{
		Request: aimodelruntime.EmbeddingRequest{ModelAlias: "embed", Inputs: []aimodelruntime.EmbeddingItem{{ID: "a", Text: "one"}, {ID: "b", Text: "two"}}},
		Model:   &aimodel.ResolvedModel{BaseURL: okServer.URL, Credential: "secret", RemoteModelID: "m"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.SubBatchCount != 2 || resp.Vectors[0].Vector[0] != 1 || resp.Vectors[1].Vector[0] != 2 || resp.Vectors[1].Index != 1 {
		t.Fatalf("unexpected sub-batch merge: %#v", resp)
	}
	if resp.Usage.TotalTokens == nil || *resp.Usage.TotalTokens != 4 {
		t.Fatalf("usage must accumulate across sub-batches: %#v", resp.Usage)
	}
}

func TestEmbeddingCosineNormalizeIsExplicit(t *testing.T) {
	adapter := aimodelruntime.NewFakeEmbeddingAdapter(2)
	in := aimodelruntime.EmbeddingAdapterRequest{
		Request: aimodelruntime.EmbeddingRequest{
			ModelAlias:    "embed",
			Preprocessing: aimodelruntime.EmbeddingPreprocessingCosineNorm,
			Inputs:        []aimodelruntime.EmbeddingItem{{ID: "a", Text: "normalize-me"}},
		},
		Model: &aimodel.ResolvedModel{DeploymentID: "d", RemoteModelID: "m"},
	}
	resp, err := adapter.Embed(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	var sum float64
	for _, v := range resp.Vectors[0].Vector {
		sum += float64(v) * float64(v)
	}
	if math.Abs(sum-1) > 1e-5 {
		t.Fatalf("expected unit vector, got norm^2=%f", sum)
	}
	if resp.PreprocessingVersion != aimodelruntime.EmbeddingPreprocessingVersionCosine {
		t.Fatalf("missing preprocessing version: %#v", resp)
	}
}

func TestModelCatalogAndEmbeddingRuntimePostgresInvocationAndTenantIsolation(t *testing.T) {
	dsn := os.Getenv("AINP_TEST_DATABASE_DSN")
	if dsn == "" {
		t.Skip("AINP_TEST_DATABASE_DSN not set")
	}
	schema := "ainp_embed_" + strings.ReplaceAll(time.Now().UTC().Format("150405.000000"), ".", "")
	admin, err := database.Open(dsn)
	if err != nil {
		t.Fatal(err)
	}
	if err = admin.Exec(fmt.Sprintf(`CREATE SCHEMA %q`, schema)).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = admin.Exec(fmt.Sprintf(`DROP SCHEMA %q CASCADE`, schema)).Error })
	db, err := database.Open(dsn + " search_path=" + schema)
	if err != nil {
		t.Fatal(err)
	}
	if err = db.Exec(`CREATE TABLE tenants(id varchar(64) PRIMARY KEY)`).Error; err != nil {
		t.Fatal(err)
	}
	if err = db.Exec(`INSERT INTO tenants(id) VALUES ('t1'),('t2')`).Error; err != nil {
		t.Fatal(err)
	}
	if err = ai.AutoMigrate(db); err != nil {
		t.Fatal(err)
	}
	if err = outbox.AutoMigrate(db); err != nil {
		t.Fatal(err)
	}
	rt, err := outbox.NewRuntime(db, config.OutboxConfig{})
	if err != nil {
		t.Fatal(err)
	}
	if err = rt.RegisterDefinition(outbox.Definition{EventType: "ai.model_invocation.finished.v1", SchemaVersion: 1}); err != nil {
		t.Fatal(err)
	}
	key := base64.StdEncoding.EncodeToString([]byte("0123456789abcdef0123456789abcdef"))
	cipher, err := aimodel.NewAESGCMCredentialCipher(key)
	if err != nil {
		t.Fatal(err)
	}
	types := aimodel.NewProviderTypeRegistry()
	if err = aimodel.RegisterBuiltinProviderTypes(types); err != nil {
		t.Fatal(err)
	}
	svc := ai.NewService(db, types, cipher)
	ctx1 := auth.WithContext(context.Background(), &auth.Context{Authenticated: true, TenantID: "t1", PrincipalID: "u1", SessionID: "s1", Realm: auth.RealmTenant, RequestID: "req-1"})
	createdProvider, err := svc.CreateProvider(ctx1, ai.ProviderCreateRequest{Code: "openai-local", Name: "OpenAI", Protocol: aimodel.ProtocolOpenAI, Credential: "sk-test", Status: "enabled"})
	if err != nil {
		t.Fatal(err)
	}
	provider := createdProvider.(ai.Provider)
	invalidChat := ai.DeploymentCreateRequest{
		ProviderID: provider.ID, Code: "chat-invalid", RemoteModelID: "chat-invalid", DisplayName: "Invalid Chat",
		ModelType: aimodel.ModelTypeChat, Limits: aimodel.Limits{ContextWindow: 128000, MaxOutputTokens: 1000000}, Status: "enabled",
	}
	if _, err = svc.CreateDeployment(ctx1, invalidChat); errs.StableCategory(err) != aimodel.ContextBudgetInvalid {
		t.Fatalf("create accepted invalid chat limits: %#v", err)
	}
	if _, err = svc.ImportDeployments(ctx1, ai.ImportRequest{Items: []ai.DeploymentCreateRequest{invalidChat}}); errs.StableCategory(err) != aimodel.ContextBudgetInvalid {
		t.Fatalf("import accepted invalid chat limits: %#v", err)
	}
	disabledChatRequest := invalidChat
	disabledChatRequest.Code = "chat-disabled-invalid"
	disabledChatRequest.RemoteModelID = "chat-disabled-invalid"
	disabledChatRequest.Status = "disabled"
	disabledChatValue, err := svc.CreateDeployment(ctx1, disabledChatRequest)
	if err != nil {
		t.Fatal(err)
	}
	disabledChat := disabledChatValue.(ai.Deployment)
	if _, err = svc.UpdateDeploymentStatus(ctx1, ai.StatusRequest{ID: disabledChat.ID, Status: "enabled"}); errs.StableCategory(err) != aimodel.ContextBudgetInvalid {
		t.Fatalf("enable accepted invalid chat limits: %#v", err)
	}
	validChatValue, err := svc.CreateDeployment(ctx1, ai.DeploymentCreateRequest{
		ProviderID: provider.ID, Code: "chat-valid", RemoteModelID: "chat-valid", DisplayName: "Valid Chat",
		ModelType: aimodel.ModelTypeChat, Limits: aimodel.Limits{ContextWindow: 128000, MaxOutputTokens: 8192}, Status: "enabled",
	})
	if err != nil {
		t.Fatal(err)
	}
	validChat := validChatValue.(ai.Deployment)
	if _, err = svc.UpdateDeployment(ctx1, ai.DeploymentUpdateRequest{
		ID: validChat.ID, DisplayName: validChat.DisplayName, ModelType: aimodel.ModelTypeChat, Limits: invalidChat.Limits,
	}); errs.StableCategory(err) != aimodel.ContextBudgetInvalid {
		t.Fatalf("update accepted invalid chat limits: %#v", err)
	}
	if _, err = svc.CreateDeployment(ctx1, ai.DeploymentCreateRequest{
		ProviderID: provider.ID, Code: "embed-invalid", RemoteModelID: "embedding-invalid", DisplayName: "Invalid Embed",
		ModelType: aimodel.ModelTypeEmbedding, Capabilities: aimodel.Capabilities{InputModalities: []aimodel.Modality{aimodel.ModalityText}, Features: []aimodel.Feature{aimodel.FeatureStreaming}}, Status: "enabled",
	}); err == nil {
		t.Fatal("enabled embedding deployment without embeddings feature must fail")
	}
	disabledDeployment, err := svc.CreateDeployment(ctx1, ai.DeploymentCreateRequest{
		ProviderID: provider.ID, Code: "embed-disabled-invalid", RemoteModelID: "embedding-disabled-invalid", DisplayName: "Disabled Invalid Embed",
		ModelType: aimodel.ModelTypeEmbedding, Capabilities: aimodel.Capabilities{InputModalities: []aimodel.Modality{aimodel.ModalityText}}, Status: "disabled",
	})
	if err != nil {
		t.Fatal(err)
	}
	disabled := disabledDeployment.(ai.Deployment)
	if _, err = svc.UpdateDeploymentStatus(ctx1, ai.StatusRequest{ID: disabled.ID, Status: "enabled"}); err == nil {
		t.Fatal("invalid stored embedding deployment must not be enabled")
	}
	if _, err = svc.UpsertAlias(ctx1, ai.AliasUpsertRequest{Code: "embed-invalid", DeploymentID: disabled.ID}); err == nil {
		t.Fatal("alias must not target disabled embedding deployment")
	}
	createdDeployment, err := svc.CreateDeployment(ctx1, ai.DeploymentCreateRequest{
		ProviderID: provider.ID, Code: "embed-small", RemoteModelID: "text-embedding-3-small", DisplayName: "Embed",
		ModelType:    aimodel.ModelTypeEmbedding,
		Capabilities: aimodel.Capabilities{InputModalities: []aimodel.Modality{aimodel.ModalityText}, Features: []aimodel.Feature{aimodel.FeatureEmbeddings}},
		Limits:       aimodel.Limits{Dimensions: 8, MaxDimensions: 16},
		Status:       "enabled",
	})
	if err != nil {
		t.Fatal(err)
	}
	deployment := createdDeployment.(ai.Deployment)
	if _, err = svc.UpsertAlias(ctx1, ai.AliasUpsertRequest{Code: "embed-default", DeploymentID: deployment.ID}); err != nil {
		t.Fatal(err)
	}
	adapters := aimodelruntime.NewAdapterRegistry()
	embeddings := aimodelruntime.NewEmbeddingAdapterRegistry()
	if err = embeddings.Register(aimodel.ProtocolOpenAI, aimodelruntime.NewFakeEmbeddingAdapter(8)); err != nil {
		t.Fatal(err)
	}
	invocations := ai.NewInvocationService(db, svc, adapters, rt.Writer)
	invocations.SetEmbeddingAdapters(embeddings)
	resp, err := invocations.Embed(ctx1, aimodelruntime.EmbeddingRequest{
		ModelAlias: "embed-default",
		Purpose:    aimodelruntime.EmbeddingPurposeKnowledgeQuery,
		Inputs:     []aimodelruntime.EmbeddingItem{{ID: "doc-1", Text: "tenant scoped knowledge"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Dimensions != 8 || len(resp.Vectors) != 1 || resp.Vectors[0].ID != "doc-1" {
		t.Fatalf("unexpected embed response: %#v", resp)
	}
	var row ai.ModelInvocation
	tdb, scopeErr := tenantdata.TenantDB(ctx1, db)
	if scopeErr != nil {
		t.Fatal(scopeErr)
	}
	rows, queryErr := tenantdata.TenantRaw[ai.ModelInvocation](tdb, "SELECT * FROM ai_model_invocations WHERE tenant_id=? AND mode=? LIMIT 1", "embedding")
	if queryErr != nil || len(rows) != 1 {
		t.Fatalf("unexpected invocation query: %#v %v", rows, queryErr)
	}
	row = rows[0]
	if row.Status != "completed" || row.EmbeddingPurpose != "knowledge_query" || row.EmbeddingItemCount != 1 || row.EmbeddingTextBytes <= 0 {
		t.Fatalf("unexpected invocation row: %#v", row)
	}
	encoded, _ := json.Marshal(row)
	if strings.Contains(string(encoded), "tenant scoped knowledge") || strings.Contains(string(encoded), "vector") {
		t.Fatalf("text/vector leaked into invocation json: %s", encoded)
	}
	var events []outbox.Event
	if err = db.Where("event_type=?", "ai.model_invocation.finished.v1").Find(&events).Error; err != nil || len(events) != 1 {
		t.Fatalf("expected one finished outbox event, got %d %v", len(events), err)
	}
	if strings.Contains(string(events[0].PayloadJSON), "tenant scoped knowledge") || strings.Contains(string(events[0].PayloadJSON), `"vector"`) {
		t.Fatalf("sensitive payload leaked: %s", events[0].PayloadJSON)
	}
	staleStarted := time.Now().UTC().Add(-20 * time.Minute)
	if err = db.Exec(`INSERT INTO ai_model_invocations
(id,tenant_id,user_id,session_id,model_alias,provider_id,deployment_id,remote_model_id,mode,status,started_at,input_modalities_json)
VALUES (?,?,?,?,?,?,?,?,?,?,?,?)`, "stale-t1", "t1", "u1", "s1", "embed-default", provider.ID, deployment.ID, "text-embedding-3-small", "embedding", "running", staleStarted, json.RawMessage(`[]`)).Error; err != nil {
		t.Fatal(err)
	}
	if err = invocations.RecoverStale(context.Background(), 10*time.Minute); err != nil {
		t.Fatal(err)
	}
	staleRows, staleErr := tenantdata.TenantRaw[ai.ModelInvocation](tdb, "SELECT * FROM ai_model_invocations WHERE tenant_id=? AND id=? LIMIT 1", "stale-t1")
	if staleErr != nil || len(staleRows) != 1 || staleRows[0].Status != "unknown" {
		t.Fatalf("stale invocation was not recovered: %#v %v", staleRows, staleErr)
	}
	ctx2 := auth.WithContext(context.Background(), &auth.Context{Authenticated: true, TenantID: "t2", PrincipalID: "u2", SessionID: "s2", Realm: auth.RealmTenant})
	if _, err = invocations.Embed(ctx2, aimodelruntime.EmbeddingRequest{
		ModelAlias: "embed-default",
		Inputs:     []aimodelruntime.EmbeddingItem{{ID: "x", Text: "cross tenant"}},
	}); err == nil {
		t.Fatal("cross-tenant alias resolve must fail")
	}
}

func asRuntimeError(err error, target **aimodelruntime.RuntimeError) bool {
	re, ok := err.(*aimodelruntime.RuntimeError)
	if !ok {
		return false
	}
	*target = re
	return true
}
