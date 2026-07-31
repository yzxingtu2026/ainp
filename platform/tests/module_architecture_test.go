package tests

import (
	"bufio"
	"fmt"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"testing"
)

const moduleImportPrefix = "github.com/yzxingtu2026/ainp/platform/modules/"

type moduleArchitectureViolation struct {
	Rule   string
	Source string
	Target string
}

func (v moduleArchitectureViolation) key() string {
	return strings.Join([]string{v.Rule, v.Source, v.Target}, "\t")
}

func TestModulePackageArchitecture(t *testing.T) {
	platformRoot := moduleArchitecturePlatformRoot()
	discovered, err := scanModuleArchitecture(filepath.Join(platformRoot, "modules"))
	if err != nil {
		t.Fatal(err)
	}
	baseline, err := readModuleArchitectureBaseline(filepath.Join(platformRoot, "tests", "testdata", "module_architecture_baseline.tsv"))
	if err != nil {
		t.Fatal(err)
	}

	failures := reconcileModuleArchitecture(discovered, baseline)
	if len(failures) > 0 {
		t.Fatalf("module package architecture violations:\n%s", strings.Join(failures, "\n"))
	}
}

func reconcileModuleArchitecture(discovered, baseline map[string]moduleArchitectureViolation) []string {
	var failures []string
	for key, violation := range discovered {
		if _, ok := baseline[key]; ok {
			continue
		}
		failures = append(failures, formatModuleArchitectureFailure(violation, false))
	}
	for key, violation := range baseline {
		if _, ok := discovered[key]; ok {
			continue
		}
		failures = append(failures, formatModuleArchitectureFailure(violation, true))
	}
	sort.Strings(failures)
	return failures
}

func TestModuleArchitectureBaselineIsBidirectional(t *testing.T) {
	violation := moduleArchitectureViolation{Rule: "module-root-implementation", Source: "modules/demo/service.go"}
	discovered := map[string]moduleArchitectureViolation{violation.key(): violation}
	baseline := map[string]moduleArchitectureViolation{}
	failures := reconcileModuleArchitecture(discovered, baseline)
	if len(failures) != 1 || !strings.Contains(failures[0], "move implementation") {
		t.Fatalf("new violation should fail with migration advice, got %v", failures)
	}

	baseline[violation.key()] = violation
	delete(discovered, violation.key())
	failures = reconcileModuleArchitecture(discovered, baseline)
	if len(failures) != 1 || !strings.Contains(failures[0], "stale baseline") {
		t.Fatalf("removed violation should require baseline cleanup, got %v", failures)
	}
}

func scanModuleArchitecture(modulesRoot string) (map[string]moduleArchitectureViolation, error) {
	violations := map[string]moduleArchitectureViolation{}
	fset := token.NewFileSet()
	err := filepath.Walk(modulesRoot, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(filepath.Dir(modulesRoot), path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		parts := strings.Split(rel, "/")
		if info.IsDir() {
			if len(parts) >= 3 && isFallbackPackage(info.Name()) {
				addModuleArchitectureViolation(violations, moduleArchitectureViolation{
					Rule: "fallback-package", Source: rel,
				})
			}
			return nil
		}
		if filepath.Ext(path) != ".go" || strings.HasSuffix(path, "_test.go") || len(parts) < 3 {
			return nil
		}

		if len(parts) == 3 && !isAllowedModuleRootFile(info.Name()) {
			addModuleArchitectureViolation(violations, moduleArchitectureViolation{
				Rule: "module-root-implementation", Source: rel,
			})
		}

		file, err := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		if err != nil {
			return err
		}
		sourceModule := parts[1]
		for _, spec := range file.Imports {
			importPath, err := strconv.Unquote(spec.Path.Value)
			if err != nil {
				return fmt.Errorf("parse import in %s: %w", rel, err)
			}
			if !strings.HasPrefix(importPath, moduleImportPrefix) {
				continue
			}
			if isSystemCapability(rel) && importPath == moduleImportPrefix+"system" {
				addModuleArchitectureViolation(violations, moduleArchitectureViolation{
					Rule: "system-capability-parent-import", Source: rel, Target: importPath,
				})
				continue
			}
			targetParts := strings.Split(strings.TrimPrefix(importPath, moduleImportPrefix), "/")
			if len(targetParts) == 0 || targetParts[0] == "" || targetParts[0] == sourceModule {
				continue
			}
			addModuleArchitectureViolation(violations, moduleArchitectureViolation{
				Rule: "cross-module-import", Source: rel, Target: importPath,
			})
		}
		return nil
	})
	return violations, err
}

func isSystemCapability(rel string) bool {
	parts := strings.Split(rel, "/")
	return len(parts) >= 4 && parts[0] == "modules" && parts[1] == "system"
}

func readModuleArchitectureBaseline(path string) (map[string]moduleArchitectureViolation, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	baseline := map[string]moduleArchitectureViolation{}
	scanner := bufio.NewScanner(file)
	for lineNumber := 1; scanner.Scan(); lineNumber++ {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Split(line, "\t")
		if len(fields) != 4 || strings.TrimSpace(fields[3]) == "" {
			return nil, fmt.Errorf("%s:%d: expected rule, source, target, and reason", path, lineNumber)
		}
		sources := []string{fields[1]}
		if fields[0] == "module-root-implementation" {
			sources = strings.Split(fields[1], ",")
		}
		for _, source := range sources {
			target := fields[2]
			if target == "-" {
				target = ""
			}
			violation := moduleArchitectureViolation{Rule: fields[0], Source: strings.TrimSpace(source), Target: target}
			if violation.Source == "" {
				return nil, fmt.Errorf("%s:%d: baseline source must not be empty", path, lineNumber)
			}
			if _, exists := baseline[violation.key()]; exists {
				return nil, fmt.Errorf("%s:%d: duplicate baseline entry %q", path, lineNumber, violation.key())
			}
			baseline[violation.key()] = violation
		}
	}
	return baseline, scanner.Err()
}

func addModuleArchitectureViolation(target map[string]moduleArchitectureViolation, violation moduleArchitectureViolation) {
	target[violation.key()] = violation
}

func isAllowedModuleRootFile(name string) bool {
	if name == "register.go" || name == "facade.go" || name == "contracts.go" || name == "constants.go" {
		return true
	}
	return strings.HasSuffix(name, "_facade.go") || strings.HasSuffix(name, "_contracts.go") || strings.HasSuffix(name, "_constants.go")
}

func isFallbackPackage(name string) bool {
	switch strings.ToLower(name) {
	case "common", "commons", "util", "utils", "misc", "shared", "helpers":
		return true
	default:
		return false
	}
}

func formatModuleArchitectureFailure(violation moduleArchitectureViolation, stale bool) string {
	if stale {
		return fmt.Sprintf("%s: stale baseline for rule %s; remove the entry from tests/testdata/module_architecture_baseline.tsv", violation.Source, violation.Rule)
	}
	switch violation.Rule {
	case "module-root-implementation":
		return fmt.Sprintf("%s: rule %s; move implementation into a capability or role package, keeping only register.go or a named facade/contracts/constants file at module root", violation.Source, violation.Rule)
	case "cross-module-import":
		return fmt.Sprintf("%s: rule %s imports %s; depend on a framework contract or add an explicitly reviewed baseline entry with a reason", violation.Source, violation.Rule, violation.Target)
	case "fallback-package":
		return fmt.Sprintf("%s: rule %s; choose a capability or technical-role package name instead of a catch-all package", violation.Source, violation.Rule)
	case "system-capability-parent-import":
		return fmt.Sprintf("%s: rule %s imports %s; depend on capability contracts or framework packages to avoid a parent-package cycle", violation.Source, violation.Rule, violation.Target)
	default:
		return fmt.Sprintf("%s: rule %s", violation.Source, violation.Rule)
	}
}

func moduleArchitecturePlatformRoot() string {
	_, current, _, _ := runtime.Caller(0)
	return filepath.Clean(filepath.Join(filepath.Dir(current), ".."))
}
