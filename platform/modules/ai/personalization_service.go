package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/yzxingtu2026/ainp/platform/framework/aimodelruntime"
	"github.com/yzxingtu2026/ainp/platform/framework/auth"
	"github.com/yzxingtu2026/ainp/platform/framework/outbox"
	"github.com/yzxingtu2026/ainp/platform/framework/prompt"
	"github.com/yzxingtu2026/ainp/platform/framework/setting"
	tenantdata "github.com/yzxingtu2026/ainp/platform/framework/tenant"
	"gorm.io/gorm"
)

const AssistantPersonalizationSettingKey = "ai.assistant_personalization"
const AssistantPersonalizationEvent = "ai.assistant_personalization.refresh_requested.v1"
const AssistantPersonalizationJobType = "ai.assistant_personalization.generate"

type PersonalizationSuggestion struct {
	Title      string  `json:"title"`
	Prompt     string  `json:"prompt"`
	SourceType string  `json:"source_type"`
	TargetRef  *string `json:"target_ref"`
}
type PersonalizationResponse struct {
	Greeting     string                      `json:"greeting"`
	Suggestions  []PersonalizationSuggestion `json:"suggestions"`
	Freshness    string                      `json:"freshness"`
	GeneratedAt  *time.Time                  `json:"generated_at"`
	ContextScope string                      `json:"context_scope"`
}
type personalizationGenerator interface {
	Generate(context.Context, aimodelruntime.GenerateRequest) (any, error)
}
type PersonalizationService struct {
	db        *gorm.DB
	settings  *setting.Provider
	writer    outbox.Writer
	generator personalizationGenerator
	now       func() time.Time
}

func NewPersonalizationService(db *gorm.DB, settings *setting.Provider, writer outbox.Writer, generator personalizationGenerator) *PersonalizationService {
	return &PersonalizationService{db: db, settings: settings, writer: writer, generator: generator, now: func() time.Time { return time.Now().UTC() }}
}
func (s *PersonalizationService) config(ctx context.Context, tenant string) (bool, string, int64, error) {
	v, e := s.settings.Get(ctx, tenant, AssistantPersonalizationSettingKey)
	if e != nil {
		return false, "", 0, e
	}
	enabled, _ := v.Values["enabled"].(bool)
	alias, _ := v.Values["model_alias"].(string)
	return enabled, strings.TrimSpace(alias), v.Revision, nil
}
func fallback(ac *auth.Context) PersonalizationResponse {
	if strings.HasPrefix(strings.ToLower(ac.Language), "en") {
		return PersonalizationResponse{Greeting: "What would you like to work on today?", Suggestions: []PersonalizationSuggestion{{Title: "Continue recent work", Prompt: "Help me continue my unfinished recent work", SourceType: "general"}, {Title: "Review priorities", Prompt: "Help me identify today's highest priority work", SourceType: "general"}}, ContextScope: "global"}
	}
	return PersonalizationResponse{Greeting: "你好，今天想先处理什么？", Suggestions: []PersonalizationSuggestion{{Title: "继续最近的工作", Prompt: "帮我继续整理最近未完成的工作", SourceType: "general"}, {Title: "查看当前重点", Prompt: "帮我梳理今天最值得优先处理的事项", SourceType: "general"}}, ContextScope: "global"}
}

func (s *PersonalizationService) Read(ctx context.Context, _ struct{}) (PersonalizationResponse, error) {
	ac := auth.MustFromContext(ctx)
	result := fallback(ac)
	enabled, _, rev, e := s.config(ctx, ac.TenantID)
	if e != nil {
		return result, e
	}
	if !enabled {
		result.Freshness = "disabled"
		return result, nil
	}
	repo, e := personalizationRepository(ctx, s.db)
	if e != nil {
		return result, e
	}
	row, e := repo.First("user_id=? AND slot_key=?", ac.UserID, WorkspaceDefaultSlot)
	if tenantdata.IsNotFound(e) {
		result.Freshness = "missing"
		_ = s.request(ctx, uint64(rev))
		return result, nil
	}
	if e != nil {
		return result, e
	}
	_ = json.Unmarshal(row.SuggestionsJSON, &result.Suggestions)
	result.Greeting = row.Greeting
	result.GeneratedAt = &row.GeneratedAt
	result.ContextScope = row.ContextScope
	result.Freshness = "fresh"
	if row.Status != "ready" || !row.ExpiresAt.After(s.now()) {
		result.Freshness = "stale"
		_ = s.request(ctx, row.GenerationRevision+1)
	}
	return result, nil
}
func (s *PersonalizationService) request(ctx context.Context, revision uint64) error {
	if s.writer == nil {
		return nil
	}
	ac := auth.MustFromContext(ctx)
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		_, e := s.writer.Append(ctx, tx, outbox.AppendInput{EventType: AssistantPersonalizationEvent, AggregateType: "assistant_personalization", AggregateID: ac.UserID + ":" + WorkspaceDefaultSlot, AggregateVersion: int64(revision), Payload: map[string]any{"user_id": ac.UserID, "slot_key": WorkspaceDefaultSlot, "generation_revision": revision}})
		return e
	})
}
func (s *PersonalizationService) Rebuild(ctx context.Context, _ struct{}) (map[string]any, error) {
	ac := auth.MustFromContext(ctx)
	enabled, _, rev, e := s.config(ctx, ac.TenantID)
	if e != nil {
		return nil, e
	}
	if !enabled {
		return map[string]any{"requested": false, "freshness": "disabled"}, nil
	}
	repo, e := personalizationRepository(ctx, s.db)
	if e != nil {
		return nil, e
	}
	if e = repo.Where("user_id=? AND slot_key=?", ac.UserID, WorkspaceDefaultSlot).Updates(map[string]any{"status": "stale"}).Error; e != nil {
		return nil, e
	}
	e = s.request(ctx, uint64(rev+1))
	return map[string]any{"requested": e == nil}, e
}

func (s *PersonalizationService) Generate(ctx context.Context, user, slot string, revision uint64) error {
	ac := auth.MustFromContext(ctx)
	enabled, alias, _, e := s.config(ctx, ac.TenantID)
	if e != nil || !enabled || alias == "" {
		return e
	}
	max := int64(700)
	temp := 0.3
	raw, e := s.generator.Generate(ctx, aimodelruntime.GenerateRequest{ModelAlias: alias, Instructions: prompt.MustGet("ai/assistant-personalization"), Messages: []aimodelruntime.Message{{Role: aimodelruntime.RoleUser, Content: []aimodelruntime.ContentPart{{Type: aimodelruntime.PartInputText, Text: `{"locale":"zh-CN","allowed_source_types":["general","recent_work","memory_profile"]}`}}}}, StructuredOutput: &aimodelruntime.StructuredOutput{Name: "assistant_personalization", Strict: true, Schema: personalizationSchema()}, MaxOutputTokens: &max, Temperature: &temp, TimeoutMS: 60000, Metadata: map[string]string{"purpose": "assistant_personalization", "prompt_version": "1.0.0"}})
	if e != nil {
		return e
	}
	r, ok := asGenerateResponse(raw)
	if !ok {
		return fmt.Errorf("personalization runtime response invalid")
	}
	var text string
	for _, o := range r.Output {
		if o.Type == aimodelruntime.PartOutputText {
			text = o.Text
		}
	}
	var value struct {
		Greeting    string                      `json:"greeting"`
		Suggestions []PersonalizationSuggestion `json:"suggestions"`
	}
	if json.Unmarshal([]byte(text), &value) != nil || !validPersonalization(value.Greeting, value.Suggestions) {
		return fmt.Errorf("personalization output invalid")
	}
	suggestions, _ := json.Marshal(value.Suggestions)
	now := s.now()
	row := AssistantPersonalizationSnapshot{ID: uuid.NewString(), TenantID: ac.TenantID, UserID: user, SlotKey: slot, Locale: "zh-CN", Timezone: "Asia/Shanghai", ContextScope: "global", Greeting: value.Greeting, SuggestionsJSON: suggestions, SourceVersionsJSON: []byte("{}"), ModelAlias: alias, PromptVersion: "1.0.0", Status: "ready", GenerationRevision: revision, GeneratedAt: now, ExpiresAt: now.Add(24 * time.Hour)}
	repo, e := personalizationRepository(ctx, s.db)
	if e != nil {
		return e
	}
	old, e := repo.First("user_id=? AND slot_key=?", user, slot)
	if e == nil && old.GenerationRevision > revision {
		return nil
	}
	if e == nil {
		values := map[string]any{"agent_version_id": row.AgentVersionID, "locale": row.Locale, "timezone": row.Timezone, "context_scope": row.ContextScope, "context_hash": row.ContextHash, "greeting": row.Greeting, "suggestions_json": row.SuggestionsJSON, "source_versions_json": row.SourceVersionsJSON, "model_alias": row.ModelAlias, "prompt_version": row.PromptVersion, "status": row.Status, "generation_revision": row.GenerationRevision, "generated_at": row.GeneratedAt, "expires_at": row.ExpiresAt, "last_error_category": ""}
		return repo.Where("id=? AND generation_revision<=?", old.ID, revision).Updates(values).Error
	}
	if !tenantdata.IsNotFound(e) {
		return e
	}
	return repo.Create(&row)
}
func validPersonalization(g string, ss []PersonalizationSuggestion) bool {
	if len([]rune(g)) == 0 || len([]rune(g)) > 120 || len(ss) == 0 || len(ss) > 4 || strings.ContainsAny(g, "<>") {
		return false
	}
	for _, x := range ss {
		if x.TargetRef != nil || len([]rune(x.Title)) > 40 || len([]rune(x.Prompt)) > 160 || strings.Contains(x.Prompt, "http") || strings.Contains(x.Prompt, "/") {
			return false
		}
	}
	return true
}
func personalizationSchema() json.RawMessage {
	return json.RawMessage(`{"type":"object","additionalProperties":false,"required":["greeting","suggestions"],"properties":{"greeting":{"type":"string","maxLength":120},"suggestions":{"type":"array","minItems":1,"maxItems":4,"items":{"type":"object","additionalProperties":false,"required":["title","prompt","source_type","target_ref"],"properties":{"title":{"type":"string","maxLength":40},"prompt":{"type":"string","maxLength":160},"source_type":{"type":"string","enum":["general","recent_work","memory_profile"]},"target_ref":{"type":["string","null"]}}}}}}`)
}
