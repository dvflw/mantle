package connector

import (
	"testing"
)

func TestSnowflakeQueryConnector_MissingCredential(t *testing.T) {
	c := &SnowflakeQueryConnector{}
	_, err := c.Execute(t.Context(), map[string]any{
		"query": "SELECT 1",
	})
	if err == nil {
		t.Fatal("expected error for missing credential")
	}
}

func TestSnowflakeQueryConnector_MissingQuery(t *testing.T) {
	c := &SnowflakeQueryConnector{}
	_, err := c.Execute(t.Context(), map[string]any{
		"_credential": map[string]string{
			"account":  "myaccount.snowflakecomputing.com",
			"user":     "myuser",
			"password": "mypassword",
		},
	})
	if err == nil {
		t.Fatal("expected error for missing query")
	}
}

func TestSnowflakeQueryConnector_MissingAccount(t *testing.T) {
	c := &SnowflakeQueryConnector{}
	_, err := c.Execute(t.Context(), map[string]any{
		"query": "SELECT 1",
		"_credential": map[string]string{
			"user":     "myuser",
			"password": "mypassword",
		},
	})
	if err == nil {
		t.Fatal("expected error for missing account")
	}
}

func TestSnowflakeQueryConnector_MissingUser(t *testing.T) {
	c := &SnowflakeQueryConnector{}
	_, err := c.Execute(t.Context(), map[string]any{
		"query": "SELECT 1",
		"_credential": map[string]string{
			"account":  "myaccount.snowflakecomputing.com",
			"password": "mypassword",
		},
	})
	if err == nil {
		t.Fatal("expected error for missing user")
	}
}

func TestSnowflakeQueryConnector_MissingPassword(t *testing.T) {
	c := &SnowflakeQueryConnector{}
	_, err := c.Execute(t.Context(), map[string]any{
		"query": "SELECT 1",
		"_credential": map[string]string{
			"account": "myaccount.snowflakecomputing.com",
			"user":    "myuser",
		},
	})
	if err == nil {
		t.Fatal("expected error for missing password")
	}
}

func TestSnowflakeQueryConnector_ArgsOptional(t *testing.T) {
	// args is optional — omitting it should not cause a validation error.
	c := &SnowflakeQueryConnector{}
	_, err := c.Execute(t.Context(), map[string]any{
		"query": "SELECT 1",
		"_credential": map[string]string{
			"account":  "myaccount",
			"user":     "myuser",
			"password": "mypassword",
		},
	})
	// Will fail trying to connect — that's fine; no panic on missing args.
	_ = err
}

func TestRegistry_SnowflakeConnectors(t *testing.T) {
	r := NewRegistry()
	for _, action := range []string{
		"snowflake/query",
	} {
		if _, err := r.Get(action); err != nil {
			t.Errorf("%s not registered: %v", action, err)
		}
	}
}
