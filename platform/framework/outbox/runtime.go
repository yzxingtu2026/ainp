package outbox

import (
	"context"
	"fmt"

	"github.com/yzxingtu2026/ainp/platform/framework/config"
	"gorm.io/gorm"
)

// Runtime wires Writer, registries, DeliveryStore, Transport and Dispatcher.
type Runtime struct {
	Definitions *DefinitionRegistry
	Handlers    *HandlerRegistry
	Writer      Writer
	Deliveries  DeliveryStore
	Transport   Transport
	Dispatcher  *Dispatcher
	Enabled     bool
}

// NewRuntime builds the Phase-1 PostgreSQL/InProcess outbox stack.
func NewRuntime(db *gorm.DB, cfg config.OutboxConfig) (*Runtime, error) {
	defs := NewDefinitionRegistry()
	handlers := NewHandlerRegistry()
	writer := NewWriterWithNotifier(defs, NewPostgresCommitNotifier())
	deliveries := NewDeliveryStore(db)

	var transport Transport
	switch cfg.Transport {
	case "", "postgres", "inprocess":
		transport = NewInProcessTransport(handlers, deliveries)
	default:
		return nil, fmt.Errorf("outbox: unsupported transport %q (phase 1: postgres)", cfg.Transport)
	}

	dcfg := DispatcherConfig{
		Enabled:         cfg.IsEnabled(),
		BatchSize:       cfg.BatchSize,
		PollInterval:    cfg.PollIntervalDuration(),
		LockTimeout:     cfg.LockTimeoutDuration(),
		MaxAttempts:     cfg.MaxAttempts,
		ShutdownTimeout: cfg.ShutdownTimeoutDuration(),
		Backoff: BackoffConfig{
			Base: cfg.BackoffBaseDuration(),
			Max:  cfg.BackoffMaxDuration(),
		},
	}
	dispatcher := NewDispatcher(db, transport, dcfg)
	dispatcher.SetNotificationListener(NewPostgresNotificationListener(db))
	return &Runtime{
		Definitions: defs,
		Handlers:    handlers,
		Writer:      writer,
		Deliveries:  deliveries,
		Transport:   transport,
		Dispatcher:  dispatcher,
		Enabled:     cfg.IsEnabled(),
	}, nil
}

func (rt *Runtime) RegisterDefinition(def Definition) error {
	return rt.Definitions.Register(def)
}

func (rt *Runtime) RegisterHandler(h Handler) error {
	return rt.Handlers.Register(h)
}

func (rt *Runtime) Start(ctx context.Context) {
	if rt == nil || !rt.Enabled || rt.Dispatcher == nil {
		return
	}
	rt.Dispatcher.Start(ctx)
}

func (rt *Runtime) Stop() {
	if rt == nil || rt.Dispatcher == nil {
		return
	}
	rt.Dispatcher.Stop()
}
