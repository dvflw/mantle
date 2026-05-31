package connector

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/twmb/franz-go/pkg/kgo"
)

// extractKafkaCredential pulls the brokers list from the _credential param.
// Expected credential shape: {brokers: "broker1:9092,broker2:9092"}.
// Deletes _credential from params.
func extractKafkaCredential(params map[string]any) (brokers []string, err error) {
	raw, ok := params["_credential"]
	if !ok || raw == nil {
		return nil, fmt.Errorf("credential is required")
	}
	delete(params, "_credential")

	var cred map[string]string
	switch v := raw.(type) {
	case map[string]string:
		cred = v
	case map[string]any:
		cred = make(map[string]string, len(v))
		for k, val := range v {
			if s, ok := val.(string); ok {
				cred[k] = s
			}
		}
	default:
		return nil, fmt.Errorf("credential is required")
	}

	brokersStr := cred["brokers"]
	if brokersStr == "" {
		return nil, fmt.Errorf("credential must contain a 'brokers' field")
	}

	parts := strings.Split(brokersStr, ",")
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			brokers = append(brokers, p)
		}
	}
	if len(brokers) == 0 {
		return nil, fmt.Errorf("credential must contain at least one broker in 'brokers' field")
	}
	return brokers, nil
}

// KafkaProduceConnector produces a message to a Kafka topic.
// Params: topic (required), message (required), key (optional).
// Output: {"ok": true}
type KafkaProduceConnector struct{}

func (c *KafkaProduceConnector) Execute(ctx context.Context, params map[string]any) (map[string]any, error) {
	topic, _ := params["topic"].(string)
	if topic == "" {
		return nil, fmt.Errorf("kafka/produce: topic is required")
	}

	message, _ := params["message"].(string)
	if message == "" {
		return nil, fmt.Errorf("kafka/produce: message is required")
	}

	brokers, err := extractKafkaCredential(params)
	if err != nil {
		return nil, fmt.Errorf("kafka/produce: %w", err)
	}

	opts := []kgo.Opt{
		kgo.SeedBrokers(brokers...),
		kgo.DefaultProduceTopic(topic),
	}

	client, err := kgo.NewClient(opts...)
	if err != nil {
		return nil, fmt.Errorf("kafka/produce: creating client: %w", err)
	}
	defer client.Close()

	record := &kgo.Record{
		Value: []byte(message),
	}

	if key, _ := params["key"].(string); key != "" {
		record.Key = []byte(key)
	}

	if err := client.ProduceSync(ctx, record).FirstErr(); err != nil {
		return nil, fmt.Errorf("kafka/produce: producing message: %w", err)
	}

	return map[string]any{"ok": true}, nil
}

// KafkaConsumeConnector consumes messages from a Kafka topic.
// Params: topic (required), group_id (required), max_messages (optional, default 10),
//
//	timeout_ms (optional, default 5000).
//
// Output: {"messages": [...], "count": N}
// Each message: {"key": "...", "value": "...", "offset": N, "partition": N}
type KafkaConsumeConnector struct{}

func (c *KafkaConsumeConnector) Execute(ctx context.Context, params map[string]any) (map[string]any, error) {
	topic, _ := params["topic"].(string)
	if topic == "" {
		return nil, fmt.Errorf("kafka/consume: topic is required")
	}

	groupID, _ := params["group_id"].(string)
	if groupID == "" {
		return nil, fmt.Errorf("kafka/consume: group_id is required")
	}

	brokers, err := extractKafkaCredential(params)
	if err != nil {
		return nil, fmt.Errorf("kafka/consume: %w", err)
	}

	maxMessages := 10
	if m, ok := extractInt(params["max_messages"]); ok && m > 0 {
		maxMessages = m
	}

	timeoutMS := 5000
	if t, ok := extractInt(params["timeout_ms"]); ok && t > 0 {
		timeoutMS = t
	}

	client, err := kgo.NewClient(
		kgo.SeedBrokers(brokers...),
		kgo.ConsumerGroup(groupID),
		kgo.ConsumeTopics(topic),
	)
	if err != nil {
		return nil, fmt.Errorf("kafka/consume: creating client: %w", err)
	}
	defer client.Close()

	tctx, cancel := context.WithTimeout(ctx, time.Duration(timeoutMS)*time.Millisecond)
	defer cancel()

	var messages []any
	for len(messages) < maxMessages {
		fetches := client.PollFetches(tctx)
		if fetches.IsClientClosed() {
			break
		}
		if errs := fetches.Errors(); len(errs) > 0 {
			// Check for context cancellation (timeout reached).
			for _, fe := range errs {
				if fe.Err == context.DeadlineExceeded || fe.Err == context.Canceled {
					goto done
				}
			}
			return nil, fmt.Errorf("kafka/consume: fetch error: %v", errs[0].Err)
		}

		fetches.EachRecord(func(r *kgo.Record) {
			if len(messages) >= maxMessages {
				return
			}
			msg := map[string]any{
				"key":       string(r.Key),
				"value":     string(r.Value),
				"offset":    r.Offset,
				"partition": r.Partition,
			}
			messages = append(messages, msg)
		})

		if tctx.Err() != nil {
			break
		}
	}
done:

	if messages == nil {
		messages = []any{}
	}
	return map[string]any{
		"messages": messages,
		"count":    len(messages),
	}, nil
}
