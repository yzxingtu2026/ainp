package tests

import (
	"strings"
	"testing"
	"testing/fstest"

	"github.com/yzxingtu2026/ainp/platform/framework/i18n"
	"github.com/yzxingtu2026/ainp/platform/framework/prompt"
)

func TestRuntimeAssetSourcesAreNamespacedDeterministicAndImmutable(t *testing.T) {
	promptSource := prompt.Source{Owner: "example.orders", FS: fstest.MapFS{
		"example.orders/summary.md": {Data: []byte("---\nversion: 1.0.0\n---\nSummarize {{.Payload}}")},
	}}
	if err := prompt.LoadSources(promptSource); err != nil {
		t.Fatal(err)
	}
	promptManifest := prompt.Manifest()
	if len(promptManifest) != 1 || promptManifest[0].Owner != "example.orders" || promptManifest[0].Key != "example.orders/summary" || !strings.HasPrefix(promptManifest[0].Digest, "sha256:") {
		t.Fatalf("unexpected Prompt manifest: %#v", promptManifest)
	}
	if err := prompt.Reload(); err == nil || !strings.Contains(err.Error(), "immutable") {
		t.Fatalf("production Prompt resources must not reload: %v", err)
	}

	i18nSource := i18n.Source{Owner: "example.orders", FS: fstest.MapFS{
		"en.yaml":    {Data: []byte("example.orders.title: Orders\n")},
		"zh-CN.yaml": {Data: []byte("example.orders.title: 订单\n")},
	}}
	if err := i18n.LoadSources(i18nSource); err != nil {
		t.Fatal(err)
	}
	i18nManifest := i18n.Manifest()
	if len(i18nManifest) != 2 || i18nManifest[0].Owner != "example.orders" || !strings.HasPrefix(i18nManifest[0].Digest, "sha256:") {
		t.Fatalf("unexpected i18n manifest: %#v", i18nManifest)
	}
	if err := i18n.Reload(); err == nil || !strings.Contains(err.Error(), "immutable") {
		t.Fatalf("production i18n resources must not reload: %v", err)
	}
}

func TestRuntimeAssetSourcesRejectOwnerAndKeyConflicts(t *testing.T) {
	validPrompt := fstest.MapFS{"example.orders/summary.md": {Data: []byte("Safe")}}
	for _, test := range []struct {
		name    string
		sources []prompt.Source
		want    string
	}{
		{name: "duplicate owner", sources: []prompt.Source{{Owner: "example.orders", FS: validPrompt}, {Owner: "example.orders", FS: validPrompt}}, want: "duplicate owner"},
		{name: "namespace escape", sources: []prompt.Source{{Owner: "example.orders", FS: fstest.MapFS{"other/summary.md": {Data: []byte("Unsafe")}}}}, want: "must use prefix"},
		{name: "built-in override", sources: []prompt.Source{{Owner: "ainp", FS: fstest.MapFS{"ai/base.md": {Data: []byte("Base")}}}, {Owner: "example.orders", FS: fstest.MapFS{"ai/base.md": {Data: []byte("Override")}}}}, want: "must use prefix"},
		{name: "invalid template", sources: []prompt.Source{{Owner: "example.orders", FS: fstest.MapFS{"example.orders/bad.md": {Data: []byte("{{")}}}}, want: "parse template"},
		{name: "empty source", sources: []prompt.Source{{Owner: "example.orders", FS: fstest.MapFS{}}}, want: "has no Markdown resources"},
	} {
		t.Run("prompt "+test.name, func(t *testing.T) {
			if err := prompt.LoadSources(test.sources...); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("expected %q, got %v", test.want, err)
			}
		})
	}

	validI18n := fstest.MapFS{"en.yaml": {Data: []byte("example.orders.title: Orders\n")}}
	for _, test := range []struct {
		name    string
		sources []i18n.Source
		want    string
	}{
		{name: "duplicate owner", sources: []i18n.Source{{Owner: "example.orders", FS: validI18n}, {Owner: "example.orders", FS: validI18n}}, want: "duplicate owner"},
		{name: "namespace escape", sources: []i18n.Source{{Owner: "example.orders", FS: fstest.MapFS{"en.yaml": {Data: []byte("other.title: Unsafe\n")}}}}, want: "must use prefix"},
		{name: "built-in override", sources: []i18n.Source{{Owner: "ainp", FS: fstest.MapFS{"en.yaml": {Data: []byte("common.ok: OK\n")}}}, {Owner: "example.orders", FS: fstest.MapFS{"en.yaml": {Data: []byte("common.ok: Override\n")}}}}, want: "must use prefix"},
		{name: "invalid yaml", sources: []i18n.Source{{Owner: "example.orders", FS: fstest.MapFS{"en.yaml": {Data: []byte("[invalid")}}}}, want: "parse"},
		{name: "empty source", sources: []i18n.Source{{Owner: "example.orders", FS: fstest.MapFS{}}}, want: "has no YAML translations"},
		{name: "missing locale", sources: []i18n.Source{{Owner: "example.orders", FS: fstest.MapFS{"en.yaml": {Data: []byte("example.orders.title: Orders\n")}, "zh-CN.yaml": {Data: []byte("example.orders.other: 其他\n")}}}}, want: "is missing locale"},
	} {
		t.Run("i18n "+test.name, func(t *testing.T) {
			if err := i18n.LoadSources(test.sources...); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("expected %q, got %v", test.want, err)
			}
		})
	}
}

func TestTrustedDevelopmentRuntimeAssetsReloadAtomically(t *testing.T) {
	promptFS := fstest.MapFS{"ai/base.md": {Data: []byte("first")}}
	if err := prompt.LoadReloadableSources(prompt.Source{Owner: "ainp", FS: promptFS}); err != nil {
		t.Fatal(err)
	}
	promptFS["ai/base.md"] = &fstest.MapFile{Data: []byte("second")}
	if err := prompt.Reload(); err != nil || prompt.Get("ai/base") != "second" {
		t.Fatalf("trusted Prompt reload failed atomically: %q, %v", prompt.Get("ai/base"), err)
	}
	promptFS["ai/base.md"] = &fstest.MapFile{Data: []byte("{{")}
	if err := prompt.Reload(); err == nil || prompt.Get("ai/base") != "second" {
		t.Fatalf("failed Prompt reload must retain the last valid snapshot: %q, %v", prompt.Get("ai/base"), err)
	}

	i18nFS := fstest.MapFS{"en.yaml": {Data: []byte("common.ok: First\n")}}
	if err := i18n.LoadReloadableSources(i18n.Source{Owner: "ainp", FS: i18nFS}); err != nil {
		t.Fatal(err)
	}
	i18nFS["en.yaml"] = &fstest.MapFile{Data: []byte("common.ok: Second\n")}
	if err := i18n.Reload(); err != nil || i18n.T("en", "common.ok") != "Second" {
		t.Fatalf("trusted i18n reload failed atomically: %q, %v", i18n.T("en", "common.ok"), err)
	}
	i18nFS["en.yaml"] = &fstest.MapFile{Data: []byte("[invalid")}
	if err := i18n.Reload(); err == nil || i18n.T("en", "common.ok") != "Second" {
		t.Fatalf("failed i18n reload must retain the last valid snapshot: %q, %v", i18n.T("en", "common.ok"), err)
	}
}
