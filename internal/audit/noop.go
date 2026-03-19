package audit

import "context"

// NoopEmitter discards all audit events. Used in V1 where audit storage is not yet implemented.
type NoopEmitter struct{}

func (n *NoopEmitter) Emit(_ context.Context, _ Event) error {
	return nil
}
