package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/yzxingtu2026/ainp/platform/framework/buildinfo"
	"github.com/yzxingtu2026/ainp/platform/framework/config"
	"github.com/yzxingtu2026/ainp/platform/framework/job"
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
		cfg.Logging.Service = "worker"
	}
	if os.Getenv("LOG_FILE_PATH") == "" {
		cfg.Logging.File.Path = logging.WorkerFilePath(cfg.Logging.File.Path)
	}
	logCloser := logging.InitConfig(logging.Config{Level: cfg.Logging.Level, Format: cfg.Logging.Format, Outputs: cfg.Logging.Outputs, Service: cfg.Logging.Service, InstanceID: cfg.Logging.InstanceID, File: logging.FileConfig{Path: cfg.Logging.File.Path, MaxSizeMB: cfg.Logging.File.MaxSizeMB, MaxBackups: cfg.Logging.File.MaxBackups, MaxAgeDays: cfg.Logging.File.MaxAgeDays, Compress: cfg.Logging.File.Compress}})
	defer logCloser.Close()

	profile, err := job.ResolveWorkerProfile(
		job.WorkerProfile{Queues: cfg.Jobs.Queues, Concurrency: cfg.Jobs.Concurrency},
		job.WorkerProfileOverrides{Queues: optionalEnv(job.WorkerQueuesEnv), Concurrency: optionalEnv(job.WorkerConcurrencyEnv)},
	)
	if err != nil {
		logging.Error(ctx, "invalid worker profile", err)
		os.Exit(1)
	}

	worker, err := host.OpenWorker(ctx, cfg, profile)
	if err != nil {
		logging.Error(ctx, "bootstrap failed", err)
		os.Exit(1)
	}
	defer worker.Close()

	runCtx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()

	info := buildinfo.Current()
	logging.Info(runCtx, "ainp-worker starting",
		"jobs", len(worker.Platform.App.Jobs.Registry.List()),
		"queues", job.FormatWorkerQueues(profile.Queues),
		"concurrency", profile.Concurrency,
		"service", cfg.Logging.Service,
		"instance_id", cfg.Logging.InstanceID,
		"version", info.Version,
		"commit", info.Commit,
		"build_time", info.BuildTime,
	)
	if err := worker.Run(runCtx); err != nil {
		logging.Error(ctx, "worker stopped", err)
		os.Exit(1)
	}
}

func optionalEnv(key string) *string {
	value, exists := os.LookupEnv(key)
	if !exists {
		return nil
	}
	return &value
}
