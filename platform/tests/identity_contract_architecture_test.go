package tests

import (
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
)

func TestIdentityContractsUseCanonicalFrameworkFields(t *testing.T) {
	_, current, _, _ := runtime.Caller(0)
	platformRoot := filepath.Clean(filepath.Join(filepath.Dir(current), ".."))

	authContext := mustReadContractFile(t, filepath.Join(platformRoot, "framework", "auth", "context.go"))
	if !strings.Contains(authContext, "PrincipalID") {
		t.Fatal("auth.Context must expose PrincipalID")
	}
	authorization := mustReadContractFile(t, filepath.Join(platformRoot, "framework", "auth", "authorization.go"))
	if strings.Count(authorization, "PrincipalID") < 2 {
		t.Fatal("authorization request and snapshot must expose PrincipalID")
	}
	auditModel := mustReadContractFile(t, filepath.Join(platformRoot, "framework", "audit", "audit.go"))
	if !strings.Contains(auditModel, "ActorID") || !strings.Contains(auditModel, `json:"actor_id"`) || strings.Contains(auditModel, `json:"user_id"`) {
		t.Fatal("audit.Record must expose only the canonical actor_id identity field")
	}
}

func TestFrameworkAndSystemAccessUserIdentityFieldsAreClassified(t *testing.T) {
	_, current, _, _ := runtime.Caller(0)
	platformRoot := filepath.Clean(filepath.Join(filepath.Dir(current), ".."))
	type exception struct {
		allowed    map[string]int
		reason     string
		deleteWhen string
	}
	exceptions := map[string]exception{
		"framework/auth/jwt.go": {
			allowed:    map[string]int{"LegacyUserID": 5},
			reason:     "兼容读取和同源双写旧租户会话 JWT uid",
			deleteWhen: "v0.3.0 前连续两个发布周期无 jwt_uid compatibility 日志",
		},
		"framework/attachment/token.go": {
			allowed:    map[string]int{"LegacyUserID": 5},
			reason:     "兼容仍在有效期内的短期附件传输 uid Token",
			deleteWhen: "v0.3.0 起始且最长传输 Token TTL 已过",
		},
		"modules/system/access/account_service.go": {
			allowed:    map[string]int{"user_id": 1},
			reason:     "AI memory 是独立业务用户数据域，不是后台账号公开契约",
			deleteWhen: "AI memory 域另行完成 Account owner 契约迁移",
		},
	}
	for path, item := range exceptions {
		if item.reason == "" || item.deleteWhen == "" {
			t.Fatalf("identity exception %s must document reason and deletion condition", path)
		}
	}

	legacyField := regexp.MustCompile(`UserID|user_id`)
	for _, root := range []string{filepath.Join(platformRoot, "framework"), filepath.Join(platformRoot, "modules", "system", "access")} {
		err := filepath.Walk(root, func(path string, info os.FileInfo, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if info.IsDir() || !strings.HasSuffix(path, ".go") {
				return nil
			}
			relative, _ := filepath.Rel(platformRoot, path)
			relative = filepath.ToSlash(relative)
			body := mustReadContractFile(t, path)
			if allowed, ok := exceptions[relative]; ok {
				for value, count := range allowed.allowed {
					if actual := strings.Count(body, value); actual != count {
						t.Errorf("identity exception %s occurrence count for %q = %d, want %d", relative, value, actual, count)
					}
					body = strings.ReplaceAll(body, value, "")
				}
			}
			if match := legacyField.FindString(body); match != "" {
				t.Errorf("unclassified identity field %q in %s", match, relative)
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
}

func TestLegacyAccountIdentifiersStayInsideCompatibilityAdapters(t *testing.T) {
	_, current, _, _ := runtime.Caller(0)
	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(current), "..", ".."))
	legacy := []string{
		"system.user_list", "system.user_get", "system.user_create", "system.user_update",
		"system.user_status_update", "system.user_roles_update", "system.user_password_reset",
		"system.users", "system.user_profile", "/api/v1/system/users/", "/system/users",
	}
	allowedPlatform := map[string]bool{
		"modules/system/access/support.go":                      true,
		"modules/system/access/account_register.go":             true,
		"modules/system/attachments/user_profile_attachment.go": true,
		"modules/system/register.go":                            true,
	}

	platformRoot := filepath.Join(repoRoot, "platform")
	err := filepath.Walk(filepath.Join(platformRoot, "modules"), func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if info.IsDir() || !strings.HasSuffix(path, ".go") {
			return nil
		}
		relative, _ := filepath.Rel(platformRoot, path)
		if allowedPlatform[filepath.ToSlash(relative)] {
			return nil
		}
		assertNoLegacyIdentity(t, relative, mustReadContractFile(t, path), legacy)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	frontendRoot := filepath.Join(repoRoot, "frontend", "src")
	err = filepath.Walk(frontendRoot, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if info.IsDir() || (!strings.HasSuffix(path, ".ts") && !strings.HasSuffix(path, ".vue")) {
			return nil
		}
		relative, _ := filepath.Rel(repoRoot, path)
		assertNoLegacyIdentity(t, relative, mustReadContractFile(t, path), legacy)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func mustReadContractFile(t *testing.T, path string) string {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}

func assertNoLegacyIdentity(t *testing.T, path, body string, legacy []string) {
	t.Helper()
	for _, value := range legacy {
		if strings.Contains(body, value) {
			t.Errorf("legacy account identifier %q escaped compatibility adapters: %s", value, filepath.ToSlash(path))
		}
	}
}
