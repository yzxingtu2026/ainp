// Package prompt loads model prompts from Markdown files on disk.
package prompt

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"text/template"

	"gopkg.in/yaml.v3"
)

type entry struct {
	body     string
	metadata Metadata
	template *template.Template
}

type Metadata struct {
	Description string `yaml:"description"`
	Version     string `yaml:"version"`
}

// Source is one immutable, owner-scoped prompt filesystem.
type Source struct {
	Owner string
	FS    fs.FS
}

// Asset is safe runtime diagnostic metadata and never includes prompt content.
type Asset struct {
	Owner   string
	Key     string
	Type    string
	Digest  string
	Version string
}

var ownerPattern = regexp.MustCompile(`^[a-z][a-z0-9]*(?:[._-][a-z0-9]+)*$`)

var registry = struct {
	sync.RWMutex
	dir     string
	sources []Source
	entries map[string]entry
	assets  []Asset
}{entries: map[string]entry{}}

// LoadDir loads all Markdown prompts below dir and remembers dir for Reload.
func LoadDir(dir string) error {
	if strings.TrimSpace(dir) == "" {
		return fmt.Errorf("prompt: directory is required")
	}
	entries, assets, err := loadSources(Source{Owner: "development", FS: os.DirFS(dir)})
	if err != nil {
		return err
	}
	registry.Lock()
	registry.dir = dir
	registry.sources = []Source{{Owner: "development", FS: os.DirFS(dir)}}
	registry.entries = entries
	registry.assets = assets
	registry.Unlock()
	return nil
}

// Reload atomically replaces the registry with prompts read from the last directory.
func Reload() error {
	registry.RLock()
	sources := append([]Source(nil), registry.sources...)
	registry.RUnlock()
	if len(sources) == 0 {
		return fmt.Errorf("prompt: loaded resources are immutable; configure an explicit development source to reload")
	}
	entries, assets, err := loadSources(sources...)
	if err != nil {
		return err
	}
	registry.Lock()
	registry.entries = entries
	registry.assets = assets
	registry.Unlock()
	return nil
}

// LoadSources atomically replaces the registry from deterministic immutable sources.
func LoadSources(sources ...Source) error {
	entries, assets, err := loadSources(sources...)
	if err != nil {
		return err
	}
	registry.Lock()
	registry.dir = ""
	registry.sources = nil
	registry.entries = entries
	registry.assets = assets
	registry.Unlock()
	return nil
}

// LoadReloadableSources loads trusted development sources and enables Reload.
func LoadReloadableSources(sources ...Source) error {
	entries, assets, err := loadSources(sources...)
	if err != nil {
		return err
	}
	registry.Lock()
	registry.dir = ""
	registry.sources = append([]Source(nil), sources...)
	registry.entries = entries
	registry.assets = assets
	registry.Unlock()
	return nil
}

// Manifest returns safe, deterministic metadata for the loaded prompt set.
func Manifest() []Asset {
	registry.RLock()
	defer registry.RUnlock()
	return append([]Asset(nil), registry.assets...)
}

// Get returns a prompt body, or an empty string when key is unknown.
func Get(key string) string {
	registry.RLock()
	value := registry.entries[key].body
	registry.RUnlock()
	return value
}

// MustGet returns a prompt body and panics when key is unknown.
func MustGet(key string) string {
	value := Get(key)
	if value == "" {
		panic(fmt.Sprintf("prompt: required prompt %q is not loaded", key))
	}
	return value
}

// MetadataFor returns immutable prompt source metadata without exposing its body.
func MetadataFor(key string) (Metadata, bool) {
	registry.RLock()
	item, ok := registry.entries[key]
	registry.RUnlock()
	return item.metadata, ok
}

// Render executes a loaded prompt as a Go text/template and panics on failure.
func Render(key string, data any) string {
	registry.RLock()
	item, ok := registry.entries[key]
	registry.RUnlock()
	if !ok {
		panic(fmt.Sprintf("prompt: required prompt %q is not loaded", key))
	}
	var output bytes.Buffer
	if err := item.template.Execute(&output, data); err != nil {
		panic(fmt.Sprintf("prompt: render %q: %v", key, err))
	}
	return output.String()
}

func loadSources(sources ...Source) (map[string]entry, []Asset, error) {
	loaded := map[string]entry{}
	assets := make([]Asset, 0)
	owners := map[string]struct{}{}
	for _, source := range sources {
		owner := strings.TrimSpace(source.Owner)
		if !ownerPattern.MatchString(owner) {
			return nil, nil, fmt.Errorf("prompt: invalid owner %q", owner)
		}
		if source.FS == nil {
			return nil, nil, fmt.Errorf("prompt: owner %q filesystem is required", owner)
		}
		if _, exists := owners[owner]; exists {
			return nil, nil, fmt.Errorf("prompt: duplicate owner %q", owner)
		}
		owners[owner] = struct{}{}
		assetCount := len(assets)
		err := fs.WalkDir(source.FS, ".", func(path string, item fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if item.IsDir() || !strings.EqualFold(filepath.Ext(item.Name()), ".md") {
				return nil
			}
			key := strings.TrimSuffix(filepath.ToSlash(strings.TrimPrefix(path, "./")), filepath.Ext(path))
			if owner != "ainp" && owner != "development" && !strings.HasPrefix(key, owner+"/") {
				return fmt.Errorf("prompt: owner %q key %q must use prefix %q", owner, key, owner+"/")
			}
			data, err := fs.ReadFile(source.FS, path)
			if err != nil {
				return fmt.Errorf("prompt: owner %q read %s: %w", owner, path, err)
			}
			metadata, body, err := parseMarkdown(string(data))
			if err != nil {
				return fmt.Errorf("prompt: owner %q parse %s: %w", owner, path, err)
			}
			if strings.TrimSpace(body) == "" {
				return fmt.Errorf("prompt: owner %q resource %q has an empty body", owner, key)
			}
			parsed, err := template.New(key).Option("missingkey=error").Parse(body)
			if err != nil {
				return fmt.Errorf("prompt: owner %q parse template %q: %w", owner, key, err)
			}
			if _, exists := loaded[key]; exists {
				return fmt.Errorf("prompt: owner %q conflicts on key %q", owner, key)
			}
			loaded[key] = entry{body: body, metadata: metadata, template: parsed}
			digest := fmt.Sprintf("sha256:%x", sha256.Sum256(data))
			assets = append(assets, Asset{Owner: owner, Key: key, Type: "prompt", Digest: digest, Version: metadata.Version})
			return nil
		})
		if err != nil {
			return nil, nil, fmt.Errorf("prompt: load owner %q: %w", owner, err)
		}
		if len(assets) == assetCount {
			return nil, nil, fmt.Errorf("prompt: owner %q has no Markdown resources", owner)
		}
	}
	if len(loaded) == 0 {
		return nil, nil, fmt.Errorf("prompt: no Markdown resources found")
	}
	sort.Slice(assets, func(i, j int) bool {
		if assets[i].Owner != assets[j].Owner {
			return assets[i].Owner < assets[j].Owner
		}
		return assets[i].Key < assets[j].Key
	})
	return loaded, assets, nil
}

func parseMarkdown(content string) (Metadata, string, error) {
	normalized := strings.ReplaceAll(content, "\r\n", "\n")
	if !strings.HasPrefix(normalized, "---\n") {
		return Metadata{}, strings.TrimSpace(normalized), nil
	}
	end := strings.Index(normalized[4:], "\n---\n")
	if end < 0 {
		return Metadata{}, "", fmt.Errorf("frontmatter closing delimiter is missing")
	}
	var metadata Metadata
	if err := yaml.Unmarshal([]byte(normalized[4:4+end]), &metadata); err != nil {
		return Metadata{}, "", fmt.Errorf("invalid frontmatter: %w", err)
	}
	return metadata, strings.TrimSpace(normalized[4+end+5:]), nil
}
