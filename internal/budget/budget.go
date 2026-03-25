package budget

import (
	"context"
	"fmt"
	"time"
)

// Reset mode constants for budget period calculation.
const (
	ResetModeCalendar = "calendar"
	ResetModeRolling  = "rolling"
)

// CurrentPeriodStart returns the start date of the current budget period.
// For "calendar" mode: first day of the current month.
// For "rolling" mode: the most recent occurrence of resetDay (1-28).
func CurrentPeriodStart(now time.Time, mode string, resetDay int) time.Time {
	now = now.UTC()
	if mode == ResetModeRolling && resetDay >= 1 && resetDay <= 28 {
		if now.Day() >= resetDay {
			return time.Date(now.Year(), now.Month(), resetDay, 0, 0, 0, 0, time.UTC)
		}
		prev := now.AddDate(0, -1, 0)
		return time.Date(prev.Year(), prev.Month(), resetDay, 0, 0, 0, 0, time.UTC)
	}
	return time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
}

// CheckInput describes the context of a budget check.
type CheckInput struct {
	TeamID   string
	Provider string
}

// CheckResult describes the outcome of a budget check.
type CheckResult struct {
	Blocked   bool
	Warning   bool
	BlockedBy string // "global", "team", "workflow", or ""
	Message   string
}

// Checker evaluates budget limits before AI step dispatch.
// Uses function fields for DB access to allow easy testing with mocks.
type Checker struct {
	GlobalMonthlyTokenLimit      int64
	DefaultTeamMonthlyTokenLimit int64
	ResetMode                    string
	ResetDay                     int

	GetTotalUsage    func(ctx context.Context, period time.Time) (int64, error)
	GetProviderUsage func(ctx context.Context, teamID, provider string, period time.Time) (int64, error)
	GetTeamBudget    func(ctx context.Context, teamID, provider string) (*TeamBudget, error)
}

// Check evaluates all budget levels (global, team+provider) and returns the result.
// Budget levels compose as AND gates — all must pass.
func (c *Checker) Check(ctx context.Context, input CheckInput) CheckResult {
	now := time.Now()
	period := CurrentPeriodStart(now, c.ResetMode, c.ResetDay)

	// 1. Global budget — hard block only
	if c.GlobalMonthlyTokenLimit > 0 && c.GetTotalUsage != nil {
		total, err := c.GetTotalUsage(ctx, period)
		if err == nil && total >= c.GlobalMonthlyTokenLimit {
			return CheckResult{
				Blocked:   true,
				BlockedBy: "global",
				Message:   fmt.Sprintf("global monthly token limit exceeded (%d/%d)", total, c.GlobalMonthlyTokenLimit),
			}
		}
	}

	// 2. Team+provider budget — configurable enforcement
	if c.GetTeamBudget != nil && c.GetProviderUsage != nil {
		tb, err := c.GetTeamBudget(ctx, input.TeamID, input.Provider)
		if err == nil && tb == nil {
			tb, err = c.GetTeamBudget(ctx, input.TeamID, "*")
		}
		if err == nil && tb == nil && c.DefaultTeamMonthlyTokenLimit > 0 {
			tb = &TeamBudget{
				MonthlyTokenLimit: c.DefaultTeamMonthlyTokenLimit,
				Enforcement:       "hard",
			}
		}
		if err == nil && tb != nil && tb.MonthlyTokenLimit > 0 {
			usage, err := c.GetProviderUsage(ctx, input.TeamID, input.Provider, period)
			if err == nil && usage >= tb.MonthlyTokenLimit {
				if tb.Enforcement == "warn" {
					return CheckResult{
						Warning:   true,
						BlockedBy: "team",
						Message:   fmt.Sprintf("team budget exceeded for provider %s (%d/%d tokens) — enforcement is warn-only", input.Provider, usage, tb.MonthlyTokenLimit),
					}
				}
				return CheckResult{
					Blocked:   true,
					BlockedBy: "team",
					Message:   fmt.Sprintf("team budget exceeded for provider %s (%d/%d tokens)", input.Provider, usage, tb.MonthlyTokenLimit),
				}
			}
		}
	}

	return CheckResult{}
}

// CheckExecutionBudget checks if a workflow execution's cumulative token usage
// has exceeded the workflow-level token_budget. budgetLimit of 0 means unlimited.
func CheckExecutionBudget(budgetLimit int64, usedTokens int64) CheckResult {
	if budgetLimit <= 0 {
		return CheckResult{}
	}
	if usedTokens >= budgetLimit {
		return CheckResult{
			Blocked:   true,
			BlockedBy: "workflow",
			Message:   fmt.Sprintf("workflow token budget exceeded (%d/%d)", usedTokens, budgetLimit),
		}
	}
	return CheckResult{}
}
