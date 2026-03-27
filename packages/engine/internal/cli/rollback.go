package cli

import (
	"database/sql"
	"fmt"
	"log"
	"strconv"

	"github.com/dvflw/mantle/internal/audit"
	"github.com/dvflw/mantle/internal/auth"
	"github.com/dvflw/mantle/internal/config"
	"github.com/dvflw/mantle/internal/db"
	"github.com/spf13/cobra"
)

func newRollbackCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "rollback <workflow>",
		Short: "Revert a workflow to a previous version",
		Long: `Creates a new version of a workflow with the content of a previous version.
In-flight executions are unaffected. The rollback is recorded in the version
history via the rollback_of column.`,
		Example: `  mantle rollback my-workflow
  mantle rollback my-workflow --to-version 3`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			workflowName := args[0]

			cfg := config.FromContext(cmd.Context())
			if cfg == nil {
				return fmt.Errorf("config not loaded")
			}

			database, err := db.Open(cfg.Database)
			if err != nil {
				return fmt.Errorf("failed to connect to database: %w", err)
			}
			defer database.Close()

			teamID := auth.TeamIDFromContext(cmd.Context())

			// Wrap the entire read+insert in a transaction to prevent race conditions.
			tx, err := database.BeginTx(cmd.Context(), nil)
			if err != nil {
				return fmt.Errorf("starting transaction: %w", err)
			}
			defer tx.Rollback()

			// 1. Get current (latest) version and its content hash.
			var currentVersion int
			var currentHash string
			err = tx.QueryRowContext(cmd.Context(),
				`SELECT version, content_hash FROM workflow_definitions WHERE name = $1 AND team_id = $2 ORDER BY version DESC LIMIT 1`,
				workflowName, teamID,
			).Scan(&currentVersion, &currentHash)
			if err == sql.ErrNoRows {
				return fmt.Errorf("workflow %q not found", workflowName)
			}
			if err != nil {
				return fmt.Errorf("querying current version: %w", err)
			}

			// 2. Determine target version.
			toVersionStr, _ := cmd.Flags().GetString("to-version")
			var targetVersion int
			if toVersionStr != "" {
				targetVersion, err = strconv.Atoi(toVersionStr)
				if err != nil {
					return fmt.Errorf("invalid --to-version value: %w", err)
				}
			} else {
				// Get the second most recent version.
				err = tx.QueryRowContext(cmd.Context(),
					`SELECT version FROM workflow_definitions WHERE name = $1 AND team_id = $2 AND version < $3 ORDER BY version DESC LIMIT 1`,
					workflowName, teamID, currentVersion,
				).Scan(&targetVersion)
				if err == sql.ErrNoRows {
					return fmt.Errorf("workflow %q has only one version — nothing to roll back to", workflowName)
				}
				if err != nil {
					return fmt.Errorf("querying previous version: %w", err)
				}
			}

			// 3. Reject version <= 0.
			if targetVersion <= 0 {
				return fmt.Errorf("target version must be greater than 0, got %d", targetVersion)
			}

			// 4. Load target version content and hash.
			var targetContent []byte
			var targetHash string
			err = tx.QueryRowContext(cmd.Context(),
				`SELECT content, content_hash FROM workflow_definitions WHERE name = $1 AND version = $2 AND team_id = $3`,
				workflowName, targetVersion, teamID,
			).Scan(&targetContent, &targetHash)
			if err == sql.ErrNoRows {
				return fmt.Errorf("workflow %q version %d not found", workflowName, targetVersion)
			}
			if err != nil {
				return fmt.Errorf("querying target version: %w", err)
			}

			// 5. Reject no-op if content_hash matches current.
			if targetHash == currentHash {
				return fmt.Errorf("version %d has the same content as the current version %d — rollback is a no-op",
					targetVersion, currentVersion)
			}

			// 6. Insert new version with rollback_of.
			newVersion := currentVersion + 1
			_, err = tx.ExecContext(cmd.Context(),
				`INSERT INTO workflow_definitions (name, version, content, content_hash, rollback_of, team_id) VALUES ($1, $2, $3, $4, $5, $6)`,
				workflowName, newVersion, targetContent, targetHash, targetVersion, teamID,
			)
			if err != nil {
				return fmt.Errorf("inserting rolled-back version: %w", err)
			}

			if err := tx.Commit(); err != nil {
				return fmt.Errorf("committing rollback: %w", err)
			}

			// 7. Emit audit event.
			emitter := &audit.PostgresEmitter{DB: database}
			if err := emitter.Emit(cmd.Context(), audit.Event{
				Actor:  "cli",
				Action: audit.ActionWorkflowRolledBack,
				Resource: audit.Resource{
					Type: "workflow_definition",
					ID:   workflowName,
				},
				Metadata: map[string]string{
					"from_version":   strconv.Itoa(currentVersion),
					"to_version":     strconv.Itoa(targetVersion),
					"new_version":    strconv.Itoa(newVersion),
					"rollback_of":    strconv.Itoa(targetVersion),
				},
			}); err != nil {
				log.Printf("warning: failed to emit audit event: %v", err)
			}

			// 8. Print result.
			fmt.Fprintf(cmd.OutOrStdout(), "Rolled back %s from version %d to content of version %d (now version %d)\n",
				workflowName, currentVersion, targetVersion, newVersion)

			return nil
		},
	}

	cmd.Flags().String("to-version", "", "Target version to roll back to (default: previous version)")
	return cmd
}
