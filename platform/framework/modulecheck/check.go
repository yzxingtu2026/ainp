// Package modulecheck enforces AINP's module package architecture for the
// platform repository and downstream Go modules.
package modulecheck

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"golang.org/x/mod/modfile"
)

type Profile string

const (
	ProfileAINP       Profile = "ainp"
	ProfileDownstream Profile = "downstream"
)

type Config struct {
	Profile     Profile
	ModulesRoot string
	TestsRoot   string
	ModulePath  string
}

type Violation struct {
	RuleID  string
	Path    string
	Target  string
	Message string
}

func (v Violation) String() string {
	if v.Target != "" {
		return fmt.Sprintf("%s: rule %s; target %s; %s", v.Path, v.RuleID, v.Target, v.Message)
	}
	return fmt.Sprintf("%s: rule %s; %s", v.Path, v.RuleID, v.Message)
}

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

type scanner struct {
	profile      Profile
	moduleRoot   string
	modulesRoot  string
	testsPath    string
	importPrefix string
	violations   map[string]Violation
}

func Check(config Config) ([]Violation, error) {
	if config.Profile != ProfileAINP && config.Profile != ProfileDownstream {
		return nil, fmt.Errorf("modulecheck: unsupported profile %q", config.Profile)
	}
	modulesRoot, err := existingDirectory("modules root", config.ModulesRoot)
	if err != nil {
		return nil, err
	}
	testsRoot, err := existingDirectory("tests root", config.TestsRoot)
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(modulesRoot)
	if err != nil {
		return nil, fmt.Errorf("modulecheck: read modules root: %w", err)
	}
	moduleDirectories := 0
	for _, entry := range entries {
		if entry.IsDir() {
			moduleDirectories++
		}
	}
	if moduleDirectories == 0 {
		return nil, fmt.Errorf("modulecheck: modules root %s is empty", modulesRoot)
	}

	moduleRoot, discoveredPath, err := findModule(modulesRoot)
	if err != nil {
		return nil, err
	}
	modulePath := strings.TrimSpace(config.ModulePath)
	if modulePath == "" {
		modulePath = discoveredPath
	}
	if modulePath == "" {
		return nil, fmt.Errorf("modulecheck: Go module path is empty")
	}
	relModules, err := filepath.Rel(moduleRoot, modulesRoot)
	if err != nil || relModules == "." || relModules == ".." || strings.HasPrefix(relModules, ".."+string(filepath.Separator)) {
		return nil, fmt.Errorf("modulecheck: modules root must be inside the Go module containing %s", filepath.Join(moduleRoot, "go.mod"))
	}
	if testsRoot == modulesRoot || strings.HasPrefix(testsRoot, modulesRoot+string(filepath.Separator)) {
		return nil, fmt.Errorf("modulecheck: tests root must be outside modules root")
	}
	relTests, err := filepath.Rel(moduleRoot, testsRoot)
	if err != nil || relTests == ".." || strings.HasPrefix(relTests, ".."+string(filepath.Separator)) {
		return nil, fmt.Errorf("modulecheck: tests root must be inside the target Go module")
	}

	s := &scanner{
		profile:      config.Profile,
		moduleRoot:   moduleRoot,
		modulesRoot:  modulesRoot,
		testsPath:    filepath.ToSlash(relTests),
		importPrefix: strings.TrimSuffix(modulePath, "/") + "/" + filepath.ToSlash(relModules) + "/",
		violations:   map[string]Violation{},
	}
	if err := s.scan(); err != nil {
		return nil, err
	}
	violations := make([]Violation, 0, len(s.violations))
	for _, violation := range s.violations {
		violations = append(violations, violation)
	}
	slices.SortFunc(violations, func(a, b Violation) int {
		return strings.Compare(a.String(), b.String())
	})
	return violations, nil
}

func existingDirectory(label, path string) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", fmt.Errorf("modulecheck: %s is required", label)
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("modulecheck: resolve %s: %w", label, err)
	}
	info, err := os.Stat(abs)
	if err != nil {
		return "", fmt.Errorf("modulecheck: %s: %w", label, err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("modulecheck: %s %s is not a directory", label, abs)
	}
	return filepath.Clean(abs), nil
}

func findModule(start string) (string, string, error) {
	current := start
	for {
		goMod := filepath.Join(current, "go.mod")
		data, err := os.ReadFile(goMod)
		if err == nil {
			path := modfile.ModulePath(data)
			if path == "" {
				return "", "", fmt.Errorf("modulecheck: %s has no valid module directive", goMod)
			}
			return current, path, nil
		}
		if !os.IsNotExist(err) {
			return "", "", fmt.Errorf("modulecheck: read %s: %w", goMod, err)
		}
		parent := filepath.Dir(current)
		if parent == current {
			return "", "", fmt.Errorf("modulecheck: no go.mod found above %s", start)
		}
		current = parent
	}
}

func (s *scanner) scan() error {
	return filepath.WalkDir(s.modulesRoot, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relModules, err := filepath.Rel(s.modulesRoot, path)
		if err != nil {
			return err
		}
		parts := strings.Split(filepath.ToSlash(relModules), "/")
		displayPath, err := filepath.Rel(s.moduleRoot, path)
		if err != nil {
			return err
		}
		displayPath = filepath.ToSlash(displayPath)

		if entry.IsDir() {
			if relModules == "." {
				return nil
			}
			entries, err := os.ReadDir(path)
			if err != nil {
				return err
			}
			if len(parts) >= 1 && hasNoPackageSource(entries) {
				s.add("empty-package-directory", displayPath, "", "remove the empty directory or add its owned implementation")
			}
			if len(parts) >= 1 && isFallbackPackage(entry.Name()) {
				s.add("fallback-package", displayPath, "", "choose a capability or technical-role package name instead of a catch-all package")
			}
			if len(parts) == 1 {
				if err := s.validateRegister(path, displayPath); err != nil {
					return err
				}
			}
			return nil
		}
		if filepath.Ext(path) != ".go" || len(parts) < 2 {
			return nil
		}
		if strings.HasSuffix(path, "_test.go") {
			s.add("module-test-placement", displayPath, s.testsPath, "move the test to the configured centralized tests root and verify behavior through public APIs")
			return nil
		}
		if s.profile == ProfileAINP && len(parts) >= 3 && parts[0] == "ai" {
			if _, planned := aiCapabilityImports[parts[1]]; !planned && !aiLegacyPackages[parts[1]] {
				s.add("ai-unplanned-package", displayPath, "", "place the implementation in a frozen AI capability or update the architecture contract before adding a new domain")
			}
		}
		if len(parts) == 2 && !isAllowedModuleRootFile(entry.Name()) {
			s.add("module-root-implementation", displayPath, "", "move implementation into a capability or role package, keeping only register.go or a named facade/contracts/constants file at module root")
		}

		file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if err != nil {
			return fmt.Errorf("modulecheck: parse %s: %w", displayPath, err)
		}
		sourceModule := parts[0]
		for _, spec := range file.Imports {
			importPath, err := strconv.Unquote(spec.Path.Value)
			if err != nil {
				return fmt.Errorf("modulecheck: parse import in %s: %w", displayPath, err)
			}
			if s.profile == ProfileDownstream {
				if strings.HasPrefix(importPath, "github.com/yzxingtu2026/ainp/platform/internal/") || importPath == "github.com/yzxingtu2026/ainp/platform/internal" {
					s.add("downstream-platform-internal-import", displayPath, importPath, "depend on a documented framework contract or platform module root facade")
					continue
				}
				const platformModules = "github.com/yzxingtu2026/ainp/platform/modules/"
				if strings.HasPrefix(importPath, platformModules) {
					remainder := strings.TrimPrefix(importPath, platformModules)
					if strings.Contains(remainder, "/") {
						s.add("downstream-platform-implementation-import", displayPath, importPath, "depend on framework/platformresource or a documented platform module root facade")
						continue
					}
				}
			}
			if !strings.HasPrefix(importPath, s.importPrefix) {
				continue
			}
			if len(parts) >= 3 && importPath == s.importPrefix+sourceModule {
				s.add("module-child-parent-import", displayPath, importPath, "depend on capability contracts or framework packages to avoid a parent-package cycle")
				continue
			}
			if s.profile == ProfileAINP {
				if sourceCapability, ok := aiCapabilityForPath(parts); ok {
					if importPath == s.importPrefix+"ai" {
						s.add("ai-capability-parent-import", displayPath, importPath, "depend on capability contracts instead of the AI parent package")
						continue
					}
					if targetCapability, targetOK := aiCapabilityForImport(importPath, s.importPrefix); targetOK && targetCapability != sourceCapability && targetCapability != "internal" && !aiCapabilityImports[sourceCapability][targetCapability] {
						s.add("ai-capability-dependency", displayPath, importPath, "follow the frozen AI capability dependency graph or inject a framework contract")
						continue
					}
				}
			}
			targetParts := strings.Split(strings.TrimPrefix(importPath, s.importPrefix), "/")
			if len(targetParts) == 0 || targetParts[0] == "" || targetParts[0] == sourceModule {
				continue
			}
			s.add("cross-module-import", displayPath, importPath, "depend on a framework contract injected by the composition root")
		}
		return nil
	})
}

func (s *scanner) validateRegister(modulePath, displayModule string) error {
	registerPath := filepath.Join(modulePath, "register.go")
	file, err := parser.ParseFile(token.NewFileSet(), registerPath, nil, 0)
	if err != nil {
		if os.IsNotExist(err) {
			s.add("module-register-entrypoint", displayModule+"/register.go", "", registerMessage(s.profile))
			return nil
		}
		return fmt.Errorf("modulecheck: parse %s/register.go: %w", displayModule, err)
	}
	valid := false
	if s.profile == ProfileAINP {
		valid = hasAINPRegister(file)
	} else {
		valid = hasDownstreamModule(file)
	}
	if !valid {
		s.add("module-register-entrypoint", displayModule+"/register.go", "", registerMessage(s.profile))
	}
	return nil
}

func hasAINPRegister(file *ast.File) bool {
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if ok && fn.Recv == nil && fn.Name.Name == "Register" && returnsSingleError(fn.Type.Results) {
			return true
		}
	}
	return false
}

func hasDownstreamModule(file *ast.File) bool {
	appAliases := map[string]bool{}
	for _, spec := range file.Imports {
		path, err := strconv.Unquote(spec.Path.Value)
		if err != nil || !strings.HasSuffix(path, "/framework/app") {
			continue
		}
		name := "app"
		if spec.Name != nil {
			name = spec.Name.Name
		}
		if name != "." && name != "_" {
			appAliases[name] = true
		}
	}
	foundType, foundID, foundRegister := false, false, false
	for _, decl := range file.Decls {
		switch value := decl.(type) {
		case *ast.GenDecl:
			for _, spec := range value.Specs {
				if typeSpec, ok := spec.(*ast.TypeSpec); ok && typeSpec.Name.Name == "Module" {
					foundType = true
				}
			}
		case *ast.FuncDecl:
			if receiverName(value.Recv) != "Module" {
				continue
			}
			if value.Name.Name == "ID" && fieldCount(value.Type.Params) == 0 && returnsSingleIdent(value.Type.Results, "string") {
				foundID = true
			}
			if value.Name.Name == "Register" && fieldCount(value.Type.Params) == 1 && returnsSingleError(value.Type.Results) && isAppPointer(value.Type.Params.List[0].Type, appAliases) {
				foundRegister = true
			}
		}
	}
	return foundType && foundID && foundRegister
}

func receiverName(fields *ast.FieldList) string {
	if fields == nil || len(fields.List) != 1 {
		return ""
	}
	expr := fields.List[0].Type
	if pointer, ok := expr.(*ast.StarExpr); ok {
		expr = pointer.X
	}
	if ident, ok := expr.(*ast.Ident); ok {
		return ident.Name
	}
	return ""
}

func isAppPointer(expr ast.Expr, aliases map[string]bool) bool {
	pointer, ok := expr.(*ast.StarExpr)
	if !ok {
		return false
	}
	selector, ok := pointer.X.(*ast.SelectorExpr)
	if !ok || selector.Sel.Name != "App" {
		return false
	}
	ident, ok := selector.X.(*ast.Ident)
	return ok && aliases[ident.Name]
}

func fieldCount(fields *ast.FieldList) int {
	if fields == nil {
		return 0
	}
	count := 0
	for _, field := range fields.List {
		if len(field.Names) == 0 {
			count++
		} else {
			count += len(field.Names)
		}
	}
	return count
}

func returnsSingleError(results *ast.FieldList) bool { return returnsSingleIdent(results, "error") }

func returnsSingleIdent(results *ast.FieldList, name string) bool {
	if results == nil || len(results.List) != 1 || fieldCount(results) != 1 {
		return false
	}
	ident, ok := results.List[0].Type.(*ast.Ident)
	return ok && ident.Name == name
}

func registerMessage(profile Profile) string {
	if profile == ProfileDownstream {
		return "define type Module with ID() string and Register(*app.App) error methods in the module root register.go"
	}
	return "expose a root Register entrypoint returning an error"
}

func (s *scanner) add(ruleID, path, target, message string) {
	violation := Violation{RuleID: ruleID, Path: path, Target: target, Message: message}
	key := strings.Join([]string{ruleID, path, target}, "\x00")
	s.violations[key] = violation
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

func hasNoPackageSource(entries []os.DirEntry) bool {
	for _, entry := range entries {
		if entry.IsDir() {
			return false
		}
		if filepath.Ext(entry.Name()) == ".go" && !strings.HasSuffix(entry.Name(), "_test.go") {
			return false
		}
	}
	return true
}

func aiCapabilityForPath(parts []string) (string, bool) {
	if len(parts) < 3 || parts[0] != "ai" {
		return "", false
	}
	_, ok := aiCapabilityImports[parts[1]]
	return parts[1], ok
}

func aiCapabilityForImport(importPath, importPrefix string) (string, bool) {
	prefix := importPrefix + "ai/"
	if !strings.HasPrefix(importPath, prefix) {
		return "", false
	}
	capability := strings.Split(strings.TrimPrefix(importPath, prefix), "/")[0]
	_, ok := aiCapabilityImports[capability]
	return capability, ok
}
