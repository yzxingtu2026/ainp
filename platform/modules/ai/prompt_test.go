package ai

import (
	"os"
	"testing"

	"github.com/yzxingtu2026/ainp/platform/framework/prompt"
)

func TestMain(m *testing.M) {
	if err := prompt.LoadDir("../../prompts"); err != nil {
		panic(err)
	}
	os.Exit(m.Run())
}
