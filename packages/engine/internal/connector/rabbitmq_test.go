package connector

import (
	"testing"
)

func TestRabbitMQPublishConnector_MissingCredential(t *testing.T) {
	c := &RabbitMQPublishConnector{}
	_, err := c.Execute(t.Context(), map[string]any{
		"exchange":    "events",
		"routing_key": "order.created",
		"body":        "hello",
	})
	if err == nil {
		t.Fatal("expected error for missing credential")
	}
}

func TestRabbitMQPublishConnector_MissingURL(t *testing.T) {
	c := &RabbitMQPublishConnector{}
	_, err := c.Execute(t.Context(), map[string]any{
		"_credential": map[string]string{"host": "localhost"},
		"exchange":    "events",
		"body":        "hello",
	})
	if err == nil {
		t.Fatal("expected error for missing url in credential")
	}
}

func TestRabbitMQPublishConnector_MissingExchangeAndRoutingKey(t *testing.T) {
	c := &RabbitMQPublishConnector{}
	_, err := c.Execute(t.Context(), map[string]any{
		"_credential": map[string]string{"url": "amqp://localhost:5672/"},
		"body":        "hello",
	})
	if err == nil {
		t.Fatal("expected error for missing exchange and routing_key")
	}
}

func TestRabbitMQPublishConnector_MissingBody(t *testing.T) {
	c := &RabbitMQPublishConnector{}
	_, err := c.Execute(t.Context(), map[string]any{
		"_credential": map[string]string{"url": "amqp://localhost:5672/"},
		"routing_key": "myqueue",
	})
	if err == nil {
		t.Fatal("expected error for missing body")
	}
}

func TestRabbitMQConsumeConnector_MissingCredential(t *testing.T) {
	c := &RabbitMQConsumeConnector{}
	_, err := c.Execute(t.Context(), map[string]any{
		"queue": "myqueue",
	})
	if err == nil {
		t.Fatal("expected error for missing credential")
	}
}

func TestRabbitMQConsumeConnector_MissingQueue(t *testing.T) {
	c := &RabbitMQConsumeConnector{}
	_, err := c.Execute(t.Context(), map[string]any{
		"_credential": map[string]string{"url": "amqp://localhost:5672/"},
	})
	if err == nil {
		t.Fatal("expected error for missing queue")
	}
}

func TestRabbitMQPublishConnector_MapAnyCredential(t *testing.T) {
	c := &RabbitMQPublishConnector{}
	_, err := c.Execute(t.Context(), map[string]any{
		"_credential": map[string]any{"url": "amqp://localhost:5672/"},
		"routing_key": "myqueue",
		"body":        "hello",
	})
	// Expect a connection error (no real RabbitMQ running), not a parse error.
	if err == nil {
		t.Fatal("expected connection error")
	}
}

func TestRegistry_RabbitMQConnectors(t *testing.T) {
	r := NewRegistry()
	for _, action := range []string{"rabbitmq/publish", "rabbitmq/consume"} {
		if _, err := r.Get(action); err != nil {
			t.Errorf("%s not registered: %v", action, err)
		}
	}
}
