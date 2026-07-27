package asynqadapter

import (
	"context"
	"time"

	"github.com/hibiken/asynq"
	"github.com/yzxingtu2026/ainp/platform/framework/config"
	"github.com/yzxingtu2026/ainp/platform/framework/job"
	"github.com/yzxingtu2026/ainp/platform/framework/logging"
	"gorm.io/gorm"
)

// RedisOptFromConfig maps deployment Redis config into adapter options.
func RedisOptFromConfig(cfg config.RedisConfig) RedisOpt {
	return RedisOpt{
		Addr:         cfg.Addr,
		Username:     cfg.Username,
		Password:     cfg.Password,
		DB:           cfg.DB,
		DialTimeout:  cfg.DialTimeoutDuration(),
		ReadTimeout:  cfg.ReadTimeoutDuration(),
		WriteTimeout: cfg.WriteTimeoutDuration(),
		PoolSize:     cfg.PoolSize,
	}
}

// WorkerConfigFromJobs maps JobsConfig into Asynq worker settings.
func WorkerConfigFromJobs(cfg config.JobsConfig) WorkerConfig {
	return WorkerConfig{
		Concurrency: cfg.Concurrency,
		Queues:      cfg.Queues,
	}
}

func WorkerConfigFromProfile(profile job.WorkerProfile) WorkerConfig {
	return WorkerConfig{Concurrency: profile.Concurrency, Queues: profile.Queues}
}

// Runner owns an Asynq server lifecycle for standalone or embedded workers.
type Runner struct {
	server *asynq.Server
	mux    *asynq.ServeMux
	done   chan error
	config WorkerConfig
}

// NewRunner builds a worker runner (does not start yet).
func NewRunner(redis RedisOpt, wcfg WorkerConfig, db *gorm.DB, registry *job.Registry, store job.RunStore) *Runner {
	return &Runner{
		server: NewServer(redis, wcfg, db, registry, store),
		mux:    NewMux(db, registry, store),
		done:   make(chan error, 1),
		config: wcfg,
	}
}

// Start runs the Asynq server in a background goroutine.
func (r *Runner) Start(ctx context.Context) {
	go func() {
		logging.Info(ctx, "asynq worker starting", "queues", job.FormatWorkerQueues(r.config.Queues), "concurrency", r.config.Concurrency)
		r.done <- r.server.Run(r.mux)
	}()
}

// Shutdown stops accepting tasks and waits up to timeout for in-flight work.
func (r *Runner) Shutdown(timeout time.Duration) error {
	if r == nil || r.server == nil {
		return nil
	}
	logging.Info(context.Background(), "asynq worker shutting down")
	r.server.Shutdown()
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	select {
	case err := <-r.done:
		return err
	case <-time.After(timeout):
		logging.Warn(context.Background(), "asynq worker shutdown timed out")
		return nil
	}
}
