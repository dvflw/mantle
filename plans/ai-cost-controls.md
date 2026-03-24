# AI Cost Controls Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement per-provider, per-team, and per-workflow token budget enforcement with configurable reset windows, hard/soft blocking, and audit trail integration.

**Architecture:** A new `ai_token_usage` counter table tracks cumulative tokens per (team, provider, model, period). Budget enforcement runs as a pre-dispatch check in `executeStep()` before any `ai/*` action. Three budget levels compose as AND gates: workflow-level (per-execution, YAML field), team+provider (monthly, DB-stored), and global (monthly, config file). Global budgets hard-block; team/workflow budgets are configurable (block or warn). A CLI command and API endpoint manage team budgets. Configurable reset windows support calendar month or rolling periods with a custom start day.

**Tech Stack:** Go, Postgres, Cobra CLI, Prometheus metrics, audit events

**GitHub Issue:** #8

---

## Design Decisions (from /grill-me session)

1. **Token-based budgets** — not dollar-based. Docs link to provider pricing pages.
2. **Three levels, AND-gated**: workflow (per-execution), team+provider (monthly), global (monthly). All must pass.
3. **Workflow budget** is a top-level YAML field (`token_budget`). Existing step-level `max_token_budget` is unchanged.
4. **Enforcement before next AI step dispatch**, not mid-step. Tokens already spent are never wasted.
5. **Global budget = hard block.** Team/workflow budgets are configurable: hard block or warn-only (log + audit event + metrics).
6. **Counter table** `ai_token_usage(team_id, provider, model, period_start)` — atomically incremented after each AI step. Budget enforcement reads from this.
7. **Configurable reset window** — calendar month or rolling with a start day (1-28). Global config in `mantle.yaml`.
8. **Three config surfaces**: `mantle.yaml` for global defaults, CLI command for team budgets, API endpoint for programmatic access.
9. **Enforcement check lives in `executeStep()`** in the engine, before dispatching `ai/*` actions. One DB read per AI step (negligible vs. LLM call cost).

---

## File Structure

| File | Responsibility |
|------|---------------|
| **Create:** `internal/budget/budget.go` | `BudgetChecker` interface, `TokenBudgetChecker` implementation, budget period calculation, enforcement logic |
| **Create:** `internal/budget/budget_test.go` | Unit tests for period calculation, enforcement logic, AND-gate composition |
| **Create:** `internal/budget/store.go` | DB operations: increment counters, query usage, CRUD for team budgets |
| **Create:** `internal/budget/store_test.go` | Integration tests against real Postgres (testcontainers) |
| **Create:** `internal/db/migrations/012_ai_cost_controls.sql` | `ai_token_usage` counter table, `team_budgets` table |
| **Create:** `internal/cli/budget.go` | `mantle budget` CLI subcommands (set, get, usage) |
| **Create:** `internal/cli/budget_test.go` | CLI command tests |
| **Modify:** `internal/config/config.go` | Add `BudgetConfig` to `EngineConfig` (global budget, reset mode, default team budget) |
| **Modify:** `internal/workflow/workflow.go` | Add `TokenBudget` field to `Workflow` struct |
| **Modify:** `internal/workflow/workflow_test.go` | Test parsing of `token_budget` field |
| **Modify:** `internal/engine/engine.go` | Inject `BudgetChecker`, call pre-dispatch check before AI steps, record usage after step completion |
| **Modify:** `internal/engine/engine_test.go` | Test budget enforcement in engine |
| **Modify:** `internal/server/api.go` | Add team budget API endpoints |
| **Modify:** `internal/server/server.go` | Register budget API routes |
| **Modify:** `internal/metrics/metrics.go` | Add budget enforcement metrics |
| **Modify:** `internal/audit/audit.go` | Add budget-related audit action constants |
| **Create:** `site/src/content/docs/ai-cost-controls.md` | User-facing docs: budget config, enforcement semantics, provider pricing links |

**Important:** Tasks 1-3 are the data foundation (migration, store, config). Task 4 adds the core enforcement logic. Task 5 wires it into the engine. Task 6 adds the workflow-level budget. Task 7 adds API endpoints. Task 8 adds the CLI. Task 9 adds docs. Tasks must be executed sequentially.

---

### Task 1: Database migration for token usage tracking and team budgets

**Files:**
- Create: `internal/db/migrations/012_ai_cost_controls.sql`

- [ ] **Step 1: Write the migration**

```sql
-- +goose Up

-- Tracks cumulative AI token usage per team/provider/model/period.
-- One row per (team, provider, model, period_start) — atomically incremented.
CREATE TABLE ai_token_usage (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    team_id         UUID NOT NULL REFERENCES teams(id),
    provider        TEXT NOT NULL,           -- "openai", "bedrock"
    model           TEXT NOT NULL,           -- "gpt-4o", "claude-3-sonnet", etc.
    period_start    DATE NOT NULL,           -- bucket start date
    prompt_tokens   BIGINT NOT NULL DEFAULT 0,
    completion_tokens BIGINT NOT NULL DEFAULT 0,
    total_tokens    BIGINT NOT NULL DEFAULT 0,
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(team_id, provider, model, period_start)
);

CREATE INDEX idx_ai_token_usage_team_period ON ai_token_usage(team_id, period_start);
CREATE INDEX idx_ai_token_usage_team_provider_period ON ai_token_usage(team_id, provider, period_start);

-- Per-team, per-provider budget configuration.
-- If no row exists for a team+provider, the global default applies.
CREATE TABLE team_budgets (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    team_id         UUID NOT NULL REFERENCES teams(id),
    provider        TEXT NOT NULL,           -- "openai", "bedrock", or "*" for all providers
    monthly_token_limit BIGINT NOT NULL,     -- 0 = unlimited
    enforcement     TEXT NOT NULL DEFAULT 'hard', -- "hard" (block) or "warn" (log + continue)
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(team_id, provider)
);

-- +goose Down
DROP TABLE IF EXISTS team_budgets;
DROP TABLE IF EXISTS ai_token_usage;
```

- [ ] **Step 2: Run migration to verify it applies cleanly**

Run: `make migrate`
Expected: Migration 012 applies without errors.

- [ ] **Step 3: Verify rollback works**

Run: `GOOSE_COMMAND=down make migrate` (or equivalent — check Makefile for down target)
Expected: Tables dropped cleanly.

- [ ] **Step 4: Re-apply and commit**

Run: `make migrate`

```bash
git add internal/db/migrations/012_ai_cost_controls.sql
git commit -m "feat(budget): add ai_token_usage and team_budgets tables (migration 012)"
```

---

### Task 2: Config — add budget settings to EngineConfig

**Files:**
- Modify: `internal/config/config.go`
- Modify: `internal/config/config_test.go` (if exists, otherwise create)

- [ ] **Step 1: Write the failing test**

Add a test that loads config with budget fields and asserts they unmarshal correctly.

```go
func TestLoad_BudgetDefaults(t *testing.T) {
	cmd := &cobra.Command{}
	cmd.Flags().String("config", "", "")

	cfg, err := Load(cmd)
	require.NoError(t, err)

	// Budget defaults
	assert.Equal(t, "calendar", cfg.Engine.Budget.ResetMode)
	assert.Equal(t, 1, cfg.Engine.Budget.ResetDay)
	assert.Equal(t, int64(0), cfg.Engine.Budget.GlobalMonthlyTokenLimit)
	assert.Equal(t, int64(0), cfg.Engine.Budget.DefaultTeamMonthlyTokenLimit)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/config/ -run TestLoad_BudgetDefaults -v`
Expected: FAIL — `cfg.Engine.Budget` field does not exist.

- [ ] **Step 3: Add BudgetConfig struct and wire into EngineConfig**

In `internal/config/config.go`, add:

```go
// BudgetConfig holds AI cost control settings.
type BudgetConfig struct {
	ResetMode                   string `mapstructure:"reset_mode"`                      // "calendar" or "rolling"
	ResetDay                    int    `mapstructure:"reset_day"`                        // 1-28, used when reset_mode is "rolling"
	GlobalMonthlyTokenLimit     int64  `mapstructure:"global_monthly_token_limit"`       // 0 = unlimited, hard block
	DefaultTeamMonthlyTokenLimit int64 `mapstructure:"default_team_monthly_token_limit"` // 0 = unlimited, applies to teams without explicit budget
}
```

Add `Budget BudgetConfig \`mapstructure:"budget"\`` to `EngineConfig`.

Add defaults in `Load()`:

```go
v.SetDefault("engine.budget.reset_mode", "calendar")
v.SetDefault("engine.budget.reset_day", 1)
v.SetDefault("engine.budget.global_monthly_token_limit", 0)
v.SetDefault("engine.budget.default_team_monthly_token_limit", 0)
```

Add env var bindings:

```go
_ = v.BindEnv("engine.budget.reset_mode", "MANTLE_ENGINE_BUDGET_RESET_MODE")
_ = v.BindEnv("engine.budget.reset_day", "MANTLE_ENGINE_BUDGET_RESET_DAY")
_ = v.BindEnv("engine.budget.global_monthly_token_limit", "MANTLE_ENGINE_BUDGET_GLOBAL_MONTHLY_TOKEN_LIMIT")
_ = v.BindEnv("engine.budget.default_team_monthly_token_limit", "MANTLE_ENGINE_BUDGET_DEFAULT_TEAM_MONTHLY_TOKEN_LIMIT")
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/config/ -run TestLoad_BudgetDefaults -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/config/config.go internal/config/config_test.go
git commit -m "feat(budget): add BudgetConfig to engine configuration"
```

---

### Task 3: Budget store — DB operations for token usage and team budgets

**Files:**
- Create: `internal/budget/store.go`
- Create: `internal/budget/store_test.go`

- [ ] **Step 1: Write the integration test for IncrementUsage**

Note: `testSetup(t)` should follow the testcontainers pattern from `internal/engine/test_helpers_test.go` — start a Postgres container, run migrations, and return a `*sql.DB`. Copy/adapt that helper.

```go
func TestStore_IncrementUsage(t *testing.T) {
	db := testSetup(t) // testcontainers Postgres with migrations applied

	store := budget.NewStore(db)
	ctx := context.Background()
	teamID := "00000000-0000-0000-0000-000000000001"
	period := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)

	// First increment creates the row
	err := store.IncrementUsage(ctx, teamID, "openai", "gpt-4o", period, 100, 50)
	require.NoError(t, err)

	// Second increment adds to existing row
	err = store.IncrementUsage(ctx, teamID, "openai", "gpt-4o", period, 200, 100)
	require.NoError(t, err)

	// Query usage for the period
	usage, err := store.GetUsage(ctx, teamID, "openai", period)
	require.NoError(t, err)
	assert.Equal(t, int64(300), usage.PromptTokens)     // 100 + 200
	assert.Equal(t, int64(150), usage.CompletionTokens)  // 50 + 100
	assert.Equal(t, int64(450), usage.TotalTokens)       // (100+50) + (200+100)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/budget/ -run TestStore_IncrementUsage -v`
Expected: FAIL — package/types don't exist.

- [ ] **Step 3: Implement the store**

Create `internal/budget/store.go`:

```go
package budget

import (
	"context"
	"database/sql"
	"time"
)

// ProviderUsage holds aggregated token usage for a team+provider in a period.
type ProviderUsage struct {
	Provider         string
	PromptTokens     int64
	CompletionTokens int64
	TotalTokens      int64
}

// TeamBudget holds a team's budget configuration for a provider.
type TeamBudget struct {
	ID                string `json:"id"`
	TeamID            string `json:"team_id"`
	Provider          string `json:"provider"`            // specific provider or "*" for all
	MonthlyTokenLimit int64  `json:"monthly_token_limit"`
	Enforcement       string `json:"enforcement"`         // "hard" or "warn"
}

// Store handles budget-related database operations.
type Store struct {
	db *sql.DB
}

// NewStore creates a new budget store.
func NewStore(db *sql.DB) *Store {
	return &Store{db: db}
}

// IncrementUsage atomically adds tokens to the counter for a (team, provider, model, period).
// Uses INSERT ... ON CONFLICT UPDATE to upsert.
func (s *Store) IncrementUsage(ctx context.Context, teamID, provider, model string, periodStart time.Time, promptTokens, completionTokens int64) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO ai_token_usage (team_id, provider, model, period_start, prompt_tokens, completion_tokens, total_tokens, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, now())
		ON CONFLICT (team_id, provider, model, period_start)
		DO UPDATE SET
			prompt_tokens = ai_token_usage.prompt_tokens + EXCLUDED.prompt_tokens,
			completion_tokens = ai_token_usage.completion_tokens + EXCLUDED.completion_tokens,
			total_tokens = ai_token_usage.total_tokens + EXCLUDED.total_tokens,
			updated_at = now()
	`, teamID, provider, model, periodStart, promptTokens, completionTokens, promptTokens+completionTokens)
	return err
}

// GetUsage returns aggregated token usage for a team+provider in a period.
// Sums across all models for the given provider.
func (s *Store) GetUsage(ctx context.Context, teamID, provider string, periodStart time.Time) (*ProviderUsage, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT COALESCE(SUM(prompt_tokens), 0),
		       COALESCE(SUM(completion_tokens), 0),
		       COALESCE(SUM(total_tokens), 0)
		FROM ai_token_usage
		WHERE team_id = $1 AND provider = $2 AND period_start = $3
	`, teamID, provider, periodStart)

	var u ProviderUsage
	u.Provider = provider
	if err := row.Scan(&u.PromptTokens, &u.CompletionTokens, &u.TotalTokens); err != nil {
		return nil, err
	}
	return &u, nil
}

// GetTotalUsage returns aggregated token usage across all providers for a team in a period.
func (s *Store) GetTotalUsage(ctx context.Context, teamID string, periodStart time.Time) (*ProviderUsage, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT COALESCE(SUM(prompt_tokens), 0),
		       COALESCE(SUM(completion_tokens), 0),
		       COALESCE(SUM(total_tokens), 0)
		FROM ai_token_usage
		WHERE team_id = $1 AND period_start = $2
	`, teamID, periodStart)

	var u ProviderUsage
	u.Provider = "*"
	if err := row.Scan(&u.PromptTokens, &u.CompletionTokens, &u.TotalTokens); err != nil {
		return nil, err
	}
	return &u, nil
}

// GetTeamBudget returns the budget for a team+provider. Returns nil if no explicit budget is set.
func (s *Store) GetTeamBudget(ctx context.Context, teamID, provider string) (*TeamBudget, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, team_id, provider, monthly_token_limit, enforcement
		FROM team_budgets
		WHERE team_id = $1 AND provider = $2
	`, teamID, provider)

	var b TeamBudget
	if err := row.Scan(&b.ID, &b.TeamID, &b.Provider, &b.MonthlyTokenLimit, &b.Enforcement); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &b, nil
}

// SetTeamBudget upserts a team's budget for a provider.
func (s *Store) SetTeamBudget(ctx context.Context, teamID, provider string, monthlyLimit int64, enforcement string) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO team_budgets (team_id, provider, monthly_token_limit, enforcement, updated_at)
		VALUES ($1, $2, $3, $4, now())
		ON CONFLICT (team_id, provider)
		DO UPDATE SET monthly_token_limit = EXCLUDED.monthly_token_limit,
		             enforcement = EXCLUDED.enforcement,
		             updated_at = now()
	`, teamID, provider, monthlyLimit, enforcement)
	return err
}

// DeleteTeamBudget removes a team's budget for a provider.
func (s *Store) DeleteTeamBudget(ctx context.Context, teamID, provider string) error {
	_, err := s.db.ExecContext(ctx, `
		DELETE FROM team_budgets WHERE team_id = $1 AND provider = $2
	`, teamID, provider)
	return err
}

// ListTeamBudgets returns all budgets for a team.
func (s *Store) ListTeamBudgets(ctx context.Context, teamID string) ([]TeamBudget, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, team_id, provider, monthly_token_limit, enforcement
		FROM team_budgets WHERE team_id = $1 ORDER BY provider
	`, teamID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var budgets []TeamBudget
	for rows.Next() {
		var b TeamBudget
		if err := rows.Scan(&b.ID, &b.TeamID, &b.Provider, &b.MonthlyTokenLimit, &b.Enforcement); err != nil {
			return nil, err
		}
		budgets = append(budgets, b)
	}
	return budgets, rows.Err()
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/budget/ -run TestStore_IncrementUsage -v -timeout 120s`
Expected: PASS

- [ ] **Step 5: Write additional store tests**

Add tests for:
- `TestStore_GetTeamBudget_NotFound` — returns nil, nil when no budget set
- `TestStore_SetTeamBudget_Upsert` — set, update, verify
- `TestStore_GetTotalUsage` — multiple providers sum correctly
- `TestStore_ListTeamBudgets` — returns sorted list

- [ ] **Step 6: Run all store tests**

Run: `go test ./internal/budget/ -v -timeout 120s`
Expected: All PASS

- [ ] **Step 7: Commit**

```bash
git add internal/budget/store.go internal/budget/store_test.go
git commit -m "feat(budget): add token usage and team budget DB store"
```

---

### Task 4: Budget checker — core enforcement logic

**Files:**
- Create: `internal/budget/budget.go`
- Create: `internal/budget/budget_test.go`

- [ ] **Step 1: Write the failing test for period calculation**

```go
func TestCurrentPeriodStart_Calendar(t *testing.T) {
	now := time.Date(2026, 3, 23, 14, 30, 0, 0, time.UTC)
	start := budget.CurrentPeriodStart(now, "calendar", 1)
	assert.Equal(t, time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC), start)
}

func TestCurrentPeriodStart_Rolling(t *testing.T) {
	// Before reset day
	now := time.Date(2026, 3, 10, 14, 30, 0, 0, time.UTC)
	start := budget.CurrentPeriodStart(now, "rolling", 15)
	assert.Equal(t, time.Date(2026, 2, 15, 0, 0, 0, 0, time.UTC), start)

	// After reset day
	now = time.Date(2026, 3, 20, 14, 30, 0, 0, time.UTC)
	start = budget.CurrentPeriodStart(now, "rolling", 15)
	assert.Equal(t, time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC), start)

	// On reset day
	now = time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC)
	start = budget.CurrentPeriodStart(now, "rolling", 15)
	assert.Equal(t, time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC), start)
}

func TestCurrentPeriodStart_RollingDay28_February(t *testing.T) {
	// Feb has 28 days — day 28 exists
	now := time.Date(2026, 3, 10, 0, 0, 0, 0, time.UTC)
	start := budget.CurrentPeriodStart(now, "rolling", 28)
	assert.Equal(t, time.Date(2026, 2, 28, 0, 0, 0, 0, time.UTC), start)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/budget/ -run TestCurrentPeriodStart -v`
Expected: FAIL — function does not exist.

- [ ] **Step 3: Implement CurrentPeriodStart**

```go
package budget

import "time"

// CurrentPeriodStart returns the start date of the current budget period.
// For "calendar" mode: first day of the current month.
// For "rolling" mode: the most recent occurrence of resetDay (1-28).
func CurrentPeriodStart(now time.Time, mode string, resetDay int) time.Time {
	if mode == "rolling" && resetDay >= 1 && resetDay <= 28 {
		if now.Day() >= resetDay {
			return time.Date(now.Year(), now.Month(), resetDay, 0, 0, 0, 0, time.UTC)
		}
		// Go back to previous month's reset day
		prev := now.AddDate(0, -1, 0)
		return time.Date(prev.Year(), prev.Month(), resetDay, 0, 0, 0, 0, time.UTC)
	}
	// Default: calendar month
	return time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/budget/ -run TestCurrentPeriodStart -v`
Expected: PASS

- [ ] **Step 5: Write the failing test for budget enforcement**

```go
func TestChecker_Check_GlobalHardBlock(t *testing.T) {
	checker := &budget.Checker{
		GlobalMonthlyTokenLimit: 1000,
		ResetMode:               "calendar",
		ResetDay:                1,
		GetTotalUsage: func(ctx context.Context, teamID string, period time.Time) (int64, error) {
			return 1001, nil // over limit
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
		GetTotalUsage: func(ctx context.Context, teamID string, period time.Time) (int64, error) {
			return 0, nil
		},
		GetProviderUsage: func(ctx context.Context, teamID, provider string, period time.Time) (int64, error) {
			return 5001, nil
		},
		GetTeamBudget: func(ctx context.Context, teamID, provider string) (*TeamBudget, error) {
			return &TeamBudget{MonthlyTokenLimit: 5000, Enforcement: "warn"}, nil
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
		GetTotalUsage: func(ctx context.Context, teamID string, period time.Time) (int64, error) {
			return 500, nil
		},
		GetProviderUsage: func(ctx context.Context, teamID, provider string, period time.Time) (int64, error) {
			return 200, nil
		},
		GetTeamBudget: func(ctx context.Context, teamID, provider string) (*TeamBudget, error) {
			return &TeamBudget{MonthlyTokenLimit: 10000, Enforcement: "hard"}, nil
		},
	}

	result := checker.Check(context.Background(), budget.CheckInput{
		TeamID:   "team-1",
		Provider: "openai",
	})

	assert.False(t, result.Blocked)
	assert.False(t, result.Warning)
}
```

- [ ] **Step 6: Run test to verify it fails**

Run: `go test ./internal/budget/ -run TestChecker_Check -v`
Expected: FAIL — Checker type does not exist.

- [ ] **Step 7: Implement the Checker**

```go
// CheckInput describes the context of a budget check.
type CheckInput struct {
	TeamID   string
	Provider string
}

// CheckResult describes the outcome of a budget check.
type CheckResult struct {
	Blocked   bool   // true if the step should be prevented from running
	Warning   bool   // true if budget exceeded but enforcement is "warn"
	BlockedBy string // "global", "team", or ""
	Message   string // human-readable explanation
}

// Checker evaluates budget limits before AI step dispatch.
// Uses function fields for DB access to allow easy testing with mocks.
type Checker struct {
	GlobalMonthlyTokenLimit     int64
	DefaultTeamMonthlyTokenLimit int64
	ResetMode                   string
	ResetDay                    int

	// DB access functions — injected by caller
	GetTotalUsage    func(ctx context.Context, teamID string, period time.Time) (int64, error)
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
		total, err := c.GetTotalUsage(ctx, input.TeamID, period)
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
		if err == nil && tb == nil && c.DefaultTeamMonthlyTokenLimit > 0 {
			// No explicit budget — use global default with hard enforcement
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
```

- [ ] **Step 8: Run tests to verify they pass**

Run: `go test ./internal/budget/ -run TestChecker_Check -v`
Expected: All PASS

- [ ] **Step 9: Add test for workflow execution budget check**

```go
func TestChecker_CheckExecutionBudget(t *testing.T) {
	result := budget.CheckExecutionBudget(50000, 50001)
	assert.True(t, result.Blocked)
	assert.Equal(t, "workflow", result.BlockedBy)

	result = budget.CheckExecutionBudget(50000, 49999)
	assert.False(t, result.Blocked)

	// 0 = unlimited
	result = budget.CheckExecutionBudget(0, 999999)
	assert.False(t, result.Blocked)
}
```

- [ ] **Step 10: Implement CheckExecutionBudget**

```go
// CheckExecutionBudget checks if a workflow execution's cumulative token usage
// has exceeded the workflow-level token_budget. Called before each AI step dispatch.
// budgetLimit of 0 means unlimited.
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
```

- [ ] **Step 11: Run all budget tests**

Run: `go test ./internal/budget/ -v -timeout 120s`
Expected: All PASS

- [ ] **Step 12: Commit**

```bash
git add internal/budget/budget.go internal/budget/budget_test.go
git commit -m "feat(budget): add budget checker with period calculation and AND-gate enforcement"
```

---

### Task 5: Add audit actions and metrics for budget enforcement

**Files:**
- Modify: `internal/audit/audit.go`
- Modify: `internal/metrics/metrics.go`

- [ ] **Step 1: Add budget audit action constants**

In `internal/audit/audit.go`, add to the Action constants:

```go
ActionBudgetExceeded  Action = "budget.exceeded"  // hard block
ActionBudgetWarning   Action = "budget.warning"    // warn-only
ActionBudgetUpdated   Action = "budget.updated"    // team budget changed
```

- [ ] **Step 2: Add budget Prometheus metrics**

In `internal/metrics/metrics.go`, add:

```go
BudgetCheckTotal = promauto.NewCounterVec(prometheus.CounterOpts{
	Name: "mantle_budget_check_total",
	Help: "Total budget checks performed before AI step dispatch",
}, []string{"team_id", "provider", "result"}) // result: "pass", "blocked", "warning"

BudgetUsageGauge = promauto.NewGaugeVec(prometheus.GaugeOpts{
	Name: "mantle_budget_usage_tokens",
	Help: "Current token usage within the budget period",
}, []string{"team_id", "provider"})
```

- [ ] **Step 3: Verify build**

Run: `go build ./...`
Expected: exit 0

- [ ] **Step 4: Commit**

```bash
git add internal/audit/audit.go internal/metrics/metrics.go
git commit -m "feat(budget): add budget audit actions and Prometheus metrics"
```

---

### Task 6: Add token_budget field to Workflow struct

**Files:**
- Modify: `internal/workflow/workflow.go`
- Modify: `internal/workflow/workflow_test.go`

- [ ] **Step 1: Write the failing test**

```go
func TestParse_TokenBudget(t *testing.T) {
	yaml := `
name: test-workflow
token_budget: 500000
steps:
  - name: step1
    action: http/request
    params:
      url: "https://example.com"
`
	result, err := workflow.ParseBytes([]byte(yaml))
	require.NoError(t, err)
	assert.Equal(t, int64(500000), result.Workflow.TokenBudget)
}

func TestParse_TokenBudget_Zero(t *testing.T) {
	yaml := `
name: test-workflow
steps:
  - name: step1
    action: http/request
    params:
      url: "https://example.com"
`
	result, err := workflow.ParseBytes([]byte(yaml))
	require.NoError(t, err)
	assert.Equal(t, int64(0), result.Workflow.TokenBudget)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/workflow/ -run TestParse_TokenBudget -v`
Expected: FAIL — `TokenBudget` field does not exist on Workflow.

- [ ] **Step 3: Add TokenBudget to Workflow struct**

In `internal/workflow/workflow.go`, add to the `Workflow` struct:

```go
TokenBudget int64 `yaml:"token_budget"` // 0 = unlimited
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/workflow/ -run TestParse_TokenBudget -v`
Expected: PASS

- [ ] **Step 5: Run all workflow tests to check for regressions**

Run: `go test ./internal/workflow/ -v`
Expected: All PASS

- [ ] **Step 6: Commit**

```bash
git add internal/workflow/workflow.go internal/workflow/workflow_test.go
git commit -m "feat(budget): add token_budget field to Workflow struct"
```

---

### Task 7: Wire budget enforcement into the engine

**Files:**
- Modify: `internal/engine/engine.go`
- Modify: `internal/engine/engine_test.go`

This is the critical integration task. The engine needs to:
1. Check team+provider and global budgets before each AI step
2. Check workflow execution budget before each AI step
3. Record token usage after each AI step completes
4. Emit audit events and metrics for budget violations

**Important architecture note:** Budget enforcement goes in `executeStepLogic()` (not `executeStep()`), because `executeStepLogic` is the shared code path used by BOTH the sequential engine (`executeStep` → `executeStepLogic`) AND the distributed worker (`MakeStepExecutor`/`MakeGlobalStepExecutor` → `executeStepLogic`). Placing checks in `executeStep` alone would leave the distributed worker path unprotected.

The workflow execution budget check requires access to completed steps and the workflow's `TokenBudget`. In the distributed path (`MakeGlobalStepExecutor`), these are loaded from DB at lines 424-446 of `engine.go`. We'll pass them to `executeStepLogic` via a new `StepContext` parameter.

- [ ] **Step 1: Write the failing test for pre-dispatch budget blocking**

In `internal/engine/engine_test.go`, write a test that sets up a workflow with `token_budget` and verifies the engine blocks an AI step when the budget is exhausted. The exact test structure depends on the existing test helpers — examine `engine_test.go` for patterns.

The test should:
- Create a workflow with `token_budget: 100`
- Mock the budget checker to return `Blocked: true`
- Execute and assert the step result is `"failed"` with a budget error message

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/engine/ -run TestEngine_BudgetBlock -v -timeout 120s`
Expected: FAIL

- [ ] **Step 3: Add BudgetChecker and BudgetStore to Engine struct**

In `internal/engine/engine.go`, add fields to the `Engine` struct:

```go
type Engine struct {
	// ... existing fields ...
	BudgetChecker *budget.Checker // nil = budget enforcement disabled
	BudgetStore   *budget.Store   // nil = token usage recording disabled
}
```

- [ ] **Step 4: Add a StepContext struct to pass workflow-level data to executeStepLogic**

The current `executeStepLogic` signature is:
```go
func (e *Engine) executeStepLogic(ctx context.Context, execID string, step workflow.Step, celCtx *mantleCEL.Context, workflowName string) (map[string]any, error)
```

Add a `StepContext` struct and update the signature:

```go
// StepContext carries workflow-level metadata needed by step execution.
// Fields are optional — zero values disable the corresponding feature.
type StepContext struct {
	WorkflowTokenBudget int64                    // from workflow.TokenBudget; 0 = unlimited
	CompletedSteps      map[string]map[string]any // outputs of completed steps (for summing tokens)
	TeamID              string                    // from auth.TeamIDFromContext
}
```

Update `executeStepLogic` to accept `StepContext`:
```go
func (e *Engine) executeStepLogic(ctx context.Context, execID string, step workflow.Step, celCtx *mantleCEL.Context, workflowName string, sc StepContext) (map[string]any, error)
```

Update all callers of `executeStepLogic`:

1. **`executeStep`** (line 235): Build `StepContext` from the `resumeExecution` caller. `resumeExecution` has access to `completedSteps` and the `Workflow` struct — pass `wf.TokenBudget`, `completedSteps`, and `auth.TeamIDFromContext(ctx)` through `executeStep`'s signature.

2. **`MakeStepExecutor`** (line 377): The caller has `wf *workflow.Workflow` and `celCtx`. Build `StepContext{WorkflowTokenBudget: wf.TokenBudget, TeamID: auth.TeamIDFromContext(ctx)}`. For `CompletedSteps`, extract from `celCtx.Steps` (each entry has `{"output": ...}`).

3. **`MakeGlobalStepExecutor`** (line 448): Already loads `completedSteps` at line 424 and `wf` at line 413. Build `StepContext{WorkflowTokenBudget: wf.TokenBudget, CompletedSteps: completedSteps, TeamID: teamID}` (teamID is at line 401).

- [ ] **Step 5: Add pre-dispatch budget check at the top of executeStepLogic**

In `executeStepLogic()`, before the existing CEL resolution and connector dispatch, add:

```go
// Budget enforcement — check before dispatching AI steps.
if strings.HasPrefix(step.Action, "ai/") {
	provider := "openai" // default
	if p, ok := step.Params["provider"].(string); ok && p != "" {
		provider = p
	}

	// 1. Workflow execution budget: sum tokens from completed steps
	if sc.WorkflowTokenBudget > 0 {
		var usedTokens int64
		for _, stepOutput := range sc.CompletedSteps {
			usedTokens += extractTotalTokens(stepOutput)
		}
		result := budget.CheckExecutionBudget(sc.WorkflowTokenBudget, usedTokens)
		if result.Blocked {
			return nil, fmt.Errorf("%s", result.Message)
		}
	}

	// 2. Team+provider and global budget check
	if e.BudgetChecker != nil && sc.TeamID != "" {
		result := e.BudgetChecker.Check(ctx, budget.CheckInput{
			TeamID:   sc.TeamID,
			Provider: provider,
		})
		if result.Blocked {
			e.Auditor.Emit(ctx, audit.Event{
				Timestamp: time.Now(),
				Actor:     "engine",
				Action:    audit.ActionBudgetExceeded,
				Resource:  audit.Resource{Type: "workflow_execution", ID: execID},
				Metadata:  map[string]string{
					"blocked_by": result.BlockedBy,
					"message":    result.Message,
					"provider":   provider,
				},
				TeamID: sc.TeamID,
			})
			metrics.BudgetCheckTotal.WithLabelValues(sc.TeamID, provider, "blocked").Inc()
			return nil, fmt.Errorf("%s", result.Message)
		}
		if result.Warning {
			e.Auditor.Emit(ctx, audit.Event{
				Timestamp: time.Now(),
				Actor:     "engine",
				Action:    audit.ActionBudgetWarning,
				Resource:  audit.Resource{Type: "workflow_execution", ID: execID},
				Metadata:  map[string]string{
					"message":  result.Message,
					"provider": provider,
				},
				TeamID: sc.TeamID,
			})
			metrics.BudgetCheckTotal.WithLabelValues(sc.TeamID, provider, "warning").Inc()
		} else {
			metrics.BudgetCheckTotal.WithLabelValues(sc.TeamID, provider, "pass").Inc()
		}
	}
}
```

- [ ] **Step 6: Add post-completion token recording**

After a successful connector execution in `executeStepLogic` (after `output, lastErr = conn.Execute(...)` succeeds), add token recording:

```go
// Record token usage for budget tracking after successful AI step
if strings.HasPrefix(step.Action, "ai/") && e.BudgetStore != nil && e.BudgetChecker != nil && sc.TeamID != "" && output != nil {
	promptTokens, completionTokens := extractTokenCounts(output)
	if promptTokens > 0 || completionTokens > 0 {
		provider := "openai"
		if p, ok := step.Params["provider"].(string); ok && p != "" {
			provider = p
		}
		model, _ := output["model"].(string)
		period := budget.CurrentPeriodStart(time.Now(), e.BudgetChecker.ResetMode, e.BudgetChecker.ResetDay)
		_ = e.BudgetStore.IncrementUsage(ctx, sc.TeamID, provider, model, period, promptTokens, completionTokens)
	}
}
```

- [ ] **Step 7: Add helper functions to extract token counts from step output**

```go
// extractTokenCounts pulls prompt and completion token counts from an AI step's output map.
func extractTokenCounts(output map[string]any) (prompt, completion int64) {
	usage, ok := output["usage"].(map[string]any)
	if !ok {
		return 0, 0
	}
	if v, ok := usage["prompt_tokens"]; ok {
		prompt = toInt64(v)
	}
	if v, ok := usage["completion_tokens"]; ok {
		completion = toInt64(v)
	}
	return prompt, completion
}

// extractTotalTokens pulls the total token count from a step's output map.
func extractTotalTokens(output map[string]any) int64 {
	usage, ok := output["usage"].(map[string]any)
	if !ok {
		return 0
	}
	return toInt64(usage["total_tokens"])
}

func toInt64(v any) int64 {
	switch n := v.(type) {
	case int:
		return int64(n)
	case int64:
		return n
	case float64:
		return int64(n)
	default:
		return 0
	}
}
```

- [ ] **Step 8: Run tests to verify they pass**

Run: `go test ./internal/engine/ -v -timeout 120s`
Expected: All PASS (including new budget test)

- [ ] **Step 9: Run full test suite to check for regressions**

Run: `go test ./... -timeout 300s`
Expected: All PASS

- [ ] **Step 10: Commit**

```bash
git add internal/engine/engine.go internal/engine/engine_test.go
git commit -m "feat(budget): wire budget enforcement into engine step dispatch"
```

---

### Task 8: API endpoints for team budget management

**Files:**
- Modify: `internal/server/api.go`
- Modify: `internal/server/server.go`

- [ ] **Step 1: Register new routes**

In `internal/server/server.go`, add route registrations:

```go
mux.HandleFunc("GET /api/v1/budgets", s.handleListBudgets)
mux.HandleFunc("PUT /api/v1/budgets/{provider}", s.handleSetBudget)
mux.HandleFunc("DELETE /api/v1/budgets/{provider}", s.handleDeleteBudget)
mux.HandleFunc("GET /api/v1/budgets/usage", s.handleGetUsage)
```

- [ ] **Step 2: Implement handlers**

In `internal/server/api.go`, add:

```go
func (s *Server) handleListBudgets(w http.ResponseWriter, r *http.Request) {
	teamID := auth.TeamIDFromContext(r.Context())
	budgets, err := s.BudgetStore.ListTeamBudgets(r.Context(), teamID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, budgets)
}

func (s *Server) handleSetBudget(w http.ResponseWriter, r *http.Request) {
	teamID := auth.TeamIDFromContext(r.Context())
	provider := r.PathValue("provider")

	var body struct {
		MonthlyTokenLimit int64  `json:"monthly_token_limit"`
		Enforcement       string `json:"enforcement"` // "hard" or "warn"
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if body.Enforcement == "" {
		body.Enforcement = "hard"
	}
	if body.Enforcement != "hard" && body.Enforcement != "warn" {
		http.Error(w, "enforcement must be 'hard' or 'warn'", http.StatusBadRequest)
		return
	}

	if err := s.BudgetStore.SetTeamBudget(r.Context(), teamID, provider, body.MonthlyTokenLimit, body.Enforcement); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Build actor string from authenticated user (auth.UserFromContext returns *User or nil)
	actor := "api"
	if u := auth.UserFromContext(r.Context()); u != nil {
		actor = u.ID
	}

	s.Auditor.Emit(r.Context(), audit.Event{
		Timestamp: time.Now(),
		Actor:     actor,
		Action:    audit.ActionBudgetUpdated,
		Resource:  audit.Resource{Type: "team_budget", ID: teamID},
		Metadata: map[string]string{
			"provider":    provider,
			"limit":       fmt.Sprintf("%d", body.MonthlyTokenLimit),
			"enforcement": body.Enforcement,
		},
		TeamID: teamID,
	})

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleDeleteBudget(w http.ResponseWriter, r *http.Request) {
	teamID := auth.TeamIDFromContext(r.Context())
	provider := r.PathValue("provider")

	if err := s.BudgetStore.DeleteTeamBudget(r.Context(), teamID, provider); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleGetUsage(w http.ResponseWriter, r *http.Request) {
	teamID := auth.TeamIDFromContext(r.Context())
	cfg := config.FromContext(r.Context())

	period := budget.CurrentPeriodStart(time.Now(), cfg.Engine.Budget.ResetMode, cfg.Engine.Budget.ResetDay)

	provider := r.URL.Query().Get("provider")
	var usage *budget.ProviderUsage
	var err error
	if provider != "" {
		usage, err = s.BudgetStore.GetUsage(r.Context(), teamID, provider, period)
	} else {
		usage, err = s.BudgetStore.GetTotalUsage(r.Context(), teamID, period)
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"period_start":      period.Format("2006-01-02"),
		"provider":          usage.Provider,
		"prompt_tokens":     usage.PromptTokens,
		"completion_tokens": usage.CompletionTokens,
		"total_tokens":      usage.TotalTokens,
	})
}
```

- [ ] **Step 3: Add BudgetStore to Server struct**

The `Server` struct is defined in `internal/server/server.go` at line 25. Add `BudgetStore *budget.Store` as a new field. The `Server` struct uses direct field assignment (not a constructor function), so add initialization wherever the server is instantiated — look for `&Server{` or `server.Server{` in `internal/cli/serve.go` or similar and add `BudgetStore: budget.NewStore(db)` there.

- [ ] **Step 4: Verify build**

Run: `go build ./...`
Expected: exit 0

- [ ] **Step 5: Commit**

```bash
git add internal/server/api.go internal/server/server.go
git commit -m "feat(budget): add team budget API endpoints (list, set, delete, usage)"
```

---

### Task 9: CLI commands for budget management

**Files:**
- Create: `internal/cli/budget.go`
- Modify: `internal/cli/root.go`

- [ ] **Step 1: Implement the budget subcommand**

**Important:** The existing CLI commands (e.g., `secrets`, `apply`) use direct DB access, NOT HTTP API calls. There is no `apiRequest` helper. Follow the same pattern as `internal/cli/secrets.go`: open a DB connection via `db.Open(cfg.Database)`, create a store, and operate directly.

Create `internal/cli/budget.go`:

```go
package cli

import (
	"fmt"
	"text/tabwriter"
	"time"

	"github.com/dvflw/mantle/internal/budget"
	"github.com/dvflw/mantle/internal/config"
	"github.com/dvflw/mantle/internal/db"
	"github.com/dvflw/mantle/internal/auth"
	"github.com/spf13/cobra"
)

func newBudgetCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "budget",
		Short: "Manage AI token budgets",
	}
	cmd.AddCommand(
		newBudgetSetCommand(),
		newBudgetGetCommand(),
		newBudgetUsageCommand(),
		newBudgetDeleteCommand(),
	)
	return cmd
}

// newBudgetStore opens a DB connection and returns a budget store (mirrors newSecretStore pattern).
func newBudgetStore(cmd *cobra.Command) (*budget.Store, func(), error) {
	cfg := config.FromContext(cmd.Context())
	database, err := db.Open(cfg.Database)
	if err != nil {
		return nil, nil, fmt.Errorf("opening database: %w", err)
	}
	return budget.NewStore(database), func() { database.Close() }, nil
}

func newBudgetSetCommand() *cobra.Command {
	var enforcement string
	cmd := &cobra.Command{
		Use:   "set <provider> <monthly-token-limit>",
		Short: "Set a monthly token budget for a provider",
		Long:  "Set a monthly token budget. Provider can be 'openai', 'bedrock', or '*' for all providers.",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			store, cleanup, err := newBudgetStore(cmd)
			if err != nil {
				return err
			}
			defer cleanup()

			provider := args[0]
			var limit int64
			if _, err := fmt.Sscanf(args[1], "%d", &limit); err != nil {
				return fmt.Errorf("invalid token limit: %s", args[1])
			}

			teamID := auth.TeamIDFromContext(cmd.Context())
			if err := store.SetTeamBudget(cmd.Context(), teamID, provider, limit, enforcement); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Budget set: %s → %d tokens/month (enforcement: %s)\n", provider, limit, enforcement)
			return nil
		},
	}
	cmd.Flags().StringVar(&enforcement, "enforcement", "hard", "Enforcement mode: 'hard' (block) or 'warn' (log only)")
	return cmd
}

func newBudgetGetCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "get",
		Short: "List all team budgets",
		RunE: func(cmd *cobra.Command, args []string) error {
			store, cleanup, err := newBudgetStore(cmd)
			if err != nil {
				return err
			}
			defer cleanup()

			teamID := auth.TeamIDFromContext(cmd.Context())
			budgets, err := store.ListTeamBudgets(cmd.Context(), teamID)
			if err != nil {
				return err
			}
			if len(budgets) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "No budgets configured (global defaults apply)")
				return nil
			}
			w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "PROVIDER\tLIMIT\tENFORCEMENT")
			for _, b := range budgets {
				fmt.Fprintf(w, "%s\t%d tokens/month\t%s\n", b.Provider, b.MonthlyTokenLimit, b.Enforcement)
			}
			return w.Flush()
		},
	}
}

func newBudgetUsageCommand() *cobra.Command {
	var provider string
	cmd := &cobra.Command{
		Use:   "usage",
		Short: "Show current period token usage",
		RunE: func(cmd *cobra.Command, args []string) error {
			store, cleanup, err := newBudgetStore(cmd)
			if err != nil {
				return err
			}
			defer cleanup()

			cfg := config.FromContext(cmd.Context())
			teamID := auth.TeamIDFromContext(cmd.Context())
			period := budget.CurrentPeriodStart(time.Now(), cfg.Engine.Budget.ResetMode, cfg.Engine.Budget.ResetDay)

			var usage *budget.ProviderUsage
			if provider != "" {
				usage, err = store.GetUsage(cmd.Context(), teamID, provider, period)
			} else {
				usage, err = store.GetTotalUsage(cmd.Context(), teamID, period)
			}
			if err != nil {
				return err
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Period:     %s\n", period.Format("2006-01-02"))
			fmt.Fprintf(cmd.OutOrStdout(), "Provider:   %s\n", usage.Provider)
			fmt.Fprintf(cmd.OutOrStdout(), "Prompt:     %d tokens\n", usage.PromptTokens)
			fmt.Fprintf(cmd.OutOrStdout(), "Completion: %d tokens\n", usage.CompletionTokens)
			fmt.Fprintf(cmd.OutOrStdout(), "Total:      %d tokens\n", usage.TotalTokens)
			return nil
		},
	}
	cmd.Flags().StringVar(&provider, "provider", "", "Filter by provider (e.g., 'openai', 'bedrock')")
	return cmd
}

func newBudgetDeleteCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "delete <provider>",
		Short: "Remove a team budget for a provider",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			store, cleanup, err := newBudgetStore(cmd)
			if err != nil {
				return err
			}
			defer cleanup()

			teamID := auth.TeamIDFromContext(cmd.Context())
			if err := store.DeleteTeamBudget(cmd.Context(), teamID, args[0]); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Budget removed for provider: %s\n", args[0])
			return nil
		},
	}
}
```

- [ ] **Step 2: Register in root command**

In `internal/cli/root.go`, add the budget command to the admin group:

```go
addToGroup(cmd, "admin",
	// ... existing admin commands ...
	newBudgetCommand(),
)
```

- [ ] **Step 3: Verify build**

Run: `go build ./...`
Expected: exit 0

- [ ] **Step 4: Commit**

```bash
git add internal/cli/budget.go internal/cli/root.go
git commit -m "feat(budget): add mantle budget CLI commands (set, get, usage, delete)"
```

---

### Task 10: Documentation

**Files:**
- Create: `site/src/content/docs/ai-cost-controls.md`

- [ ] **Step 1: Write the docs page**

Create `site/src/content/docs/ai-cost-controls.md` with the following content:

```mdx
---
title: AI Cost Controls
description: Configure token budgets to control AI usage at the workflow, team, and global level.
---

# AI Cost Controls

Mantle provides token-based budgets at three levels to help you control AI usage costs. Budgets are denominated in **tokens** (not dollars) to stay provider-agnostic and avoid stale pricing data.

## Budget Levels

All three budget levels compose as **AND gates** — every applicable level must pass before an AI step is dispatched. If any level is exceeded, the step is blocked (or warned, depending on configuration).

| Level | Scope | Configured In | Enforcement | Reset |
|-------|-------|---------------|-------------|-------|
| **Global** | All teams, all providers | `mantle.yaml` | Hard block only | Monthly |
| **Team + Provider** | One team, one provider | API / CLI | Configurable (hard or warn) | Monthly |
| **Workflow** | Single execution | Workflow YAML | Configurable (hard or warn) | Per execution |

### Enforcement Behavior

- **Hard block (default):** The AI step fails with a budget error. The execution stops at that step. Tokens consumed by prior steps in the execution are preserved — Mantle never cancels a step mid-execution.
- **Warn only:** The AI step proceeds, but a warning is logged and an audit event is emitted. The `mantle_budget_check_total` Prometheus metric is incremented with `result="warning"`.

## Configuration

### Global Budget (mantle.yaml)

```yaml
engine:
  budget:
    global_monthly_token_limit: 10000000  # 0 = unlimited
    default_team_monthly_token_limit: 1000000  # applied to teams without explicit budgets
    reset_mode: calendar  # "calendar" or "rolling"
    reset_day: 1          # 1-28, only used when reset_mode is "rolling"
```

**Environment variables:**

| Variable | Description |
|----------|-------------|
| `MANTLE_ENGINE_BUDGET_GLOBAL_MONTHLY_TOKEN_LIMIT` | Global token cap (hard block) |
| `MANTLE_ENGINE_BUDGET_DEFAULT_TEAM_MONTHLY_TOKEN_LIMIT` | Default per-team cap |
| `MANTLE_ENGINE_BUDGET_RESET_MODE` | `calendar` or `rolling` |
| `MANTLE_ENGINE_BUDGET_RESET_DAY` | Start day for rolling periods (1-28) |

### Reset Windows

- **Calendar month:** Budget resets on the 1st of each month (UTC).
- **Rolling period:** Budget resets on your configured `reset_day` each month. For example, `reset_day: 15` means the current period runs from the 15th of the current month to the 14th of the next. The maximum `reset_day` is 28 to avoid February edge cases.

### Team + Provider Budget (API / CLI)

Set per-provider budgets for your team:

```bash
# Set a hard budget of 1M tokens/month for OpenAI
mantle budget set openai 1000000

# Set a warn-only budget for Bedrock
mantle budget set bedrock 500000 --enforcement warn

# Set a budget for all providers
mantle budget set '*' 2000000

# View current budgets
mantle budget get

# View current usage
mantle budget usage
mantle budget usage --provider openai

# Remove a budget
mantle budget delete openai
```

**API:**

```bash
# Set budget
curl -X PUT /api/v1/budgets/openai \
  -d '{"monthly_token_limit": 1000000, "enforcement": "hard"}'

# List budgets
curl /api/v1/budgets

# Get usage
curl /api/v1/budgets/usage?provider=openai

# Delete budget
curl -X DELETE /api/v1/budgets/openai
```

### Workflow Budget (YAML)

Add a `token_budget` field at the workflow level to cap total tokens consumed in a single execution:

```yaml
name: my-analysis-workflow
token_budget: 500000  # max tokens across all AI steps in one execution

steps:
  - name: summarize
    action: ai/completion
    params:
      model: gpt-4o
      prompt: "Summarize this document: {{ inputs.document }}"
      max_token_budget: 100000  # per-step limit (existing feature)

  - name: analyze
    action: ai/completion
    params:
      model: gpt-4o
      prompt: "Analyze the summary: {{ steps.summarize.output.text }}"
```

The workflow `token_budget` is checked before each AI step. If the cumulative tokens from prior steps exceed the budget, the next AI step is blocked. Non-AI steps (HTTP, etc.) are unaffected.

The existing per-step `max_token_budget` param continues to work independently — it caps tokens within a single step's tool-use loop.

## Monitoring

### Prometheus Metrics

| Metric | Labels | Description |
|--------|--------|-------------|
| `mantle_budget_check_total` | `team_id`, `provider`, `result` | Budget checks (result: pass/blocked/warning) |
| `mantle_budget_usage_tokens` | `team_id`, `provider` | Current token usage in the budget period |
| `mantle_ai_tokens_total` | `workflow`, `step`, `model`, `provider`, `token_type` | Raw token consumption (existing) |

### Audit Events

| Action | When |
|--------|------|
| `budget.exceeded` | AI step blocked by budget |
| `budget.warning` | AI step proceeded with warn-only budget exceeded |
| `budget.updated` | Team budget created, updated, or deleted |

## Provider Pricing Reference

Token budgets are provider-agnostic. To estimate costs, refer to your provider's pricing page:

- **OpenAI:** [https://openai.com/api/pricing](https://openai.com/api/pricing)
- **AWS Bedrock:** [https://aws.amazon.com/bedrock/pricing](https://aws.amazon.com/bedrock/pricing)
- **Anthropic:** [https://www.anthropic.com/pricing](https://www.anthropic.com/pricing)
- **Google AI:** [https://ai.google.dev/pricing](https://ai.google.dev/pricing)

> **Tip:** Most providers charge differently for prompt (input) vs. completion (output) tokens. Mantle tracks both separately in the `ai_token_usage` table and Prometheus metrics, so you can compute costs externally using your provider's rate card.
```

- [ ] **Step 2: Verify the site builds (if applicable)**

Run: `cd site && npm run build` (or whatever the site build command is)
Expected: Build succeeds with the new docs page.

- [ ] **Step 3: Commit**

```bash
git add site/src/content/docs/ai-cost-controls.md
git commit -m "docs: add AI cost controls documentation with provider pricing links"
```

---

## Verification Checklist

After all tasks are complete, verify end-to-end:

- [ ] `make migrate` applies migration 012 cleanly
- [ ] `go build ./...` succeeds
- [ ] `go test ./... -timeout 300s` all pass
- [ ] `go vet ./...` clean
- [ ] Config loads budget defaults correctly
- [ ] Workflow YAML with `token_budget` parses correctly
- [ ] Budget store increments and queries work against real Postgres
- [ ] Engine blocks AI step when global budget exceeded
- [ ] Engine blocks AI step when team budget exceeded (hard mode)
- [ ] Engine warns but continues when team budget exceeded (warn mode)
- [ ] Engine blocks AI step when workflow execution budget exceeded
- [ ] API endpoints return correct data
- [ ] CLI commands work against running server
- [ ] Audit events emitted for budget violations
- [ ] Prometheus metrics recorded for budget checks
- [ ] Docs page builds and renders correctly
