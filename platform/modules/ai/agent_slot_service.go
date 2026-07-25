package ai

import (
	"context"
	"strings"
	"sync"

	"github.com/google/uuid"
	"github.com/yzxingtu2026/ainp/platform/framework/agentslot"
	"github.com/yzxingtu2026/ainp/platform/framework/auth"
	"github.com/yzxingtu2026/ainp/platform/framework/errs"
	"github.com/yzxingtu2026/ainp/platform/framework/logging"
	"github.com/yzxingtu2026/ainp/platform/framework/pagination"
	tenantdata "github.com/yzxingtu2026/ainp/platform/framework/tenant"
	"gorm.io/gorm"
)

const WorkspaceDefaultSlot = "workspace.default"

var agentSlotModels struct {
	sync.Once
	err error
}

func registerAgentSlotModels() error {
	agentSlotModels.Do(func() {
		for _, model := range []any{AgentSlotBinding{}, AgentSlotPreference{}} {
			if err := tenantdata.DefaultRegistry.Register(model, tenantdata.ModelOptions{Kind: tenantdata.TenantOwned, InjectOnCreate: true, SystemAccess: true}); err != nil {
				agentSlotModels.err = err
				return
			}
		}
	})
	return agentSlotModels.err
}

type AgentSlotService struct {
	db      *gorm.DB
	slots   *agentslot.Registry
	agents  *AgentService
	changed func(context.Context, string) error
}

func NewAgentSlotService(db *gorm.DB, slots *agentslot.Registry, agents *AgentService) *AgentSlotService {
	if err := registerAgentSlotModels(); err != nil {
		panic(err)
	}
	return &AgentSlotService{db: db, slots: slots, agents: agents}
}
func (s *AgentSlotService) SetChangedHook(hook func(context.Context, string) error) { s.changed = hook }
func (s *AgentSlotService) notifyChanged(ctx context.Context, slotKey string) error {
	if s.changed == nil {
		return nil
	}
	if err := s.changed(ctx, slotKey); err != nil {
		logging.Warn(ctx, "agent slot derived refresh failed", "slot_key", slotKey, "error", err)
	}
	return nil
}

type AgentSlotListRequest struct {
	pagination.Request
	Keyword string `json:"keyword" validate:"omitempty"`
	Module  string `json:"module" validate:"omitempty"`
	Status  string `json:"status" validate:"omitempty"`
	Source  string `json:"source" validate:"omitempty"`
}
type AgentSlotGetRequest struct {
	SlotKey string `json:"slot_key" validate:"required"`
}
type AgentSlotMutationRequest struct {
	SlotKey  string `json:"slot_key" validate:"required"`
	AgentID  string `json:"agent_id" validate:"required"`
	Revision uint64 `json:"revision" validate:"omitempty"`
}
type AgentSlotResetRequest struct {
	SlotKey  string `json:"slot_key" validate:"required"`
	Revision uint64 `json:"revision" validate:"required"`
}
type AgentSlotResolution struct {
	Slot               agentslot.Slot         `json:"slot"`
	Agent              *Agent                 `json:"agent,omitempty"`
	AgentVersion       *AgentVersion          `json:"agent_version,omitempty"`
	Source             string                 `json:"source"`
	Healthy            bool                   `json:"healthy"`
	Diagnostics        []AgentDiagnosticIssue `json:"diagnostics"`
	BindingRevision    uint64                 `json:"binding_revision,omitempty"`
	PreferenceRevision uint64                 `json:"preference_revision,omitempty"`
}

func slotActor(ctx context.Context) (*auth.Context, error) {
	ac := auth.MustFromContext(ctx)
	if !ac.Authenticated || ac.TenantID == "" || ac.UserID == "" {
		return nil, errs.New(errs.CodeUnauthorized, "unauthorized")
	}
	return ac, nil
}
func (s *AgentSlotService) tenantDB(ctx context.Context) (tenantdata.TenantDatabase, error) {
	return tenantdata.TenantDB(ctx, s.db)
}
func slotUnavailable(code, message string, slot agentslot.Slot, source string) *AgentSlotResolution {
	return &AgentSlotResolution{Slot: slot, Source: source, Healthy: false, Diagnostics: []AgentDiagnosticIssue{{Code: code, Message: message}}}
}

func optionalSlotRecord[T any](repo *tenantdata.Repository[T], where any, args ...any) (T, bool, error) {
	rows, err := repo.Find(where, args...)
	if err != nil {
		var zero T
		return zero, false, err
	}
	if len(rows) == 0 {
		var zero T
		return zero, false, nil
	}
	return rows[0], true, nil
}

func (s *AgentSlotService) resolveAgent(ctx context.Context, slot agentslot.Slot, agentID, source string) (*AgentSlotResolution, error) {
	ac, _ := slotActor(ctx)
	repos, err := newAgentRepositories(ctx, s.db)
	if err != nil {
		return nil, err
	}
	agent, err := repos.agents.First("id=? AND (owner_user_id=? OR visibility='tenant')", agentID, ac.UserID)
	if err != nil {
		return slotUnavailable("agent.unavailable", "configured agent is unavailable", slot, source), nil
	}
	if agent.Status != "enabled" || agent.CurrentVersion <= 0 {
		return slotUnavailable("agent.disabled_or_unpublished", "configured agent is disabled or unpublished", slot, source), nil
	}
	if !s.slots.AcceptsContract(slot, agent.ContractKey, agent.ContractVersion) {
		return slotUnavailable("agent.contract_mismatch", "configured agent does not satisfy the slot contract", slot, source), nil
	}
	version, err := repos.versions.First("agent_id=? AND version=? AND kind=?", agent.ID, agent.CurrentVersion, AgentVersionKindRelease)
	if err != nil {
		return slotUnavailable("version.unavailable", "configured agent current version is unavailable", slot, source), nil
	}
	diagnostics := s.agents.agentDiagnostics(ctx, &agent)
	return &AgentSlotResolution{Slot: slot, Agent: &agent, AgentVersion: &version, Source: source, Healthy: diagnostics.Healthy, Diagnostics: diagnostics.Issues}, nil
}

func (s *AgentSlotService) Resolve(ctx context.Context, r AgentSlotGetRequest) (*AgentSlotResolution, error) {
	ac, err := slotActor(ctx)
	if err != nil {
		return nil, err
	}
	slot, ok := s.slots.Current(strings.TrimSpace(r.SlotKey))
	if !ok {
		return nil, errs.New(errs.CodeNotFound, "agent slot not found")
	}
	tdb, err := s.tenantDB(ctx)
	if err != nil {
		return nil, err
	}
	prefs, _ := tenantdata.NewTenantRepository[AgentSlotPreference](tdb)
	bindings, _ := tenantdata.NewTenantRepository[AgentSlotBinding](tdb)
	if slot.UserOverride {
		pref, found, lookupErr := optionalSlotRecord(prefs, "user_id=? AND slot_key=?", ac.UserID, slot.Key)
		if lookupErr != nil {
			return nil, lookupErr
		}
		if found {
			result, e := s.resolveAgent(ctx, slot, pref.AgentID, "user")
			if result != nil {
				result.PreferenceRevision = pref.Revision
			}
			return result, e
		}
	}
	if slot.TenantOverride {
		binding, found, lookupErr := optionalSlotRecord(bindings, "slot_key=?", slot.Key)
		if lookupErr != nil {
			return nil, lookupErr
		}
		if found {
			result, e := s.resolveAgent(ctx, slot, binding.AgentID, "tenant")
			if result != nil {
				result.BindingRevision = binding.Revision
			}
			return result, e
		}
	}
	repos, err := newAgentRepositories(ctx, s.db)
	if err != nil {
		return nil, err
	}
	agent, err := repos.agents.First("source='builtin' AND contract_key=?", slot.DefaultContractKey)
	if err != nil {
		return slotUnavailable("default.unavailable", "system default agent is unavailable", slot, "builtin"), nil
	}
	return s.resolveAgent(ctx, slot, agent.ID, "builtin")
}

func (s *AgentSlotService) Get(ctx context.Context, r AgentSlotGetRequest) (any, error) {
	return s.Resolve(ctx, r)
}
func (s *AgentSlotService) List(ctx context.Context, r AgentSlotListRequest) (any, error) {
	if _, err := slotActor(ctx); err != nil {
		return nil, err
	}
	r.Normalize()
	items := []AgentSlotResolution{}
	for _, slot := range s.slots.List() {
		if r.Keyword != "" && !strings.Contains(strings.ToLower(slot.Title+" "+slot.Key), strings.ToLower(r.Keyword)) {
			continue
		}
		if r.Module != "" && slot.Module != r.Module {
			continue
		}
		resolved, err := s.Resolve(ctx, AgentSlotGetRequest{SlotKey: slot.Key})
		if err != nil {
			return nil, err
		}
		if r.Status != "" && (r.Status == "healthy") != resolved.Healthy {
			continue
		}
		if r.Source != "" && resolved.Source != r.Source {
			continue
		}
		items = append(items, *resolved)
	}
	total := len(items)
	start := r.Offset()
	if start > total {
		start = total
	}
	end := start + r.PageSize
	if end > total {
		end = total
	}
	return pagination.NewResult(items[start:end], int64(total), r.Request), nil
}

func (s *AgentSlotService) validateChoice(ctx context.Context, slot agentslot.Slot, agentID string, tenantBinding bool) error {
	resolved, err := s.resolveAgent(ctx, slot, agentID, "candidate")
	if err != nil {
		return err
	}
	if !resolved.Healthy || (tenantBinding && (resolved.Agent == nil || resolved.Agent.Visibility != "tenant")) {
		return errs.New(errs.CodeNotFound, "compatible agent not found")
	}
	return nil
}
func (s *AgentSlotService) Update(ctx context.Context, r AgentSlotMutationRequest) (any, error) {
	ac, err := slotActor(ctx)
	if err != nil {
		return nil, err
	}
	slot, ok := s.slots.Current(r.SlotKey)
	if !ok {
		return nil, errs.New(errs.CodeNotFound, "agent slot not found")
	}
	if !slot.TenantOverride {
		return nil, errs.New(errs.CodeForbidden, "tenant override is not allowed")
	}
	if err = s.validateChoice(ctx, slot, r.AgentID, true); err != nil {
		return nil, err
	}
	tdb, err := s.tenantDB(ctx)
	if err != nil {
		return nil, err
	}
	repo, _ := tenantdata.NewTenantRepository[AgentSlotBinding](tdb)
	row, found, qerr := optionalSlotRecord(repo, "slot_key=?", slot.Key)
	if qerr != nil {
		return nil, qerr
	}
	if !found {
		if r.Revision != 0 {
			return nil, errs.New(errs.CodeConflict, "agent slot binding revision conflict")
		}
		row = AgentSlotBinding{ID: uuid.NewString(), SlotKey: slot.Key, SlotVersion: slot.Version, AgentID: r.AgentID, Revision: 1, CreatedBy: ac.UserID, UpdatedBy: ac.UserID}
		if err = repo.Create(&row); err != nil {
			return nil, err
		}
		if err = s.notifyChanged(ctx, slot.Key); err != nil {
			return nil, err
		}
		return s.Resolve(ctx, AgentSlotGetRequest{SlotKey: slot.Key})
	}
	if row.Revision != r.Revision {
		return nil, errs.New(errs.CodeConflict, "agent slot binding revision conflict")
	}
	res := repo.Where("id=? AND revision=?", row.ID, r.Revision).Updates(map[string]any{"slot_version": slot.Version, "agent_id": r.AgentID, "updated_by": ac.UserID, "revision": r.Revision + 1})
	if res.Error != nil {
		return nil, res.Error
	}
	if res.RowsAffected != 1 {
		return nil, errs.New(errs.CodeConflict, "agent slot binding revision conflict")
	}
	if err = s.notifyChanged(ctx, slot.Key); err != nil {
		return nil, err
	}
	return s.Resolve(ctx, AgentSlotGetRequest{SlotKey: slot.Key})
}
func (s *AgentSlotService) Reset(ctx context.Context, r AgentSlotResetRequest) (any, error) {
	if _, err := slotActor(ctx); err != nil {
		return nil, err
	}
	tdb, err := s.tenantDB(ctx)
	if err != nil {
		return nil, err
	}
	repo, _ := tenantdata.NewTenantRepository[AgentSlotBinding](tdb)
	res := repo.DeleteWhere("slot_key=? AND revision=?", r.SlotKey, r.Revision)
	if res.Error != nil {
		return nil, res.Error
	}
	if res.RowsAffected != 1 {
		return nil, errs.New(errs.CodeConflict, "agent slot binding revision conflict")
	}
	if err = s.notifyChanged(ctx, r.SlotKey); err != nil {
		return nil, err
	}
	return s.Resolve(ctx, AgentSlotGetRequest{SlotKey: r.SlotKey})
}
func (s *AgentSlotService) UpdatePreference(ctx context.Context, r AgentSlotMutationRequest) (any, error) {
	return s.mutatePreference(ctx, r, false)
}
func (s *AgentSlotService) ResetPreference(ctx context.Context, r AgentSlotResetRequest) (any, error) {
	_, err := slotActor(ctx)
	if err != nil {
		return nil, err
	}
	tdb, err := s.tenantDB(ctx)
	if err != nil {
		return nil, err
	}
	repo, _ := tenantdata.NewTenantRepository[AgentSlotPreference](tdb)
	ac := auth.MustFromContext(ctx)
	res := repo.DeleteWhere("user_id=? AND slot_key=? AND revision=?", ac.UserID, r.SlotKey, r.Revision)
	if res.Error != nil {
		return nil, res.Error
	}
	if res.RowsAffected != 1 {
		return nil, errs.New(errs.CodeConflict, "agent slot preference revision conflict")
	}
	if err = s.notifyChanged(ctx, r.SlotKey); err != nil {
		return nil, err
	}
	return s.Resolve(ctx, AgentSlotGetRequest{SlotKey: r.SlotKey})
}
func (s *AgentSlotService) mutatePreference(ctx context.Context, r AgentSlotMutationRequest, _ bool) (any, error) {
	ac, err := slotActor(ctx)
	if err != nil {
		return nil, err
	}
	slot, ok := s.slots.Current(r.SlotKey)
	if !ok {
		return nil, errs.New(errs.CodeNotFound, "agent slot not found")
	}
	if !slot.UserOverride {
		return nil, errs.New(errs.CodeForbidden, "user override is not allowed")
	}
	if err = s.validateChoice(ctx, slot, r.AgentID, false); err != nil {
		return nil, err
	}
	tdb, err := s.tenantDB(ctx)
	if err != nil {
		return nil, err
	}
	repo, _ := tenantdata.NewTenantRepository[AgentSlotPreference](tdb)
	row, found, qerr := optionalSlotRecord(repo, "user_id=? AND slot_key=?", ac.UserID, slot.Key)
	if qerr != nil {
		return nil, qerr
	}
	if !found {
		if r.Revision != 0 {
			return nil, errs.New(errs.CodeConflict, "agent slot preference revision conflict")
		}
		row = AgentSlotPreference{ID: uuid.NewString(), UserID: ac.UserID, SlotKey: slot.Key, SlotVersion: slot.Version, AgentID: r.AgentID, Revision: 1, UpdatedBy: ac.UserID}
		if err = repo.Create(&row); err != nil {
			return nil, err
		}
		if err = s.notifyChanged(ctx, slot.Key); err != nil {
			return nil, err
		}
		return s.Resolve(ctx, AgentSlotGetRequest{SlotKey: slot.Key})
	}
	if row.Revision != r.Revision {
		return nil, errs.New(errs.CodeConflict, "agent slot preference revision conflict")
	}
	res := repo.Where("id=? AND revision=?", row.ID, r.Revision).Updates(map[string]any{"slot_version": slot.Version, "agent_id": r.AgentID, "updated_by": ac.UserID, "revision": r.Revision + 1})
	if res.Error != nil {
		return nil, res.Error
	}
	if res.RowsAffected != 1 {
		return nil, errs.New(errs.CodeConflict, "agent slot preference revision conflict")
	}
	if err = s.notifyChanged(ctx, slot.Key); err != nil {
		return nil, err
	}
	return s.Resolve(ctx, AgentSlotGetRequest{SlotKey: slot.Key})
}
func (s *AgentSlotService) Reconcile(ctx context.Context, _ AgentEmptyRequest) (any, error) {
	if _, err := slotActor(ctx); err != nil {
		return nil, err
	}
	results := []AgentSlotResolution{}
	for _, slot := range s.slots.List() {
		resolved, err := s.Resolve(ctx, AgentSlotGetRequest{SlotKey: slot.Key})
		if err != nil {
			return nil, err
		}
		results = append(results, *resolved)
	}
	return map[string]any{"slots": results, "changed": false}, nil
}
