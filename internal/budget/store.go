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
	Provider          string `json:"provider"`
	MonthlyTokenLimit int64  `json:"monthly_token_limit"`
	Enforcement       string `json:"enforcement"`
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

// GetGlobalTotalUsage returns aggregated token usage across ALL teams in a period.
func (s *Store) GetGlobalTotalUsage(ctx context.Context, periodStart time.Time) (*ProviderUsage, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT COALESCE(SUM(prompt_tokens), 0),
		       COALESCE(SUM(completion_tokens), 0),
		       COALESCE(SUM(total_tokens), 0)
		FROM ai_token_usage
		WHERE period_start = $1
	`, periodStart)

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
