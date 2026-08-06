package host

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/yzxingtu2026/ainp/platform/framework/app"
	"github.com/yzxingtu2026/ainp/platform/migrations"
	"gorm.io/gorm"
)

var moduleIDPattern = regexp.MustCompile(`^[a-z][a-z0-9]*(?:[._-][a-z0-9]+)*$`)

// Module is the stable downstream extension contract shared by API and Worker.
// Register runs after all built-in modules have registered.
type Module interface {
	ID() string
	Register(*app.App) error
}

// Preparer is an optional early phase for provider-neutral framework
// injections, such as authenticators needed by built-in system registration.
type Preparer interface {
	Prepare(*app.App) error
}

// MigrationProvider optionally contributes a namespaced forward-only source.
type MigrationProvider interface {
	MigrationSource() migrations.Source
}

// AutoMigrator creates a downstream module's current schema during first-time
// installation. Existing environments must use MigrationSource instead.
type AutoMigrator interface {
	AutoMigrate(*gorm.DB) error
}

type Option func(*options) error

type options struct {
	modules     []Module
	demoEnabled bool
}

// WithModules adds one immutable module declaration to both API and Worker.
func WithModules(modules ...Module) Option {
	return func(opts *options) error {
		opts.modules = append(opts.modules, modules...)
		return nil
	}
}

// WithDemo explicitly includes AINP's development and showcase module.
// Production downstream hosts omit this option and therefore exclude Demo.
func WithDemo() Option {
	return func(opts *options) error {
		opts.demoEnabled = true
		return nil
	}
}

// Composition validates and owns a deterministic downstream module list.
type Composition struct {
	modules     []Module
	migrations  migrations.Set
	demoEnabled bool
}

func New(optionFns ...Option) (*Composition, error) {
	opts := options{}
	for _, option := range optionFns {
		if option == nil {
			return nil, fmt.Errorf("host: option is required")
		}
		if err := option(&opts); err != nil {
			return nil, err
		}
	}
	seen := make(map[string]struct{}, len(opts.modules))
	modules := append([]Module(nil), opts.modules...)
	for _, module := range modules {
		if module == nil {
			return nil, fmt.Errorf("host: module is required")
		}
		id := strings.TrimSpace(module.ID())
		if !moduleIDPattern.MatchString(id) {
			return nil, fmt.Errorf("host: invalid module id %q", id)
		}
		if _, exists := seen[id]; exists {
			return nil, fmt.Errorf("host: duplicate module id %q", id)
		}
		seen[id] = struct{}{}
	}
	sort.SliceStable(modules, func(i, j int) bool { return modules[i].ID() < modules[j].ID() })
	sources := make([]migrations.Source, 0, len(modules))
	for _, module := range modules {
		provider, ok := module.(MigrationProvider)
		if !ok {
			continue
		}
		source := provider.MigrationSource()
		if source.Owner == "" {
			source.Owner = module.ID()
		}
		if source.Owner != module.ID() {
			return nil, fmt.Errorf("host: module %q migration owner must match module id", module.ID())
		}
		sources = append(sources, source)
	}
	set, err := migrations.NewSet(sources...)
	if err != nil {
		return nil, fmt.Errorf("host: migrations: %w", err)
	}
	return &Composition{modules: modules, migrations: set, demoEnabled: opts.demoEnabled}, nil
}

func (c *Composition) Migrations() migrations.Set { return c.migrations }

// DemoEnabled reports the immutable Demo selection used by API and Worker.
func (c *Composition) DemoEnabled() bool { return c.demoEnabled }

func (c *Composition) prepare(application *app.App) error {
	for _, module := range c.modules {
		if preparer, ok := module.(Preparer); ok {
			if err := preparer.Prepare(application); err != nil {
				return fmt.Errorf("host: prepare module %q: %w", module.ID(), err)
			}
		}
	}
	return nil
}

func (c *Composition) register(application *app.App) error {
	for _, module := range c.modules {
		if err := module.Register(application); err != nil {
			return fmt.Errorf("host: register module %q: %w", module.ID(), err)
		}
	}
	return nil
}

func (c *Composition) autoMigrate(db *gorm.DB) error {
	for _, module := range c.modules {
		migrator, ok := module.(AutoMigrator)
		if !ok {
			continue
		}
		if err := migrator.AutoMigrate(db); err != nil {
			return fmt.Errorf("host: auto migrate module %q: %w", module.ID(), err)
		}
	}
	return nil
}
