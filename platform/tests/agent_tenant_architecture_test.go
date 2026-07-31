package tests

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestAgentManagementUsesTenantRepository(t *testing.T) {
	_, file, _, _ := runtime.Caller(0)
	path := filepath.Join(filepath.Dir(file), "..", "modules", "ai", "agent", "service.go")
	f, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	var violations []string
	ast.Inspect(f, func(n ast.Node) bool {
		lit, ok := n.(*ast.BasicLit)
		if !ok || lit.Kind != token.STRING {
			return true
		}
		value := strings.Trim(lit.Value, "`\"")
		if strings.Contains(value, "tenant_id") && value != "tenant_id=? AND id=? AND status=?" {
			violations = append(violations, value)
		}
		return true
	})
	if len(violations) > 0 {
		t.Fatalf("Agent management reintroduced top-level tenant predicates: %v", violations)
	}
}
