package server

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/dvflw/mantle/internal/auth"
)

// CronScheduler polls the database for cron triggers and executes workflows on schedule.
type CronScheduler struct {
	server *Server
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// NewCronScheduler creates a cron scheduler attached to the server.
func NewCronScheduler(s *Server) *CronScheduler {
	return &CronScheduler{server: s}
}

// Start begins the cron polling loop. It checks for due triggers every 30 seconds.
func (c *CronScheduler) Start(ctx context.Context) error {
	ctx, c.cancel = context.WithCancel(ctx)
	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		c.run(ctx)
	}()
	return nil
}

// Stop halts the cron scheduler and waits for the polling loop to exit.
func (c *CronScheduler) Stop() {
	if c.cancel != nil {
		c.cancel()
	}
	c.wg.Wait()
}

func (c *CronScheduler) run(ctx context.Context) {
	// Run once immediately, then on a ticker.
	c.tick(ctx)

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			c.tick(ctx)
		}
	}
}

func (c *CronScheduler) tick(ctx context.Context) {
	// Acquire a Postgres advisory lock (non-blocking) so that only one replica
	// fires cron triggers when replicaCount > 1. Lock ID 42 is arbitrary but
	// must be consistent across all replicas.
	var acquired bool
	if err := c.server.DB.QueryRowContext(ctx, "SELECT pg_try_advisory_lock(42)").Scan(&acquired); err != nil {
		c.server.Logger.Error("cron: advisory lock query failed", "error", err)
		return
	}
	if !acquired {
		return // Another replica holds the lock; skip this cycle.
	}
	defer c.server.DB.ExecContext(ctx, "SELECT pg_advisory_unlock(42)")

	triggers, err := ListCronTriggers(ctx, c.server.DB)
	if err != nil {
		c.server.Logger.Error("cron: listing triggers", "error", err)
		return
	}

	now := time.Now()
	for _, t := range triggers {
		if shouldFire(t.Schedule, now) {
			c.server.Logger.Info("cron: firing workflow",
				"workflow", t.WorkflowName,
				"version", t.WorkflowVersion,
				"schedule", t.Schedule,
				"team_id", t.TeamID)

			// Inject team context so executeWorkflow runs with proper tenant scoping.
			teamCtx := auth.WithUser(ctx, &auth.User{TeamID: t.TeamID})
			execID, err := c.server.executeWorkflow(teamCtx, t.WorkflowName, t.WorkflowVersion, nil)
			if err != nil {
				c.server.Logger.Error("cron: execution failed",
					"workflow", t.WorkflowName,
					"error", err)
				continue
			}
			c.server.Logger.Info("cron: execution started",
				"workflow", t.WorkflowName,
				"execution_id", execID)
		}
	}
}

// shouldFire checks if a cron expression matches the current minute.
// Supports standard 5-field cron: minute hour day-of-month month day-of-week
// Supports: *, */N, N, N-M, comma-separated values.
func shouldFire(schedule string, now time.Time) bool {
	fields := strings.Fields(schedule)
	if len(fields) != 5 {
		return false
	}

	checks := []int{now.Minute(), now.Hour(), now.Day(), int(now.Month()), int(now.Weekday())}
	maxVals := []int{59, 23, 31, 12, 6}

	for i, field := range fields {
		if !matchField(field, checks[i], maxVals[i]) {
			return false
		}
	}
	return true
}

func matchField(field string, value, max int) bool {
	// Handle comma-separated values.
	for _, part := range strings.Split(field, ",") {
		if matchPart(part, value, max) {
			return true
		}
	}
	return false
}

func matchPart(part string, value, max int) bool {
	// Wildcard.
	if part == "*" {
		return true
	}

	// Step: */N or N-M/S
	if strings.Contains(part, "/") {
		pieces := strings.SplitN(part, "/", 2)
		step, err := strconv.Atoi(pieces[1])
		if err != nil || step <= 0 {
			return false
		}
		if pieces[0] == "*" {
			return value%step == 0
		}
		// Range with step.
		rangeParts := strings.SplitN(pieces[0], "-", 2)
		if len(rangeParts) == 2 {
			start, _ := strconv.Atoi(rangeParts[0])
			end, _ := strconv.Atoi(rangeParts[1])
			if value < start || value > end {
				return false
			}
			return (value-start)%step == 0
		}
		return false
	}

	// Range: N-M
	if strings.Contains(part, "-") {
		rangeParts := strings.SplitN(part, "-", 2)
		start, err1 := strconv.Atoi(rangeParts[0])
		end, err2 := strconv.Atoi(rangeParts[1])
		if err1 != nil || err2 != nil {
			return false
		}
		return value >= start && value <= end
	}

	// Exact value.
	n, err := strconv.Atoi(part)
	if err != nil {
		return false
	}
	return value == n
}

// FormatCronDescription returns a human-readable description of a cron schedule.
func FormatCronDescription(schedule string) string {
	fields := strings.Fields(schedule)
	if len(fields) != 5 {
		return schedule
	}
	if schedule == "* * * * *" {
		return "every minute"
	}
	if fields[0] == "0" && fields[1] == "*" && fields[2] == "*" && fields[3] == "*" && fields[4] == "*" {
		return "every hour"
	}
	if strings.HasPrefix(fields[0], "*/") {
		n := strings.TrimPrefix(fields[0], "*/")
		return fmt.Sprintf("every %s minutes", n)
	}
	return schedule
}
