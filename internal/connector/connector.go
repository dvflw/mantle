package connector

import (
	"context"
	"fmt"
)

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
