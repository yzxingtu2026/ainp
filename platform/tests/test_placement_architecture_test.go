package tests

import (
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func scatteredProductionTests(root string) ([]string, error) {
	var violations []string
	for _, directory := range []string{"framework", "modules"} {
		base := filepath.Join(root, directory)
		err := filepath.WalkDir(base, func(path string, entry fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if !entry.IsDir() && strings.HasSuffix(entry.Name(), "_test.go") {
				relative, relErr := filepath.Rel(root, path)
				if relErr != nil {
					return relErr
				}
				violations = append(violations, filepath.ToSlash(relative))
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	slices.Sort(violations)
	return violations, nil
}

func TestBackendTestsStayInCentralTestPackage(t *testing.T) {
	violations, err := scatteredProductionTests("..")
	if err != nil {
		t.Fatal(err)
	}
	if len(violations) > 0 {
		t.Fatalf("backend tests must live in platform/tests; move these files:\n%s", strings.Join(violations, "\n"))
	}
}

func TestBackendTestPlacementGateReportsEveryViolation(t *testing.T) {
	root := t.TempDir()
	paths := []string{"framework/action/schema_test.go", "modules/ai/runtime_test.go"}
	for _, path := range paths {
		absolute := filepath.Join(root, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(absolute), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(absolute, []byte("package misplaced"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	got, err := scatteredProductionTests(root)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(got, paths) {
		t.Fatalf("violations = %#v, want %#v", got, paths)
	}
}
