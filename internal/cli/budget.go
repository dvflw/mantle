package cli

import (
	"fmt"
	"text/tabwriter"
	"time"

	"github.com/dvflw/mantle/internal/auth"
	"github.com/dvflw/mantle/internal/budget"
	"github.com/dvflw/mantle/internal/config"
	"github.com/dvflw/mantle/internal/db"
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
