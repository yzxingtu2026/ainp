package tests

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"testing"
)

const moduleImportPrefix = "github.com/yzxingtu2026/ainp/platform/modules/"

var aiCapabilityImports = map[string]map[string]bool{
	"modelruntime": {},
	"tooling":      {},
	"agent":        {},
	"conversation": {"agent": true, "modelruntime": true, "tooling": true},
	"knowledge":    {"agent": true},
	"memory":       {"conversation": true},
	"experience":   {"agent": true},
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
	if len(discovered) > 0 {
		failures := make([]string, 0, len(discovered))
		for _, violation := range discovered {
			failures = append(failures, formatModuleArchitectureFailure(violation))
		}
		slices.Sort(failures)
		t.Fatalf("module package architecture violations (no migration baseline is allowed):\n%s", strings.Join(failures, "\n"))
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
	if _, ok := violations[(moduleArchitectureViolation{Rule: "module-child-parent-import", Source: "modules/ai/knowledge/parent.go", Target: moduleImportPrefix + "ai"}).key()]; !ok {
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

func TestModuleStrictArchitectureFailures(t *testing.T) {
	modulesRoot := filepath.Join(t.TempDir(), "modules")
	files := map[string]string{
		"valid/register.go":       "package valid\nfunc Register() error { return nil }\n",
		"valid/service.go":        "package valid\n",
		"missing/service/main.go": "package service\n",
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
	if err := os.MkdirAll(filepath.Join(modulesRoot, "valid", "empty"), 0o755); err != nil {
		t.Fatal(err)
	}

	violations, err := scanModuleArchitecture(modulesRoot)
	if err != nil {
		t.Fatal(err)
	}
	want := []moduleArchitectureViolation{
		{Rule: "module-root-implementation", Source: "modules/valid/service.go"},
		{Rule: "module-register-entrypoint", Source: "modules/missing/register.go"},
		{Rule: "empty-package-directory", Source: "modules/valid/empty"},
	}
	for _, violation := range want {
		if _, ok := violations[violation.key()]; !ok {
			t.Errorf("expected strict architecture violation %+v, got %v", violation, violations)
		}
		if message := formatModuleArchitectureFailure(violation); !strings.Contains(message, violation.Rule) {
			t.Errorf("failure message must name rule %s, got %q", violation.Rule, message)
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
			entries, err := os.ReadDir(path)
			if err != nil {
				return err
			}
			if len(parts) >= 3 && len(entries) == 0 {
				addModuleArchitectureViolation(violations, moduleArchitectureViolation{Rule: "empty-package-directory", Source: rel})
			}
			if len(parts) >= 3 && isFallbackPackage(info.Name()) {
				addModuleArchitectureViolation(violations, moduleArchitectureViolation{
					Rule: "fallback-package", Source: rel,
				})
			}
			if len(parts) == 2 {
				if err := validateModuleRegister(path, rel, violations); err != nil {
					return err
				}
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
			if len(parts) >= 4 && importPath == moduleImportPrefix+sourceModule {
				addModuleArchitectureViolation(violations, moduleArchitectureViolation{
					Rule: "module-child-parent-import", Source: rel, Target: importPath,
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

func validateModuleRegister(modulePath, rel string, violations map[string]moduleArchitectureViolation) error {
	registerPath := filepath.Join(modulePath, "register.go")
	file, err := parser.ParseFile(token.NewFileSet(), registerPath, nil, 0)
	if err != nil {
		if os.IsNotExist(err) {
			addModuleArchitectureViolation(violations, moduleArchitectureViolation{Rule: "module-register-entrypoint", Source: rel + "/register.go"})
			return nil
		}
		return err
	}
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if ok && fn.Recv == nil && fn.Name.Name == "Register" && fn.Type.Results != nil && len(fn.Type.Results.List) == 1 {
			result, ok := fn.Type.Results.List[0].Type.(*ast.Ident)
			if ok && result.Name == "error" {
				return nil
			}
		}
	}
	addModuleArchitectureViolation(violations, moduleArchitectureViolation{Rule: "module-register-entrypoint", Source: rel + "/register.go"})
	return nil
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

func formatModuleArchitectureFailure(violation moduleArchitectureViolation) string {
	switch violation.Rule {
	case "module-root-implementation":
		return fmt.Sprintf("%s: rule %s; move implementation into a capability or role package, keeping only register.go or a named facade/contracts/constants file at module root", violation.Source, violation.Rule)
	case "cross-module-import":
		return fmt.Sprintf("%s: rule %s imports %s; depend on a framework contract injected by the composition root", violation.Source, violation.Rule, violation.Target)
	case "fallback-package":
		return fmt.Sprintf("%s: rule %s; choose a capability or technical-role package name instead of a catch-all package", violation.Source, violation.Rule)
	case "module-child-parent-import":
		return fmt.Sprintf("%s: rule %s imports %s; depend on capability contracts or framework packages to avoid a parent-package cycle", violation.Source, violation.Rule, violation.Target)
	case "empty-package-directory":
		return fmt.Sprintf("%s: rule %s; remove the empty directory or add its owned implementation", violation.Source, violation.Rule)
	case "module-register-entrypoint":
		return fmt.Sprintf("%s: rule %s; every module must expose a root Register entrypoint returning an error", violation.Source, violation.Rule)
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
