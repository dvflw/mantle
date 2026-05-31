package connector

import (
	"testing"
)

// ---- MySQL ----

func TestMySQLQueryConnector_MissingQuery(t *testing.T) {
	c := &MySQLQueryConnector{}
	_, err := c.Execute(t.Context(), map[string]any{
		"_credential": map[string]string{
			"host":     "localhost",
			"user":     "root",
			"password": "secret",
			"database": "mydb",
		},
	})
	if err == nil {
		t.Fatal("expected error for missing query")
	}
}

func TestMySQLQueryConnector_MissingCredential(t *testing.T) {
	c := &MySQLQueryConnector{}
	_, err := c.Execute(t.Context(), map[string]any{
		"query": "SELECT 1",
	})
	if err == nil {
		t.Fatal("expected error for missing credential")
	}
}

func TestMySQLQueryConnector_MissingHost(t *testing.T) {
	c := &MySQLQueryConnector{}
	_, err := c.Execute(t.Context(), map[string]any{
		"query": "SELECT 1",
		"_credential": map[string]string{
			"user":     "root",
			"password": "secret",
			"database": "mydb",
		},
	})
	if err == nil {
		t.Fatal("expected error for missing host")
	}
}

func TestMySQLExecuteConnector_MissingStatement(t *testing.T) {
	c := &MySQLExecuteConnector{}
	_, err := c.Execute(t.Context(), map[string]any{
		"_credential": map[string]string{
			"host":     "localhost",
			"user":     "root",
			"password": "secret",
			"database": "mydb",
		},
	})
	if err == nil {
		t.Fatal("expected error for missing statement")
	}
}

func TestMySQLExecuteConnector_MissingCredential(t *testing.T) {
	c := &MySQLExecuteConnector{}
	_, err := c.Execute(t.Context(), map[string]any{
		"statement": "INSERT INTO t VALUES (1)",
	})
	if err == nil {
		t.Fatal("expected error for missing credential")
	}
}

func TestRegistry_MySQLConnectors(t *testing.T) {
	r := NewRegistry()
	for _, action := range []string{
		"mysql/query",
		"mysql/execute",
	} {
		if _, err := r.Get(action); err != nil {
			t.Errorf("%s not registered: %v", action, err)
		}
	}
}

// ---- MSSQL ----

func TestMSSQLQueryConnector_MissingQuery(t *testing.T) {
	c := &MSSQLQueryConnector{}
	_, err := c.Execute(t.Context(), map[string]any{
		"_credential": map[string]string{
			"host":     "localhost",
			"user":     "sa",
			"password": "secret",
			"database": "mydb",
		},
	})
	if err == nil {
		t.Fatal("expected error for missing query")
	}
}

func TestMSSQLQueryConnector_MissingCredential(t *testing.T) {
	c := &MSSQLQueryConnector{}
	_, err := c.Execute(t.Context(), map[string]any{
		"query": "SELECT 1",
	})
	if err == nil {
		t.Fatal("expected error for missing credential")
	}
}

func TestMSSQLExecuteConnector_MissingStatement(t *testing.T) {
	c := &MSSQLExecuteConnector{}
	_, err := c.Execute(t.Context(), map[string]any{
		"_credential": map[string]string{
			"host":     "localhost",
			"user":     "sa",
			"password": "secret",
			"database": "mydb",
		},
	})
	if err == nil {
		t.Fatal("expected error for missing statement")
	}
}

func TestMSSQLExecuteConnector_MissingCredential(t *testing.T) {
	c := &MSSQLExecuteConnector{}
	_, err := c.Execute(t.Context(), map[string]any{
		"statement": "INSERT INTO t VALUES (1)",
	})
	if err == nil {
		t.Fatal("expected error for missing credential")
	}
}

func TestRegistry_MSSQLConnectors(t *testing.T) {
	r := NewRegistry()
	for _, action := range []string{
		"mssql/query",
		"mssql/execute",
	} {
		if _, err := r.Get(action); err != nil {
			t.Errorf("%s not registered: %v", action, err)
		}
	}
}

// ---- Redshift ----

func TestRedshiftQueryConnector_MissingQuery(t *testing.T) {
	c := &RedshiftQueryConnector{}
	_, err := c.Execute(t.Context(), map[string]any{
		"_credential": map[string]string{
			"host":     "my-cluster.us-east-1.redshift.amazonaws.com",
			"user":     "awsuser",
			"password": "secret",
			"database": "dev",
		},
	})
	if err == nil {
		t.Fatal("expected error for missing query")
	}
}

func TestRedshiftQueryConnector_MissingCredential(t *testing.T) {
	c := &RedshiftQueryConnector{}
	_, err := c.Execute(t.Context(), map[string]any{
		"query": "SELECT 1",
	})
	if err == nil {
		t.Fatal("expected error for missing credential")
	}
}

func TestRegistry_RedshiftConnectors(t *testing.T) {
	r := NewRegistry()
	for _, action := range []string{
		"redshift/query",
	} {
		if _, err := r.Get(action); err != nil {
			t.Errorf("%s not registered: %v", action, err)
		}
	}
}
