package connector

import (
	"testing"
)

// --- KafkaProduceConnector ---

func TestKafkaProduceConnector_MissingTopic(t *testing.T) {
	c := &KafkaProduceConnector{}
	_, err := c.Execute(t.Context(), map[string]any{
		"message": "hello",
		"_credential": map[string]string{
			"brokers": "localhost:9092",
		},
	})
	if err == nil {
		t.Fatal("expected error for missing topic")
	}
}

func TestKafkaProduceConnector_MissingMessage(t *testing.T) {
	c := &KafkaProduceConnector{}
	_, err := c.Execute(t.Context(), map[string]any{
		"topic": "my-topic",
		"_credential": map[string]string{
			"brokers": "localhost:9092",
		},
	})
	if err == nil {
		t.Fatal("expected error for missing message")
	}
}

func TestKafkaProduceConnector_MissingCredential(t *testing.T) {
	c := &KafkaProduceConnector{}
	_, err := c.Execute(t.Context(), map[string]any{
		"topic":   "my-topic",
		"message": "hello",
	})
	if err == nil {
		t.Fatal("expected error for missing credential")
	}
}

func TestKafkaProduceConnector_MissingBrokers(t *testing.T) {
	c := &KafkaProduceConnector{}
	_, err := c.Execute(t.Context(), map[string]any{
		"topic":       "my-topic",
		"message":     "hello",
		"_credential": map[string]string{},
	})
	if err == nil {
		t.Fatal("expected error for missing brokers in credential")
	}
}

// --- KafkaConsumeConnector ---

func TestKafkaConsumeConnector_MissingTopic(t *testing.T) {
	c := &KafkaConsumeConnector{}
	_, err := c.Execute(t.Context(), map[string]any{
		"group_id": "my-group",
		"_credential": map[string]string{
			"brokers": "localhost:9092",
		},
	})
	if err == nil {
		t.Fatal("expected error for missing topic")
	}
}

func TestKafkaConsumeConnector_MissingGroupID(t *testing.T) {
	c := &KafkaConsumeConnector{}
	_, err := c.Execute(t.Context(), map[string]any{
		"topic": "my-topic",
		"_credential": map[string]string{
			"brokers": "localhost:9092",
		},
	})
	if err == nil {
		t.Fatal("expected error for missing group_id")
	}
}

func TestKafkaConsumeConnector_MissingCredential(t *testing.T) {
	c := &KafkaConsumeConnector{}
	_, err := c.Execute(t.Context(), map[string]any{
		"topic":    "my-topic",
		"group_id": "my-group",
	})
	if err == nil {
		t.Fatal("expected error for missing credential")
	}
}

func TestKafkaConsumeConnector_MissingBrokers(t *testing.T) {
	c := &KafkaConsumeConnector{}
	_, err := c.Execute(t.Context(), map[string]any{
		"topic":       "my-topic",
		"group_id":    "my-group",
		"_credential": map[string]string{},
	})
	if err == nil {
		t.Fatal("expected error for missing brokers in credential")
	}
}

// --- Registry ---

func TestRegistry_KafkaConnectors(t *testing.T) {
	r := NewRegistry()
	for _, action := range []string{
		"kafka/produce",
		"kafka/consume",
	} {
		if _, err := r.Get(action); err != nil {
			t.Errorf("%s not registered: %v", action, err)
		}
	}
}
