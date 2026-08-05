package tests

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"testing"

	accessmodel "github.com/yzxingtu2026/ainp/platform/modules/system/access/model"
)

var legacyAccountSQL = regexp.MustCompile(`(?i)\b(?:from|join|update|into|table|references|delete\s+from)\s+(?:users|user_roles)\b`)

func TestTenantAccountCanonicalArchitecture(t *testing.T) {
	forbiddenDeclarations := map[string]struct{}{
		"User": {}, "UserRole": {}, "UserService": {}, "UserProfile": {}, "UserView": {}, "UserRoleBrief": {}, "SelfProfileView": {},
	}
	fset := token.NewFileSet()
	err := filepath.Walk("../modules/system/access", func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if info.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		file, parseErr := parser.ParseFile(fset, path, nil, 0)
		if parseErr != nil {
			return parseErr
		}
		for _, declaration := range file.Decls {
			gen, ok := declaration.(*ast.GenDecl)
			if !ok || gen.Tok != token.TYPE {
				continue
			}
			for _, spec := range gen.Specs {
				name := spec.(*ast.TypeSpec).Name.Name
				if _, forbidden := forbiddenDeclarations[name]; forbidden {
					t.Errorf("%s declares legacy canonical access type %s", path, name)
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	for _, root := range []string{"../framework", "../internal", "../modules", "../cmd"} {
		err = filepath.Walk(root, func(path string, info os.FileInfo, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if info.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			body, readErr := os.ReadFile(path)
			if readErr != nil {
				return readErr
			}
			if match := legacyAccountSQL.Find(body); match != nil {
				t.Errorf("%s contains legacy account table SQL %q", path, match)
			}
			if strings.Contains(string(body), `.Table("users")`) || strings.Contains(string(body), `.Table("user_roles")`) {
				t.Errorf("%s dynamically accesses a legacy account table", path)
			}
			file, parseErr := parser.ParseFile(fset, path, body, 0)
			if parseErr != nil {
				return parseErr
			}
			legacyImports := map[string]map[string]struct{}{}
			for _, imported := range file.Imports {
				importPath, _ := strconv.Unquote(imported.Path.Value)
				var forbidden map[string]struct{}
				switch importPath {
				case "github.com/yzxingtu2026/ainp/platform/modules/system":
					forbidden = map[string]struct{}{"UserService": {}, "UserProfile": {}, "UserView": {}, "UserRoleBrief": {}, "SelfProfileView": {}, "NewUserService": {}}
				case "github.com/yzxingtu2026/ainp/platform/modules/system/model",
					"github.com/yzxingtu2026/ainp/platform/modules/system/access/model":
					forbidden = map[string]struct{}{"User": {}, "UserRole": {}}
				case "github.com/yzxingtu2026/ainp/platform/modules/system/access":
					forbidden = map[string]struct{}{"UserService": {}, "UserProfile": {}, "UserView": {}, "UserRoleBrief": {}, "SelfProfileView": {}}
				}
				if forbidden == nil {
					continue
				}
				name := filepath.Base(importPath)
				if imported.Name != nil {
					name = imported.Name.Name
				}
				legacyImports[name] = forbidden
			}
			ast.Inspect(file, func(node ast.Node) bool {
				selector, ok := node.(*ast.SelectorExpr)
				if !ok {
					return true
				}
				pkg, ok := selector.X.(*ast.Ident)
				if !ok {
					return true
				}
				if forbidden := legacyImports[pkg.Name]; forbidden != nil {
					if _, found := forbidden[selector.Sel.Name]; found {
						t.Errorf("%s depends on deprecated account alias %s.%s", path, pkg.Name, selector.Sel.Name)
					}
				}
				return true
			})
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}

	if got := (accessmodel.TenantAccount{}).TableName(); got != "tenant_accounts" {
		t.Fatalf("TenantAccount table = %q", got)
	}
	if got := (accessmodel.AccountRole{}).TableName(); got != "tenant_account_roles" {
		t.Fatalf("AccountRole table = %q", got)
	}
	for _, modelType := range []reflect.Type{reflect.TypeOf(accessmodel.AuthIdentity{}), reflect.TypeOf(accessmodel.AuthSession{}), reflect.TypeOf(accessmodel.AccountRole{})} {
		if _, ok := modelType.FieldByName("AccountID"); !ok {
			t.Errorf("%s must expose AccountID", modelType.Name())
		}
		if _, ok := modelType.FieldByName("UserID"); ok {
			t.Errorf("%s must not expose legacy UserID", modelType.Name())
		}
	}
}
