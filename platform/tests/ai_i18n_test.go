package tests

import (
	"path/filepath"
	"testing"

	"github.com/yzxingtu2026/ainp/platform/framework/i18n"
)

func TestAIRegistrationTranslationKeysExist(t *testing.T) {
	if err := i18n.LoadDir(filepath.Join("..", "i18n")); err != nil {
		t.Fatal(err)
	}
	keys := []string{
		"capability.ai.name", "capability.ai.description",
		"permission_group.ai.provider_management", "permission_group.ai.model_catalog", "permission_group.ai.agent_management",
		"menu.ai.root.title", "menu.ai.providers.title", "menu.ai.models.title", "menu.ai.agents.title", "menu.ai.knowledge.title",
		"ai.model.streaming_not_supported", "ai.model.vision_not_supported", "ai.model.capability_not_supported",
	}
	actions := []string{
		"provider_type_list", "provider_list", "provider_get", "provider_create", "provider_update",
		"provider_status_update", "provider_discover_models", "deployment_list", "deployment_get",
		"deployment_create", "deployment_import", "deployment_update", "deployment_status_update",
		"deployment_delete",
		"alias_list", "alias_upsert", "alias_delete",
		"agent_list", "agent_get", "agent_create", "agent_update", "agent_duplicate", "agent_status_update",
		"agent_version_list", "agent_version_get", "agent_version_create", "agent_version_publish",
		"agent_tool_search", "agent_tool_list", "agent_tool_bind", "agent_tool_unbind",
		"agent_diagnostics_get", "agent_builtin_reconcile",
	}
	for _, actionID := range actions {
		keys = append(keys, "action.ai."+actionID+".name", "action.ai."+actionID+".description")
	}
	for _, lang := range []string{i18n.LangZhCN, i18n.LangEn} {
		for _, key := range keys {
			if translated := i18n.T(lang, key); translated == key || translated == "" {
				t.Errorf("missing %s translation for %s", lang, key)
			}
		}
	}
}
