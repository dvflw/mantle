package artifact

import (
	"context"
	"fmt"
	"log/slog"
	"time"
)

// Reaper cleans up expired artifacts from both tmp storage and the database.
type Reaper struct {
	Store      *Store
	TmpStorage TmpStorage
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

	// Group by execution for batch deletion.
	byExecution := make(map[string][]Artifact)
	for _, a := range expired {
		byExecution[a.ExecutionID] = append(byExecution[a.ExecutionID], a)
	}

	cleaned := 0
	for execID, arts := range byExecution {
		// Delete files from tmp storage.
		for _, a := range arts {
			if delErr := r.TmpStorage.DeleteByPrefix(ctx, a.URL); delErr != nil {
				logger.Error("failed to delete artifact file",
					"artifact", a.Name, "url", a.URL, "error", delErr)
			}
		}

		// Delete metadata from database.
		if err := r.Store.DeleteByExecution(ctx, execID); err != nil {
			logger.Error("failed to delete artifact metadata",
				"execution_id", execID, "error", err)
			continue
		}
		cleaned += len(arts)
		logger.Info("cleaned expired artifacts",
			"execution_id", execID, "count", len(arts))
	}

	return cleaned, nil
}
