package menu

import (
	"fmt"
	"sort"
	"strings"
)

const (
	TypeDirectory = "directory"
	TypeMenu      = "menu"
	TypeButton    = "button"
)

type Menu struct {
	ID         string `json:"id"`
	ParentID   string `json:"parent_id,omitempty"`
	Title      string `json:"title"`
	TitleKey   string `json:"title_key,omitempty"`
	Icon       string `json:"icon,omitempty"`
	LocalIcon  string `json:"local_icon,omitempty"`
	Order      int    `json:"order"`
	Type       string `json:"type"`
	Path       string `json:"path,omitempty"`
	Component  string `json:"component,omitempty"`
	Permission string `json:"permission,omitempty"`
	BindAction string `json:"bind_action,omitempty"`
	HideInMenu bool   `json:"hide_in_menu,omitempty"`
	KeepAlive  bool   `json:"keep_alive,omitempty"`
	Module     string `json:"module,omitempty"`
}

type Registry struct {
	items map[string]Menu
}

func NewRegistry() *Registry {
	return &Registry{items: make(map[string]Menu)}
}

func (r *Registry) Register(m Menu) error {
	if m.ID == "" {
		return fmt.Errorf("menu id is required")
	}
	if m.Type == "" {
		return fmt.Errorf("menu type is required")
	}
	switch m.Type {
	case TypeDirectory, TypeMenu, TypeButton:
	default:
		return fmt.Errorf("invalid menu type: %s", m.Type)
	}
	if m.Type == TypeButton && m.Permission == "" {
		return fmt.Errorf("button menu requires permission: %s", m.ID)
	}
	if m.Icon != "" && !strings.Contains(m.Icon, ":") {
		return fmt.Errorf("menu icon must be collection:name: %s", m.Icon)
	}
	if _, exists := r.items[m.ID]; exists {
		return fmt.Errorf("menu already registered: %s", m.ID)
	}
	r.items[m.ID] = m
	return nil
}

func (r *Registry) List() []Menu {
	out := make([]Menu, 0, len(r.items))
	for _, m := range r.items {
		out = append(out, m)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Order == out[j].Order {
			return out[i].ID < out[j].ID
		}
		return out[i].Order < out[j].Order
	})
	return out
}

// TreeForUser 返回侧栏可见节点（过滤 button）；按 permission 过滤。
func (r *Registry) TreeForUser(hasPerm func(string) bool) []Menu {
	all := r.List()
	out := make([]Menu, 0, len(all))
	for _, m := range all {
		if m.Type == TypeButton || m.HideInMenu {
			continue
		}
		if m.Permission != "" && hasPerm != nil && !hasPerm(m.Permission) {
			continue
		}
		out = append(out, m)
	}
	return out
}

func (r *Registry) ButtonsForUser(hasPerm func(string) bool) []Menu {
	all := r.List()
	out := make([]Menu, 0)
	for _, m := range all {
		if m.Type != TypeButton {
			continue
		}
		if m.Permission != "" && hasPerm != nil && !hasPerm(m.Permission) {
			continue
		}
		out = append(out, m)
	}
	return out
}
