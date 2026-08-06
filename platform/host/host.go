package host

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"

	"github.com/yzxingtu2026/ainp/platform/framework/app"
	"github.com/yzxingtu2026/ainp/platform/framework/asynqadapter"
	"github.com/yzxingtu2026/ainp/platform/framework/attachment"
	"github.com/yzxingtu2026/ainp/platform/framework/audit"
	"github.com/yzxingtu2026/ainp/platform/framework/auth"
	"github.com/yzxingtu2026/ainp/platform/framework/config"
	"github.com/yzxingtu2026/ainp/platform/framework/database"
	"github.com/yzxingtu2026/ainp/platform/framework/i18n"
	"github.com/yzxingtu2026/ainp/platform/framework/job"
	"github.com/yzxingtu2026/ainp/platform/framework/logging"
	"github.com/yzxingtu2026/ainp/platform/framework/outbox"
	"github.com/yzxingtu2026/ainp/platform/framework/prompt"
	"github.com/yzxingtu2026/ainp/platform/framework/state"
	"github.com/yzxingtu2026/ainp/platform/framework/storage"
	tenantdata "github.com/yzxingtu2026/ainp/platform/framework/tenant"
	builtini18n "github.com/yzxingtu2026/ainp/platform/i18n"
	"github.com/yzxingtu2026/ainp/platform/modules/ai"
	"github.com/yzxingtu2026/ainp/platform/modules/demo"
	"github.com/yzxingtu2026/ainp/platform/modules/operations"
	"github.com/yzxingtu2026/ainp/platform/modules/system"
	builtinprompts "github.com/yzxingtu2026/ainp/platform/prompts"
	"gorm.io/gorm"
)

// Platform is the shared assembled runtime for HTTP server and Worker.
type Platform struct {
	Config        *config.Config
	DB            *gorm.DB
	App           *app.App
	RuntimeAssets []RuntimeAssetDescriptor
	close         sync.Once
	err           error
}

// RuntimeAssetDescriptor is safe diagnostic metadata without resource content.
type RuntimeAssetDescriptor struct {
	Owner   string
	Key     string
	Type    string
	Digest  string
	Version string
	Locale  string
}

// Close releases all resources owned by the assembled platform. It is safe to
// call more than once during partial startup cleanup.
func (p *Platform) Close() error {
	if p == nil {
		return nil
	}
	p.close.Do(func() {
		var closeErrors []error
		if p.App != nil && p.App.Outbox != nil {
			p.App.Outbox.Stop()
		}
		if p.App != nil && p.App.Jobs != nil {
			closeErrors = append(closeErrors, p.App.Jobs.Close())
		}
		if p.DB != nil {
			sqlDB, err := p.DB.DB()
			if err != nil {
				closeErrors = append(closeErrors, err)
			} else {
				closeErrors = append(closeErrors, sqlDB.Close())
			}
		}
		p.err = errors.Join(closeErrors...)
	})
	return p.err
}

// LoadPrompts loads and validates all runtime model prompts.
func LoadPrompts() error {
	if err := prompt.LoadSources(prompt.Source{Owner: "ainp", FS: builtinprompts.FS()}); err != nil {
		return fmt.Errorf("prompt load: %w", err)
	}
	for _, key := range requiredPromptKeys() {
		if prompt.Get(key) == "" {
			return fmt.Errorf("prompt load: required prompt %q is missing", key)
		}
	}
	return nil
}

// LoadI18n loads AINP's embedded baseline and applies the configured language.
func LoadI18n(cfg *config.Config) {
	if err := i18n.LoadSources(i18n.Source{Owner: "ainp", FS: builtini18n.FS()}); err != nil {
		logging.Error(context.Background(), "i18n load failed", err)
	}
	if cfg.I18n.DefaultLanguage != "" {
		i18n.SetDefaultLang(cfg.I18n.DefaultLanguage)
	}
}

func (c *Composition) loadRuntimeAssets(cfg *config.Config) ([]RuntimeAssetDescriptor, error) {
	loadPrompts := prompt.LoadSources
	loadI18n := i18n.LoadSources
	if c.reloadableAssets {
		loadPrompts = prompt.LoadReloadableSources
		loadI18n = i18n.LoadReloadableSources
	}
	if err := loadPrompts(c.promptSources...); err != nil {
		return nil, fmt.Errorf("runtime assets: %w", err)
	}
	for _, key := range requiredPromptKeys() {
		if prompt.Get(key) == "" {
			return nil, fmt.Errorf("runtime assets: required Prompt %q owned by %q is missing", key, "ainp")
		}
	}
	if err := loadI18n(c.i18nSources...); err != nil {
		return nil, fmt.Errorf("runtime assets: %w", err)
	}
	languages := i18n.AvailableLangs()
	if !slices.Contains(languages, i18n.LangZhCN) || !slices.Contains(languages, i18n.LangEn) || i18n.KeyCount() == 0 {
		return nil, fmt.Errorf("runtime assets: built-in i18n owned by %q is empty", "ainp")
	}
	if cfg.I18n.DefaultLanguage != "" {
		i18n.SetDefaultLang(cfg.I18n.DefaultLanguage)
	}
	manifest := make([]RuntimeAssetDescriptor, 0, len(prompt.Manifest())+len(i18n.Manifest()))
	for _, asset := range prompt.Manifest() {
		manifest = append(manifest, RuntimeAssetDescriptor{Owner: asset.Owner, Key: asset.Key, Type: asset.Type, Digest: asset.Digest, Version: asset.Version})
	}
	for _, asset := range i18n.Manifest() {
		manifest = append(manifest, RuntimeAssetDescriptor{Owner: asset.Owner, Key: asset.Key, Type: asset.Type, Digest: asset.Digest, Locale: asset.Locale})
	}
	return manifest, nil
}

func requiredPromptKeys() []string {
	return []string{
		"ai/memory-extraction",
		"ai/memory-consolidation",
		"ai/context-compaction",
		"ai/trusted-runtime-context",
		"ai/page-context",
		"ai/untrusted-user-memory",
		"ai/assistant-personalization",
		"agent/system-operations",
	}
}

// OpenInstalled opens DB, migrates, seeds, and registers modules for an installed instance.
func OpenInstalled(ctx context.Context, cfg *config.Config, options ...Option) (*Platform, error) {
	composition, err := New(options...)
	if err != nil {
		return nil, err
	}
	return composition.OpenInstalled(ctx, cfg)
}

// OpenAPI loads installation state and assembles either the setup API or the
// installed API through one public lifecycle entrypoint.
func OpenAPI(ctx context.Context, cfg *config.Config, options ...Option) (*Platform, error) {
	composition, err := New(options...)
	if err != nil {
		return nil, err
	}
	return composition.OpenAPI(ctx, cfg)
}

func (c *Composition) OpenAPI(ctx context.Context, cfg *config.Config) (*Platform, error) {
	st, err := state.Load(cfg.StateFile)
	if err != nil {
		return nil, fmt.Errorf("load state: %w", err)
	}
	if st.Installed {
		st.ApplyConfig(cfg)
		return c.OpenInstalled(ctx, cfg)
	}
	return c.openSetup(ctx, cfg, st)
}

func (c *Composition) openSetup(ctx context.Context, cfg *config.Config, st *state.State) (*Platform, error) {
	manifest, err := c.loadRuntimeAssets(cfg)
	if err != nil {
		return nil, err
	}
	tokens, err := auth.NewTokenService(cfg.Auth.JWT.Secret, cfg.Auth.JWT.Issuer, cfg.Auth.JWT.Duration())
	if err != nil {
		return nil, fmt.Errorf("token service: %w", err)
	}
	application := app.New(cfg, nil, nil, tokens)
	application.Installed = false
	setupSvc := system.NewSetupService(cfg, cfg.StateFile, st, func() { application.Installed = true })
	contracts, err := ai.NewBuiltinAgentContractRegistry()
	if err != nil {
		return nil, fmt.Errorf("register builtin agent contracts: %w", err)
	}
	agents := ai.NewAgentService(nil, contracts, nil, nil)
	setupSvc.SetInitialDataHook(func(ctx context.Context, db *gorm.DB, tenantID, principalID string) error {
		if err := audit.NewGormRecorder(db).AutoMigrate(); err != nil {
			return err
		}
		if err := ai.AutoMigrate(db); err != nil {
			return err
		}
		if err := c.autoMigrate(db); err != nil {
			return err
		}
		if err := agents.ReconcileBuiltinForTenant(ctx, db, tenantID, principalID); err != nil {
			return err
		}
		_, err := c.migrations.Baseline(db)
		return err
	})
	if err := system.RegisterSetupActions(application, setupSvc); err != nil {
		return nil, fmt.Errorf("register setup: %w", err)
	}
	return &Platform{Config: cfg, App: application, RuntimeAssets: manifest}, nil
}

// OpenInstalled assembles an installed API/Worker runtime from one immutable
// module declaration.
func (c *Composition) OpenInstalled(ctx context.Context, cfg *config.Config) (*Platform, error) {
	if err := cfg.Validate(true); err != nil {
		return nil, err
	}
	manifest, err := c.loadRuntimeAssets(cfg)
	if err != nil {
		return nil, err
	}

	db, err := database.Open(cfg.Database.DSN)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}
	if err := database.Ping(db); err != nil {
		return nil, fmt.Errorf("ping database: %w", err)
	}

	auditor := audit.NewGormRecorder(db)
	if cfg.Database.IsAutoMigrateEnabled() {
		if err := ensureIdentityContractSchema(db); err != nil {
			return nil, fmt.Errorf("schema preflight: %w", err)
		}
		if err := auditor.AutoMigrate(); err != nil {
			return nil, fmt.Errorf("audit migrate: %w", err)
		}
		if err := system.AutoMigrate(db); err != nil {
			return nil, fmt.Errorf("system migrate: %w", err)
		}
		if err := ai.AutoMigrate(db); err != nil {
			return nil, fmt.Errorf("ai migrate: %w", err)
		}
		if err := c.autoMigrate(db); err != nil {
			return nil, err
		}
	}
	if c.demoEnabled {
		if err := system.SeedDemo(ctx, db); err != nil {
			return nil, fmt.Errorf("system demo seed: %w", err)
		}
	}

	tokens, err := auth.NewTokenService(cfg.Auth.JWT.Secret, cfg.Auth.JWT.Issuer, cfg.Auth.JWT.Duration())
	if err != nil {
		return nil, fmt.Errorf("token service: %w", err)
	}

	application := app.New(cfg, db, auditor, tokens)

	store, err := storage.NewFromConfig(storage.Config{
		Provider: cfg.Storage.Provider,
		Local:    storage.LocalConfig{RootDir: cfg.Storage.Local.RootDir},
	}, nil)
	if err != nil {
		return nil, fmt.Errorf("object store: %w", err)
	}
	application.Storage = store

	transferTokens, err := attachment.NewTransferTokenService(cfg.Auth.JWT.Secret, cfg.Auth.JWT.Issuer)
	if err != nil {
		return nil, fmt.Errorf("transfer tokens: %w", err)
	}
	application.TransferTokens = transferTokens

	outboxRT, err := outbox.NewRuntime(db, cfg.Outbox)
	if err != nil {
		return nil, fmt.Errorf("outbox: %w", err)
	}
	application.Outbox = outboxRT

	var transport job.Transport
	if cfg.Jobs.IsEnabled() {
		if cfg.Redis.Addr == "" {
			return nil, fmt.Errorf("jobs: redis.addr is required when jobs.enabled=true")
		}
		transport = asynqadapter.NewClientTransport(asynqadapter.RedisOpt{
			Addr:         cfg.Redis.Addr,
			Username:     cfg.Redis.Username,
			Password:     cfg.Redis.Password,
			DB:           cfg.Redis.DB,
			DialTimeout:  cfg.Redis.DialTimeoutDuration(),
			ReadTimeout:  cfg.Redis.ReadTimeoutDuration(),
			WriteTimeout: cfg.Redis.WriteTimeoutDuration(),
			PoolSize:     cfg.Redis.PoolSize,
		})
	}
	jobRT, err := job.NewRuntime(db, job.RuntimeOptions{
		Enabled:    cfg.Jobs.IsEnabled(),
		MaxPayload: cfg.Jobs.MaxPayloadBytes,
		Transport:  transport,
	})
	if err != nil {
		if transport != nil {
			_ = transport.Close()
		}
		return nil, fmt.Errorf("jobs: %w", err)
	}
	application.Jobs = jobRT
	if err := c.prepare(application); err != nil {
		_ = jobRT.Close()
		return nil, err
	}

	st, _ := state.Load(cfg.StateFile)
	if st == nil {
		st = &state.State{Installed: true}
	}
	setupSvc := system.NewSetupService(cfg, cfg.StateFile, st, nil)
	if err := system.RegisterSetupActions(application, setupSvc); err != nil {
		return nil, fmt.Errorf("register setup: %w", err)
	}
	if err := system.Register(application); err != nil {
		return nil, fmt.Errorf("register system: %w", err)
	}
	if err := operations.Register(application, system.NewOperationsNotificationDeliveryService(application)); err != nil {
		return nil, fmt.Errorf("register operations: %w", err)
	}
	if err := ai.Register(application); err != nil {
		return nil, fmt.Errorf("register ai: %w", err)
	}
	setupSvc.SetInitialDataHook(application.AgentTenantInitializer)
	if c.demoEnabled {
		if err := demo.Register(application); err != nil {
			return nil, fmt.Errorf("register demo: %w", err)
		}
	}
	if err := c.register(application); err != nil {
		return nil, err
	}
	if err := validateAgentToolCapabilityCoverage(application); err != nil {
		return nil, err
	}
	if err := application.PageHelp.ValidateMenus(application.Menus); err != nil {
		return nil, fmt.Errorf("validate page help: %w", err)
	}
	if err := application.PageHelp.ValidateAllModulesCoverage(application.Menus, "zh-CN"); err != nil {
		return nil, fmt.Errorf("validate page help coverage: %w", err)
	}
	if err := reconcileToolCatalogs(ctx, db, application); err != nil {
		return nil, fmt.Errorf("reconcile tool catalogs: %w", err)
	}
	if err := reconcileNavigationCatalogs(ctx, db, application); err != nil {
		return nil, fmt.Errorf("reconcile navigation catalogs: %w", err)
	}

	return &Platform{Config: cfg, DB: db, App: application, RuntimeAssets: manifest}, nil
}

func validateAgentToolCapabilityCoverage(application *app.App) error {
	if err := application.AgentContracts.ValidateActionToolCapabilityCoverage(application.Actions, nil); err != nil {
		return fmt.Errorf("validate AI tool capability coverage: %w", err)
	}
	return nil
}

func reconcileNavigationCatalogs(ctx context.Context, db *gorm.DB, application *app.App) error {
	if application.NavigationInitializer == nil {
		return nil
	}
	systemCtx := auth.WithContext(ctx, &auth.Context{Authenticated: true, Realm: auth.RealmPlatform, IsSystemOperator: true, PrincipalID: "system:navigation-catalog", Client: "bootstrap"})
	sdb, err := tenantdata.SystemDB(systemCtx, db)
	if err != nil {
		return err
	}
	tenants, err := tenantdata.SystemRaw[tenantdata.Tenant](sdb, "SELECT * FROM tenants WHERE status IN (?, ?)", "enabled", "disabled")
	if err != nil {
		return err
	}
	for _, tenant := range tenants {
		if err := application.NavigationInitializer(ctx, tenant.ID, "system:navigation-catalog"); err != nil {
			return fmt.Errorf("tenant %s: %w", tenant.ID, err)
		}
	}
	return nil
}

func reconcileToolCatalogs(ctx context.Context, db *gorm.DB, application *app.App) error {
	if application.ToolCatalogInitializer == nil {
		return nil
	}
	systemCtx := auth.WithContext(ctx, &auth.Context{Authenticated: true, Realm: auth.RealmPlatform, IsSystemOperator: true, PrincipalID: "system:tool-catalog", Client: "bootstrap"})
	sdb, err := tenantdata.SystemDB(systemCtx, db)
	if err != nil {
		return err
	}
	tenants, err := tenantdata.SystemRaw[tenantdata.Tenant](sdb, "SELECT * FROM tenants WHERE status IN (?, ?)", "enabled", "disabled")
	if err != nil {
		return err
	}
	for _, tenant := range tenants {
		if err := application.ToolCatalogInitializer(ctx, tenant.ID, "system:tool-catalog"); err != nil {
			return fmt.Errorf("tenant %s: %w", tenant.ID, err)
		}
	}
	return nil
}
