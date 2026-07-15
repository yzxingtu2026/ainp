package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/yzxingtu2026/ainp/platform/framework/app"
	"github.com/yzxingtu2026/ainp/platform/framework/auth"
	"github.com/yzxingtu2026/ainp/platform/framework/config"
	"github.com/yzxingtu2026/ainp/platform/framework/logging"
	"github.com/yzxingtu2026/ainp/platform/framework/state"
	"github.com/yzxingtu2026/ainp/platform/internal/boot"
	"github.com/yzxingtu2026/ainp/platform/modules/system"
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
	logging.Init(cfg.Logging.Level, cfg.Logging.Format)

	st, err := state.Load(cfg.StateFile)
	if err != nil {
		logging.Error(ctx, "load state failed", err)
		os.Exit(1)
	}
	if !st.Installed {
		runSetupMode(ctx, cfg, st)
		return
	}
	if cfg.Database.DSN == "" && st.DSN != "" {
		cfg.Database.DSN = st.DSN
	}

	logging.Info(ctx, "starting ainp", "addr", cfg.Server.Addr)
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

	if p.App.Jobs != nil && p.App.Jobs.Enabled {
		p.App.Jobs.StartRecovery(ctx, cfg.Jobs.RecoveryIntervalDuration())
	}

	logging.Info(ctx, "ainp is ready",
		"actions", len(p.App.Actions.List()),
		"menus", len(p.App.Menus.List()),
		"jobs_enabled", cfg.Jobs.IsEnabled(),
		"dev_token", cfg.Auth.DevTokenEnabled,
	)
	if err := p.App.Run(); err != nil {
		logging.Error(ctx, "server stopped", err)
		os.Exit(1)
	}
}

func runSetupMode(ctx context.Context, cfg *config.Config, st *state.State) {
	logging.Info(ctx, "starting in setup mode", "addr", cfg.Server.Addr)
	boot.LoadI18n(cfg)
	tokens, err := auth.NewTokenService(cfg.Auth.JWT.Secret, cfg.Auth.JWT.Issuer, cfg.Auth.JWT.Duration())
	if err != nil {
		logging.Error(ctx, "init token service failed", err)
		os.Exit(1)
	}
	application := app.New(cfg, nil, nil, tokens)
	application.Installed = false
	setupSvc := system.NewSetupService(cfg.Database.DSN, cfg.StateFile, st, func() {
		application.Installed = true
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
