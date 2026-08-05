package personalization

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/yzxingtu2026/ainp/platform/framework/aimodelruntime"
	"github.com/yzxingtu2026/ainp/platform/framework/auth"
	"github.com/yzxingtu2026/ainp/platform/framework/outbox"
	"github.com/yzxingtu2026/ainp/platform/framework/prompt"
	"github.com/yzxingtu2026/ainp/platform/framework/runtimecontext"
	"github.com/yzxingtu2026/ainp/platform/framework/setting"
	tenantdata "github.com/yzxingtu2026/ainp/platform/framework/tenant"
	"github.com/yzxingtu2026/ainp/platform/modules/ai/agent"
	"gorm.io/gorm"
)

const AssistantPersonalizationSettingKey = "ai.assistant_personalization"
const AssistantPersonalizationEvent = "ai.assistant_personalization.refresh_requested.v1"
const AssistantPersonalizationJobType = "ai.assistant_personalization.generate"
const assistantPersonalizationPromptVersion = "1.2.1"

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

func asGenerateResponse(value any) (aimodelruntime.GenerateResponse, bool) {
	response, ok := value.(aimodelruntime.GenerateResponse)
	if ok {
		return response, true
	}
	pointer, ok := value.(*aimodelruntime.GenerateResponse)
	if ok && pointer != nil {
		return *pointer, true
	}
	return aimodelruntime.GenerateResponse{}, false
}

type memoryProfileProjection struct {
	ProfileJSON    json.RawMessage
	SourceRevision uint64
}

type runProjection struct {
	ConversationID  string
	PageContextType string
	FinishedAt      *time.Time
	Revision        uint64
}

type conversationProjection struct {
	ID            string
	Title         string
	Revision      uint64
	LastMessageAt *time.Time
}

type messageProjection struct {
	Role     string
	Content  json.RawMessage
	Sequence uint64
}

type PersonalizationService struct {
	db             *gorm.DB
	settings       *setting.Provider
	writer         outbox.Writer
	generator      personalizationGenerator
	slots          *agent.AgentSlotService
	runtimeContext runtimecontext.Provider
	now            func() time.Time
}

func (s *PersonalizationService) SetContextSources(slots *agent.AgentSlotService, provider runtimecontext.Provider) {
	s.slots, s.runtimeContext = slots, provider
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
func (s *PersonalizationService) fallback(ctx context.Context, ac *auth.Context) (PersonalizationResponse, *runtimecontext.TrustedRuntimeContext, error) {
	trusted := &runtimecontext.TrustedRuntimeContext{Locale: ac.Language}
	trusted.Time, _ = runtimecontext.NewTimeSnapshot(s.now(), runtimecontext.SystemFallbackTimezone, "system_default")
	if s.runtimeContext != nil {
		var err error
		trusted, err = s.runtimeContext.Build(ctx, runtimecontext.BuildRequest{TenantID: ac.TenantID, PrincipalID: ac.PrincipalID})
		if err != nil {
			return PersonalizationResponse{}, nil, err
		}
	}
	locale, address := trusted.Locale, trusted.PreferredAddress
	if strings.HasPrefix(strings.ToLower(locale), "en") {
		body := "What would you like to work on today?"
		if address != "" {
			body = address + ", " + body
		}
		return PersonalizationResponse{Greeting: renderTimeGreeting(body, locale, trusted.Time), Suggestions: []PersonalizationSuggestion{{Title: "Continue recent work", Prompt: "Help me continue my unfinished recent work", SourceType: "general"}, {Title: "Review priorities", Prompt: "Help me identify today's highest priority work", SourceType: "general"}}, ContextScope: "global"}, trusted, nil
	}
	body := "今天想先处理什么？"
	if address != "" {
		body = address + "。" + body
	}
	return PersonalizationResponse{Greeting: renderTimeGreeting(body, locale, trusted.Time), Suggestions: []PersonalizationSuggestion{{Title: "继续最近的工作", Prompt: "帮我继续整理最近未完成的工作", SourceType: "general"}, {Title: "查看当前重点", Prompt: "帮我梳理今天最值得优先处理的事项", SourceType: "general"}}, ContextScope: "global"}, trusted, nil
}

func (s *PersonalizationService) Read(ctx context.Context, _ struct{}) (PersonalizationResponse, error) {
	ac := auth.MustFromContext(ctx)
	result, trusted, e := s.fallback(ctx, ac)
	if e != nil {
		return result, e
	}
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
	row, found, e := optionalPersonalizationSnapshot(repo, ac.PrincipalID, agent.WorkspaceDefaultSlot)
	if !found && e == nil {
		result.Freshness = "missing"
		_ = s.request(ctx, uint64(rev))
		return result, nil
	}
	if e != nil {
		return result, e
	}
	_ = json.Unmarshal(row.SuggestionsJSON, &result.Suggestions)
	result.Greeting = renderTimeGreeting(row.Greeting, trusted.Locale, trusted.Time)
	result.GeneratedAt = &row.GeneratedAt
	result.ContextScope = row.ContextScope
	result.Freshness = "fresh"
	if row.Status != "ready" || row.PromptVersion != assistantPersonalizationPromptVersion || !row.ExpiresAt.After(s.now()) {
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
		_, e := s.writer.Append(ctx, tx, outbox.AppendInput{EventType: AssistantPersonalizationEvent, AggregateType: "assistant_personalization", AggregateID: ac.PrincipalID + ":" + agent.WorkspaceDefaultSlot, AggregateVersion: int64(revision), Payload: map[string]any{"account_id": ac.PrincipalID, "slot_key": agent.WorkspaceDefaultSlot, "generation_revision": revision}})
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
	if e = repo.Where("account_id=? AND slot_key=?", ac.PrincipalID, agent.WorkspaceDefaultSlot).Updates(map[string]any{"status": "stale"}).Error; e != nil {
		return nil, e
	}
	e = s.request(ctx, uint64(rev+1))
	return map[string]any{"requested": e == nil}, e
}

func (s *PersonalizationService) InvalidateAndRequest(ctx context.Context, userID string, revision uint64) error {
	ac := auth.MustFromContext(ctx)
	enabled, _, settingRevision, err := s.config(ctx, ac.TenantID)
	if err != nil || !enabled {
		return err
	}
	repo, err := personalizationRepository(ctx, s.db)
	if err != nil {
		return err
	}
	if current, found, lookupErr := optionalPersonalizationSnapshot(repo, userID, agent.WorkspaceDefaultSlot); lookupErr == nil && found {
		revision = current.GenerationRevision + 1
	}
	if result := repo.Where("account_id=? AND slot_key=?", userID, agent.WorkspaceDefaultSlot).Updates(map[string]any{"status": "stale"}); result.Error != nil {
		return result.Error
	}
	requestContext := ctx
	if userID != ac.PrincipalID {
		copied := *ac
		copied.PrincipalID = userID
		requestContext = auth.WithContext(ctx, &copied)
	}
	if revision < uint64(settingRevision+1) {
		revision = uint64(settingRevision + 1)
	}
	return s.request(requestContext, revision)
}

type personalizationContext struct {
	Locale               string                     `json:"locale"`
	Timezone             string                     `json:"timezone"`
	CurrentLocalDateTime string                     `json:"current_local_datetime"`
	PreferredAddress     string                     `json:"preferred_address,omitempty"`
	AgentVersionID       string                     `json:"agent_version_id,omitempty"`
	MemoryProfile        map[string]any             `json:"memory_profile,omitempty"`
	RecentWork           []map[string]any           `json:"recent_work,omitempty"`
	RecentConversation   *recentConversationContext `json:"recent_conversation,omitempty"`
	AllowedSourceTypes   []string                   `json:"allowed_source_types"`
}

type recentConversationContext struct {
	Title     string                      `json:"title"`
	Messages  []recentConversationMessage `json:"messages"`
	Untrusted bool                        `json:"untrusted"`
}
type recentConversationMessage struct {
	Role string `json:"role"`
	Text string `json:"text"`
}

func (s *PersonalizationService) buildContext(ctx context.Context, user, slot string) (personalizationContext, map[string]any, error) {
	ac := auth.MustFromContext(ctx)
	input := personalizationContext{Locale: "zh-CN", Timezone: "UTC", AllowedSourceTypes: []string{"general", "recent_work", "recent_conversation", "memory_profile"}}
	versions := map[string]any{"prompt": assistantPersonalizationPromptVersion}
	var agentID string
	if s.slots != nil {
		resolved, err := s.slots.Resolve(ctx, agent.AgentSlotGetRequest{SlotKey: slot})
		if err == nil && resolved.AgentVersion != nil {
			input.AgentVersionID = resolved.AgentVersion.ID
			agentID = resolved.Agent.ID
			versions["agent_version_id"] = resolved.AgentVersion.ID
		}
	}
	if s.runtimeContext != nil {
		trusted, err := s.runtimeContext.Build(ctx, runtimecontext.BuildRequest{TenantID: ac.TenantID, PrincipalID: user, AgentID: agentID, AgentVersionID: input.AgentVersionID})
		if err != nil {
			return input, nil, err
		}
		input.Locale = trusted.Locale
		input.Timezone = trusted.Time.Timezone
		input.CurrentLocalDateTime = trusted.Time.CurrentLocalDateTime
		input.PreferredAddress = trusted.PreferredAddress
	}
	tdb, err := tenantdata.TenantDB(ctx, s.db)
	if err != nil {
		return input, nil, err
	}
	if rows, e := tenantdata.TenantRaw[memoryProfileProjection](tdb, `SELECT profile_json,source_revision FROM ai_memory_current_profiles WHERE tenant_id=? AND user_id=? AND status='ready' ORDER BY source_revision DESC LIMIT 1`, user); e == nil && len(rows) == 1 {
		input.MemoryProfile = safeProfile(rows[0].ProfileJSON)
		versions["memory_profile_revision"] = rows[0].SourceRevision
	}
	if rows, e := tenantdata.TenantRaw[runProjection](tdb, `SELECT conversation_id,page_context_type,finished_at,revision FROM ai_agent_runs WHERE tenant_id=? AND user_id=? AND status='completed' ORDER BY finished_at DESC LIMIT 5`, user); e == nil {
		for _, run := range rows {
			item := map[string]any{"page_context_type": run.PageContextType, "finished_at": run.FinishedAt}
			if matches, lookupErr := tenantdata.TenantRaw[conversationProjection](tdb, `SELECT id,title,revision,last_message_at FROM ai_conversations WHERE tenant_id=? AND id=? LIMIT 1`, run.ConversationID); lookupErr == nil && len(matches) == 1 {
				title := strings.TrimSpace(matches[0].Title)
				if len([]rune(title)) > 120 {
					title = string([]rune(title)[:120])
				}
				item["title"] = title
			}
			input.RecentWork = append(input.RecentWork, item)
		}
		if len(rows) > 0 {
			versions["recent_run_revision"] = rows[0].Revision
		}
	}
	if rows, e := tenantdata.TenantRaw[conversationProjection](tdb, `SELECT id,title,revision,last_message_at FROM ai_conversations WHERE tenant_id=? AND owner_user_id=? AND purpose='workspace' AND last_message_at IS NOT NULL ORDER BY last_message_at DESC LIMIT 1`, user); e == nil && len(rows) == 1 {
		if records, messageErr := tenantdata.TenantRaw[messageProjection](tdb, `SELECT role,content,sequence FROM ai_messages WHERE tenant_id=? AND conversation_id=? AND status='committed' AND role IN ('user','assistant') ORDER BY sequence DESC LIMIT 4`, rows[0].ID); messageErr == nil {
			projected := projectConversationEnding(records, 1200)
			if len(projected) > 0 {
				input.RecentConversation = &recentConversationContext{Title: truncateRunes(strings.TrimSpace(rows[0].Title), 120), Messages: projected, Untrusted: true}
				versions["recent_conversation_revision"] = rows[0].Revision
			}
		}
	}
	return input, versions, nil
}

func projectConversationEnding(records []messageProjection, budget int) []recentConversationMessage {
	if budget <= 0 {
		return nil
	}
	result := make([]recentConversationMessage, 0, len(records))
	used := 0
	for index := len(records) - 1; index >= 0; index-- {
		var parts []aimodelruntime.ContentPart
		if json.Unmarshal(records[index].Content, &parts) != nil {
			continue
		}
		var text strings.Builder
		for _, part := range parts {
			if part.Type == aimodelruntime.PartInputText || part.Type == aimodelruntime.PartOutputText {
				if text.Len() > 0 {
					text.WriteString("\n")
				}
				text.WriteString(part.Text)
			}
		}
		value := strings.TrimSpace(text.String())
		if value == "" {
			continue
		}
		remaining := budget - used
		if remaining <= 0 {
			break
		}
		value = truncateRunes(value, minInt(400, remaining))
		used += len([]rune(value))
		result = append(result, recentConversationMessage{Role: records[index].Role, Text: value})
	}
	return result
}

func truncateRunes(value string, limit int) string {
	runes := []rune(value)
	if limit >= 0 && len(runes) > limit {
		return string(runes[:limit])
	}
	return value
}
func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func safeProfile(raw []byte) map[string]any {
	var source map[string]any
	if json.Unmarshal(raw, &source) != nil {
		return nil
	}
	out := map[string]any{}
	keys := make([]string, 0, len(source))
	for key := range source {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		if len(out) >= 12 {
			break
		}
		switch value := source[key].(type) {
		case string:
			if len([]rune(value)) > 200 {
				value = string([]rune(value)[:200])
			}
			out[key] = value
		case map[string]any:
			nested := map[string]any{}
			nestedKeys := make([]string, 0, len(value))
			for k := range value {
				nestedKeys = append(nestedKeys, k)
			}
			sort.Strings(nestedKeys)
			for _, k := range nestedKeys {
				if len(nested) >= 8 {
					break
				}
				if text, ok := value[k].(string); ok {
					if len([]rune(text)) > 160 {
						text = string([]rune(text)[:160])
					}
					nested[k] = text
				}
			}
			out[key] = nested
		}
	}
	return out
}

func (s *PersonalizationService) Generate(ctx context.Context, user, slot string, revision uint64) error {
	ac := auth.MustFromContext(ctx)
	enabled, alias, _, e := s.config(ctx, ac.TenantID)
	if e != nil || !enabled || alias == "" {
		return e
	}
	contextInput, sourceVersions, e := s.buildContext(ctx, user, slot)
	if e != nil {
		return e
	}
	body, _ := json.Marshal(contextInput)
	max := int64(700)
	temp := 0.3
	raw, e := s.generator.Generate(ctx, aimodelruntime.GenerateRequest{ModelAlias: alias, Instructions: prompt.MustGet("ai/assistant-personalization"), Messages: []aimodelruntime.Message{{Role: aimodelruntime.RoleUser, Content: []aimodelruntime.ContentPart{{Type: aimodelruntime.PartInputText, Text: string(body)}}}}, StructuredOutput: &aimodelruntime.StructuredOutput{Name: "assistant_personalization", Strict: true, Schema: personalizationSchema()}, MaxOutputTokens: &max, Temperature: &temp, TimeoutMS: 60000, Metadata: map[string]string{"purpose": "assistant_personalization", "prompt_version": assistantPersonalizationPromptVersion}})
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
	if json.Unmarshal([]byte(text), &value) != nil {
		return fmt.Errorf("personalization output invalid: invalid_json")
	}
	if err := validatePersonalization(value.Greeting, value.Suggestions); err != nil {
		return fmt.Errorf("personalization output invalid: %w", err)
	}
	suggestions, _ := json.Marshal(value.Suggestions)
	versionsJSON, _ := json.Marshal(sourceVersions)
	contextDigest := sha256.Sum256(body)
	now := s.now()
	row := AssistantPersonalizationSnapshot{ID: uuid.NewString(), TenantID: ac.TenantID, AccountID: user, SlotKey: slot, AgentVersionID: contextInput.AgentVersionID, Locale: contextInput.Locale, Timezone: contextInput.Timezone, ContextScope: "global", ContextHash: fmt.Sprintf("%x", contextDigest), Greeting: canonicalGreeting(value.Greeting), SuggestionsJSON: suggestions, SourceVersionsJSON: versionsJSON, ModelAlias: alias, PromptVersion: assistantPersonalizationPromptVersion, Status: "ready", GenerationRevision: revision, GeneratedAt: now, ExpiresAt: now.Add(24 * time.Hour)}
	repo, e := personalizationRepository(ctx, s.db)
	if e != nil {
		return e
	}
	old, found, e := optionalPersonalizationSnapshot(repo, user, slot)
	if e == nil && found && old.GenerationRevision > revision {
		return nil
	}
	if e == nil && found {
		values := map[string]any{"agent_version_id": row.AgentVersionID, "locale": row.Locale, "timezone": row.Timezone, "context_scope": row.ContextScope, "context_hash": row.ContextHash, "greeting": row.Greeting, "suggestions_json": row.SuggestionsJSON, "source_versions_json": row.SourceVersionsJSON, "model_alias": row.ModelAlias, "prompt_version": row.PromptVersion, "status": row.Status, "generation_revision": row.GenerationRevision, "generated_at": row.GeneratedAt, "expires_at": row.ExpiresAt, "last_error_category": ""}
		return repo.Where("id=? AND generation_revision<=?", old.ID, revision).Updates(values).Error
	}
	if e != nil {
		return e
	}
	return repo.Create(&row)
}

func optionalPersonalizationSnapshot(repo *tenantdata.Repository[AssistantPersonalizationSnapshot], userID, slotKey string) (AssistantPersonalizationSnapshot, bool, error) {
	rows, err := repo.Find("account_id=? AND slot_key=?", userID, slotKey)
	if err != nil || len(rows) == 0 {
		return AssistantPersonalizationSnapshot{}, false, err
	}
	return rows[0], true, nil
}

func canonicalGreeting(value string) string {
	return runtimecontext.NormalizeGreetingBody(value)
}

func renderTimeGreeting(body, locale string, snapshot runtimecontext.TimeSnapshot) string {
	body = canonicalGreeting(body)
	return runtimecontext.RenderTimeGreeting(body, locale, snapshot)
}
func validatePersonalization(g string, ss []PersonalizationSuggestion) error {
	if len([]rune(g)) == 0 || len([]rune(g)) > 120 || len(ss) == 0 || len(ss) > 4 || strings.ContainsAny(g, "<>") {
		return fmt.Errorf("greeting_or_count")
	}
	for _, x := range ss {
		if x.TargetRef != nil {
			return fmt.Errorf("target_ref_not_allowed")
		}
		if len([]rune(x.Title)) > 40 || len([]rune(x.Prompt)) > 160 {
			return fmt.Errorf("suggestion_length")
		}
		lower := strings.ToLower(x.Prompt)
		if strings.ContainsAny(x.Prompt, "<>") || strings.Contains(lower, "http://") || strings.Contains(lower, "https://") || strings.Contains(lower, "/api/") || strings.Contains(lower, "/ai/") {
			return fmt.Errorf("url_or_internal_path")
		}
	}
	return nil
}
func personalizationSchema() json.RawMessage {
	return json.RawMessage(`{"type":"object","additionalProperties":false,"required":["greeting","suggestions"],"properties":{"greeting":{"type":"string","maxLength":120},"suggestions":{"type":"array","minItems":1,"maxItems":4,"items":{"type":"object","additionalProperties":false,"required":["title","prompt","source_type","target_ref"],"properties":{"title":{"type":"string","maxLength":40},"prompt":{"type":"string","maxLength":160},"source_type":{"type":"string","enum":["general","recent_work","recent_conversation","memory_profile"]},"target_ref":{"type":"null"}}}}}}`)
}
