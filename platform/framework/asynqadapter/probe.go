package asynqadapter

import (
	"context"
	"errors"
	"strings"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

type CompatibilityFailure string

const (
	CompatibilityConnectionFailed     CompatibilityFailure = "redis_connection_failed"
	CompatibilityAuthenticationFailed CompatibilityFailure = "redis_authentication_failed"
	CompatibilityLuaUnavailable       CompatibilityFailure = "redis_lua_unavailable"
	CompatibilityQueueIncompatible    CompatibilityFailure = "redis_queue_incompatible"
)

type CompatibilityCapabilities struct {
	Ping      bool `json:"ping"`
	Lua       bool `json:"lua"`
	String    bool `json:"string"`
	Hash      bool `json:"hash"`
	List      bool `json:"list"`
	Set       bool `json:"set"`
	SortedSet bool `json:"sorted_set"`
	PubSub    bool `json:"pub_sub"`
	Cleanup   bool `json:"cleanup"`
}

type CompatibilityResult struct {
	Reachable       bool                      `json:"reachable"`
	QueueCompatible bool                      `json:"queue_compatible"`
	Capabilities    CompatibilityCapabilities `json:"capabilities"`
	FailureCode     CompatibilityFailure      `json:"failure_code,omitempty"`
}

var (
	luaProbeScript   = redis.NewScript(`return 1`)
	queueProbeScript = redis.NewScript(`
redis.call("SET", KEYS[1], "probe")
redis.call("HSET", KEYS[2], "probe", "1")
redis.call("LPUSH", KEYS[3], "probe")
redis.call("SADD", KEYS[4], "probe")
redis.call("ZADD", KEYS[5], 1, "probe")
redis.call("DEL", KEYS[1], KEYS[2], KEYS[3], KEYS[4], KEYS[5])
return 1
`)
)

func ProbeCompatibility(ctx context.Context, opt RedisOpt) CompatibilityResult {
	client := redis.NewClient(&redis.Options{
		Addr: opt.Addr, Username: opt.Username, Password: opt.Password, DB: opt.DB,
		DialTimeout: opt.DialTimeout, ReadTimeout: opt.ReadTimeout,
		WriteTimeout: opt.WriteTimeout, PoolSize: opt.PoolSize,
	})
	defer client.Close()

	result := CompatibilityResult{}
	if err := client.Ping(ctx).Err(); err != nil {
		result.FailureCode = classifyConnectionFailure(err)
		return result
	}
	result.Reachable = true
	result.Capabilities.Ping = true

	if _, err := luaProbeScript.Run(ctx, client, nil).Int64(); err != nil {
		result.FailureCode = classifyProbeFailure(err, CompatibilityLuaUnavailable)
		return result
	}
	result.Capabilities.Lua = true

	prefix := "ainp:queue-compatibility-probe:" + uuid.NewString()
	keys := []string{prefix + ":string", prefix + ":hash", prefix + ":list", prefix + ":set", prefix + ":zset"}
	defer client.Del(context.WithoutCancel(ctx), keys...)
	if _, err := queueProbeScript.Run(ctx, client, keys).Int64(); err != nil {
		result.FailureCode = classifyProbeFailure(err, CompatibilityQueueIncompatible)
		return result
	}
	result.Capabilities.String = true
	result.Capabilities.Hash = true
	result.Capabilities.List = true
	result.Capabilities.Set = true
	result.Capabilities.SortedSet = true
	remaining, err := client.Exists(ctx, keys...).Result()
	if err != nil || remaining != 0 {
		result.FailureCode = CompatibilityQueueIncompatible
		return result
	}
	result.Capabilities.Cleanup = true

	channel := prefix + ":pubsub"
	pubsub := client.Subscribe(ctx, channel)
	if _, err := pubsub.Receive(ctx); err != nil {
		_ = pubsub.Close()
		result.FailureCode = classifyProbeFailure(err, CompatibilityQueueIncompatible)
		return result
	}
	subscribers, err := client.Publish(ctx, channel, "probe").Result()
	if err != nil {
		_ = pubsub.Close()
		result.FailureCode = classifyProbeFailure(err, CompatibilityQueueIncompatible)
		return result
	}
	if subscribers < 1 {
		_ = pubsub.Close()
		result.FailureCode = CompatibilityQueueIncompatible
		return result
	}
	if err := pubsub.Close(); err != nil {
		result.FailureCode = CompatibilityQueueIncompatible
		return result
	}
	result.Capabilities.PubSub = true
	result.QueueCompatible = true
	return result
}

func classifyProbeFailure(err error, serverFailure CompatibilityFailure) CompatibilityFailure {
	if failure := classifyConnectionFailure(err); failure == CompatibilityAuthenticationFailed {
		return failure
	}
	var redisErr redis.Error
	if errors.As(err, &redisErr) {
		return serverFailure
	}
	return CompatibilityConnectionFailed
}

func classifyConnectionFailure(err error) CompatibilityFailure {
	var redisErr redis.Error
	if errors.As(err, &redisErr) {
		message := strings.ToUpper(strings.TrimSpace(redisErr.Error()))
		message = strings.TrimPrefix(message, "ERR ")
		if strings.HasPrefix(message, "NOAUTH") || strings.HasPrefix(message, "WRONGPASS") || strings.HasPrefix(message, "AUTH ") {
			return CompatibilityAuthenticationFailed
		}
	}
	return CompatibilityConnectionFailed
}
