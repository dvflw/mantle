package cli

import (
	"encoding/json"
	"fmt"
	"text/tabwriter"

	"github.com/dvflw/mantle/internal/auth"
	"github.com/dvflw/mantle/internal/config"
	"github.com/dvflw/mantle/internal/db"
	"github.com/dvflw/mantle/internal/library"
	"github.com/dvflw/mantle/internal/workflow"
	"github.com/spf13/cobra"
)

func newLibraryCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "library",
		Short: "Manage the shared workflow template library",
		Long:  "Publish, browse, and deploy shared workflow templates.",
	}

	cmd.AddCommand(newLibraryPublishCommand())
	cmd.AddCommand(newLibraryListCommand())
	cmd.AddCommand(newLibraryDeployCommand())

	return cmd
}

func newLibraryPublishCommand() *cobra.Command {
	var workflowName string

	cmd := &cobra.Command{
		Use:   "publish",
		Short: "Publish a workflow as a shared template",
		Long:  "Reads the latest workflow definition from the database and stores it as a shared template.",
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

			content, _, err := workflow.GetLatestContent(cmd.Context(), database, workflowName)
			if err != nil {
				return fmt.Errorf("reading workflow: %w", err)
			}
			if content == nil {
				return fmt.Errorf("workflow %q not found", workflowName)
			}

			// Extract description from the stored workflow content if available.
			var wf struct {
				Description string `json:"description"`
			}
			_ = json.Unmarshal(content, &wf)

			if err := library.Publish(cmd.Context(), database, workflowName, wf.Description, content, auth.DefaultTeamID); err != nil {
				return err
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Published %q to shared library\n", workflowName)
			return nil
		},
	}

	cmd.Flags().StringVar(&workflowName, "workflow", "", "workflow name to publish (required)")
	_ = cmd.MarkFlagRequired("workflow")

	return cmd
}

func newLibraryListCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List all shared workflow templates",
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

			templates, err := library.List(cmd.Context(), database)
			if err != nil {
				return err
			}

			if len(templates) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "(no templates)")
				return nil
			}

			w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "NAME\tDESCRIPTION")
			for _, t := range templates {
				fmt.Fprintf(w, "%s\t%s\n", t.Name, t.Description)
			}
			return w.Flush()
		},
	}
}

func newLibraryDeployCommand() *cobra.Command {
	var templateName string
	var teamID string

	cmd := &cobra.Command{
		Use:   "deploy",
		Short: "Deploy a shared template as a workflow definition",
		Long:  "Copies a shared template into the target team's workflow definitions.",
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

			if teamID == "" {
				teamID = auth.DefaultTeamID
			}

			version, err := library.Deploy(cmd.Context(), database, templateName, teamID)
			if err != nil {
				return err
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Deployed %q as version %d\n", templateName, version)
			return nil
		},
	}

	cmd.Flags().StringVar(&templateName, "template", "", "template name to deploy (required)")
	cmd.Flags().StringVar(&teamID, "team", "", "target team ID (default: default team)")
	_ = cmd.MarkFlagRequired("template")

	return cmd
}

