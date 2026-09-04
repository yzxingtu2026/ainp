package action

import (
	"fmt"
	"slices"
	"strings"

	"github.com/yzxingtu2026/ainp/platform/framework/auth"
)

const canonicalAPIPrefix = "/api/v1/"

// RuntimeSurfaceActionIDs is the Host-owned whitelist of conversation runtime
// Actions that may be projected onto registered Client Surfaces.
func RuntimeSurfaceActionIDs() []string {
	return []string{
		"ai.conversation_list",
		"ai.conversation_get",
		"ai.conversation_create_for_slot",
		"ai.conversation_update",
		"ai.conversation_archive",
		"ai.conversation_restore",
		"ai.message_list",
		"ai.message_get",
		"ai.message_create",
		"ai.agent_run_start",
		"ai.agent_run_get",
		"ai.agent_run_list",
		"ai.agent_run_cancel",
		"ai.agent_run_resume",
		"ai.agent_run_retry",
		"ai.agent_run_events",
		"ai.tool_confirmation_list",
		"ai.tool_confirmation_approve",
		"ai.tool_confirmation_reject",
		"ai.agent_slot_get",
		"ai.agent_slot_preference_get",
		"ai.agent_slot_preference_update",
		"ai.agent_slot_preference_reset",
		"system.attachment_mine_list",
		"system.attachment_upload_prepare",
		"system.attachment_download_prepare",
		"system.attachment_policy_get",
		"system.attachment_content_inspect",
	}
}

// ProjectedHTTPPath rewrites a canonical /api/v1/... path onto a Surface prefix.
func ProjectedHTTPPath(pathPrefix, canonicalPath string) (string, error) {
	prefix := strings.TrimSpace(pathPrefix)
	path := strings.TrimSpace(canonicalPath)
	if !strings.HasPrefix(prefix, "/") || !strings.HasPrefix(path, canonicalAPIPrefix) {
		return "", fmt.Errorf("action runtime surface path must map /api/v1/ onto a registered prefix")
	}
	return strings.TrimSuffix(prefix, "/") + "/" + strings.TrimPrefix(path, canonicalAPIPrefix), nil
}

// ProjectRuntimeSurfaces expands whitelist Actions onto every registered Client
// Surface and projects additional HTTP paths. It does not create a second
// Action identity. Permission-mode Actions are rejected.
func ProjectRuntimeSurfaces(actions *Registry, surfaces *auth.ClientSurfaceRegistry) error {
	if actions == nil || surfaces == nil {
		return fmt.Errorf("action runtime surface projection requires action and client surface registries")
	}
	for _, id := range RuntimeSurfaceActionIDs() {
		current, ok := actions.Get(id)
		if !ok {
			return fmt.Errorf("runtime surface action not registered: %s", id)
		}
		if current.Meta.AccessMode() != AccessAuthenticated {
			return fmt.Errorf("runtime surface action %s must use authenticated access", id)
		}
		if current.Meta.HTTP == nil || !current.Meta.HTTP.IsEnabled() || strings.TrimSpace(current.Meta.HTTP.Path) == "" {
			return fmt.Errorf("runtime surface action %s requires an HTTP path", id)
		}
		kinds := append([]auth.PrincipalKind(nil), current.Meta.PrincipalKinds...)
		surfaceIDs := append([]string(nil), current.Meta.ClientSurfaces...)
		for _, surface := range surfaces.List() {
			if surface.ID == auth.TenantConsoleSurface {
				continue
			}
			if !slices.Contains(surfaceIDs, surface.ID) {
				surfaceIDs = append(surfaceIDs, surface.ID)
			}
			for _, kind := range surface.PrincipalKinds {
				if !slices.Contains(kinds, kind) {
					kinds = append(kinds, kind)
				}
			}
			path, err := ProjectedHTTPPath(surface.PathPrefix, current.Meta.HTTP.Path)
			if err != nil {
				return fmt.Errorf("runtime surface action %s: %w", id, err)
			}
			if err := actions.RegisterHTTPRoute(HTTPRoute{ActionID: id, HTTP: &HTTPMeta{Method: current.Meta.HTTP.Method, Path: path}}); err != nil {
				return fmt.Errorf("runtime surface action %s: %w", id, err)
			}
		}
		if err := actions.ExpandClientAccess(id, kinds, surfaceIDs); err != nil {
			return err
		}
	}
	return nil
}

// ExpandClientAccess replaces an Action's PrincipalKinds and ClientSurfaces
// after registration. It does not change Action identity or HTTP canonical path.
func (r *Registry) ExpandClientAccess(id string, kinds []auth.PrincipalKind, surfaces []string) error {
	current, ok := r.items[id]
	if !ok {
		return fmt.Errorf("action not registered: %s", id)
	}
	if len(kinds) == 0 || len(surfaces) == 0 {
		return fmt.Errorf("action %s: expanded client access requires principal kinds and surfaces", id)
	}
	for _, kind := range kinds {
		if !kind.Valid() {
			return fmt.Errorf("action %s: invalid principal kind %q", id, kind)
		}
	}
	for _, surface := range surfaces {
		if strings.TrimSpace(surface) == "" {
			return fmt.Errorf("action %s: client surface is required", id)
		}
	}
	current.Meta.PrincipalKinds = append([]auth.PrincipalKind(nil), kinds...)
	current.Meta.ClientSurfaces = append([]string(nil), surfaces...)
	r.items[id] = current
	return nil
}
