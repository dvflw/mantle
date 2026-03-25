package connector

import (
	"context"
	"fmt"
)

// DefaultMaxResponseBytes is the maximum number of bytes read from any HTTP
// response body across all connectors. This prevents OOM from large or
// malicious responses. 10 MB.
const DefaultMaxResponseBytes = 10 * 1024 * 1024

// Connector executes an action with the given parameters and returns output data.
type Connector interface {
	Execute(ctx context.Context, params map[string]any) (map[string]any, error)
}

// Registry maps action names to connector implementations.
type Registry struct {
	connectors map[string]Connector
}

// NewRegistry creates a registry with the built-in connectors registered.
func NewRegistry() *Registry {
	r := &Registry{
		connectors: make(map[string]Connector),
	}
	r.Register("http/request", &HTTPConnector{})
	r.Register("ai/completion", &AIConnector{})
	r.Register("slack/send", &SlackSendConnector{})
	r.Register("slack/history", &SlackHistoryConnector{})
	r.Register("postgres/query", &PostgresQueryConnector{})
	r.Register("email/send", &EmailSendConnector{})
	r.Register("s3/put", &S3PutConnector{})
	r.Register("s3/get", &S3GetConnector{})
	r.Register("s3/list", &S3ListConnector{})
	r.Register("docker/run", &DockerRunConnector{})
	return r
}

// Register adds a connector for the given action name.
func (r *Registry) Register(action string, c Connector) {
	r.connectors[action] = c
}

// Get returns the connector for the given action, or an error if not found.
func (r *Registry) Get(action string) (Connector, error) {
	c, ok := r.connectors[action]
	if !ok {
		return nil, fmt.Errorf("unknown action %q", action)
	}
	return c, nil
}
