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
		"permission_group.ai.provider_management", "permission_group.ai.model_catalog",
		"menu.ai.root.title", "menu.ai.providers.title", "menu.ai.models.title",
	}
	actions := []string{
		"provider_type_list", "provider_list", "provider_get", "provider_create", "provider_update",
		"provider_status_update", "provider_discover_models", "deployment_list", "deployment_get",
		"deployment_create", "deployment_import", "deployment_update", "deployment_status_update",
		"alias_list", "alias_upsert", "alias_delete",
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
