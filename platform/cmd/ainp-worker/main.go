package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/yzxingtu2026/ainp/platform/framework/asynqadapter"
	"github.com/yzxingtu2026/ainp/platform/framework/buildinfo"
	"github.com/yzxingtu2026/ainp/platform/framework/config"
	"github.com/yzxingtu2026/ainp/platform/framework/logging"
	"github.com/yzxingtu2026/ainp/platform/framework/state"
	"github.com/yzxingtu2026/ainp/platform/internal/boot"
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
		logging.Error(ctx, "worker requires installed platform", fmt.Errorf("not installed"))
		os.Exit(1)
	}
	st.ApplyConfig(cfg)
	if !cfg.Jobs.IsEnabled() {
		logging.Error(ctx, "jobs disabled", fmt.Errorf("set jobs.enabled=true"))
		os.Exit(1)
	}

	p, err := boot.OpenInstalled(ctx, cfg)
	if err != nil {
		logging.Error(ctx, "bootstrap failed", err)
		os.Exit(1)
	}
	defer func() {
		if p.App.Jobs != nil {
			_ = p.App.Jobs.Close()
		}
		if p.App.Outbox != nil {
			p.App.Outbox.Stop()
		}
	}()

	runCtx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()

	if p.App.Outbox != nil {
		p.App.Outbox.Start(runCtx)
	}
	if p.App.Jobs != nil {
		p.App.Jobs.StartRecovery(runCtx, cfg.Jobs.RecoveryIntervalDuration())
	}

	runner := asynqadapter.NewRunner(
		asynqadapter.RedisOptFromConfig(cfg.Redis),
		asynqadapter.WorkerConfigFromJobs(cfg.Jobs),
		p.DB,
		p.App.Jobs.Registry,
		p.App.Jobs.Store,
	)

	info := buildinfo.Current()
	logging.Info(runCtx, "ainp-worker starting",
		"jobs", len(p.App.Jobs.Registry.List()),
		"concurrency", cfg.Jobs.Concurrency,
		"version", info.Version,
		"commit", info.Commit,
		"build_time", info.BuildTime,
	)
	runner.Start(runCtx)

	<-runCtx.Done()
	if err := runner.Shutdown(cfg.Jobs.ShutdownTimeoutDuration()); err != nil {
		logging.Error(ctx, "worker stopped", err)
		os.Exit(1)
	}
}
