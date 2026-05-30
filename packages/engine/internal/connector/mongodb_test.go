package connector

import (
	"testing"
)

func TestMongoFindConnector_MissingCredential(t *testing.T) {
	c := &MongoFindConnector{}
	_, err := c.Execute(t.Context(), map[string]any{
		"database":   "mydb",
		"collection": "users",
	})
	if err == nil {
		t.Fatal("expected error for missing credential")
	}
}

func TestMongoFindConnector_MissingURI(t *testing.T) {
	c := &MongoFindConnector{}
	_, err := c.Execute(t.Context(), map[string]any{
		"_credential": map[string]string{"database": "mydb"},
		"database":    "mydb",
		"collection":  "users",
	})
	if err == nil {
		t.Fatal("expected error for missing uri in credential")
	}
}

func TestMongoFindConnector_MissingDatabase(t *testing.T) {
	c := &MongoFindConnector{}
	_, err := c.Execute(t.Context(), map[string]any{
		"_credential": map[string]string{"uri": "mongodb://localhost:27017"},
		"collection":  "users",
	})
	if err == nil {
		t.Fatal("expected error for missing database")
	}
}

func TestMongoFindConnector_MissingCollection(t *testing.T) {
	c := &MongoFindConnector{}
	_, err := c.Execute(t.Context(), map[string]any{
		"_credential": map[string]string{"uri": "mongodb://localhost:27017", "database": "mydb"},
	})
	if err == nil {
		t.Fatal("expected error for missing collection")
	}
}

func TestMongoAggregateConnector_MissingCredential(t *testing.T) {
	c := &MongoAggregateConnector{}
	_, err := c.Execute(t.Context(), map[string]any{
		"database":   "mydb",
		"collection": "orders",
		"pipeline":   []any{map[string]any{"$match": map[string]any{}}},
	})
	if err == nil {
		t.Fatal("expected error for missing credential")
	}
}

func TestMongoAggregateConnector_MissingPipeline(t *testing.T) {
	c := &MongoAggregateConnector{}
	_, err := c.Execute(t.Context(), map[string]any{
		"_credential": map[string]string{"uri": "mongodb://localhost:27017", "database": "mydb"},
		"collection":  "orders",
	})
	if err == nil {
		t.Fatal("expected error for missing pipeline")
	}
}

func TestMongoAggregateConnector_EmptyPipeline(t *testing.T) {
	c := &MongoAggregateConnector{}
	_, err := c.Execute(t.Context(), map[string]any{
		"_credential": map[string]string{"uri": "mongodb://localhost:27017", "database": "mydb"},
		"collection":  "orders",
		"pipeline":    []any{},
	})
	if err == nil {
		t.Fatal("expected error for empty pipeline")
	}
}

func TestMongoFindConnector_DatabaseFromCredential(t *testing.T) {
	// Verify database can come from credential — error should be about connection, not about missing database.
	c := &MongoFindConnector{}
	_, err := c.Execute(t.Context(), map[string]any{
		"_credential": map[string]string{"uri": "mongodb://localhost:27017", "database": "mydb"},
		"collection":  "users",
	})
	// We expect a connection error (no real MongoDB), not a "database is required" error.
	if err == nil {
		t.Fatal("expected connection error")
	}
	if err.Error() == "mongodb/find: database is required (set in params or credential)" {
		t.Errorf("database should have been resolved from credential, got: %v", err)
	}
}

func TestMongoFindConnector_MapAnyCredential(t *testing.T) {
	c := &MongoFindConnector{}
	_, err := c.Execute(t.Context(), map[string]any{
		"_credential": map[string]any{"uri": "mongodb://localhost:27017", "database": "mydb"},
		"collection":  "users",
	})
	// Expect a connection error (no real MongoDB running), not a credential parse error.
	if err == nil {
		t.Fatal("expected connection error")
	}
}

func TestRegistry_MongoDBConnectors(t *testing.T) {
	r := NewRegistry()
	if _, err := r.Get("mongodb/find"); err != nil {
		t.Errorf("mongodb/find not registered: %v", err)
	}
	if _, err := r.Get("mongodb/aggregate"); err != nil {
		t.Errorf("mongodb/aggregate not registered: %v", err)
	}
}
