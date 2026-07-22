package tests

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
)

func TestProductionActionMetadataDeclaresAIClassification(t *testing.T) {
	_, current, _, _ := runtime.Caller(0)
	modulesRoot := filepath.Clean(filepath.Join(filepath.Dir(current), "..", "modules"))
	fset := token.NewFileSet()
	violationsByModule := map[string][]string{}

	err := filepath.Walk(modulesRoot, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if info.IsDir() {
			if path == filepath.Join(modulesRoot, "demo") {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			return err
		}
		ast.Inspect(file, func(node ast.Node) bool {
			literal, ok := node.(*ast.CompositeLit)
			if !ok || !isActionMetadataType(literal.Type) {
				return true
			}
			for _, element := range literal.Elts {
				field, ok := element.(*ast.KeyValueExpr)
				if ok && fieldName(field.Key) == "AI" {
					return true
				}
			}
			relative, _ := filepath.Rel(modulesRoot, path)
			module := strings.Split(filepath.ToSlash(relative), "/")[0]
			position := fset.Position(literal.Pos())
			violationsByModule[module] = append(violationsByModule[module], fmt.Sprintf("%s:%d", filepath.ToSlash(relative), position.Line))
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(violationsByModule) == 0 {
		return
	}
	modules := make([]string, 0, len(violationsByModule))
	for module := range violationsByModule {
		modules = append(modules, module)
	}
	sort.Strings(modules)
	var lines []string
	for _, module := range modules {
		items := violationsByModule[module]
		sort.Strings(items)
		lines = append(lines, fmt.Sprintf("%s: %d unclassified metadata declarations", module, len(items)))
		for _, item := range items {
			lines = append(lines, "  "+item)
		}
	}
	t.Fatalf("production Action AI classification violations:\n%s", strings.Join(lines, "\n"))
}

func isActionMetadataType(expr ast.Expr) bool {
	selector, ok := expr.(*ast.SelectorExpr)
	if !ok || selector.Sel.Name != "Metadata" {
		return false
	}
	ident, ok := selector.X.(*ast.Ident)
	return ok && ident.Name == "action"
}

func fieldName(expr ast.Expr) string {
	ident, _ := expr.(*ast.Ident)
	if ident == nil {
		return ""
	}
	return ident.Name
}
