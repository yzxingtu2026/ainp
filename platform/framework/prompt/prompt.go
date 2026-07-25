// Package prompt loads model prompts from Markdown files on disk.
package prompt

import (
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
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

var registry = struct {
	sync.RWMutex
	dir     string
	entries map[string]entry
}{entries: map[string]entry{}}

// LoadDir loads all Markdown prompts below dir and remembers dir for Reload.
func LoadDir(dir string) error {
	if strings.TrimSpace(dir) == "" {
		return fmt.Errorf("prompt: directory is required")
	}
	entries, err := load(dir)
	if err != nil {
		return err
	}
	registry.Lock()
	registry.dir = dir
	registry.entries = entries
	registry.Unlock()
	return nil
}

// Reload atomically replaces the registry with prompts read from the last directory.
func Reload() error {
	registry.RLock()
	dir := registry.dir
	registry.RUnlock()
	if dir == "" {
		return fmt.Errorf("prompt: directory not set, call LoadDir first")
	}
	entries, err := load(dir)
	if err != nil {
		return err
	}
	registry.Lock()
	registry.entries = entries
	registry.Unlock()
	return nil
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

func load(dir string) (map[string]entry, error) {
	loaded := map[string]entry{}
	err := filepath.WalkDir(dir, func(path string, item fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if item.IsDir() || !strings.EqualFold(filepath.Ext(item.Name()), ".md") {
			return nil
		}
		relative, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		key := filepath.ToSlash(strings.TrimSuffix(relative, filepath.Ext(relative)))
		data, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("prompt: read %s: %w", path, err)
		}
		metadata, body, err := parseMarkdown(string(data))
		if err != nil {
			return fmt.Errorf("prompt: parse %s: %w", path, err)
		}
		if strings.TrimSpace(body) == "" {
			return fmt.Errorf("prompt: %s has an empty body", path)
		}
		parsed, err := template.New(key).Option("missingkey=error").Parse(body)
		if err != nil {
			return fmt.Errorf("prompt: parse template %s: %w", path, err)
		}
		if _, exists := loaded[key]; exists {
			return fmt.Errorf("prompt: duplicate key %q", key)
		}
		loaded[key] = entry{body: body, metadata: metadata, template: parsed}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("prompt: load directory %s: %w", dir, err)
	}
	if len(loaded) == 0 {
		return nil, fmt.Errorf("prompt: no Markdown files found in %s", dir)
	}
	return loaded, nil
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
