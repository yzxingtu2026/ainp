package idempotency

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/yzxingtu2026/ainp/platform/framework/auth"
	"github.com/yzxingtu2026/ainp/platform/framework/errs"
	"github.com/yzxingtu2026/ainp/platform/framework/logging"
)

const (
	defaultTTL               = 24 * time.Hour
	defaultInProgressTimeout = 30 * time.Second
	maxCachedResponseSize    = 64 * 1024 // 64KB
)

// ExecuteResult holds the outcome of an idempotent execution.
type ExecuteResult struct {
	Data     any
	IsReplay bool // true if the result was served from cache
}

// HandlerFunc is the business handler signature.
type HandlerFunc func(ctx context.Context, input json.RawMessage) (any, error)

// ExecuteInput describes what to execute idempotently.
type ExecuteInput struct {
	ActionID string
	Input    json.RawMessage
	Meta     *IdempotencyConfig
	Handler  HandlerFunc
}

// IdempotencyConfig is a simplified view of action.IdempotencyMeta for the executor.
type IdempotencyConfig struct {
	Mode                string
	TTL                 time.Duration
	InProgressTimeout   time.Duration
	CacheBusinessErrors bool
}

// Executor orchestrates idempotent action execution.
type Executor struct {
	repo *Repository
}

// NewExecutor creates a new Executor.
func NewExecutor(repo *Repository) *Executor {
	return &Executor{repo: repo}
}

// Execute runs a handler with idempotency semantics.
func (e *Executor) Execute(ctx context.Context, in ExecuteInput) (*ExecuteResult, error) {
	ac := auth.MustFromContext(ctx)
	key := ac.IdempotencyKey

	// Mode check
	if key == "" {
		switch in.Meta.Mode {
		case "required":
			return nil, errs.NewKey(errs.CodeIdempotencyKeyRequired, "idempotency.key_required", "idempotency key required")
		case "optional":
			// No key provided, execute directly without idempotency
			data, err := in.Handler(ctx, in.Input)
			if err != nil {
				return nil, err
			}
			return &ExecuteResult{Data: data}, nil
		default:
			data, err := in.Handler(ctx, in.Input)
			if err != nil {
				return nil, err
			}
			return &ExecuteResult{Data: data}, nil
		}
	}

	ttl := in.Meta.TTL
	if ttl == 0 {
		ttl = defaultTTL
	}
	inProgressTimeout := in.Meta.InProgressTimeout
	if inProgressTimeout == 0 {
		inProgressTimeout = defaultInProgressTimeout
	}

	hash := HashRequest(in.Input)
	start := time.Now()

	// Check for existing record
	existing, err := e.repo.GetByKey(ctx, ac.TenantID, ac.PrincipalID, in.ActionID, key)
	if err != nil {
		return nil, errs.Wrap(errs.CodeInternal, "idempotency lookup failed", err)
	}

	if existing != nil {
		result, execErr := e.handleExisting(ctx, existing, hash, inProgressTimeout, in.Meta.CacheBusinessErrors)
		logging.Info(ctx, "idempotency resolved",
			"action_id", in.ActionID,
			"key_summary", HashSummary(hash),
			"status", string(existing.Status),
			"replay", result != nil && result.IsReplay,
			"duration_ms", time.Since(start).Milliseconds(),
		)
		return result, execErr
	}

	// No existing record: create processing record and execute
	rec, err := e.repo.Create(ctx, ac.TenantID, ac.PrincipalID, in.ActionID, key, hash, ttl)
	if err != nil {
		if errors.Is(err, ErrDuplicateKey) {
			// Race condition: another request created the record between our check and insert
			existing, lookupErr := e.repo.GetByKey(ctx, ac.TenantID, ac.PrincipalID, in.ActionID, key)
			if lookupErr != nil {
				return nil, errs.Wrap(errs.CodeInternal, "idempotency race lookup failed", lookupErr)
			}
			if existing != nil {
				result, execErr := e.handleExisting(ctx, existing, hash, inProgressTimeout, in.Meta.CacheBusinessErrors)
				logging.Info(ctx, "idempotency race resolved",
					"action_id", in.ActionID,
					"key_summary", HashSummary(hash),
					"status", string(existing.Status),
					"duration_ms", time.Since(start).Milliseconds(),
				)
				return result, execErr
			}
		}
		return nil, errs.Wrap(errs.CodeInternal, "idempotency create failed", err)
	}

	// Execute the handler
	data, handlerErr := in.Handler(ctx, in.Input)
	if handlerErr != nil {
		// Cache business error if configured
		if in.Meta.CacheBusinessErrors {
			if errJSON, marshalErr := marshalError(handlerErr); marshalErr == nil && len(errJSON) <= maxCachedResponseSize {
				_ = e.repo.Complete(ctx, rec.ID, StatusFailed, nil, errJSON)
			} else {
				_ = e.repo.Complete(ctx, rec.ID, StatusFailed, nil, nil)
			}
		} else {
			_ = e.repo.Complete(ctx, rec.ID, StatusFailed, nil, nil)
		}
		logging.Info(ctx, "idempotency handler failed",
			"action_id", in.ActionID,
			"key_summary", HashSummary(hash),
			"duration_ms", time.Since(start).Milliseconds(),
		)
		return nil, handlerErr
	}

	// Cache successful response
	var respJSON []byte
	if respBytes, marshalErr := json.Marshal(data); marshalErr == nil && len(respBytes) <= maxCachedResponseSize {
		respJSON = respBytes
	}
	_ = e.repo.Complete(ctx, rec.ID, StatusCompleted, respJSON, nil)

	logging.Info(ctx, "idempotency completed",
		"action_id", in.ActionID,
		"key_summary", HashSummary(hash),
		"duration_ms", time.Since(start).Milliseconds(),
	)

	return &ExecuteResult{Data: data}, nil
}

func (e *Executor) handleExisting(ctx context.Context, rec *Record, hash string, inProgressTimeout time.Duration, cacheBusinessErrors bool) (*ExecuteResult, error) {
	switch rec.Status {
	case StatusCompleted:
		if rec.RequestHash != hash {
			return nil, errs.NewKey(errs.CodeIdempotencyConflict, "idempotency.conflict", "same idempotency key used for different request")
		}
		// Replay: return cached response
		var data any
		if len(rec.ResponseJSON) > 0 {
			if err := json.Unmarshal(rec.ResponseJSON, &data); err == nil {
				return &ExecuteResult{Data: data, IsReplay: true}, nil
			}
		}
		// Response not cached or unmarshalable — return empty replay
		return &ExecuteResult{IsReplay: true}, nil

	case StatusFailed:
		if rec.RequestHash != hash {
			return nil, errs.NewKey(errs.CodeIdempotencyConflict, "idempotency.conflict", "same idempotency key used for different request")
		}
		if cacheBusinessErrors && len(rec.ErrorJSON) > 0 {
			var cachedErr cachedError
			if err := json.Unmarshal(rec.ErrorJSON, &cachedErr); err == nil {
				return nil, errs.NewKey(cachedErr.Code, cachedErr.Key, cachedErr.Message)
			}
		}
		return nil, errs.NewKey(errs.CodeConflict, "idempotency.previous_failure", "previous execution failed")

	case StatusProcessing:
		if time.Since(rec.StartedAt) > inProgressTimeout {
			// Stale processing: try to claim
			claimed, err := e.repo.ClaimProcessing(ctx, rec.ID, inProgressTimeout)
			if err != nil {
				return nil, errs.Wrap(errs.CodeInternal, "idempotency claim failed", err)
			}
			if claimed {
				// Reset to allow re-execution by the caller
				// The caller should re-invoke Execute
				return nil, errs.NewKey(errs.CodeOperationInProgress, "idempotency.stale_recovered", "stale operation recovered, please retry")
			}
		}
		return nil, errs.NewKey(errs.CodeOperationInProgress, "idempotency.in_progress", "operation is in progress")

	case StatusConflict:
		return nil, errs.NewKey(errs.CodeIdempotencyConflict, "idempotency.conflict", "idempotency conflict")
	}

	return nil, errs.NewKey(errs.CodeInternal, "idempotency.unknown_status", "unknown idempotency status")
}

type cachedError struct {
	Code    int    `json:"code"`
	Key     string `json:"key"`
	Message string `json:"message"`
}

func marshalError(err error) ([]byte, error) {
	ae, ok := errs.AsAppError(err)
	if !ok {
		return nil, errors.New("not an AppError")
	}
	return json.Marshal(cachedError{Code: ae.Code, Key: ae.Key(), Message: ae.Message})
}
