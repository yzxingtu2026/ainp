package boot

import (
	"context"
	"fmt"

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
	"github.com/yzxingtu2026/ainp/platform/framework/state"
	"github.com/yzxingtu2026/ainp/platform/framework/storage"
	"github.com/yzxingtu2026/ainp/platform/modules/ai"
	"github.com/yzxingtu2026/ainp/platform/modules/demo"
	"github.com/yzxingtu2026/ainp/platform/modules/operations"
	"github.com/yzxingtu2026/ainp/platform/modules/system"
	"gorm.io/gorm"
)

// Platform is the shared assembled runtime for HTTP server and Worker.
type Platform struct {
	Config *config.Config
	DB     *gorm.DB
	App    *app.App
}

// LoadI18n loads translation files from config.
func LoadI18n(cfg *config.Config) {
	dir := cfg.I18n.Dir
	if dir == "" {
		dir = "i18n"
	}
	if err := i18n.LoadDir(dir); err != nil {
		logging.Error(context.Background(), "i18n load failed", err)
	}
	if cfg.I18n.DefaultLanguage != "" {
		i18n.SetDefaultLang(cfg.I18n.DefaultLanguage)
	}
}

// OpenInstalled opens DB, migrates, seeds, and registers modules for an installed instance.
func OpenInstalled(ctx context.Context, cfg *config.Config) (*Platform, error) {
	if err := cfg.Validate(true); err != nil {
		return nil, err
	}
	LoadI18n(cfg)

	db, err := database.Open(cfg.Database.DSN)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}
	if err := database.Ping(db); err != nil {
		return nil, fmt.Errorf("ping database: %w", err)
	}

	auditor := audit.NewGormRecorder(db)
	if err := auditor.AutoMigrate(); err != nil {
		return nil, fmt.Errorf("audit migrate: %w", err)
	}
	if err := system.AutoMigrate(db); err != nil {
		return nil, fmt.Errorf("system migrate: %w", err)
	}
	if err := ai.AutoMigrate(db); err != nil {
		return nil, fmt.Errorf("ai migrate: %w", err)
	}
	if err := system.SeedDemo(ctx, db); err != nil {
		return nil, fmt.Errorf("system seed: %w", err)
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

	st, _ := state.Load(cfg.StateFile)
	if st == nil {
		st = &state.State{Installed: true}
	}
	setupSvc := system.NewSetupService(cfg.Database.DSN, cfg.StateFile, st, nil)
	if err := system.RegisterSetupActions(application, setupSvc); err != nil {
		return nil, fmt.Errorf("register setup: %w", err)
	}
	if err := system.Register(application); err != nil {
		return nil, fmt.Errorf("register system: %w", err)
	}
	if err := ai.Register(application); err != nil {
		return nil, fmt.Errorf("register ai: %w", err)
	}
	if err := operations.Register(application, system.NewOperationsNotificationDeliveryService(application)); err != nil {
		return nil, fmt.Errorf("register operations: %w", err)
	}
	if err := demo.Register(application); err != nil {
		return nil, fmt.Errorf("register demo: %w", err)
	}

	return &Platform{Config: cfg, DB: db, App: application}, nil
}
