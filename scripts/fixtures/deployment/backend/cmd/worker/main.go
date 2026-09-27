package main

import (
	"context"
	"encoding/json"
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"

	"example.com/ainp-downstream/composition"
	"github.com/yzxingtu2026/ainp/platform/framework/buildinfo"
	"github.com/yzxingtu2026/ainp/platform/framework/config"
	"github.com/yzxingtu2026/ainp/platform/framework/job"
	"github.com/yzxingtu2026/ainp/platform/host"
)

func main() {
	version := flag.Bool("version", false, "print immutable build identity")
	path := flag.String("config", "configs/config.yaml", "application config")
	flag.Parse()
	if *version {
		_ = json.NewEncoder(os.Stdout).Encode(buildinfo.Current())
		return
	}
	cfg, err := config.Load(*path)
	if err != nil {
		log.Fatal(err)
	}
	profile, err := job.ResolveWorkerProfile(job.WorkerProfile{Queues: cfg.Jobs.Queues, Concurrency: cfg.Jobs.Concurrency}, job.WorkerProfileOverrides{})
	if err != nil {
		log.Fatal(err)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	worker, err := host.OpenWorker(ctx, cfg, profile, host.WithModules(composition.Modules...))
	if err != nil {
		log.Fatal(err)
	}
	defer worker.Close()
	if err := worker.Run(ctx); err != nil {
		log.Fatal(err)
	}
}
