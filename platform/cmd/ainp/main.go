package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/yzxingtu2026/ainp/platform/framework/app"
	"github.com/yzxingtu2026/ainp/platform/framework/audit"
	"github.com/yzxingtu2026/ainp/platform/framework/auth"
	"github.com/yzxingtu2026/ainp/platform/framework/buildinfo"
	"github.com/yzxingtu2026/ainp/platform/framework/config"
	"github.com/yzxingtu2026/ainp/platform/framework/logging"
	"github.com/yzxingtu2026/ainp/platform/framework/state"
	"github.com/yzxingtu2026/ainp/platform/internal/boot"
	"github.com/yzxingtu2026/ainp/platform/migrations"
	"github.com/yzxingtu2026/ainp/platform/modules/ai"
	"github.com/yzxingtu2026/ainp/platform/modules/system"
	"gorm.io/gorm"
)

func main() {
	cfgPath := flag.String("config", "configs/config.yaml", "path to yaml config")
	flag.Parse()

	ctx := context.Background()
	cfg, err := config.Load(*cfgPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "load config: %v\n", err)
		os.Exit(1)
	}
	if cfg.Logging.Service == "" {
		cfg.Logging.Service = "api"
	}
	logCloser := logging.InitConfig(logging.Config{Level: cfg.Logging.Level, Format: cfg.Logging.Format, Outputs: cfg.Logging.Outputs, Service: cfg.Logging.Service, InstanceID: cfg.Logging.InstanceID, File: logging.FileConfig{Path: cfg.Logging.File.Path, MaxSizeMB: cfg.Logging.File.MaxSizeMB, MaxBackups: cfg.Logging.File.MaxBackups, MaxAgeDays: cfg.Logging.File.MaxAgeDays, Compress: cfg.Logging.File.Compress}})
	defer logCloser.Close()

	st, err := state.Load(cfg.StateFile)
	if err != nil {
		logging.Error(ctx, "load state failed", err)
		os.Exit(1)
	}
	if !st.Installed {
		runSetupMode(ctx, cfg, st)
		return
	}
	st.ApplyConfig(cfg)

	info := buildinfo.Current()
	logging.Info(ctx, "starting ainp", "addr", cfg.Server.Addr,
		"version", info.Version, "commit", info.Commit, "build_time", info.BuildTime)
	p, err := boot.OpenInstalled(ctx, cfg)
	if err != nil {
		logging.Error(ctx, "bootstrap failed", err)
		os.Exit(1)
	}
	defer func() {
		if p.App.Jobs != nil {
			_ = p.App.Jobs.Close()
		}
	}()

	logging.Info(ctx, "ainp is ready",
		"actions", len(p.App.Actions.List()),
		"menus", len(p.App.Menus.List()),
		"jobs_enabled", cfg.Jobs.IsEnabled(),
		"embedded_worker", cfg.Jobs.IsEnabled() && cfg.Jobs.EmbeddedWorker,
		"dev_token", cfg.Auth.DevTokenEnabled,
	)
	if err := p.App.Run(); err != nil {
		logging.Error(ctx, "server stopped", err)
		os.Exit(1)
	}
}

func runSetupMode(ctx context.Context, cfg *config.Config, st *state.State) {
	logging.Info(ctx, "starting in setup mode", "addr", cfg.Server.Addr)
	if err := boot.LoadPrompts(); err != nil {
		logging.Error(ctx, "load prompts failed", err)
		os.Exit(1)
	}
	boot.LoadI18n(cfg)
	tokens, err := auth.NewTokenService(cfg.Auth.JWT.Secret, cfg.Auth.JWT.Issuer, cfg.Auth.JWT.Duration())
	if err != nil {
		logging.Error(ctx, "init token service failed", err)
		os.Exit(1)
	}
	application := app.New(cfg, nil, nil, tokens)
	application.Installed = false
	setupSvc := system.NewSetupService(cfg, cfg.StateFile, st, func() {
		application.Installed = true
	})
	contracts, err := ai.NewBuiltinAgentContractRegistry()
	if err != nil {
		logging.Error(ctx, "register builtin agent contracts failed", err)
		os.Exit(1)
	}
	agents := ai.NewAgentService(nil, contracts, nil, nil)
	setupSvc.SetInitialDataHook(func(ctx context.Context, db *gorm.DB, tenantID, userID string) error {
		if err := audit.NewGormRecorder(db).AutoMigrate(); err != nil {
			return err
		}
		if err := ai.AutoMigrate(db); err != nil {
			return err
		}
		if err := agents.ReconcileBuiltinForTenant(ctx, db, tenantID, userID); err != nil {
			return err
		}
		_, err := migrations.Baseline(db)
		return err
	})
	if err := system.RegisterSetupActions(application, setupSvc); err != nil {
		logging.Error(ctx, "register setup failed", err)
		os.Exit(1)
	}
	logging.Info(ctx, "ainp setup wizard is ready", "actions", len(application.Actions.List()))
	if err := application.Run(); err != nil {
		logging.Error(ctx, "server stopped", err)
		os.Exit(1)
	}
}
