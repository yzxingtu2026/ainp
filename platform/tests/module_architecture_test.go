package tests

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/yzxingtu2026/ainp/platform/framework/modulecheck"
)

func TestModulePackageArchitecture(t *testing.T) {
	platformRoot := moduleArchitecturePlatformRoot()
	assertArchitecturePasses(t, modulecheck.Config{
		Profile:     modulecheck.ProfileAINP,
		ModulesRoot: filepath.Join(platformRoot, "modules"),
		TestsRoot:   filepath.Join(platformRoot, "tests"),
	})
}

func TestOfficialDownstreamModuleArchitecture(t *testing.T) {
	root := filepath.Join(moduleArchitecturePlatformRoot(), "examples", "downstream")
	assertArchitecturePasses(t, modulecheck.Config{
		Profile:     modulecheck.ProfileDownstream,
		ModulesRoot: filepath.Join(root, "modules"),
		TestsRoot:   filepath.Join(root, "tests"),
	})
}

func TestAICapabilityDependencyGraph(t *testing.T) {
	root := newArchitectureFixture(t)
	writeArchitectureFiles(t, root, map[string]string{
		"modules/ai/register.go":             "package ai\nfunc Register() error { return nil }\n",
		"modules/ai/conversation/runtime.go": "package conversation\nimport _ \"example.com/fixture/modules/ai/tooling\"\n",
		"modules/ai/tooling/reverse.go":      "package tooling\nimport _ \"example.com/fixture/modules/ai/conversation\"\n",
		"modules/ai/knowledge/parent.go":     "package knowledge\nimport _ \"example.com/fixture/modules/ai\"\n",
		"modules/ai/unplanned/service.go":    "package unplanned\n",
	})
	violations := checkFixture(t, root, modulecheck.ProfileAINP)
	assertViolation(t, violations, "ai-capability-dependency", "modules/ai/tooling/reverse.go", "example.com/fixture/modules/ai/conversation")
	assertViolation(t, violations, "module-child-parent-import", "modules/ai/knowledge/parent.go", "example.com/fixture/modules/ai")
	assertViolation(t, violations, "ai-unplanned-package", "modules/ai/unplanned/service.go", "")
	for _, violation := range violations {
		if violation.Path == "modules/ai/conversation/runtime.go" {
			t.Fatalf("approved AI capability dependency should pass, got %+v", violation)
		}
	}
}

func TestDownstreamStrictArchitectureFailures(t *testing.T) {
	root := newArchitectureFixture(t)
	validRegister := `package valid
import "github.com/yzxingtu2026/ainp/platform/framework/app"
type Module struct{}
func (Module) ID() string { return "valid" }
func (Module) Register(*app.App) error { return nil }
`
	writeArchitectureFiles(t, root, map[string]string{
		"modules/valid/register.go":             validRegister,
		"modules/valid/service.go":              "package valid\ntype Service struct{}\n",
		"modules/valid/model.go":                "package valid\ntype Model struct{}\n",
		"modules/valid/http.go":                 "package valid\nfunc MountHTTP() {}\n",
		"modules/valid/catalog/service.go":      "package catalog\n",
		"modules/valid/catalog/service_test.go": "package catalog_test\n",
		"modules/valid/catalog/reverse.go":      "package catalog\nimport _ \"example.com/fixture/modules/valid\"\n",
		"modules/valid/utils/value.go":          "package utils\n",
		"modules/other/register.go": strings.ReplaceAll(validRegister, "valid", "other") +
			"\n",
		"modules/other/catalog/service.go":   "package catalog\nimport _ \"example.com/fixture/modules/valid/catalog\"\n",
		"modules/missing/catalog/service.go": "package catalog\n",
	})
	if err := os.MkdirAll(filepath.Join(root, "modules", "valid", "empty"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "modules", "nosource"), 0o755); err != nil {
		t.Fatal(err)
	}

	violations := checkFixture(t, root, modulecheck.ProfileDownstream)
	for _, path := range []string{"modules/valid/service.go", "modules/valid/model.go", "modules/valid/http.go"} {
		assertViolation(t, violations, "module-root-implementation", path, "")
	}
	assertViolation(t, violations, "cross-module-import", "modules/other/catalog/service.go", "example.com/fixture/modules/valid/catalog")
	assertViolation(t, violations, "module-child-parent-import", "modules/valid/catalog/reverse.go", "example.com/fixture/modules/valid")
	assertViolation(t, violations, "fallback-package", "modules/valid/utils", "")
	assertViolation(t, violations, "module-test-placement", "modules/valid/catalog/service_test.go", "tests")
	assertViolation(t, violations, "module-register-entrypoint", "modules/missing/register.go", "")
	assertViolation(t, violations, "empty-package-directory", "modules/valid/empty", "")
	assertViolation(t, violations, "module-register-entrypoint", "modules/nosource/register.go", "")
	assertViolation(t, violations, "empty-package-directory", "modules/nosource", "")

	formatted := make([]string, 0, len(violations))
	for _, violation := range violations {
		formatted = append(formatted, violation.String())
		if !strings.Contains(violation.String(), "rule "+violation.RuleID) || !strings.Contains(violation.String(), violation.Path) {
			t.Fatalf("violation output is missing its stable rule ID or path: %q", violation.String())
		}
	}
	if !slices.IsSorted(formatted) {
		t.Fatalf("violations must have deterministic path/rule ordering: %#v", formatted)
	}
}

func TestDownstreamRejectsPlatformImplementationImports(t *testing.T) {
	root := newArchitectureFixture(t)
	validRegister := `package valid
import "github.com/yzxingtu2026/ainp/platform/framework/app"
type Module struct{}
func (Module) ID() string { return "valid" }
func (Module) Register(*app.App) error { return nil }
`
	writeArchitectureFiles(t, root, map[string]string{
		"modules/valid/register.go":          validRegister,
		"modules/valid/catalog/contracts.go": "package catalog\nimport _ \"github.com/yzxingtu2026/ainp/platform/framework/platformresource\"\nimport _ \"github.com/yzxingtu2026/ainp/platform/modules/system\"\n",
		"modules/valid/catalog/model.go":     "package catalog\nimport _ \"github.com/yzxingtu2026/ainp/platform/modules/system/model\"\n",
		"modules/valid/catalog/internal.go":  "package catalog\nimport _ \"github.com/yzxingtu2026/ainp/platform/internal/bootstrap\"\n",
	})
	violations := checkFixture(t, root, modulecheck.ProfileDownstream)
	assertViolation(t, violations, "downstream-platform-implementation-import", "modules/valid/catalog/model.go", "github.com/yzxingtu2026/ainp/platform/modules/system/model")
	assertViolation(t, violations, "downstream-platform-internal-import", "modules/valid/catalog/internal.go", "github.com/yzxingtu2026/ainp/platform/internal/bootstrap")
	for _, violation := range violations {
		if violation.Path == "modules/valid/catalog/contracts.go" {
			t.Fatalf("stable framework contract and module root facade must remain allowed: %+v", violation)
		}
	}
}

func TestDownstreamRegisterShapeFailsClosed(t *testing.T) {
	tests := map[string]string{
		"missing module type": `package invalid
import "github.com/yzxingtu2026/ainp/platform/framework/app"
func Register(*app.App) error { return nil }
`,
		"wrong register parameter": `package invalid
type Module struct{}
func (Module) ID() string { return "invalid" }
func (Module) Register(any) error { return nil }
`,
		"missing ID": `package invalid
import "github.com/yzxingtu2026/ainp/platform/framework/app"
type Module struct{}
func (Module) Register(*app.App) error { return nil }
`,
	}
	for name, register := range tests {
		t.Run(name, func(t *testing.T) {
			root := newArchitectureFixture(t)
			writeArchitectureFiles(t, root, map[string]string{
				"modules/invalid/register.go":        register,
				"modules/invalid/catalog/service.go": "package catalog\n",
			})
			assertViolation(t, checkFixture(t, root, modulecheck.ProfileDownstream), "module-register-entrypoint", "modules/invalid/register.go", "")
		})
	}
}

func TestModuleCheckConfigurationFailsClosed(t *testing.T) {
	root := t.TempDir()
	if _, err := modulecheck.Check(modulecheck.Config{Profile: "unknown", ModulesRoot: root, TestsRoot: root}); err == nil {
		t.Fatal("unknown profile must fail")
	}
	if _, err := modulecheck.Check(modulecheck.Config{Profile: modulecheck.ProfileDownstream, ModulesRoot: filepath.Join(root, "missing"), TestsRoot: root}); err == nil {
		t.Fatal("missing modules root must fail")
	}
	emptyModules := filepath.Join(root, "modules")
	if err := os.MkdirAll(emptyModules, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := modulecheck.Check(modulecheck.Config{Profile: modulecheck.ProfileDownstream, ModulesRoot: emptyModules, TestsRoot: root}); err == nil {
		t.Fatal("empty modules root must fail")
	}
}

func TestArchitectureCLIExitsNonzeroForViolations(t *testing.T) {
	root := newArchitectureFixture(t)
	writeArchitectureFiles(t, root, map[string]string{
		"modules/orders/service.go": "package orders\n",
	})
	command := exec.Command("go", "run", "./cmd/ainp-archcheck", "--profile", "downstream", "--modules-root", filepath.Join(root, "modules"), "--tests-root", filepath.Join(root, "tests"))
	command.Dir = moduleArchitecturePlatformRoot()
	command.Env = append(os.Environ(), "GOCACHE="+filepath.Join(t.TempDir(), "go-build"))
	output, err := command.CombinedOutput()
	if err == nil {
		t.Fatalf("CLI must exit nonzero for violations, output: %s", output)
	}
	if !strings.Contains(string(output), "rule module-root-implementation") || !strings.Contains(string(output), "modules/orders/service.go") {
		t.Fatalf("CLI output must identify the rule and file, got: %s", output)
	}
}

func assertArchitecturePasses(t *testing.T, config modulecheck.Config) {
	t.Helper()
	violations, err := modulecheck.Check(config)
	if err != nil {
		t.Fatal(err)
	}
	if len(violations) == 0 {
		return
	}
	failures := make([]string, 0, len(violations))
	for _, violation := range violations {
		failures = append(failures, violation.String())
	}
	t.Fatalf("module package architecture violations (no migration baseline is allowed):\n%s", strings.Join(failures, "\n"))
}

func newArchitectureFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	writeArchitectureFiles(t, root, map[string]string{"go.mod": "module example.com/fixture\n\ngo 1.25.6\n", "tests/.keep": ""})
	return root
}

func writeArchitectureFiles(t *testing.T, root string, files map[string]string) {
	t.Helper()
	for rel, content := range files {
		path := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func checkFixture(t *testing.T, root string, profile modulecheck.Profile) []modulecheck.Violation {
	t.Helper()
	violations, err := modulecheck.Check(modulecheck.Config{
		Profile:     profile,
		ModulesRoot: filepath.Join(root, "modules"),
		TestsRoot:   filepath.Join(root, "tests"),
	})
	if err != nil {
		t.Fatal(err)
	}
	return violations
}

func assertViolation(t *testing.T, violations []modulecheck.Violation, ruleID, path, target string) {
	t.Helper()
	for _, violation := range violations {
		if violation.RuleID == ruleID && violation.Path == path && violation.Target == target {
			return
		}
	}
	t.Fatalf("expected violation %s %s %s, got %#v", ruleID, path, target, violations)
}

func moduleArchitecturePlatformRoot() string {
	_, current, _, _ := runtime.Caller(0)
	return filepath.Clean(filepath.Join(filepath.Dir(current), ".."))
}
