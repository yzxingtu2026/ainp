package job

import (
	"context"
	"fmt"
	"time"

	"github.com/yzxingtu2026/ainp/platform/framework/logging"
	"gorm.io/gorm"
)

// Runtime wires job registry, store, enqueuer and optional Transport.
type Runtime struct {
	Registry  *Registry
	Store     RunStore
	Enqueuer  Enqueuer
	Transport Transport
	Enabled   bool
	db        *gorm.DB
	enqueuer  *enqueuer
}

// RuntimeOptions configures the job runtime.
type RuntimeOptions struct {
	Enabled    bool
	MaxPayload int
	Transport  Transport // required when Enabled
}

func NewRuntime(db *gorm.DB, opts RuntimeOptions) (*Runtime, error) {
	reg := NewRegistry()
	store := NewRunStore(db)
	rt := &Runtime{
		Registry: reg,
		Store:    store,
		Enabled:  opts.Enabled,
		db:       db,
	}
	if !opts.Enabled {
		rt.Enqueuer = NewEnqueuer(db, reg, store, nil, false, opts.MaxPayload)
		return rt, nil
	}
	if opts.Transport == nil {
		return nil, fmt.Errorf("job: transport is required when enabled")
	}
	if err := opts.Transport.Ping(context.Background()); err != nil {
		return nil, fmt.Errorf("job: transport ping failed: %w", err)
	}
	eq := &enqueuer{
		db: db, registry: reg, store: store, transport: opts.Transport,
		enabled: true, maxPayload: opts.MaxPayload,
	}
	if eq.maxPayload <= 0 {
		eq.maxPayload = 256 * 1024
	}
	rt.Transport = opts.Transport
	rt.Enqueuer = eq
	rt.enqueuer = eq
	return rt, nil
}

func (rt *Runtime) Register(def Definition, h Handler) error {
	return rt.Registry.Register(def, h)
}

func (rt *Runtime) Close() error {
	if rt.Transport != nil {
		return rt.Transport.Close()
	}
	return nil
}

func (rt *Runtime) StartRecovery(ctx context.Context, interval time.Duration) {
	if rt == nil || !rt.Enabled || rt.enqueuer == nil {
		return
	}
	if interval <= 0 {
		interval = 30 * time.Second
	}
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				n, err := RecoverPendingEnqueue(ctx, rt.enqueuer, 50)
				if err != nil {
					logging.Error(ctx, "job pending_enqueue recovery failed", err)
				} else if n > 0 {
					logging.Info(ctx, "job pending_enqueue recovered", "count", n)
				}
			}
		}
	}()
}
