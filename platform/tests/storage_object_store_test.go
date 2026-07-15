package tests

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/yzxingtu2026/ainp/platform/framework/storage"
)

func newTestLocalStore(t *testing.T) *storage.LocalObjectStore {
	t.Helper()
	root := t.TempDir()
	store, err := storage.NewLocalObjectStore(storage.LocalConfig{RootDir: root}, nil)
	if err != nil {
		t.Fatal(err)
	}
	return store
}

func TestStorageKeyValidationAndBuilder(t *testing.T) {
	if _, err := storage.BuildKey(storage.KeyParts{TenantID: "../x", Namespace: "attachments"}); !errors.Is(err, storage.ErrInvalidKey) {
		t.Fatalf("traversal tenant: %v", err)
	}
	if err := storage.ValidateKey("../etc/passwd"); !errors.Is(err, storage.ErrInvalidKey) {
		t.Fatalf("validate traversal: %v", err)
	}
	if err := storage.ValidateKey("/abs"); !errors.Is(err, storage.ErrInvalidKey) {
		t.Fatalf("validate abs: %v", err)
	}
	if err := storage.ValidateKey(`a\b`); !errors.Is(err, storage.ErrInvalidKey) {
		t.Fatalf("validate backslash: %v", err)
	}
	if err := storage.ValidateKey("a//b"); !errors.Is(err, storage.ErrInvalidKey) {
		t.Fatalf("validate empty segment: %v", err)
	}
	key, err := storage.BuildKey(storage.KeyParts{
		TenantID: "tenant_demo", Namespace: "attachments", ObjectID: "obj-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(key, "tenant/tenant_demo/attachments/") || !strings.HasSuffix(key, "/obj-1") {
		t.Fatalf("unexpected key %q", key)
	}
}

func TestLocalObjectStoreContract(t *testing.T) {
	runObjectStoreContract(t, newTestLocalStore(t))
}

func runObjectStoreContract(t *testing.T, store storage.ObjectStore) {
	t.Helper()
	ctx := context.Background()
	key := "tenant/t1/attachments/2026/07/obj-contract"
	payload := []byte("hello-object-store")

	info, err := store.Put(ctx, storage.PutRequest{
		Key: key, Body: bytes.NewReader(payload), Size: int64(len(payload)), ContentType: "text/plain",
	})
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(payload)
	if info.Size != int64(len(payload)) || info.ETag != hex.EncodeToString(sum[:]) || info.Provider == "" {
		t.Fatalf("put info = %#v", info)
	}

	exists, err := store.Exists(ctx, key)
	if err != nil || !exists {
		t.Fatalf("exists = %v %v", exists, err)
	}
	st, err := store.Stat(ctx, key)
	if err != nil || st.Size != int64(len(payload)) {
		t.Fatalf("stat = %#v %v", st, err)
	}

	rc, openInfo, err := store.Open(ctx, key)
	if err != nil {
		t.Fatal(err)
	}
	defer rc.Close()
	got, err := io.ReadAll(rc)
	if err != nil || !bytes.Equal(got, payload) || openInfo.Size != int64(len(payload)) {
		t.Fatalf("open got %q info %#v err %v", got, openInfo, err)
	}

	rrc, _, err := store.OpenRange(ctx, key, 6, 6)
	if err != nil {
		t.Fatal(err)
	}
	defer rrc.Close()
	part, err := io.ReadAll(rrc)
	if err != nil || string(part) != "object" {
		t.Fatalf("range = %q err %v", part, err)
	}
	if _, _, err := store.OpenRange(ctx, key, 100, 1); !errors.Is(err, storage.ErrRangeInvalid) {
		t.Fatalf("invalid range err = %v", err)
	}

	if err := store.Delete(ctx, key); err != nil {
		t.Fatal(err)
	}
	if err := store.Delete(ctx, key); err != nil {
		t.Fatalf("idempotent delete: %v", err)
	}
	exists, err = store.Exists(ctx, key)
	if err != nil || exists {
		t.Fatalf("after delete exists=%v err=%v", exists, err)
	}
	if _, err := store.Stat(ctx, key); !errors.Is(err, storage.ErrNotFound) {
		t.Fatalf("stat missing = %v", err)
	}
}

func TestLocalObjectStoreRejectsPathTraversalAndSymlink(t *testing.T) {
	store := newTestLocalStore(t)
	ctx := context.Background()
	if _, err := store.Put(ctx, storage.PutRequest{Key: "../escape", Body: strings.NewReader("x"), Size: 1}); !errors.Is(err, storage.ErrInvalidKey) {
		t.Fatalf("traversal put: %v", err)
	}

	root := t.TempDir()
	outside := t.TempDir()
	outsideFile := filepath.Join(outside, "secret")
	if err := os.WriteFile(outsideFile, []byte("secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	linkDir := filepath.Join(root, "tenant")
	if err := os.MkdirAll(linkDir, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(linkDir, "t1")); err != nil {
		t.Fatal(err)
	}
	linked, err := storage.NewLocalObjectStore(storage.LocalConfig{RootDir: root}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := linked.Put(ctx, storage.PutRequest{
		Key: "tenant/t1/attachments/2026/07/obj", Body: strings.NewReader("x"), Size: 1,
	}); !errors.Is(err, storage.ErrInvalidKey) {
		t.Fatalf("symlink escape put: %v", err)
	}
}

func TestLocalObjectStoreAtomicPutAndSizeMismatch(t *testing.T) {
	store := newTestLocalStore(t)
	ctx := context.Background()
	key := "tenant/t1/attachments/2026/07/obj-mismatch"
	_, err := store.Put(ctx, storage.PutRequest{Key: key, Body: strings.NewReader("abc"), Size: 10})
	if !errors.Is(err, storage.ErrSizeMismatch) {
		t.Fatalf("size mismatch: %v", err)
	}
	exists, err := store.Exists(ctx, key)
	if err != nil || exists {
		t.Fatalf("mismatch must not leave object: exists=%v err=%v", exists, err)
	}
	// No leftover temps under root — find via TempDir walk of store root is hard;
	// ensure Exists false and a second put succeeds.
	if _, err := store.Put(ctx, storage.PutRequest{Key: key, Body: strings.NewReader("abc"), Size: 3}); err != nil {
		t.Fatal(err)
	}
}

func TestLocalObjectStoreConcurrentPutOpen(t *testing.T) {
	store := newTestLocalStore(t)
	ctx := context.Background()
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			key := "tenant/t1/attachments/2026/07/c-" + string(rune('a'+i))
			body := strings.Repeat("x", 1024)
			if _, err := store.Put(ctx, storage.PutRequest{Key: key, Body: strings.NewReader(body), Size: int64(len(body))}); err != nil {
				t.Errorf("put: %v", err)
				return
			}
			rc, _, err := store.Open(ctx, key)
			if err != nil {
				t.Errorf("open: %v", err)
				return
			}
			_, _ = io.Copy(io.Discard, rc)
			_ = rc.Close()
		}(i)
	}
	wg.Wait()
}

func TestStorageFactoryUnknownProvider(t *testing.T) {
	_, err := storage.NewFromConfig(storage.Config{Provider: "oss"}, nil)
	if !errors.Is(err, storage.ErrUnknownProvider) {
		t.Fatalf("unknown provider: %v", err)
	}
}

func TestLocalObjectStoreCheckReady(t *testing.T) {
	store := newTestLocalStore(t)
	if err := store.CheckReady(context.Background()); err != nil {
		t.Fatal(err)
	}
}
