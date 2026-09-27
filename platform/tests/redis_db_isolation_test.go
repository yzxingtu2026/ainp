package tests

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/hibiken/asynq"
	"github.com/yzxingtu2026/ainp/platform/framework/asynqadapter"
	"github.com/yzxingtu2026/ainp/platform/framework/config"
	"github.com/yzxingtu2026/ainp/platform/framework/job"
)

// Redis is shared, queues and task types are deliberately identical; only the DB separates applications.
// Requires a disposable standalone Redis (never a production instance) with SELECT support.
func TestRedisLogicalDBSeparatesApplicationQueues(t *testing.T) {
	addr := os.Getenv("AINP_TEST_REDIS_ADDR")
	if addr == "" {
		t.Skip("AINP_TEST_REDIS_ADDR is not set")
	}
	const dbA, dbB = 13, 14
	id := fmt.Sprintf("redis-db-isolation-%d", time.Now().UnixNano())
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()

	type app struct {
		db   int
		name string
	}
	apps := []app{{dbA, id + "-a"}, {dbB, id + "-b"}}
	results := make(chan string, 4)
	for _, application := range apps {
		opt := asynqadapter.RedisOptFromConfig(config.RedisConfig{Addr: addr, DB: application.db})
		if opt.DB != application.db {
			t.Fatalf("adapter lost database ID %d", application.db)
		}
		redisOpt := asynq.RedisClientOpt{Addr: opt.Addr, DB: opt.DB}
		server := asynq.NewServer(redisOpt, asynq.Config{Concurrency: 1, Queues: map[string]int{job.QueueDefault: 1}})
		mux := asynq.NewServeMux()
		own := application.name
		mux.HandleFunc("ainp.job.run", func(_ context.Context, task *asynq.Task) error {
			var payload struct {
				JobRunID string `json:"job_run_id"`
			}
			if err := json.Unmarshal(task.Payload(), &payload); err != nil {
				return err
			}
			results <- own + ":" + payload.JobRunID
			return nil
		})
		if err := server.Start(mux); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(server.Shutdown)
	}
	for _, application := range apps {
		opt := asynqadapter.RedisOptFromConfig(config.RedisConfig{Addr: addr, DB: application.db})
		transport := asynqadapter.NewClientTransport(opt)
		t.Cleanup(func() { _ = transport.Close() })
		if err := transport.Enqueue(ctx, job.Envelope{ID: application.name}, job.QueueDefault, job.EnqueueTransportOpts{TaskID: application.name, MaxRetry: 0, Timeout: 5 * time.Second}); err != nil {
			t.Fatal(err)
		}
	}
	seen := make(map[string]bool)
	for len(seen) != len(apps) {
		select {
		case got := <-results:
			seen[got] = true
		case <-ctx.Done():
			t.Fatalf("missing isolated queue deliveries: %v (%v)", seen, ctx.Err())
		}
	}
	for _, application := range apps {
		if !seen[application.name+":"+application.name] {
			t.Fatalf("another application's worker consumed a task: %v", seen)
		}
	}
}
