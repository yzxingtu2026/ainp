package statemachine

import (
	"fmt"
	"sort"
	"sync"

	"github.com/yzxingtu2026/ainp/platform/framework/action"
	"github.com/yzxingtu2026/ainp/platform/framework/codeset"
)

type Transition struct {
	From       string `json:"from"`
	To         string `json:"to"`
	Action     string `json:"action"`
	Permission string `json:"permission,omitempty"`
	Guard      string `json:"guard,omitempty"`
}

type StateMachine struct {
	Key         string       `json:"key"`
	CodeSetKey  string       `json:"codeset_key"`
	Terminal    []string     `json:"terminal"`
	Transitions []Transition `json:"transitions"`
}

type Registry struct {
	mu    sync.RWMutex
	items map[string]StateMachine
}

func NewRegistry() *Registry { return &Registry{items: make(map[string]StateMachine)} }

func (r *Registry) Register(machine StateMachine, sets *codeset.Registry, actions *action.Registry) error {
	if machine.Key == "" {
		return fmt.Errorf("state machine key is required")
	}
	set, ok := sets.Get(machine.CodeSetKey)
	if !ok {
		return fmt.Errorf("state machine codeset not registered: %s", machine.CodeSetKey)
	}
	values := make(map[string]struct{}, len(set.Items))
	for _, item := range set.Items {
		values[item.Value] = struct{}{}
	}
	terminal := make(map[string]struct{}, len(machine.Terminal))
	for _, value := range machine.Terminal {
		if _, ok := values[value]; !ok {
			return fmt.Errorf("unknown terminal state: %s", value)
		}
		terminal[value] = struct{}{}
	}
	seen := make(map[string]struct{}, len(machine.Transitions))
	for _, tr := range machine.Transitions {
		if _, ok := values[tr.From]; !ok {
			return fmt.Errorf("unknown transition from state: %s", tr.From)
		}
		if _, ok := values[tr.To]; !ok {
			return fmt.Errorf("unknown transition to state: %s", tr.To)
		}
		if _, ok := terminal[tr.From]; ok {
			return fmt.Errorf("terminal state has outgoing transition: %s", tr.From)
		}
		if tr.Action == "" {
			return fmt.Errorf("transition action is required: %s -> %s", tr.From, tr.To)
		}
		act, ok := actions.Get(tr.Action)
		if !ok {
			return fmt.Errorf("transition action not registered: %s", tr.Action)
		}
		permission := tr.Permission
		if permission == "" {
			permission = act.Meta.PermissionCode(act.ID)
		}
		if permission != act.Meta.PermissionCode(act.ID) {
			return fmt.Errorf("transition permission does not match action: %s", tr.Action)
		}
		key := tr.From + "\x00" + tr.To + "\x00" + tr.Action
		if _, ok := seen[key]; ok {
			return fmt.Errorf("duplicate transition: %s -> %s via %s", tr.From, tr.To, tr.Action)
		}
		seen[key] = struct{}{}
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.items[machine.Key]; ok {
		return fmt.Errorf("state machine already registered: %s", machine.Key)
	}
	r.items[machine.Key] = machine
	return nil
}

func (r *Registry) Can(key, from, to, actionID string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	machine, ok := r.items[key]
	if !ok {
		return false
	}
	for _, tr := range machine.Transitions {
		if tr.From == from && tr.To == to && tr.Action == actionID {
			return true
		}
	}
	return false
}
func (r *Registry) List() []StateMachine {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]StateMachine, 0, len(r.items))
	for _, item := range r.items {
		out = append(out, item)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out
}
