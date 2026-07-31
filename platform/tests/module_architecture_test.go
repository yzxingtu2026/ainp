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

var aiCapabilityImports = map[string]map[string]bool{
	"modelruntime": {},
	"tooling":      {},
	"agent":        {},
	"conversation": {"modelruntime": true, "tooling": true},
	"knowledge":    {"agent": true},
	"memory":       {"conversation": true},
	"experience":   {},
	"internal":     {},
}

var aiLegacyPackages = map[string]bool{"help": true, "tools": true}

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

func TestAICapabilityMigrationMapCoversRootFiles(t *testing.T) {
	platformRoot := moduleArchitecturePlatformRoot()
	mapped, err := readAICapabilityMigrationMap(filepath.Join(platformRoot, "tests", "testdata", "ai_capability_migration.tsv"))
	if err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(filepath.Join(platformRoot, "modules", "ai"))
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".go" || entry.Name() == "register.go" {
			continue
		}
		seen[entry.Name()] = true
		if _, ok := mapped[entry.Name()]; !ok {
			t.Errorf("modules/ai/%s: missing capability migration mapping", entry.Name())
		}
	}
	for source := range mapped {
		if !seen[source] {
			t.Errorf("modules/ai/%s: stale capability migration mapping", source)
		}
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

func TestAICapabilityDependencyGraph(t *testing.T) {
	root := t.TempDir()
	modulesRoot := filepath.Join(root, "modules")
	files := map[string]string{
		"ai/conversation/runtime.go": "package conversation\nimport _ \"github.com/yzxingtu2026/ainp/platform/modules/ai/tooling\"\n",
		"ai/tooling/reverse.go":      "package tooling\nimport _ \"github.com/yzxingtu2026/ainp/platform/modules/ai/conversation\"\n",
		"ai/knowledge/parent.go":     "package knowledge\nimport _ \"github.com/yzxingtu2026/ainp/platform/modules/ai\"\n",
		"ai/unplanned/service.go":    "package unplanned\n",
	}
	for rel, content := range files {
		path := filepath.Join(modulesRoot, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	violations, err := scanModuleArchitecture(modulesRoot)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := violations[(moduleArchitectureViolation{Rule: "ai-capability-dependency", Source: "modules/ai/tooling/reverse.go", Target: moduleImportPrefix + "ai/conversation"}).key()]; !ok {
		t.Fatal("reverse AI capability dependency should be rejected")
	}
	if _, ok := violations[(moduleArchitectureViolation{Rule: "ai-capability-parent-import", Source: "modules/ai/knowledge/parent.go", Target: moduleImportPrefix + "ai"}).key()]; !ok {
		t.Fatal("AI capability parent import should be rejected")
	}
	if _, ok := violations[(moduleArchitectureViolation{Rule: "ai-unplanned-package", Source: "modules/ai/unplanned/service.go"}).key()]; !ok {
		t.Fatal("unplanned AI capability package should be rejected")
	}
	for _, violation := range violations {
		if violation.Source == "modules/ai/conversation/runtime.go" {
			t.Fatalf("approved AI capability dependency should pass, got %+v", violation)
		}
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
		if len(parts) >= 4 && parts[1] == "ai" {
			if _, planned := aiCapabilityImports[parts[2]]; !planned && !aiLegacyPackages[parts[2]] {
				addModuleArchitectureViolation(violations, moduleArchitectureViolation{
					Rule: "ai-unplanned-package", Source: rel,
				})
			}
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
			if sourceCapability, ok := aiCapabilityForPath(rel); ok {
				if importPath == moduleImportPrefix+"ai" {
					addModuleArchitectureViolation(violations, moduleArchitectureViolation{
						Rule: "ai-capability-parent-import", Source: rel, Target: importPath,
					})
					continue
				}
				if targetCapability, targetOK := aiCapabilityForImport(importPath); targetOK && targetCapability != sourceCapability && targetCapability != "internal" && !aiCapabilityImports[sourceCapability][targetCapability] {
					addModuleArchitectureViolation(violations, moduleArchitectureViolation{
						Rule: "ai-capability-dependency", Source: rel, Target: importPath,
					})
					continue
				}
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

func aiCapabilityForPath(rel string) (string, bool) {
	parts := strings.Split(rel, "/")
	if len(parts) < 4 || parts[0] != "modules" || parts[1] != "ai" {
		return "", false
	}
	_, ok := aiCapabilityImports[parts[2]]
	return parts[2], ok
}

func aiCapabilityForImport(importPath string) (string, bool) {
	prefix := moduleImportPrefix + "ai/"
	if !strings.HasPrefix(importPath, prefix) {
		return "", false
	}
	capability := strings.Split(strings.TrimPrefix(importPath, prefix), "/")[0]
	_, ok := aiCapabilityImports[capability]
	return capability, ok
}

func readAICapabilityMigrationMap(path string) (map[string]string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	mapped := map[string]string{}
	scanner := bufio.NewScanner(file)
	for lineNumber := 1; scanner.Scan(); lineNumber++ {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Split(line, "\t")
		if len(fields) != 4 {
			return nil, fmt.Errorf("%s:%d: expected source, target, compatibility, and note", path, lineNumber)
		}
		source, targets, compatibility, note := strings.TrimSpace(fields[0]), strings.TrimSpace(fields[1]), strings.TrimSpace(fields[2]), strings.TrimSpace(fields[3])
		if filepath.Ext(source) != ".go" || targets == "" || compatibility == "" || note == "" {
			return nil, fmt.Errorf("%s:%d: migration mapping fields must be complete", path, lineNumber)
		}
		if _, exists := mapped[source]; exists {
			return nil, fmt.Errorf("%s:%d: duplicate migration mapping for %s", path, lineNumber, source)
		}
		for _, target := range strings.Split(targets, ",") {
			root := strings.Split(strings.TrimSpace(target), "/")[0]
			if _, ok := aiCapabilityImports[root]; !ok {
				return nil, fmt.Errorf("%s:%d: unknown AI capability %s", path, lineNumber, root)
			}
		}
		mapped[source] = targets
	}
	return mapped, scanner.Err()
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
	case "ai-capability-parent-import":
		return fmt.Sprintf("%s: rule %s imports %s; use framework or capability-owned contracts instead of the AI root Facade", violation.Source, violation.Rule, violation.Target)
	case "ai-capability-dependency":
		return fmt.Sprintf("%s: rule %s imports %s; follow the frozen AI capability dependency graph or inject a framework contract", violation.Source, violation.Rule, violation.Target)
	case "ai-unplanned-package":
		return fmt.Sprintf("%s: rule %s; place the implementation in a frozen AI capability or update the architecture contract before adding a new domain", violation.Source, violation.Rule)
	default:
		return fmt.Sprintf("%s: rule %s", violation.Source, violation.Rule)
	}
}

func moduleArchitecturePlatformRoot() string {
	_, current, _, _ := runtime.Caller(0)
	return filepath.Clean(filepath.Join(filepath.Dir(current), ".."))
}
