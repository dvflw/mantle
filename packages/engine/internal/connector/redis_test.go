package connector

import (
	"testing"
)

func TestRedisGetConnector_MissingCredential(t *testing.T) {
	c := &RedisGetConnector{}
	_, err := c.Execute(t.Context(), map[string]any{
		"key": "mykey",
	})
	if err == nil {
		t.Fatal("expected error for missing credential")
	}
}

func TestRedisGetConnector_MissingKey(t *testing.T) {
	c := &RedisGetConnector{}
	_, err := c.Execute(t.Context(), map[string]any{
		"_credential": map[string]string{"url": "redis://localhost:6379"},
	})
	if err == nil {
		t.Fatal("expected error for missing key")
	}
}

func TestRedisGetConnector_InvalidURL(t *testing.T) {
	c := &RedisGetConnector{}
	_, err := c.Execute(t.Context(), map[string]any{
		"_credential": map[string]string{"url": "not-a-valid-redis-url://???"},
		"key":         "mykey",
	})
	if err == nil {
		t.Fatal("expected error for invalid redis url")
	}
}

func TestRedisGetConnector_MissingURLInCredential(t *testing.T) {
	c := &RedisGetConnector{}
	_, err := c.Execute(t.Context(), map[string]any{
		"_credential": map[string]string{"host": "localhost"},
		"key":         "mykey",
	})
	if err == nil {
		t.Fatal("expected error for missing url in credential")
	}
}

func TestRedisSetConnector_MissingKey(t *testing.T) {
	c := &RedisSetConnector{}
	_, err := c.Execute(t.Context(), map[string]any{
		"_credential": map[string]string{"url": "redis://localhost:6379"},
		"value":       "hello",
	})
	if err == nil {
		t.Fatal("expected error for missing key")
	}
}

func TestRedisSetConnector_MissingValue(t *testing.T) {
	c := &RedisSetConnector{}
	_, err := c.Execute(t.Context(), map[string]any{
		"_credential": map[string]string{"url": "redis://localhost:6379"},
		"key":         "mykey",
	})
	if err == nil {
		t.Fatal("expected error for missing value")
	}
}

func TestRedisSetConnector_MissingCredential(t *testing.T) {
	c := &RedisSetConnector{}
	_, err := c.Execute(t.Context(), map[string]any{
		"key":   "mykey",
		"value": "hello",
	})
	if err == nil {
		t.Fatal("expected error for missing credential")
	}
}

func TestRedisPublishConnector_MissingChannel(t *testing.T) {
	c := &RedisPublishConnector{}
	_, err := c.Execute(t.Context(), map[string]any{
		"_credential": map[string]string{"url": "redis://localhost:6379"},
		"message":     "hello",
	})
	if err == nil {
		t.Fatal("expected error for missing channel")
	}
}

func TestRedisPublishConnector_MissingMessage(t *testing.T) {
	c := &RedisPublishConnector{}
	_, err := c.Execute(t.Context(), map[string]any{
		"_credential": map[string]string{"url": "redis://localhost:6379"},
		"channel":     "events",
	})
	if err == nil {
		t.Fatal("expected error for missing message")
	}
}

func TestRedisGetConnector_MapAnyCredential(t *testing.T) {
	c := &RedisGetConnector{}
	_, err := c.Execute(t.Context(), map[string]any{
		"_credential": map[string]any{"url": "not-a-valid-redis-url://???"},
		"key":         "mykey",
	})
	if err == nil {
		t.Fatal("expected error for invalid url from map[string]any credential")
	}
}

func TestRegistry_RedisConnectors(t *testing.T) {
	r := NewRegistry()
	if _, err := r.Get("redis/get"); err != nil {
		t.Errorf("redis/get not registered: %v", err)
	}
	if _, err := r.Get("redis/set"); err != nil {
		t.Errorf("redis/set not registered: %v", err)
	}
	if _, err := r.Get("redis/publish"); err != nil {
		t.Errorf("redis/publish not registered: %v", err)
	}
}
