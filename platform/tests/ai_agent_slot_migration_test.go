package tests

import (
	"os"
	"strings"
	"testing"
)

func TestAgentSlotModelsAreIncludedInStartupMigrations(t *testing.T) {
	files := []string{
		"../modules/ai/agent_model.go",
		"../modules/ai/model.go",
	}
	for _, file := range files {
		body, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		for _, model := range []string{"&AgentSlotBinding{}", "&AgentSlotPreference{}"} {
			if !strings.Contains(string(body), model) {
				t.Errorf("%s must migrate %s", file, model)
			}
		}
	}
}
