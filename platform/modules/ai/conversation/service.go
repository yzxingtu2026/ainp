package conversation

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/yzxingtu2026/ainp/platform/framework/aiconversation"
	"github.com/yzxingtu2026/ainp/platform/framework/aimodelruntime"
	"github.com/yzxingtu2026/ainp/platform/framework/auth"
	"github.com/yzxingtu2026/ainp/platform/framework/errs"
	"github.com/yzxingtu2026/ainp/platform/framework/pagination"
	tenantdata "github.com/yzxingtu2026/ainp/platform/framework/tenant"
	"github.com/yzxingtu2026/ainp/platform/modules/ai/agent"
	"github.com/yzxingtu2026/ainp/platform/modules/ai/modelruntime"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type ConversationService struct {
	db     tenantdata.GuardedDatabase
	assets aimodelruntime.AssetResolver
}

func NewConversationService(db *gorm.DB, assets aimodelruntime.AssetResolver) *ConversationService {
	if err := registerConversationTenantModels(); err != nil {
		panic(err)
	}
	return &ConversationService{db: tenantdata.GuardedDB(db), assets: assets}
}

type ConversationListRequest struct {
	pagination.Request
	Status     string `json:"status" validate:"omitempty"`
	Visibility string `json:"visibility" validate:"omitempty"`
	Purpose    string `json:"purpose" validate:"omitempty"`
}
type ConversationCreateRequest struct {
	AgentID        string `json:"agent_id" validate:"required"`
	AgentVersionID string `json:"agent_version_id" validate:"required"`
	Title          string `json:"title" validate:"omitempty"`
	Visibility     string `json:"visibility" validate:"omitempty"`
	Purpose        string `json:"purpose" validate:"omitempty"`
}
type ConversationCreateForSlotRequest struct {
	SlotKey    string `json:"slot_key" validate:"required"`
	Title      string `json:"title" validate:"omitempty"`
	Visibility string `json:"visibility" validate:"omitempty"`
}
type ConversationGetRequest struct {
	ID string `json:"id" validate:"required"`
}
type ConversationUpdateRequest struct {
	ID       string `json:"id" validate:"required"`
	Title    string `json:"title" validate:"required"`
	Revision uint64 `json:"revision" validate:"required"`
}
type ConversationStateRequest struct {
	ID       string `json:"id" validate:"required"`
	Revision uint64 `json:"revision" validate:"required"`
}
type MessageListRequest struct {
	ConversationID string `json:"conversation_id" validate:"required"`
	AfterSequence  uint64 `json:"after_sequence" validate:"omitempty"`
	Limit          int    `json:"limit" validate:"omitempty,min=1,max=100"`
}
type MessageGetRequest struct {
	ID string `json:"id" validate:"required"`
}
type MessageCreateRequest struct {
	ConversationID   string                       `json:"conversation_id" validate:"required"`
	Role             string                       `json:"role"`
	Content          []aimodelruntime.ContentPart `json:"content" validate:"required,min=1"`
	ReplyToMessageID *string                      `json:"reply_to_message_id"`
}
type SnapshotCreateRequest struct {
	ConversationID, ModelDeploymentID, Kind, PolicyVersion, IdempotencyKey      string
	SchemaVersion, PromptVersion, SummarizerDeploymentID, Reason, QualityStatus string
	CompactionDepth                                                             int
	ParentSnapshotID                                                            *string
	Payload                                                                     json.RawMessage
	TokenEstimate                                                               int
	SourceSequenceFrom, SourceSequenceTo                                        uint64
}

func conversationActor(ctx context.Context) (*auth.Context, error) {
	ac := auth.MustFromContext(ctx)
	if !ac.Authenticated || ac.TenantID == "" || ac.PrincipalID == "" {
		return nil, errs.New(errs.CodeUnauthorized, "unauthorized")
	}
	return ac, nil
}

func canManageConversations(ac *auth.Context) bool {
	return ac.HasPermission(agent.ActionCreate)
}

func (s *ConversationService) visibleQuery(ctx context.Context) (*gorm.DB, *auth.Context, error) {
	ac, err := conversationActor(ctx)
	if err != nil {
		return nil, nil, err
	}
	q := s.db.WithContext(ctx).Where("owner_user_id = ? OR visibility = ?", ac.PrincipalID, "tenant")
	return q, ac, nil
}

func (s *ConversationService) get(ctx context.Context, id string, mutate bool) (*Conversation, *auth.Context, error) {
	q, ac, err := s.visibleQuery(ctx)
	if err != nil {
		return nil, nil, err
	}
	var row Conversation
	if err = q.Where("id = ?", id).First(&row).Error; err != nil {
		return nil, nil, errs.New(errs.CodeNotFound, "conversation not found")
	}
	if mutate && row.OwnerUserID != ac.PrincipalID && !canManageConversations(ac) {
		return nil, nil, errs.New(errs.CodeNotFound, "conversation not found")
	}
	return &row, ac, nil
}

func (s *ConversationService) List(ctx context.Context, r ConversationListRequest) (any, error) {
	r.Normalize()
	q, _, err := s.visibleQuery(ctx)
	if err != nil {
		return nil, err
	}
	if r.Status != "" {
		q = q.Where("status = ?", r.Status)
	}
	if r.Visibility != "" {
		q = q.Where("visibility = ?", r.Visibility)
	}
	purpose := strings.TrimSpace(r.Purpose)
	if purpose == "" {
		purpose = ConversationPurposeWorkspace
	}
	if purpose != ConversationPurposeWorkspace && purpose != ConversationPurposeBuilderPreview {
		return nil, errs.New(errs.CodeValidation, "invalid conversation purpose")
	}
	q = q.Where("purpose = ?", purpose)
	var total int64
	if err = q.Model(&Conversation{}).Count(&total).Error; err != nil {
		return nil, err
	}
	var rows []Conversation
	err = q.Order("last_message_at DESC, created_at DESC").Offset(r.Offset()).Limit(r.PageSize).Find(&rows).Error
	return pagination.NewResult(rows, total, r.Request), err
}

func (s *ConversationService) Get(ctx context.Context, r ConversationGetRequest) (any, error) {
	row, _, err := s.get(ctx, r.ID, false)
	return row, err
}

func (s *ConversationService) Create(ctx context.Context, r ConversationCreateRequest) (any, error) {
	ac, err := conversationActor(ctx)
	if err != nil {
		return nil, err
	}
	if r.Visibility == "" {
		r.Visibility = "private"
	}
	if r.Visibility != "private" && r.Visibility != "tenant" {
		return nil, errs.New(errs.CodeValidation, "invalid visibility")
	}
	if r.Visibility == "tenant" && !canManageConversations(ac) {
		return nil, errs.New(errs.CodeForbidden, "tenant conversation permission required")
	}
	_, err = agent.FindVisible(ctx, s.db.WithContext(ctx), r.AgentID, ac.PrincipalID)
	if err != nil {
		return nil, errs.New(errs.CodeNotFound, "agent not found")
	}
	version, err := agent.GetVersionForAgent(ctx, s.db.WithContext(ctx), r.AgentVersionID, r.AgentID)
	if err != nil {
		return nil, errs.New(errs.CodeNotFound, "agent version not found")
	}
	purpose := strings.TrimSpace(r.Purpose)
	if purpose == "" {
		purpose = ConversationPurposeWorkspace
	}
	if purpose != ConversationPurposeWorkspace && purpose != ConversationPurposeBuilderPreview {
		return nil, errs.New(errs.CodeValidation, "invalid conversation purpose")
	}
	if purpose == ConversationPurposeWorkspace && version.Kind == agent.AgentVersionKindPreview {
		return nil, errs.New(errs.CodeValidation, "preview agent version cannot be used in workspace conversations")
	}
	if purpose == ConversationPurposeBuilderPreview && version.Kind != agent.AgentVersionKindPreview && version.Kind != agent.AgentVersionKindRelease {
		return nil, errs.New(errs.CodeValidation, "invalid agent version kind")
	}
	title := strings.TrimSpace(r.Title)
	if title == "" {
		if purpose == ConversationPurposeBuilderPreview {
			title = "Builder 预览"
		} else {
			title = "新会话"
		}
	}
	if utf8.RuneCountInString(title) > 256 {
		return nil, errs.New(errs.CodeValidation, "title too long")
	}
	row := Conversation{
		ID: uuid.NewString(), TenantID: ac.TenantID, OwnerUserID: ac.PrincipalID, Title: title,
		AgentID: r.AgentID, AgentVersionID: r.AgentVersionID, Purpose: purpose,
		Visibility: r.Visibility, Status: "active", Revision: 1,
	}
	if err = s.db.WithContext(ctx).Create(&row).Error; err != nil {
		return nil, dbError(err, "create conversation")
	}
	return row, nil
}

// CreateForSlot resolves and fixes the release version in one server-side request.
func (s *ConversationService) CreateForSlot(slots *agent.AgentSlotService) func(context.Context, ConversationCreateForSlotRequest) (any, error) {
	return func(ctx context.Context, r ConversationCreateForSlotRequest) (any, error) {
		resolved, err := slots.Resolve(ctx, agent.AgentSlotGetRequest{SlotKey: r.SlotKey})
		if err != nil {
			return nil, err
		}
		if !resolved.Healthy || resolved.Agent == nil || resolved.AgentVersion == nil {
			return nil, errs.New(errs.CodeConflict, "agent slot is unavailable")
		}
		return s.Create(ctx, ConversationCreateRequest{AgentID: resolved.Agent.ID, AgentVersionID: resolved.AgentVersion.ID, Title: r.Title, Visibility: r.Visibility, Purpose: ConversationPurposeWorkspace})
	}
}

func (s *ConversationService) Update(ctx context.Context, r ConversationUpdateRequest) (any, error) {
	row, _, err := s.get(ctx, r.ID, true)
	if err != nil {
		return nil, err
	}
	title := strings.TrimSpace(r.Title)
	if title == "" || utf8.RuneCountInString(title) > 256 {
		return nil, errs.New(errs.CodeValidation, "invalid title")
	}
	res := s.db.WithContext(ctx).Model(&Conversation{}).Where("id=? AND revision=?", row.ID, r.Revision).Updates(map[string]any{"title": title, "revision": gorm.Expr("revision + 1")})
	if res.Error != nil {
		return nil, res.Error
	}
	if res.RowsAffected != 1 {
		return nil, errs.New(errs.CodeConflict, "conversation revision conflict")
	}
	return s.Get(ctx, ConversationGetRequest{ID: r.ID})
}

func (s *ConversationService) setStatus(ctx context.Context, r ConversationStateRequest, from, to string) (any, error) {
	row, _, err := s.get(ctx, r.ID, true)
	if err != nil {
		return nil, err
	}
	if row.Status != from {
		return nil, errs.New(errs.CodeConflict, "invalid conversation state")
	}
	res := s.db.WithContext(ctx).Model(&Conversation{}).Where("id=? AND revision=? AND status=?", row.ID, r.Revision, from).Updates(map[string]any{"status": to, "revision": gorm.Expr("revision + 1")})
	if res.Error != nil {
		return nil, res.Error
	}
	if res.RowsAffected != 1 {
		return nil, errs.New(errs.CodeConflict, "conversation revision conflict")
	}
	return s.Get(ctx, ConversationGetRequest{ID: r.ID})
}
func (s *ConversationService) Archive(ctx context.Context, r ConversationStateRequest) (any, error) {
	return s.setStatus(ctx, r, "active", "archived")
}
func (s *ConversationService) Restore(ctx context.Context, r ConversationStateRequest) (any, error) {
	return s.setStatus(ctx, r, "archived", "active")
}

func (s *ConversationService) validateContent(ctx context.Context, _ *auth.Context, content []aimodelruntime.ContentPart) error {
	if err := aimodelruntime.ValidateMessage(aimodelruntime.Message{Role: aimodelruntime.RoleUser, Content: content}); err != nil {
		return errs.New(errs.CodeValidation, err.Error())
	}
	for _, p := range content {
		if p.ImageAttachmentID == "" {
			continue
		}
		if p.AttachmentRevision == "" || s.assets == nil {
			return errs.New(errs.CodeNotFound, "attachment not found or revision changed")
		}
		resolved, err := s.assets.Resolve(ctx, aimodelruntime.AssetResolveRequest{AttachmentID: p.ImageAttachmentID, Purpose: "model_input"})
		if err != nil || resolved.Revision != p.AttachmentRevision {
			return errs.New(errs.CodeNotFound, "attachment not found or revision changed")
		}
	}
	return nil
}

func (s *ConversationService) CreateMessage(ctx context.Context, r MessageCreateRequest) (any, error) {
	row, ac, err := s.get(ctx, r.ConversationID, true)
	if err != nil {
		return nil, err
	}
	if row.Status != "active" {
		return nil, errs.New(errs.CodeConflict, "conversation is archived")
	}
	if r.Role != "" && r.Role != "user" {
		return nil, errs.New(errs.CodeForbidden, "clients may only create user messages")
	}
	if err = s.validateContent(ctx, ac, r.Content); err != nil {
		return nil, err
	}
	key := strings.TrimSpace(ac.IdempotencyKey)
	if key == "" {
		return nil, errs.New(errs.CodeIdempotencyKeyRequired, "idempotency key required")
	}
	if len(key) > 191 {
		return nil, errs.New(errs.CodeValidation, "idempotency key too long")
	}
	payload, _ := json.Marshal(r.Content)
	var result ConversationMessage
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var locked Conversation
		if e := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id=?", row.ID).First(&locked).Error; e != nil {
			return e
		}
		if e := tx.Where("conversation_id=? AND idempotency_key=?", row.ID, key).First(&result).Error; e == nil {
			return nil
		} else if !errors.Is(e, gorm.ErrRecordNotFound) {
			return e
		}
		if r.ReplyToMessageID != nil {
			var parent ConversationMessage
			if e := tx.Where("conversation_id=? AND id=? AND status=?", row.ID, *r.ReplyToMessageID, "committed").First(&parent).Error; e != nil {
				return errs.New(errs.CodeValidation, "reply target is not a committed message in this conversation")
			}
		}
		var last uint64
		if e := tx.Model(&ConversationMessage{}).Where("conversation_id=?", row.ID).Select("COALESCE(MAX(sequence),0)").Scan(&last).Error; e != nil {
			return e
		}
		now := time.Now().UTC()
		author := ac.PrincipalID
		result = ConversationMessage{ID: uuid.NewString(), TenantID: ac.TenantID, ConversationID: row.ID, Role: "user", Status: "committed", Sequence: last + 1, ReplyToMessageID: r.ReplyToMessageID, AuthorUserID: &author, Content: payload, IdempotencyKey: key, CommittedAt: &now}
		if e := tx.Create(&result).Error; e != nil {
			return e
		}
		return tx.Model(&Conversation{}).Where("id=?", row.ID).Updates(map[string]any{"last_message_at": now, "revision": gorm.Expr("revision + 1")}).Error
	})
	if err != nil {
		return nil, dbError(err, "create message")
	}
	return result, nil
}

func (s *ConversationService) AppendRuntimeMessage(ctx context.Context, conversationID, runID, role, idempotencyKey string, content []aimodelruntime.ContentPart) (*ConversationMessage, error) {
	row, ac, err := s.get(ctx, conversationID, true)
	if err != nil {
		return nil, err
	}
	if row.Status != "active" {
		return nil, errs.New(errs.CodeConflict, "conversation is archived")
	}
	runtimeRole := aimodelruntime.Role(role)
	if runtimeRole != aimodelruntime.RoleTool && runtimeRole != aimodelruntime.RoleSystem && runtimeRole != aimodelruntime.RoleAssistant {
		return nil, errs.New(errs.CodeValidation, "invalid runtime message role")
	}
	if err = aimodelruntime.ValidateMessage(aimodelruntime.Message{Role: runtimeRole, Content: content}); err != nil {
		return nil, errs.New(errs.CodeValidation, err.Error())
	}
	payload, _ := json.Marshal(content)
	var result ConversationMessage
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var locked Conversation
		if e := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id=?", row.ID).First(&locked).Error; e != nil {
			return e
		}
		if e := tx.Where("conversation_id=? AND idempotency_key=?", row.ID, idempotencyKey).First(&result).Error; e == nil {
			return nil
		} else if !errors.Is(e, gorm.ErrRecordNotFound) {
			return e
		}
		var last uint64
		if e := tx.Model(&ConversationMessage{}).Where("conversation_id=?", row.ID).Select("COALESCE(MAX(sequence),0)").Scan(&last).Error; e != nil {
			return e
		}
		now := time.Now().UTC()
		rid := runID
		result = ConversationMessage{ID: uuid.NewString(), TenantID: ac.TenantID, ConversationID: row.ID, Role: role, Status: "committed", Sequence: last + 1, RunID: &rid, Content: payload, IdempotencyKey: idempotencyKey, CommittedAt: &now}
		if e := tx.Create(&result).Error; e != nil {
			return e
		}
		return tx.Model(&Conversation{}).Where("id=?", row.ID).Updates(map[string]any{"last_message_at": now, "revision": gorm.Expr("revision + 1")}).Error
	})
	if err != nil {
		return nil, err
	}
	return &result, nil
}

func (s *ConversationService) ListMessages(ctx context.Context, r MessageListRequest) (any, error) {
	row, _, err := s.get(ctx, r.ConversationID, false)
	if err != nil {
		return nil, err
	}
	if r.Limit == 0 {
		r.Limit = 50
	}
	var rows []ConversationMessage
	err = s.db.WithContext(ctx).Where("conversation_id=? AND sequence>?", row.ID, r.AfterSequence).Order("sequence ASC").Limit(r.Limit + 1).Find(&rows).Error
	if err != nil {
		return nil, err
	}
	hasMore := len(rows) > r.Limit
	if hasMore {
		rows = rows[:r.Limit]
	}
	next := r.AfterSequence
	if len(rows) > 0 {
		next = rows[len(rows)-1].Sequence
	}
	return map[string]any{"items": rows, "next_sequence": next, "has_more": hasMore}, nil
}
func (s *ConversationService) GetMessage(ctx context.Context, r MessageGetRequest) (any, error) {
	_, err := conversationActor(ctx)
	if err != nil {
		return nil, err
	}
	var row ConversationMessage
	if err = s.db.WithContext(ctx).Where("id=?", r.ID).First(&row).Error; err != nil {
		return nil, errs.New(errs.CodeNotFound, "message not found")
	}
	if _, _, err = s.get(ctx, row.ConversationID, false); err != nil {
		return nil, err
	}
	return row, nil
}

func (s *ConversationService) Assemble(ctx context.Context, r aiconversation.AssemblyRequest) (*aiconversation.AssemblyResult, error) {
	row, _, err := s.get(ctx, r.ConversationID, false)
	if err != nil {
		return nil, err
	}
	if r.MaxBytes <= 0 || r.MaxTokens <= 0 {
		return nil, errs.New(errs.CodeValidation, "positive budgets required")
	}
	var records []ConversationMessage
	if err = s.db.WithContext(ctx).Where("conversation_id=? AND status=?", row.ID, "committed").Order("sequence ASC").Find(&records).Error; err != nil {
		return nil, err
	}
	out := &aiconversation.AssemblyResult{Status: "within_budget", ConversationRevision: row.Revision, AgentVersionID: row.AgentVersionID, Budget: aiconversation.Budget{MaxBytes: r.MaxBytes, MaxTokens: r.MaxTokens}}
	for _, record := range records {
		var content []aimodelruntime.ContentPart
		if json.Unmarshal(record.Content, &content) != nil {
			return nil, errs.New(errs.CodeInternal, "invalid canonical message")
		}
		for _, part := range content {
			if part.ImageAttachmentID == "" {
				continue
			}
			if s.assets == nil {
				return nil, errs.New(errs.CodeNotFound, "attachment access revoked")
			}
			asset, resolveErr := s.assets.Resolve(ctx, aimodelruntime.AssetResolveRequest{AttachmentID: part.ImageAttachmentID, Purpose: "model_input"})
			if resolveErr != nil || asset.Revision != part.AttachmentRevision {
				return nil, errs.New(errs.CodeNotFound, "attachment access revoked or revision changed")
			}
		}
		out.Messages = append(out.Messages, aiconversation.Message{ID: record.ID, Sequence: record.Sequence, Role: aimodelruntime.Role(record.Role), Content: content})
		out.Budget.UsedBytes += len(record.Content)
		if out.SourceSequenceFrom == 0 {
			out.SourceSequenceFrom = record.Sequence
		}
		out.SourceSequenceTo = record.Sequence
	}
	out.Budget.EstimatedTokens = (out.Budget.UsedBytes + 3) / 4
	if out.Budget.UsedBytes > r.MaxBytes || out.Budget.EstimatedTokens > r.MaxTokens {
		out.Status = "compaction_required"
		out.Reason = "context budget exceeded"
	}
	return out, nil
}

func (s *ConversationService) CreateSnapshot(ctx context.Context, r SnapshotCreateRequest) (*ContextSnapshot, error) {
	row, _, err := s.get(ctx, r.ConversationID, false)
	if err != nil {
		return nil, err
	}
	if r.Kind != "raw" && r.Kind != "assembled" && r.Kind != "compacted" {
		return nil, errs.New(errs.CodeValidation, "unsupported snapshot kind")
	}
	if r.IdempotencyKey == "" || r.ModelDeploymentID == "" || r.PolicyVersion == "" || !json.Valid(r.Payload) {
		return nil, errs.New(errs.CodeValidation, "invalid snapshot")
	}
	if r.SourceSequenceFrom > r.SourceSequenceTo {
		return nil, errs.New(errs.CodeValidation, "invalid snapshot source range")
	}
	tdb, err := s.db.Tenant(ctx)
	if err != nil {
		return nil, err
	}
	deployments, err := tenantdata.TenantRaw[modelruntime.Deployment](tdb,
		`SELECT * FROM ai_model_deployments WHERE tenant_id=? AND id=? LIMIT 1`, r.ModelDeploymentID)
	if err != nil || len(deployments) != 1 {
		return nil, errs.New(errs.CodeNotFound, "model deployment not found")
	}
	if r.ParentSnapshotID != nil {
		parents, parentErr := tenantdata.TenantRaw[ContextSnapshot](tdb,
			`SELECT * FROM ai_context_snapshots WHERE tenant_id=? AND conversation_id=? AND id=? LIMIT 1`, row.ID, *r.ParentSnapshotID)
		if parentErr != nil || len(parents) != 1 {
			return nil, errs.New(errs.CodeNotFound, "parent snapshot not found")
		}
	}
	if r.QualityStatus == "" {
		r.QualityStatus = "valid"
	}
	snap := ContextSnapshot{ID: uuid.NewString(), TenantID: row.TenantID, ConversationID: row.ID, ConversationRevision: row.Revision, AgentVersionID: row.AgentVersionID, ModelDeploymentID: r.ModelDeploymentID, SourceSequenceFrom: r.SourceSequenceFrom, SourceSequenceTo: r.SourceSequenceTo, ParentSnapshotID: r.ParentSnapshotID, Kind: r.Kind, PolicyVersion: r.PolicyVersion, SchemaVersion: r.SchemaVersion, PromptVersion: r.PromptVersion, SummarizerDeploymentID: r.SummarizerDeploymentID, CompactionDepth: r.CompactionDepth, Reason: r.Reason, QualityStatus: r.QualityStatus, TokenEstimate: r.TokenEstimate, Status: "ready", Payload: r.Payload, IdempotencyKey: r.IdempotencyKey}
	if err = s.db.WithContext(ctx).Create(&snap).Error; err != nil {
		if e := s.db.WithContext(ctx).Where("conversation_id=? AND idempotency_key=?", row.ID, r.IdempotencyKey).First(&snap).Error; e == nil {
			if snap.ConversationRevision != row.Revision || snap.ModelDeploymentID != r.ModelDeploymentID || snap.Kind != r.Kind || string(snap.Payload) != string(r.Payload) {
				return nil, errs.New(errs.CodeIdempotencyConflict, "snapshot idempotency conflict")
			}
			snap.Current = snap.ConversationRevision == row.Revision
			return &snap, nil
		}
		return nil, err
	}
	snap.Current = true
	return &snap, nil
}
func (s *ConversationService) GetSnapshot(ctx context.Context, id string) (*ContextSnapshot, error) {
	_, err := conversationActor(ctx)
	if err != nil {
		return nil, err
	}
	var snap ContextSnapshot
	if err = s.db.WithContext(ctx).Where("id=?", id).First(&snap).Error; err != nil {
		return nil, errs.New(errs.CodeNotFound, "snapshot not found")
	}
	conversation, _, err := s.get(ctx, snap.ConversationID, false)
	if err != nil {
		return nil, err
	}
	snap.Current = conversation.Revision == snap.ConversationRevision
	return &snap, nil
}

func (s *ConversationService) Begin(ctx context.Context, conversationID, idempotencyKey, runID string) (string, error) {
	row, _, err := s.get(ctx, conversationID, true)
	if err != nil {
		return "", err
	}
	if row.Status != "active" || strings.TrimSpace(idempotencyKey) == "" {
		return "", errs.New(errs.CodeValidation, "invalid assistant stream")
	}
	stream := AssistantStream{ID: uuid.NewString(), TenantID: row.TenantID, ConversationID: row.ID, IdempotencyKey: idempotencyKey, Status: "streaming"}
	if strings.TrimSpace(runID) != "" {
		stream.RunID = &runID
	}
	if err = s.db.WithContext(ctx).Create(&stream).Error; err != nil {
		if e := s.db.WithContext(ctx).Where("conversation_id=? AND idempotency_key=?", row.ID, idempotencyKey).First(&stream).Error; e == nil {
			return stream.ID, nil
		}
		return "", err
	}
	return stream.ID, nil
}

func (s *ConversationService) stream(ctx context.Context, id string) (*AssistantStream, error) {
	_, err := conversationActor(ctx)
	if err != nil {
		return nil, err
	}
	var stream AssistantStream
	if err = s.db.WithContext(ctx).Where("id=?", id).First(&stream).Error; err != nil {
		return nil, errs.New(errs.CodeNotFound, "assistant stream not found")
	}
	if _, _, err = s.get(ctx, stream.ConversationID, false); err != nil {
		return nil, err
	}
	return &stream, nil
}

func (s *ConversationService) Append(ctx context.Context, streamID string, delta aiconversation.StreamDelta) error {
	stream, err := s.stream(ctx, streamID)
	if err != nil {
		return err
	}
	if stream.Status != "streaming" || delta.Cursor == 0 || delta.Type == "" || len(delta.Data) == 0 || len(delta.Data) > 256*1024 {
		return errs.New(errs.CodeValidation, "invalid stream delta")
	}
	record := AssistantStreamDelta{ID: uuid.NewString(), TenantID: stream.TenantID, StreamID: stream.ID, Cursor: delta.Cursor, Type: delta.Type, Data: delta.Data}
	if err = s.db.WithContext(ctx).Create(&record).Error; err != nil {
		var existing AssistantStreamDelta
		if e := s.db.WithContext(ctx).Where("stream_id=? AND cursor=?", stream.ID, delta.Cursor).First(&existing).Error; e == nil && existing.Type == delta.Type && string(existing.Data) == string(delta.Data) {
			return nil
		}
		return errs.New(errs.CodeConflict, "stream cursor conflict")
	}
	return nil
}

func (s *ConversationService) Replay(ctx context.Context, streamID string, after uint64) ([]aiconversation.StreamDelta, error) {
	stream, err := s.stream(ctx, streamID)
	if err != nil {
		return nil, err
	}
	var rows []AssistantStreamDelta
	if err = s.db.WithContext(ctx).Where("stream_id=? AND cursor>?", stream.ID, after).Order("cursor ASC").Limit(1000).Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]aiconversation.StreamDelta, 0, len(rows))
	for _, row := range rows {
		out = append(out, aiconversation.StreamDelta{Cursor: row.Cursor, Type: row.Type, Data: row.Data})
	}
	return out, nil
}

func (s *ConversationService) Commit(ctx context.Context, streamID string, content []aimodelruntime.ContentPart) (string, error) {
	stream, err := s.stream(ctx, streamID)
	if err != nil {
		return "", err
	}
	if err = aimodelruntime.ValidateMessage(aimodelruntime.Message{Role: aimodelruntime.RoleAssistant, Content: content}); err != nil {
		return "", errs.New(errs.CodeValidation, err.Error())
	}
	var messageID string
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var locked AssistantStream
		if e := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id=?", stream.ID).First(&locked).Error; e != nil {
			return e
		}
		if locked.Status == "committed" && locked.MessageID != nil {
			messageID = *locked.MessageID
			return nil
		}
		if locked.Status != "streaming" {
			return errs.New(errs.CodeConflict, "assistant stream is not active")
		}
		var conversation Conversation
		if e := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id=? AND status=?", stream.ConversationID, "active").First(&conversation).Error; e != nil {
			return errs.New(errs.CodeConflict, "conversation is not active")
		}
		var last uint64
		if e := tx.Model(&ConversationMessage{}).Where("conversation_id=?", stream.ConversationID).Select("COALESCE(MAX(sequence),0)").Scan(&last).Error; e != nil {
			return e
		}
		payload, _ := json.Marshal(content)
		now := time.Now().UTC()
		sid := stream.ID
		message := ConversationMessage{ID: uuid.NewString(), TenantID: stream.TenantID, ConversationID: stream.ConversationID, Role: "assistant", Status: "committed", Sequence: last + 1, RunID: locked.RunID, StreamID: &sid, Content: payload, IdempotencyKey: "assistant-stream:" + stream.ID, CommittedAt: &now}
		if e := tx.Create(&message).Error; e != nil {
			return e
		}
		messageID = message.ID
		if e := tx.Model(&AssistantStream{}).Where("id=? AND status=?", stream.ID, "streaming").Updates(map[string]any{"status": "committed", "message_id": message.ID, "completed_at": now}).Error; e != nil {
			return e
		}
		return tx.Model(&Conversation{}).Where("id=?", stream.ConversationID).Updates(map[string]any{"last_message_at": now, "revision": gorm.Expr("revision + 1")}).Error
	})
	return messageID, err
}

// DeleteArchived permanently removes an archived conversation in dependency order.
// It is intentionally internal until retention policy and an administrator Action are defined.
func (s *ConversationService) DeleteArchived(ctx context.Context, id string) error {
	row, _, err := s.get(ctx, id, true)
	if err != nil {
		return err
	}
	if row.Status != "archived" {
		return errs.New(errs.CodeConflict, "conversation must be archived before cleanup")
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("conversation_id=?", row.ID).Delete(&ContextSnapshot{}).Error; err != nil {
			return err
		}
		var streamIDs []string
		if err := tx.Model(&AssistantStream{}).Where("conversation_id=?", row.ID).Pluck("id", &streamIDs).Error; err != nil {
			return err
		}
		if len(streamIDs) > 0 {
			if err := tx.Where("stream_id IN ?", streamIDs).Delete(&AssistantStreamDelta{}).Error; err != nil {
				return err
			}
		}
		if err := tx.Where("conversation_id=?", row.ID).Delete(&ConversationMessage{}).Error; err != nil {
			return err
		}
		if err := tx.Where("conversation_id=?", row.ID).Delete(&AssistantStream{}).Error; err != nil {
			return err
		}
		return tx.Where("id=? AND status=?", row.ID, "archived").Delete(&Conversation{}).Error
	})
}
