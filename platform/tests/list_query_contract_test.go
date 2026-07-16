package tests

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"testing"
)

func TestListActionsDoNotReintroduceLegacyPaginationProtocol(t *testing.T) {
	legacyRequest := regexp.MustCompile(`json:"current"`)
	legacyResult := regexp.MustCompile(`"records"\s*:`)
	err := filepath.WalkDir("../modules", func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() || filepath.Ext(path) != ".go" {
			return err
		}
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		if legacyRequest.Match(data) || legacyResult.Match(data) {
			t.Errorf("legacy list pagination protocol found in %s", path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
