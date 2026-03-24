package budget_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/dvflw/mantle/internal/budget"
	"github.com/stretchr/testify/assert"
)

func TestCurrentPeriodStart_Calendar(t *testing.T) {
	now := time.Date(2026, 3, 23, 14, 30, 0, 0, time.UTC)
	start := budget.CurrentPeriodStart(now, "calendar", 1)
	assert.Equal(t, time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC), start)
}

func TestCurrentPeriodStart_Rolling(t *testing.T) {
	now := time.Date(2026, 3, 10, 14, 30, 0, 0, time.UTC)
	start := budget.CurrentPeriodStart(now, "rolling", 15)
	assert.Equal(t, time.Date(2026, 2, 15, 0, 0, 0, 0, time.UTC), start)

	now = time.Date(2026, 3, 20, 14, 30, 0, 0, time.UTC)
	start = budget.CurrentPeriodStart(now, "rolling", 15)
	assert.Equal(t, time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC), start)

	now = time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC)
	start = budget.CurrentPeriodStart(now, "rolling", 15)
	assert.Equal(t, time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC), start)
}

func TestCurrentPeriodStart_RollingDay28_February(t *testing.T) {
	now := time.Date(2026, 3, 10, 0, 0, 0, 0, time.UTC)
	start := budget.CurrentPeriodStart(now, "rolling", 28)
	assert.Equal(t, time.Date(2026, 2, 28, 0, 0, 0, 0, time.UTC), start)
}

func TestChecker_Check_GlobalHardBlock(t *testing.T) {
	checker := &budget.Checker{
		GlobalMonthlyTokenLimit: 1000,
		ResetMode:               "calendar",
		ResetDay:                1,
		GetTotalUsage: func(ctx context.Context, period time.Time) (int64, error) {
			return 1001, nil
		},
	}

	result := checker.Check(context.Background(), budget.CheckInput{
		TeamID:   "team-1",
		Provider: "openai",
	})

	assert.True(t, result.Blocked)
	assert.Equal(t, "global", result.BlockedBy)
	assert.Contains(t, result.Message, "global monthly token limit")
}

func TestChecker_Check_TeamWarnOnly(t *testing.T) {
	checker := &budget.Checker{
		ResetMode: "calendar",
		ResetDay:  1,
		GetTotalUsage: func(ctx context.Context, period time.Time) (int64, error) {
			return 0, nil
		},
		GetProviderUsage: func(ctx context.Context, teamID, provider string, period time.Time) (int64, error) {
			return 5001, nil
		},
		GetTeamBudget: func(ctx context.Context, teamID, provider string) (*budget.TeamBudget, error) {
			return &budget.TeamBudget{MonthlyTokenLimit: 5000, Enforcement: "warn"}, nil
		},
	}

	result := checker.Check(context.Background(), budget.CheckInput{
		TeamID:   "team-1",
		Provider: "openai",
	})

	assert.False(t, result.Blocked)
	assert.True(t, result.Warning)
	assert.Contains(t, result.Message, "team budget exceeded")
}

func TestChecker_Check_AllPass(t *testing.T) {
	checker := &budget.Checker{
		GlobalMonthlyTokenLimit: 100000,
		ResetMode:               "calendar",
		ResetDay:                1,
		GetTotalUsage: func(ctx context.Context, period time.Time) (int64, error) {
			return 500, nil
		},
		GetProviderUsage: func(ctx context.Context, teamID, provider string, period time.Time) (int64, error) {
			return 200, nil
		},
		GetTeamBudget: func(ctx context.Context, teamID, provider string) (*budget.TeamBudget, error) {
			return &budget.TeamBudget{MonthlyTokenLimit: 10000, Enforcement: "hard"}, nil
		},
	}

	result := checker.Check(context.Background(), budget.CheckInput{
		TeamID:   "team-1",
		Provider: "openai",
	})

	assert.False(t, result.Blocked)
	assert.False(t, result.Warning)
}

func TestChecker_Check_FailOpenOnGetTotalUsageError(t *testing.T) {
	checker := &budget.Checker{
		GlobalMonthlyTokenLimit: 1000,
		ResetMode:               "calendar",
		ResetDay:                1,
		GetTotalUsage: func(ctx context.Context, period time.Time) (int64, error) {
			return 0, fmt.Errorf("db connection lost")
		},
	}

	result := checker.Check(context.Background(), budget.CheckInput{
		TeamID:   "team-1",
		Provider: "openai",
	})

	// Intentional fail-open: DB errors should not block AI steps.
	assert.False(t, result.Blocked)
	assert.False(t, result.Warning)
}

func TestChecker_Check_FailOpenOnGetTeamBudgetError(t *testing.T) {
	checker := &budget.Checker{
		ResetMode: "calendar",
		ResetDay:  1,
		GetTotalUsage: func(ctx context.Context, period time.Time) (int64, error) {
			return 0, nil
		},
		GetProviderUsage: func(ctx context.Context, teamID, provider string, period time.Time) (int64, error) {
			return 0, nil
		},
		GetTeamBudget: func(ctx context.Context, teamID, provider string) (*budget.TeamBudget, error) {
			return nil, fmt.Errorf("db timeout")
		},
	}

	result := checker.Check(context.Background(), budget.CheckInput{
		TeamID:   "team-1",
		Provider: "openai",
	})

	assert.False(t, result.Blocked)
	assert.False(t, result.Warning)
}

func TestChecker_Check_FailOpenOnGetProviderUsageError(t *testing.T) {
	checker := &budget.Checker{
		ResetMode: "calendar",
		ResetDay:  1,
		GetTotalUsage: func(ctx context.Context, period time.Time) (int64, error) {
			return 0, nil
		},
		GetProviderUsage: func(ctx context.Context, teamID, provider string, period time.Time) (int64, error) {
			return 0, fmt.Errorf("provider usage query failed")
		},
		GetTeamBudget: func(ctx context.Context, teamID, provider string) (*budget.TeamBudget, error) {
			return &budget.TeamBudget{MonthlyTokenLimit: 5000, Enforcement: "hard"}, nil
		},
	}

	result := checker.Check(context.Background(), budget.CheckInput{
		TeamID:   "team-1",
		Provider: "openai",
	})

	assert.False(t, result.Blocked)
	assert.False(t, result.Warning)
}

func TestChecker_Check_TeamHardBlock(t *testing.T) {
	checker := &budget.Checker{
		ResetMode: "calendar",
		ResetDay:  1,
		GetTotalUsage: func(ctx context.Context, period time.Time) (int64, error) {
			return 0, nil
		},
		GetProviderUsage: func(ctx context.Context, teamID, provider string, period time.Time) (int64, error) {
			return 5001, nil
		},
		GetTeamBudget: func(ctx context.Context, teamID, provider string) (*budget.TeamBudget, error) {
			return &budget.TeamBudget{MonthlyTokenLimit: 5000, Enforcement: "hard"}, nil
		},
	}

	result := checker.Check(context.Background(), budget.CheckInput{
		TeamID:   "team-1",
		Provider: "openai",
	})

	assert.True(t, result.Blocked)
	assert.Equal(t, "team", result.BlockedBy)
	assert.Contains(t, result.Message, "team budget exceeded")
}

func TestChecker_CheckExecutionBudget(t *testing.T) {
	result := budget.CheckExecutionBudget(50000, 50001)
	assert.True(t, result.Blocked)
	assert.Equal(t, "workflow", result.BlockedBy)

	result = budget.CheckExecutionBudget(50000, 50000)
	assert.True(t, result.Blocked)
	assert.Equal(t, "workflow", result.BlockedBy)

	result = budget.CheckExecutionBudget(50000, 49999)
	assert.False(t, result.Blocked)

	result = budget.CheckExecutionBudget(0, 999999)
	assert.False(t, result.Blocked)
}
