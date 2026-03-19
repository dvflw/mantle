package cli

import (
	"fmt"
	"strings"
	"time"

	"github.com/dvflw/mantle/internal/audit"
	"github.com/dvflw/mantle/internal/config"
	"github.com/dvflw/mantle/internal/db"
	"github.com/spf13/cobra"
)

func newAuditCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "audit",
		Short: "Query audit events",
		Long:  "Lists recent audit events from the immutable audit trail. Supports filtering by action, actor, resource, and time range.",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg := config.FromContext(cmd.Context())
			if cfg == nil {
				return fmt.Errorf("config not loaded")
			}

			database, err := db.Open(cfg.Database.URL)
			if err != nil {
				return fmt.Errorf("failed to connect to database: %w", err)
			}
			defer database.Close()

			params := audit.QueryParams{}

			params.Action, _ = cmd.Flags().GetString("action")
			params.Actor, _ = cmd.Flags().GetString("actor")
			params.Limit, _ = cmd.Flags().GetInt("limit")

			resource, _ := cmd.Flags().GetString("resource")
			if resource != "" {
				parts := strings.SplitN(resource, "/", 2)
				params.ResourceType = parts[0]
				if len(parts) == 2 {
					params.ResourceID = parts[1]
				}
			}

			since, _ := cmd.Flags().GetString("since")
			if since != "" {
				d, err := parseDuration(since)
				if err != nil {
					return fmt.Errorf("invalid --since duration: %w", err)
				}
				params.Since = time.Now().Add(-d)
			}

			rows, err := audit.Query(cmd.Context(), database, params)
			if err != nil {
				return err
			}

			if len(rows) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "No audit events found.")
				return nil
			}

			for _, row := range rows {
				fmt.Fprintf(cmd.OutOrStdout(), "%s  %-12s  %-22s  %s/%s\n",
					row.Timestamp.Format(time.RFC3339),
					row.Actor,
					row.Action,
					row.ResourceType,
					row.ResourceID,
				)
			}

			return nil
		},
	}

	cmd.Flags().String("action", "", "filter by action (e.g. workflow.applied, step.completed)")
	cmd.Flags().String("actor", "", "filter by actor (e.g. cli, engine)")
	cmd.Flags().String("resource", "", "filter by resource as type/id (e.g. workflow_definition/my-workflow)")
	cmd.Flags().String("since", "", "show events since duration ago (e.g. 1h, 24h, 7d)")
	cmd.Flags().Int("limit", 50, "maximum number of events to show")

	return cmd
}
