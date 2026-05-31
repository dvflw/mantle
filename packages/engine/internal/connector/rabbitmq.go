package connector

import (
	"context"
	"fmt"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

// RabbitMQPublishConnector publishes a message to a RabbitMQ exchange or queue.
type RabbitMQPublishConnector struct{}

func (c *RabbitMQPublishConnector) Execute(ctx context.Context, params map[string]any) (map[string]any, error) {
	conn, ch, err := newRabbitMQChannel(params)
	if err != nil {
		return nil, fmt.Errorf("rabbitmq/publish: %w", err)
	}
	defer conn.Close()
	defer ch.Close()

	exchange, _ := params["exchange"].(string)
	routingKey, _ := params["routing_key"].(string)
	if exchange == "" && routingKey == "" {
		return nil, fmt.Errorf("rabbitmq/publish: exchange or routing_key is required")
	}

	body, _ := params["body"].(string)
	if body == "" {
		return nil, fmt.Errorf("rabbitmq/publish: body is required")
	}

	contentType := "text/plain"
	if ct, ok := params["content_type"].(string); ok && ct != "" {
		contentType = ct
	}

	msg := amqp.Publishing{
		ContentType:  contentType,
		Body:         []byte(body),
		DeliveryMode: amqp.Persistent,
		Timestamp:    time.Now(),
	}

	if err := ch.PublishWithContext(ctx, exchange, routingKey, false, false, msg); err != nil {
		return nil, fmt.Errorf("rabbitmq/publish: %w", err)
	}
	return map[string]any{"ok": true}, nil
}

// RabbitMQConsumeConnector consumes up to N messages from a RabbitMQ queue (non-blocking poll).
type RabbitMQConsumeConnector struct{}

func (c *RabbitMQConsumeConnector) Execute(ctx context.Context, params map[string]any) (map[string]any, error) {
	conn, ch, err := newRabbitMQChannel(params)
	if err != nil {
		return nil, fmt.Errorf("rabbitmq/consume: %w", err)
	}
	defer conn.Close()
	defer ch.Close()

	queue, _ := params["queue"].(string)
	if queue == "" {
		return nil, fmt.Errorf("rabbitmq/consume: queue is required")
	}

	maxMessages := 10
	if m, ok := extractInt(params["max_messages"]); ok && m > 0 {
		maxMessages = m
	}

	autoAck := true
	if a, ok := params["auto_ack"].(bool); ok {
		autoAck = a
	}

	var messages []any
	for i := 0; i < maxMessages; i++ {
		msg, ok, err := ch.Get(queue, autoAck)
		if err != nil {
			return nil, fmt.Errorf("rabbitmq/consume: %w", err)
		}
		if !ok {
			break
		}
		messages = append(messages, map[string]any{
			"body":         string(msg.Body),
			"content_type": msg.ContentType,
			"delivery_tag": msg.DeliveryTag,
			"routing_key":  msg.RoutingKey,
			"exchange":     msg.Exchange,
		})
	}

	if messages == nil {
		messages = []any{}
	}
	return map[string]any{"messages": messages, "count": len(messages)}, nil
}

// newRabbitMQChannel dials a RabbitMQ connection and opens a channel.
// Credential: {url: "amqp://user:pass@host:5672/"}
func newRabbitMQChannel(params map[string]any) (*amqp.Connection, *amqp.Channel, error) {
	raw, ok := params["_credential"]
	if !ok || raw == nil {
		return nil, nil, fmt.Errorf("credential is required")
	}
	delete(params, "_credential")

	var amqpURL string
	switch cred := raw.(type) {
	case map[string]string:
		amqpURL = cred["url"]
	case map[string]any:
		amqpURL, _ = cred["url"].(string)
	default:
		return nil, nil, fmt.Errorf("credential is required")
	}
	if amqpURL == "" {
		return nil, nil, fmt.Errorf("credential must contain a 'url' field (amqp://...)")
	}

	conn, err := amqp.Dial(amqpURL)
	if err != nil {
		return nil, nil, fmt.Errorf("connecting to RabbitMQ: %w", err)
	}

	ch, err := conn.Channel()
	if err != nil {
		conn.Close()
		return nil, nil, fmt.Errorf("opening channel: %w", err)
	}
	return conn, ch, nil
}
