package access

import (
	"sort"

	"github.com/yzxingtu2026/ainp/platform/framework/action"
	"github.com/yzxingtu2026/ainp/platform/framework/auth"
)

// AssignablePermissions returns the permissions that tenant roles may hold.
func AssignablePermissions(actions *action.Registry) []string {
	if actions == nil {
		return nil
	}
	set := make(map[string]struct{})
	for _, registered := range actions.List() {
		if auth.NormalizeRealm(registered.Meta.Surface) != auth.RealmTenant || registered.Meta.AccessMode() != action.AccessPermission {
			continue
		}
		set[registered.Meta.PermissionCode(registered.ID)] = struct{}{}
	}
	permissions := make([]string, 0, len(set))
	for permission := range set {
		permissions = append(permissions, permission)
	}
	sort.Strings(permissions)
	return permissions
}
