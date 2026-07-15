package storage

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const ProviderLocal = "local"

// LocalConfig configures the filesystem ObjectStore.
type LocalConfig struct {
	RootDir string
}

// LocalObjectStore stores objects under a rooted directory tree.
type LocalObjectStore struct {
	root    string
	metrics MetricsRecorder
}

func NewLocalObjectStore(cfg LocalConfig, metrics MetricsRecorder) (*LocalObjectStore, error) {
	if strings.TrimSpace(cfg.RootDir) == "" {
		return nil, fmt.Errorf("%w: local root_dir is required", ErrProviderUnavailable)
	}
	abs, err := filepath.Abs(cfg.RootDir)
	if err != nil {
		return nil, fmt.Errorf("%w: resolve root: %v", ErrProviderUnavailable, err)
	}
	info, err := os.Stat(abs)
	if err != nil {
		if os.IsNotExist(err) {
			if mkErr := os.MkdirAll(abs, 0o750); mkErr != nil {
				return nil, fmt.Errorf("%w: create root: %v", ErrProviderUnavailable, mkErr)
			}
			info, err = os.Stat(abs)
		}
		if err != nil {
			return nil, fmt.Errorf("%w: stat root: %v", ErrProviderUnavailable, err)
		}
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("%w: root is not a directory", ErrProviderUnavailable)
	}
	// Ensure we can write by creating the root if needed; probe write access lightly.
	probe := filepath.Join(abs, ".write_probe")
	if err := os.WriteFile(probe, []byte("ok"), 0o640); err != nil {
		return nil, fmt.Errorf("%w: root not writable: %v", ErrProviderUnavailable, err)
	}
	_ = os.Remove(probe)

	resolved, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return nil, fmt.Errorf("%w: eval root: %v", ErrProviderUnavailable, err)
	}
	return &LocalObjectStore{root: resolved, metrics: metrics}, nil
}

func (s *LocalObjectStore) Put(ctx context.Context, req PutRequest) (ObjectInfo, error) {
	start := time.Now()
	info, err := s.put(ctx, req)
	s.observe(start, "put", info.Size, err)
	return info, err
}

func (s *LocalObjectStore) put(ctx context.Context, req PutRequest) (ObjectInfo, error) {
	if err := ctx.Err(); err != nil {
		return ObjectInfo{}, err
	}
	if req.Body == nil {
		return ObjectInfo{}, fmt.Errorf("%w: nil body", ErrIO)
	}
	target, err := s.resolvePath(req.Key)
	if err != nil {
		return ObjectInfo{}, err
	}
	dir := filepath.Dir(target)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return ObjectInfo{}, fmt.Errorf("%w: mkdir: %v", ErrIO, err)
	}

	tmp, err := os.CreateTemp(dir, ".ainp-put-*")
	if err != nil {
		return ObjectInfo{}, fmt.Errorf("%w: create temp: %v", ErrIO, err)
	}
	tmpName := tmp.Name()
	cleanup := true
	defer func() {
		_ = tmp.Close()
		if cleanup {
			_ = os.Remove(tmpName)
		}
	}()
	if err := tmp.Chmod(0o640); err != nil {
		return ObjectInfo{}, fmt.Errorf("%w: chmod temp: %v", ErrIO, err)
	}

	hash := sha256.New()
	writer := io.MultiWriter(tmp, hash)
	written, err := io.Copy(writer, &ctxReader{ctx: ctx, r: req.Body})
	if err != nil {
		return ObjectInfo{}, fmt.Errorf("%w: write: %v", ErrIO, err)
	}
	if req.Size >= 0 && written != req.Size {
		return ObjectInfo{}, fmt.Errorf("%w: expected %d got %d", ErrSizeMismatch, req.Size, written)
	}
	if err := tmp.Sync(); err != nil {
		return ObjectInfo{}, fmt.Errorf("%w: sync: %v", ErrIO, err)
	}
	if err := tmp.Close(); err != nil {
		return ObjectInfo{}, fmt.Errorf("%w: close temp: %v", ErrIO, err)
	}
	if err := os.Rename(tmpName, target); err != nil {
		return ObjectInfo{}, fmt.Errorf("%w: rename: %v", ErrIO, err)
	}
	cleanup = false

	stat, err := os.Stat(target)
	if err != nil {
		return ObjectInfo{}, fmt.Errorf("%w: stat after put: %v", ErrIO, err)
	}
	return ObjectInfo{
		Key:         req.Key,
		Size:        written,
		ContentType: req.ContentType,
		ETag:        hex.EncodeToString(hash.Sum(nil)),
		ModifiedAt:  stat.ModTime().UTC(),
		Provider:    ProviderLocal,
	}, nil
}

func (s *LocalObjectStore) Open(ctx context.Context, key string) (io.ReadCloser, ObjectInfo, error) {
	start := time.Now()
	rc, info, err := s.openRange(ctx, key, 0, -1)
	s.observe(start, "open", info.Size, err)
	return rc, info, err
}

func (s *LocalObjectStore) OpenRange(ctx context.Context, key string, offset, length int64) (io.ReadCloser, ObjectInfo, error) {
	start := time.Now()
	rc, info, err := s.openRange(ctx, key, offset, length)
	s.observe(start, "open_range", length, err)
	return rc, info, err
}

func (s *LocalObjectStore) openRange(ctx context.Context, key string, offset, length int64) (io.ReadCloser, ObjectInfo, error) {
	if err := ctx.Err(); err != nil {
		return nil, ObjectInfo{}, err
	}
	path, err := s.resolvePath(key)
	if err != nil {
		return nil, ObjectInfo{}, err
	}
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ObjectInfo{}, ErrNotFound
		}
		return nil, ObjectInfo{}, fmt.Errorf("%w: open: %v", ErrIO, err)
	}
	stat, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return nil, ObjectInfo{}, fmt.Errorf("%w: stat: %v", ErrIO, err)
	}
	if stat.IsDir() {
		_ = f.Close()
		return nil, ObjectInfo{}, ErrNotFound
	}
	size := stat.Size()
	if offset < 0 || offset > size {
		_ = f.Close()
		return nil, ObjectInfo{}, ErrRangeInvalid
	}
	if length == 0 {
		_ = f.Close()
		return nil, ObjectInfo{}, ErrRangeInvalid
	}
	if length < 0 {
		length = size - offset
	}
	if offset+length > size {
		_ = f.Close()
		return nil, ObjectInfo{}, ErrRangeInvalid
	}
	if offset > 0 {
		if _, err := f.Seek(offset, io.SeekStart); err != nil {
			_ = f.Close()
			return nil, ObjectInfo{}, fmt.Errorf("%w: seek: %v", ErrIO, err)
		}
	}
	info := ObjectInfo{
		Key:        key,
		Size:       size,
		ModifiedAt: stat.ModTime().UTC(),
		Provider:   ProviderLocal,
	}
	if length == size && offset == 0 {
		return f, info, nil
	}
	return &limitedReadCloser{ReadCloser: f, remain: length}, info, nil
}

func (s *LocalObjectStore) Stat(ctx context.Context, key string) (ObjectInfo, error) {
	if err := ctx.Err(); err != nil {
		return ObjectInfo{}, err
	}
	path, err := s.resolvePath(key)
	if err != nil {
		return ObjectInfo{}, err
	}
	stat, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return ObjectInfo{}, ErrNotFound
		}
		return ObjectInfo{}, fmt.Errorf("%w: lstat: %v", ErrIO, err)
	}
	if stat.Mode()&os.ModeSymlink != 0 {
		return ObjectInfo{}, fmt.Errorf("%w: symlink not allowed", ErrInvalidKey)
	}
	if stat.IsDir() {
		return ObjectInfo{}, ErrNotFound
	}
	return ObjectInfo{
		Key:        key,
		Size:       stat.Size(),
		ModifiedAt: stat.ModTime().UTC(),
		Provider:   ProviderLocal,
	}, nil
}

func (s *LocalObjectStore) Delete(ctx context.Context, key string) error {
	start := time.Now()
	err := s.delete(ctx, key)
	s.observe(start, "delete", 0, err)
	return err
}

func (s *LocalObjectStore) delete(ctx context.Context, key string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	path, err := s.resolvePath(key)
	if err != nil {
		return err
	}
	stat, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("%w: lstat: %v", ErrIO, err)
	}
	if stat.IsDir() {
		return fmt.Errorf("%w: refuse to delete directory", ErrInvalidKey)
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("%w: remove: %v", ErrIO, err)
	}
	return nil
}

func (s *LocalObjectStore) Exists(ctx context.Context, key string) (bool, error) {
	_, err := s.Stat(ctx, key)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, ErrNotFound) {
		return false, nil
	}
	return false, err
}

// CheckReady verifies the root is still a writable directory without leaving objects.
func (s *LocalObjectStore) CheckReady(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	info, err := os.Stat(s.root)
	if err != nil || !info.IsDir() {
		return fmt.Errorf("%w: root unavailable", ErrProviderUnavailable)
	}
	probe := filepath.Join(s.root, ".ready_probe")
	if err := os.WriteFile(probe, []byte("1"), 0o640); err != nil {
		return fmt.Errorf("%w: root not writable", ErrProviderUnavailable)
	}
	if err := os.Remove(probe); err != nil {
		return fmt.Errorf("%w: cleanup probe: %v", ErrIO, err)
	}
	return nil
}

func (s *LocalObjectStore) resolvePath(key string) (string, error) {
	if err := ValidateKey(key); err != nil {
		return "", err
	}
	joined := filepath.Join(append([]string{s.root}, strings.Split(key, "/")...)...)
	cleaned := filepath.Clean(joined)
	rel, err := filepath.Rel(s.root, cleaned)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("%w: path escapes root", ErrInvalidKey)
	}
	// Walk existing prefix only; reject symlink escape. Missing parents are allowed for Put.
	cur := s.root
	for _, part := range strings.Split(key, "/") {
		next := filepath.Join(cur, part)
		fi, err := os.Lstat(next)
		if err != nil {
			if os.IsNotExist(err) {
				break
			}
			return "", fmt.Errorf("%w: lstat: %v", ErrIO, err)
		}
		if fi.Mode()&os.ModeSymlink != 0 {
			return "", fmt.Errorf("%w: symlink segment not allowed", ErrInvalidKey)
		}
		cur = next
	}
	return cleaned, nil
}

func (s *LocalObjectStore) observe(start time.Time, op string, bytes int64, err error) {
	if s.metrics == nil {
		return
	}
	s.metrics.ObserveDuration(op, time.Since(start))
	if bytes > 0 {
		s.metrics.ObserveBytes(op, bytes)
	}
	if err != nil {
		s.metrics.ObserveFailure(op)
	}
}

type limitedReadCloser struct {
	io.ReadCloser
	remain int64
}

func (l *limitedReadCloser) Read(p []byte) (int, error) {
	if l.remain <= 0 {
		return 0, io.EOF
	}
	if int64(len(p)) > l.remain {
		p = p[:l.remain]
	}
	n, err := l.ReadCloser.Read(p)
	l.remain -= int64(n)
	return n, err
}

type ctxReader struct {
	ctx context.Context
	r   io.Reader
}

func (c *ctxReader) Read(p []byte) (int, error) {
	if err := c.ctx.Err(); err != nil {
		return 0, err
	}
	return c.r.Read(p)
}
