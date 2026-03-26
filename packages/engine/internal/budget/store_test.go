package budget_test

import (
	"context"
	"database/sql"
	"os"
	"testing"
	"time"

	"github.com/dvflw/mantle/internal/budget"
	"github.com/dvflw/mantle/internal/config"
	"github.com/dvflw/mantle/internal/db"
	"github.com/dvflw/mantle/internal/dbdefaults"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

func setupTestDB(t *testing.T) *sql.DB {
	t.Helper()
	ctx := context.Background()
	pgContainer, err := postgres.Run(ctx,
		dbdefaults.PostgresImage,
		postgres.WithDatabase(dbdefaults.TestDatabase),
		postgres.WithUsername(dbdefaults.User),
		postgres.WithPassword(dbdefaults.Password),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(30*time.Second),
		),
	)
	if err != nil {
		if os.Getenv("CI") != "" {
			t.Fatalf("Could not start Postgres container (CI): %v", err)
		}
		t.Skipf("Could not start Postgres container: %v", err)
	}
	t.Cleanup(func() {
		if err := pgContainer.Terminate(ctx); err != nil {
			t.Logf("Failed to terminate container: %v", err)
		}
	})
	connStr, err := pgContainer.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("Failed to get connection string: %v", err)
	}
	database, err := db.Open(config.DatabaseConfig{URL: connStr})
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	if err := db.Migrate(ctx, database); err != nil {
		t.Fatalf("Failed to run migrations: %v", err)
	}
	return database
}

func TestStore_IncrementUsage(t *testing.T) {
	database := setupTestDB(t)
	store := budget.NewStore(database)
	ctx := context.Background()
	teamID := "00000000-0000-0000-0000-000000000001"
	period := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)

	err := store.IncrementUsage(ctx, teamID, "openai", "gpt-4o", period, 100, 50)
	require.NoError(t, err)

	err = store.IncrementUsage(ctx, teamID, "openai", "gpt-4o", period, 200, 100)
	require.NoError(t, err)

	usage, err := store.GetUsage(ctx, teamID, "openai", period)
	require.NoError(t, err)
	assert.Equal(t, int64(300), usage.PromptTokens)
	assert.Equal(t, int64(150), usage.CompletionTokens)
	assert.Equal(t, int64(450), usage.TotalTokens)
}

func TestStore_GetTeamBudget_NotFound(t *testing.T) {
	database := setupTestDB(t)
	store := budget.NewStore(database)
	ctx := context.Background()

	b, err := store.GetTeamBudget(ctx, "00000000-0000-0000-0000-000000000001", "openai")
	require.NoError(t, err)
	assert.Nil(t, b)
}

func TestStore_SetTeamBudget_Upsert(t *testing.T) {
	database := setupTestDB(t)
	store := budget.NewStore(database)
	ctx := context.Background()
	teamID := "00000000-0000-0000-0000-000000000001"

	err := store.SetTeamBudget(ctx, teamID, "openai", 1000000, "hard")
	require.NoError(t, err)

	b, err := store.GetTeamBudget(ctx, teamID, "openai")
	require.NoError(t, err)
	require.NotNil(t, b)
	assert.Equal(t, int64(1000000), b.MonthlyTokenLimit)
	assert.Equal(t, "hard", b.Enforcement)

	// Upsert with new values
	err = store.SetTeamBudget(ctx, teamID, "openai", 2000000, "warn")
	require.NoError(t, err)

	b, err = store.GetTeamBudget(ctx, teamID, "openai")
	require.NoError(t, err)
	assert.Equal(t, int64(2000000), b.MonthlyTokenLimit)
	assert.Equal(t, "warn", b.Enforcement)
}

func TestStore_GetTotalUsage(t *testing.T) {
	database := setupTestDB(t)
	store := budget.NewStore(database)
	ctx := context.Background()
	teamID := "00000000-0000-0000-0000-000000000001"
	period := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)

	err := store.IncrementUsage(ctx, teamID, "openai", "gpt-4o", period, 100, 50)
	require.NoError(t, err)
	err = store.IncrementUsage(ctx, teamID, "bedrock", "claude-3", period, 200, 100)
	require.NoError(t, err)

	usage, err := store.GetTotalUsage(ctx, teamID, period)
	require.NoError(t, err)
	assert.Equal(t, int64(300), usage.PromptTokens)
	assert.Equal(t, int64(150), usage.CompletionTokens)
	assert.Equal(t, int64(450), usage.TotalTokens)
	assert.Equal(t, "*", usage.Provider)
}

func TestStore_ListTeamBudgets(t *testing.T) {
	database := setupTestDB(t)
	store := budget.NewStore(database)
	ctx := context.Background()
	teamID := "00000000-0000-0000-0000-000000000001"

	err := store.SetTeamBudget(ctx, teamID, "openai", 1000000, "hard")
	require.NoError(t, err)
	err = store.SetTeamBudget(ctx, teamID, "bedrock", 500000, "warn")
	require.NoError(t, err)

	budgets, err := store.ListTeamBudgets(ctx, teamID)
	require.NoError(t, err)
	require.Len(t, budgets, 2)
	assert.Equal(t, "bedrock", budgets[0].Provider) // sorted
	assert.Equal(t, "openai", budgets[1].Provider)
}
