package tenant

import (
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"sync"
)

type ModelKind string

const (
	TenantOwned ModelKind = "tenant"
	GlobalModel ModelKind = "global"
)

var (
	ErrDuplicateModel = errors.New("tenant model already registered")
	ErrInvalidModel   = errors.New("invalid tenant model registration")
)

type ModelOptions struct {
	Kind           ModelKind
	TenantColumn   string
	InjectOnCreate bool
	SoftDelete     bool
	SystemAccess   bool
}

type ModelInfo struct {
	Type  reflect.Type
	Table string
	Model string
	ModelOptions
}

type Registry struct {
	mu     sync.RWMutex
	models map[reflect.Type]ModelInfo
}

func NewRegistry() *Registry { return &Registry{models: make(map[reflect.Type]ModelInfo)} }

var DefaultRegistry = NewRegistry()

func modelType(model any) (reflect.Type, error) {
	if model == nil {
		return nil, fmt.Errorf("%w: model is nil", ErrInvalidModel)
	}
	t := reflect.TypeOf(model)
	for t.Kind() == reflect.Pointer || t.Kind() == reflect.Slice || t.Kind() == reflect.Array {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct {
		return nil, fmt.Errorf("%w: model must be a struct", ErrInvalidModel)
	}
	return t, nil
}

func (r *Registry) Register(model any, opts ModelOptions) error {
	t, err := modelType(model)
	if err != nil {
		return err
	}
	if opts.Kind != TenantOwned && opts.Kind != GlobalModel {
		return fmt.Errorf("%w: unknown kind %q", ErrInvalidModel, opts.Kind)
	}
	if opts.TenantColumn == "" {
		opts.TenantColumn = "tenant_id"
	}
	if opts.Kind == TenantOwned {
		field, ok := t.FieldByName("TenantID")
		if !ok || field.Type.Kind() != reflect.String {
			return fmt.Errorf("%w: %s requires string TenantID", ErrInvalidModel, t.Name())
		}
	} else if opts.InjectOnCreate {
		return fmt.Errorf("%w: global model cannot inject tenant", ErrInvalidModel)
	}
	table := strings.ToLower(t.Name())
	if named, ok := reflect.New(t).Interface().(interface{ TableName() string }); ok {
		table = named.TableName()
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.models[t]; ok {
		return fmt.Errorf("%w: %s", ErrDuplicateModel, t.Name())
	}
	r.models[t] = ModelInfo{Type: t, Table: table, Model: t.PkgPath() + "." + t.Name(), ModelOptions: opts}
	return nil
}

func (r *Registry) Lookup(model any) (ModelInfo, bool) {
	t, err := modelType(model)
	if err != nil {
		return ModelInfo{}, false
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	info, ok := r.models[t]
	return info, ok
}

func (r *Registry) Inventory() []ModelInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]ModelInfo, 0, len(r.models))
	for _, info := range r.models {
		out = append(out, info)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Model < out[j].Model })
	return out
}

func (r *Registry) ValidateKnown(models ...any) error {
	for _, model := range models {
		if _, ok := r.Lookup(model); !ok {
			t, _ := modelType(model)
			return fmt.Errorf("%w: known model %s is not registered", ErrInvalidModel, t.Name())
		}
	}
	return nil
}
