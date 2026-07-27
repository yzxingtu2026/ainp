package job

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

const (
	WorkerQueuesEnv      = "WORKER_QUEUES"
	WorkerConcurrencyEnv = "WORKER_CONCURRENCY"
)

type WorkerProfile struct {
	Queues      map[string]int
	Concurrency int
}

type WorkerProfileOverrides struct {
	Queues      *string
	Concurrency *string
}

// ResolveWorkerProfile applies process-only overrides without mutating shared
// JobsConfig or persisted platform state.
func ResolveWorkerProfile(base WorkerProfile, overrides WorkerProfileOverrides) (WorkerProfile, error) {
	resolved := WorkerProfile{Queues: cloneQueues(base.Queues), Concurrency: base.Concurrency}
	if overrides.Queues != nil {
		queues, err := parseQueues(*overrides.Queues)
		if err != nil {
			return WorkerProfile{}, err
		}
		resolved.Queues = queues
	}
	if overrides.Concurrency != nil {
		value, err := strconv.Atoi(strings.TrimSpace(*overrides.Concurrency))
		if err != nil {
			return WorkerProfile{}, fmt.Errorf("worker: concurrency must be a positive integer")
		}
		resolved.Concurrency = value
	}
	if err := ValidateWorkerProfile(resolved); err != nil {
		return WorkerProfile{}, err
	}
	return resolved, nil
}

func ValidateWorkerProfile(profile WorkerProfile) error {
	if profile.Concurrency <= 0 {
		return fmt.Errorf("worker: concurrency must be greater than zero")
	}
	if len(profile.Queues) == 0 {
		return fmt.Errorf("worker: queue set must not be empty")
	}
	for queue, weight := range profile.Queues {
		if !ValidQueue(queue) {
			return fmt.Errorf("worker: invalid queue %q", queue)
		}
		if weight <= 0 {
			return fmt.Errorf("worker: queue %q weight must be greater than zero", queue)
		}
	}
	return nil
}

func FormatWorkerQueues(queues map[string]int) string {
	keys := make([]string, 0, len(queues))
	for queue := range queues {
		keys = append(keys, queue)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, queue := range keys {
		parts = append(parts, fmt.Sprintf("%s=%d", queue, queues[queue]))
	}
	return strings.Join(parts, ",")
}

func parseQueues(raw string) (map[string]int, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, fmt.Errorf("worker: queue override must not be empty")
	}
	queues := map[string]int{}
	for _, item := range strings.Split(raw, ",") {
		parts := strings.Split(strings.TrimSpace(item), "=")
		if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" {
			return nil, fmt.Errorf("worker: queue override must use queue=weight entries")
		}
		queue := strings.TrimSpace(parts[0])
		if _, exists := queues[queue]; exists {
			return nil, fmt.Errorf("worker: duplicate queue %q", queue)
		}
		weight, err := strconv.Atoi(strings.TrimSpace(parts[1]))
		if err != nil {
			return nil, fmt.Errorf("worker: queue %q weight must be a positive integer", queue)
		}
		queues[queue] = weight
	}
	return queues, nil
}

func cloneQueues(source map[string]int) map[string]int {
	cloned := make(map[string]int, len(source))
	for queue, weight := range source {
		cloned[queue] = weight
	}
	return cloned
}
