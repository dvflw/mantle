package engine

import (
	"context"
	"encoding/json"
	"log/slog"
	"math/rand"
	"sync/atomic"
	"time"

	"github.com/dvflw/mantle/internal/metrics"
)

// StepExecutor is called by the worker to execute a step.
type StepExecutor func(ctx context.Context, stepName string, attempt int) (map[string]any, error)

// executionIDKey is the context key for passing the execution ID to the StepExecutor.
type executionIDKey struct{}

// WithExecutionID returns a context that carries the given execution ID.
func WithExecutionID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, executionIDKey{}, id)
}

// ExecutionIDFromContext extracts the execution ID from the context, or returns
// an empty string if none is present.
func ExecutionIDFromContext(ctx context.Context) string {
	id, _ := ctx.Value(executionIDKey{}).(string)
	return id
}

// Worker polls for pending steps and executes them with lease renewal.
type Worker struct {
	Claimer            *Claimer
	StepExecutor       StepExecutor
	PollInterval       time.Duration
	MaxBackoff         time.Duration
	LeaseRenewInterval time.Duration
	Logger             *slog.Logger

	// Liveness tracking.
	lastPollAt atomic.Int64 // unix nanos of last poll cycle
	degraded   atomic.Bool  // set if the Run goroutine panicked and recovered
}

// LastPollAt returns the time of the last poll cycle.
func (w *Worker) LastPollAt() time.Time {
	nanos := w.lastPollAt.Load()
	if nanos == 0 {
		return time.Time{}
	}
	return time.Unix(0, nanos)
}

// IsAlive returns true if the worker has polled within the given threshold.
func (w *Worker) IsAlive(threshold time.Duration) bool {
	if w.degraded.Load() {
		return false
	}
	last := w.LastPollAt()
	if last.IsZero() {
		return true // not yet started
	}
	return time.Since(last) <= threshold
}

// Run is a blocking loop that claims and executes steps until ctx is cancelled.
func (w *Worker) Run(ctx context.Context) {
	defer func() {
		if r := recover(); r != nil {
			w.degraded.Store(true)
			if w.Logger != nil {
				w.Logger.Error("worker panicked, entering degraded state", "panic", r)
			}
		}
	}()

	backoff := w.PollInterval
	if backoff == 0 {
		backoff = time.Second
	}
	if w.MaxBackoff == 0 {
		w.MaxBackoff = 30 * time.Second
	}
	if w.Logger == nil {
		w.Logger = slog.Default()
	}

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		w.lastPollAt.Store(time.Now().UnixNano())

		pollStart := time.Now()
		claim, executionID, err := w.Claimer.ClaimAnyStep(ctx)
		if err != nil {
			w.Logger.Error("claim error", "error", err)
			w.sleep(ctx, backoff)
			backoff = w.nextBackoff(backoff)
			continue
		}

		if claim == nil {
			// No work available — back off.
			w.sleep(ctx, backoff)
			backoff = w.nextBackoff(backoff)
			continue
		}

		// Work found — record claim duration and reset backoff.
		metrics.RecordClaimDuration(time.Since(pollStart))
		backoff = w.PollInterval
		if backoff == 0 {
			backoff = time.Second
		}

		w.Logger.Info("claimed step",
			"step_id", claim.ID,
			"step_name", claim.StepName,
			"execution_id", executionID,
			"attempt", claim.Attempt,
		)

		// Inject execution ID into context so StepExecutor can look up
		// the workflow definition and CEL context for this execution.
		stepCtx := WithExecutionID(ctx, executionID)
		w.executeWithLeaseRenewal(stepCtx, claim)
	}
}

// executeWithLeaseRenewal runs the step executor while renewing the lease in
// a background goroutine. If lease renewal fails, the execution context is
// cancelled to prevent stale work.
func (w *Worker) executeWithLeaseRenewal(ctx context.Context, claim *StepClaim) {
	execCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	renewInterval := w.LeaseRenewInterval
	if renewInterval == 0 {
		renewInterval = w.Claimer.LeaseDuration / 3
	}
	if renewInterval <= 0 {
		renewInterval = 10 * time.Second
	}

	// Start lease renewal goroutine.
	renewDone := make(chan struct{})
	go func() {
		defer close(renewDone)
		ticker := time.NewTicker(renewInterval)
		defer ticker.Stop()
		for {
			select {
			case <-execCtx.Done():
				return
			case <-ticker.C:
				ok, err := w.Claimer.RenewLease(execCtx, claim.ID)
				if err != nil {
					w.Logger.Error("lease renewal error",
						"step_id", claim.ID,
						"error", err,
					)
					cancel()
					return
				}
				if !ok {
					w.Logger.Warn("lease lost, cancelling execution",
						"step_id", claim.ID,
					)
					cancel()
					return
				}
				metrics.RecordLeaseRenewal()
			}
		}
	}()

	// Execute the step.
	output, stepErr := w.StepExecutor(execCtx, claim.StepName, claim.Attempt)

	// Stop lease renewal.
	cancel()
	<-renewDone

	// Marshal output for CompleteStep.
	var outputJSON []byte
	if output != nil {
		var err error
		outputJSON, err = json.Marshal(output)
		if err != nil {
			w.Logger.Error("marshal output error",
				"step_id", claim.ID,
				"error", err,
			)
		}
	}

	// Complete the step — fencing token protects against stale completion.
	ok, err := w.Claimer.CompleteStep(ctx, claim.ID, outputJSON, stepErr)
	if err != nil {
		w.Logger.Error("complete step error",
			"step_id", claim.ID,
			"error", err,
		)
		return
	}
	if !ok {
		w.Logger.Warn("step completion rejected (fencing)",
			"step_id", claim.ID,
		)
		return
	}

	status := "completed"
	if stepErr != nil {
		status = "failed"
	}
	w.Logger.Info("step finished",
		"step_id", claim.ID,
		"status", status,
	)
}

// nextBackoff calculates the next backoff duration with exponential growth and
// +/-25% jitter, capped at MaxBackoff.
func (w *Worker) nextBackoff(current time.Duration) time.Duration {
	next := current * 2
	if next > w.MaxBackoff {
		next = w.MaxBackoff
	}
	// Apply jitter: multiply by a random factor in [0.75, 1.25).
	jitter := time.Duration(float64(next) * (0.75 + rand.Float64()*0.5))
	return jitter
}

// sleep waits for the given duration or until ctx is cancelled.
func (w *Worker) sleep(ctx context.Context, d time.Duration) {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
	case <-timer.C:
	}
}
