package system

import (
	"github.com/yzxingtu2026/ainp/platform/framework/action"
	"github.com/yzxingtu2026/ainp/platform/framework/pagination"
)

const aiExclusionMutatingIdentity = "identity and authorization mutations are excluded from the initial AI action allowlist"

type permissionCatalogResponse struct {
	Groups []PermissionCatalogGroup `json:"groups"`
}

func userActionAI(actionID string) *action.AIMeta {
	switch actionID {
	case ActionUserList:
		return exposedReadAI(
			"查询当前租户的用户列表，可按关键词、状态和角色筛选",
			"查询系统用户列表",
			[]string{"用户", "用户管理", "列表", "查询"},
			action.SchemaFor[userListRequest](),
			action.SchemaFor[pagination.Result[UserView]](),
			"已返回当前用户有权查看的系统用户列表",
		)
	case ActionUserGet:
		return exposedReadAI(
			"按用户 ID 查询当前租户中的用户资料和角色",
			"查询系统用户详情",
			[]string{"用户", "用户管理", "详情", "角色"},
			action.SchemaFor[userIDRequest](),
			action.SchemaFor[UserView](),
			"已返回当前用户有权查看的系统用户详情",
		)
	default:
		return action.ExcludeFromAI(aiExclusionMutatingIdentity)
	}
}

func roleActionAI(actionID string) *action.AIMeta {
	switch actionID {
	case ActionRoleList:
		return exposedReadAI(
			"查询当前租户的角色列表，可按关键词和状态筛选",
			"查询系统角色列表",
			[]string{"角色", "权限管理", "列表", "查询"},
			action.SchemaFor[roleListRequest](),
			action.SchemaFor[pagination.Result[RoleView]](),
			"已返回当前用户有权查看的系统角色列表",
		)
	case ActionRoleGet:
		return exposedReadAI(
			"按角色 ID 查询当前租户中的角色详情和权限",
			"查询系统角色详情",
			[]string{"角色", "权限管理", "详情", "权限"},
			action.SchemaFor[roleIDRequest](),
			action.SchemaFor[RoleView](),
			"已返回当前用户有权查看的系统角色详情",
		)
	case ActionPermissionCatalog:
		return exposedReadAI(
			"查询当前租户可分配的权限目录及其功能分组",
			"查询系统权限目录",
			[]string{"权限", "权限目录", "功能", "查询"},
			action.SchemaFor[struct{}](),
			action.SchemaFor[permissionCatalogResponse](),
			"已返回当前用户有权查看的系统权限目录",
		)
	default:
		return action.ExcludeFromAI(aiExclusionMutatingIdentity)
	}
}

func exposedReadAI(summary, human string, tags []string, input, output []byte, resultSummary string) *action.AIMeta {
	return &action.AIMeta{
		ExposeTool:       true,
		Risk:             "low",
		SideEffect:       "read",
		Summary:          summary,
		HumanDescription: human,
		Tags:             tags,
		ResultSummary:    resultSummary,
		InputSchema:      input,
		OutputSchema:     output,
	}
}
