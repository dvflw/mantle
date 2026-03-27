package artifact

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"time"
)

// Reaper cleans up expired artifacts from both storage and the database.
type Reaper struct {
	Store      *Store
	Storage    Storage
	Retention  time.Duration
	Logger     *slog.Logger
}

// Sweep finds and removes artifacts older than the retention period.
// Returns the number of artifacts cleaned up.
func (r *Reaper) Sweep(ctx context.Context) (int, error) {
	if r.Retention <= 0 {
		return 0, nil // cleanup disabled
	}

	logger := r.Logger
	if logger == nil {
		logger = slog.Default()
	}

	expired, err := r.Store.ListExpired(ctx, r.Retention)
	if err != nil {
		return 0, fmt.Errorf("listing expired artifacts: %w", err)
	}

	if len(expired) == 0 {
		return 0, nil
	}

	cleaned := 0
	for _, a := range expired {
		// Delete file from storage.
		if delErr := r.Storage.Delete(ctx, a.URL); delErr != nil {
			// If the blob is already gone, still clean up the metadata.
			if !errors.Is(delErr, os.ErrNotExist) {
				logger.Error("failed to delete artifact file",
					"artifact", a.Name, "url", a.URL, "error", delErr)
				continue // skip metadata deletion if file delete genuinely failed
			}
			logger.Warn("artifact file already missing, cleaning metadata",
				"artifact", a.Name, "url", a.URL)
		}

		// Delete metadata from database.
		if err := r.Store.DeleteByID(ctx, a.ID); err != nil {
			logger.Error("failed to delete artifact metadata",
				"artifact", a.Name, "id", a.ID, "error", err)
			continue
		}
		cleaned++
		logger.Info("cleaned expired artifact",
			"artifact", a.Name, "execution_id", a.ExecutionID)
	}

	return cleaned, nil
}
