package host

import (
	"context"
	"fmt"

	"github.com/yzxingtu2026/ainp/platform/framework/asynqadapter"
	"github.com/yzxingtu2026/ainp/platform/framework/config"
	"github.com/yzxingtu2026/ainp/platform/framework/job"
	"github.com/yzxingtu2026/ainp/platform/framework/state"
)

// WorkerHost owns the assembled platform and its queue runner lifecycle.
type WorkerHost struct {
	Platform *Platform
	runner   *asynqadapter.Runner
}

func OpenWorker(ctx context.Context, cfg *config.Config, profile job.WorkerProfile, options ...Option) (*WorkerHost, error) {
	composition, err := New(options...)
	if err != nil {
		return nil, err
	}
	return composition.OpenWorker(ctx, cfg, profile)
}

func (c *Composition) OpenWorker(ctx context.Context, cfg *config.Config, profile job.WorkerProfile) (*WorkerHost, error) {
	st, err := state.Load(cfg.StateFile)
	if err != nil {
		return nil, fmt.Errorf("load state: %w", err)
	}
	if !st.Installed {
		return nil, fmt.Errorf("worker requires installed platform")
	}
	st.ApplyConfig(cfg)
	if !cfg.Jobs.IsEnabled() {
		return nil, fmt.Errorf("worker requires jobs.enabled=true")
	}
	platform, err := c.OpenInstalled(ctx, cfg)
	if err != nil {
		return nil, err
	}
	runner := asynqadapter.NewRunner(
		asynqadapter.RedisOptFromConfig(cfg.Redis),
		asynqadapter.WorkerConfigFromProfile(profile),
		platform.DB,
		platform.App.Jobs.Registry,
		platform.App.Jobs.Store,
	)
	return &WorkerHost{Platform: platform, runner: runner}, nil
}

func (h *WorkerHost) Run(ctx context.Context) error {
	if h == nil || h.Platform == nil || h.runner == nil {
		return fmt.Errorf("worker host is not initialized")
	}
	if h.Platform.App.Outbox != nil {
		h.Platform.App.Outbox.Start(ctx)
	}
	if h.Platform.App.Jobs != nil {
		h.Platform.App.Jobs.StartRecovery(ctx, h.Platform.Config.Jobs.RecoveryIntervalDuration())
	}
	h.runner.Start(ctx)
	<-ctx.Done()
	return h.runner.Shutdown(h.Platform.Config.Jobs.ShutdownTimeoutDuration())
}

func (h *WorkerHost) Close() error {
	if h == nil {
		return nil
	}
	return h.Platform.Close()
}
