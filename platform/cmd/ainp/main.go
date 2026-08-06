package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/yzxingtu2026/ainp/platform/framework/buildinfo"
	"github.com/yzxingtu2026/ainp/platform/framework/config"
	"github.com/yzxingtu2026/ainp/platform/framework/logging"
	"github.com/yzxingtu2026/ainp/platform/host"
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

	info := buildinfo.Current()
	logging.Info(ctx, "starting ainp", "addr", cfg.Server.Addr,
		"version", info.Version, "commit", info.Commit, "build_time", info.BuildTime)
	p, err := host.OpenAPI(ctx, cfg, host.WithDemo())
	if err != nil {
		logging.Error(ctx, "bootstrap failed", err)
		os.Exit(1)
	}
	defer p.Close()

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
