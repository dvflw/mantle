package connector

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// RedisGetConnector retrieves the value of a key from Redis.
type RedisGetConnector struct{}

func (c *RedisGetConnector) Execute(ctx context.Context, params map[string]any) (map[string]any, error) {
	client, err := newRedisClient(params)
	if err != nil {
		return nil, fmt.Errorf("redis/get: %w", err)
	}
	defer client.Close()

	key, _ := params["key"].(string)
	if key == "" {
		return nil, fmt.Errorf("redis/get: key is required")
	}

	val, err := client.Get(ctx, key).Result()
	if err == redis.Nil {
		return map[string]any{"value": nil, "exists": false}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("redis/get: %w", err)
	}
	return map[string]any{"value": val, "exists": true}, nil
}

// RedisSetConnector sets the value of a key in Redis.
type RedisSetConnector struct{}

func (c *RedisSetConnector) Execute(ctx context.Context, params map[string]any) (map[string]any, error) {
	client, err := newRedisClient(params)
	if err != nil {
		return nil, fmt.Errorf("redis/set: %w", err)
	}
	defer client.Close()

	key, _ := params["key"].(string)
	if key == "" {
		return nil, fmt.Errorf("redis/set: key is required")
	}
	value, ok := params["value"]
	if !ok {
		return nil, fmt.Errorf("redis/set: value is required")
	}

	var ttl time.Duration
	if ttlSec, ok := extractInt(params["ttl_seconds"]); ok && ttlSec > 0 {
		ttl = time.Duration(ttlSec) * time.Second
	}

	if err := client.Set(ctx, key, value, ttl).Err(); err != nil {
		return nil, fmt.Errorf("redis/set: %w", err)
	}
	return map[string]any{"ok": true}, nil
}

// RedisPublishConnector publishes a message to a Redis pub/sub channel.
type RedisPublishConnector struct{}

func (c *RedisPublishConnector) Execute(ctx context.Context, params map[string]any) (map[string]any, error) {
	client, err := newRedisClient(params)
	if err != nil {
		return nil, fmt.Errorf("redis/publish: %w", err)
	}
	defer client.Close()

	channel, _ := params["channel"].(string)
	if channel == "" {
		return nil, fmt.Errorf("redis/publish: channel is required")
	}
	message, _ := params["message"].(string)
	if message == "" {
		return nil, fmt.Errorf("redis/publish: message is required")
	}

	receivers, err := client.Publish(ctx, channel, message).Result()
	if err != nil {
		return nil, fmt.Errorf("redis/publish: %w", err)
	}
	return map[string]any{"receivers": receivers}, nil
}

// newRedisClient builds a redis.Client from the _credential param.
// Credential: {url: "redis://[:password@]host:port/db"}
func newRedisClient(params map[string]any) (*redis.Client, error) {
	raw, ok := params["_credential"]
	if !ok || raw == nil {
		return nil, fmt.Errorf("credential is required")
	}
	delete(params, "_credential")

	var redisURL string
	switch cred := raw.(type) {
	case map[string]string:
		redisURL = cred["url"]
	case map[string]any:
		redisURL, _ = cred["url"].(string)
	default:
		return nil, fmt.Errorf("credential is required")
	}
	if redisURL == "" {
		return nil, fmt.Errorf("credential must contain a 'url' field")
	}

	opts, err := redis.ParseURL(redisURL)
	if err != nil {
		return nil, fmt.Errorf("credential url is invalid: %w", err)
	}
	return redis.NewClient(opts), nil
}
