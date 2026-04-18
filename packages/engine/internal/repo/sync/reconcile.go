package sync

import (
	"context"
	"errors"
	"fmt"

	"github.com/dvflw/mantle/internal/config"
	"github.com/dvflw/mantle/internal/repo"
)

// Reconcile applies every entry in cfg as either a create (name new to
// the DB) or an update (name already present). Repos that exist in the
// DB but not in cfg are untouched — operators may have registered them
// via `mantle repos add` and we refuse to silently nuke their work.
func Reconcile(ctx context.Context, store *repo.Store, cfg []config.GitSyncRepo) error {
	for _, entry := range cfg {
		existing, err := store.Get(ctx, entry.Name)
		if errors.Is(err, repo.ErrNotFound) {
			_, createErr := store.Create(ctx, repo.CreateParams{
				Name:         entry.Name,
				URL:          entry.URL,
				Branch:       defaultString(entry.Branch, "main"),
				Path:         defaultString(entry.Path, "/"),
				PollInterval: defaultString(entry.PollInterval, "60s"),
				Credential:   entry.Credential,
				AutoApply:    entry.AutoApply,
				Prune:        entry.Prune,
			})
			if createErr != nil {
				return fmt.Errorf("reconcile create %q: %w", entry.Name, createErr)
			}
			continue
		}
		if err != nil {
			return fmt.Errorf("reconcile lookup %q: %w", entry.Name, err)
		}
		_, err = store.Update(ctx, entry.Name, repo.UpdateParams{
			Branch:       defaultString(entry.Branch, "main"),
			Path:         defaultString(entry.Path, "/"),
			PollInterval: defaultString(entry.PollInterval, "60s"),
			Credential:   entry.Credential,
			AutoApply:    entry.AutoApply,
			Prune:        entry.Prune,
			Enabled:      existing.Enabled,
		})
		if err != nil {
			return fmt.Errorf("reconcile update %q: %w", entry.Name, err)
		}
	}
	return nil
}

// defaultString returns fallback when v is empty, so config entries that
// omit a field pick up the same defaults the DB schema uses.
func defaultString(v, fallback string) string {
	if v == "" {
		return fallback
	}
	return v
}
