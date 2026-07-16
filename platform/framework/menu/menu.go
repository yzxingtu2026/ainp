package menu

import (
	"fmt"
	"sort"
	"strings"

	"github.com/yzxingtu2026/ainp/platform/framework/auth"
)

const (
	TypeDirectory = "directory"
	TypeMenu      = "menu"
	TypeButton    = "button"
)

type Menu struct {
	ID         string     `json:"id"`
	ParentID   string     `json:"parent_id,omitempty"`
	Title      string     `json:"title"`
	TitleKey   string     `json:"title_key,omitempty"`
	Icon       string     `json:"icon,omitempty"`
	LocalIcon  string     `json:"local_icon,omitempty"`
	Order      int        `json:"order"`
	Type       string     `json:"type"`
	Path       string     `json:"path,omitempty"`
	Component  string     `json:"component,omitempty"`
	Permission string     `json:"permission,omitempty"`
	BindAction string     `json:"bind_action,omitempty"`
	HideInMenu bool       `json:"hide_in_menu,omitempty"`
	KeepAlive  bool       `json:"keep_alive,omitempty"`
	Module     string     `json:"module,omitempty"`
	Surface    auth.Realm `json:"surface"`
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
	m.Surface = auth.NormalizeRealm(m.Surface)
	if !m.Surface.Valid() {
		return fmt.Errorf("invalid menu surface: %s", m.Surface)
	}
	switch m.Type {
	case TypeDirectory, TypeMenu, TypeButton:
	default:
		return fmt.Errorf("invalid menu type: %s", m.Type)
	}
	if m.Type == TypeButton && m.Permission == "" && m.BindAction == "" {
		return fmt.Errorf("button menu requires permission or bind_action: %s", m.ID)
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

// TreeForUser 返回用户可见菜单节点（过滤 button，保留 HideInMenu 以供前端路由注册）。
// HideInMenu 的菜单仍返回给前端用于路由注册，前端通过 hideInMenu meta 控制侧栏隐藏。
func (r *Registry) TreeForUser(realm auth.Realm, hasPerm func(string) bool) []Menu {
	all := r.List()
	out := make([]Menu, 0, len(all))
	for _, m := range all {
		if m.Surface != auth.NormalizeRealm(realm) {
			continue
		}
		if m.Type == TypeButton {
			continue
		}
		if m.Permission != "" && hasPerm != nil && !hasPerm(m.Permission) {
			continue
		}
		out = append(out, m)
	}
	return out
}

func (r *Registry) ButtonsForUser(realm auth.Realm, hasPerm func(string) bool) []Menu {
	all := r.List()
	out := make([]Menu, 0)
	for _, m := range all {
		if m.Surface != auth.NormalizeRealm(realm) {
			continue
		}
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
