package outbox

import (
	"math"
	"math/rand"
	"time"
)

// BackoffConfig controls exponential backoff with jitter after transport/handler failures.
type BackoffConfig struct {
	Base       time.Duration
	Max        time.Duration
	Multiplier float64
}

func (c BackoffConfig) withDefaults() BackoffConfig {
	if c.Base <= 0 {
		c.Base = time.Second
	}
	if c.Max <= 0 {
		c.Max = 5 * time.Minute
	}
	if c.Multiplier <= 1 {
		c.Multiplier = 2
	}
	return c
}

// NextDelay returns the delay before the next attempt (attempt is 1-based after failure).
func (c BackoffConfig) NextDelay(attempt int) time.Duration {
	c = c.withDefaults()
	if attempt < 1 {
		attempt = 1
	}
	exp := math.Pow(c.Multiplier, float64(attempt-1))
	d := time.Duration(float64(c.Base) * exp)
	if d > c.Max {
		d = c.Max
	}
	// Full jitter: [0, d]
	if d <= 0 {
		return c.Base
	}
	return time.Duration(rand.Int63n(int64(d) + 1))
}
