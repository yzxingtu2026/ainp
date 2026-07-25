package tests

import (
	"os"
	"path/filepath"
	"sync"
	"testing"

	. "github.com/yzxingtu2026/ainp/platform/framework/prompt"
)

func TestLoadGetRenderAndReload(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ai")
	if err := os.Mkdir(path, 0o755); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(path, "hello.md")
	if err := os.WriteFile(file, []byte("---\ndescription: test\nversion: 1.2.3\n---\n\n你好，{{.Name}}"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := LoadDir(dir); err != nil {
		t.Fatal(err)
	}
	if got := Get("ai/hello"); got != "你好，{{.Name}}" {
		t.Fatalf("unexpected body: %q", got)
	}
	if metadata, ok := MetadataFor("ai/hello"); !ok || metadata.Version != "1.2.3" || metadata.Description != "test" {
		t.Fatalf("unexpected metadata: %#v, %v", metadata, ok)
	}
	if got := Render("ai/hello", map[string]string{"Name": "小明"}); got != "你好，小明" {
		t.Fatalf("unexpected render: %q", got)
	}
	if err := os.WriteFile(file, []byte("您好，{{.Name}}"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := Reload(); err != nil {
		t.Fatal(err)
	}
	if got := Render("ai/hello", map[string]string{"Name": "小明"}); got != "您好，小明" {
		t.Fatalf("unexpected reload: %q", got)
	}
}

func TestFailedReloadPreservesPreviousSnapshot(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "stable.md")
	if err := os.WriteFile(file, []byte("stable"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := LoadDir(dir); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, []byte("{{"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := Reload(); err == nil {
		t.Fatal("expected invalid template error")
	}
	if got := Get("stable"); got != "stable" {
		t.Fatalf("previous snapshot lost: %q", got)
	}
}

func TestConcurrentGetAndReload(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "value.md")
	if err := os.WriteFile(file, []byte("value"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := LoadDir(dir); err != nil {
		t.Fatal(err)
	}
	var wait sync.WaitGroup
	for range 10 {
		wait.Add(1)
		go func() {
			defer wait.Done()
			for range 100 {
				_ = Get("value")
			}
		}()
	}
	for range 10 {
		if err := Reload(); err != nil {
			t.Fatal(err)
		}
	}
	wait.Wait()
}
