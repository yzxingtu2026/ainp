package logcenter

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

var (
	ErrUnavailable = errors.New("log provider unavailable")
	ErrTimeout     = errors.New("log provider timeout")
	ErrInvalid     = errors.New("invalid log query")
	ErrForbidden   = errors.New("log query forbidden")
	ErrTooLarge    = errors.New("log result too large")
	ErrNotFound    = errors.New("log entry not found")
)

type Capabilities struct {
	TenantFilter bool     `json:"tenant_filter"`
	Levels       []string `json:"levels,omitempty"`
	Description  string   `json:"description,omitempty"`
}

type Source struct {
	Name         string       `json:"name"`
	DisplayName  string       `json:"display_name"`
	Capabilities Capabilities `json:"capabilities"`
}

type Query struct {
	StartAt   time.Time `json:"start_at"`
	EndAt     time.Time `json:"end_at"`
	Level     string    `json:"level,omitempty"`
	Service   string    `json:"service,omitempty"`
	Component string    `json:"component,omitempty"`
	Keyword   string    `json:"keyword,omitempty"`
	RequestID string    `json:"request_id,omitempty"`
	TraceID   string    `json:"trace_id,omitempty"`
	ActionID  string    `json:"action_id,omitempty"`
	JobType   string    `json:"job_type,omitempty"`
	JobRunID  string    `json:"job_run_id,omitempty"`
	EventID   string    `json:"event_id,omitempty"`
	Page      int       `json:"page"`
	PageSize  int       `json:"page_size"`
	TenantID  string    `json:"-"`
}

type Ref struct {
	ID       string `json:"id"`
	TenantID string `json:"-"`
}

type Entry struct {
	ID         string         `json:"id"`
	Timestamp  time.Time      `json:"timestamp"`
	Level      string         `json:"level"`
	Service    string         `json:"service,omitempty"`
	Component  string         `json:"component,omitempty"`
	Message    string         `json:"message"`
	RequestID  string         `json:"request_id,omitempty"`
	TraceID    string         `json:"trace_id,omitempty"`
	TenantID   string         `json:"tenant_id,omitempty"`
	ActorID    string         `json:"actor_id,omitempty"`
	ActionID   string         `json:"action_id,omitempty"`
	JobType    string         `json:"job_type,omitempty"`
	JobRunID   string         `json:"job_run_id,omitempty"`
	EventID    string         `json:"event_id,omitempty"`
	Source     string         `json:"source"`
	Attributes map[string]any `json:"attributes,omitempty"`
}

type Page struct {
	Items    []Entry `json:"items"`
	Total    int64   `json:"total"`
	Page     int     `json:"page"`
	PageSize int     `json:"page_size"`
}

type Provider interface {
	Name() string
	DisplayName() string
	Capabilities() Capabilities
	Query(context.Context, Query) (Page, error)
	Get(context.Context, Ref) (*Entry, error)
}

type Registry struct {
	mu    sync.RWMutex
	items map[string]Provider
}

func NewRegistry() *Registry { return &Registry{items: map[string]Provider{}} }

func (r *Registry) Register(provider Provider) error {
	if provider == nil || strings.TrimSpace(provider.Name()) == "" {
		return errors.New("log provider name is required")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.items[provider.Name()]; exists {
		return fmt.Errorf("log provider already registered: %s", provider.Name())
	}
	r.items[provider.Name()] = provider
	return nil
}

func (r *Registry) Get(name string) (Provider, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	p, ok := r.items[name]
	return p, ok
}

func (r *Registry) List() []Source {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Source, 0, len(r.items))
	for _, p := range r.items {
		out = append(out, Source{Name: p.Name(), DisplayName: p.DisplayName(), Capabilities: p.Capabilities()})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

type FakeProvider struct {
	SourceName string
	Label      string
	Caps       Capabilities
	Entries    []Entry
	Err        error
}

func (p *FakeProvider) Name() string { return p.SourceName }
func (p *FakeProvider) DisplayName() string {
	if p.Label != "" {
		return p.Label
	}
	return p.SourceName
}
func (p *FakeProvider) Capabilities() Capabilities { return p.Caps }

func (p *FakeProvider) Query(ctx context.Context, q Query) (Page, error) {
	if p.Err != nil {
		return Page{}, p.Err
	}
	var rows []Entry
	for _, row := range p.Entries {
		if err := ctx.Err(); err != nil {
			return Page{}, ErrTimeout
		}
		if row.Timestamp.Before(q.StartAt) || row.Timestamp.After(q.EndAt) || (q.TenantID != "" && row.TenantID != q.TenantID) {
			continue
		}
		if !matches(row, q) {
			continue
		}
		row.Source = p.Name()
		rows = append(rows, row)
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].Timestamp.After(rows[j].Timestamp) })
	total := int64(len(rows))
	start := (q.Page - 1) * q.PageSize
	if start > len(rows) {
		start = len(rows)
	}
	end := start + q.PageSize
	if end > len(rows) {
		end = len(rows)
	}
	return Page{Items: rows[start:end], Total: total, Page: q.Page, PageSize: q.PageSize}, nil
}

func (p *FakeProvider) Get(ctx context.Context, ref Ref) (*Entry, error) {
	if p.Err != nil {
		return nil, p.Err
	}
	for _, row := range p.Entries {
		if row.ID == ref.ID && (ref.TenantID == "" || row.TenantID == ref.TenantID) {
			row.Source = p.Name()
			return &row, nil
		}
	}
	return nil, ErrNotFound
}

func matches(e Entry, q Query) bool {
	values := [][2]string{{e.Level, q.Level}, {e.Service, q.Service}, {e.Component, q.Component}, {e.RequestID, q.RequestID}, {e.TraceID, q.TraceID}, {e.ActionID, q.ActionID}, {e.JobType, q.JobType}, {e.JobRunID, q.JobRunID}, {e.EventID, q.EventID}}
	for _, pair := range values {
		if pair[1] != "" && pair[0] != pair[1] {
			return false
		}
	}
	return q.Keyword == "" || strings.Contains(strings.ToLower(e.Message), strings.ToLower(q.Keyword))
}
