package workflow

import (
	"context"
	"database/sql"
	"testing"

	"github.com/dvflw/mantle/internal/auth"
)

// countTriggers returns the number of workflow_triggers rows for a workflow,
// optionally filtering to enabled rows only.
func countTriggers(t *testing.T, database *sql.DB, name string, enabledOnly bool) int {
	t.Helper()
	q := `SELECT COUNT(*) FROM workflow_triggers WHERE workflow_name = $1`
	if enabledOnly {
		q += ` AND enabled = true`
	}
	var n int
	if err := database.QueryRowContext(context.Background(), q, name).Scan(&n); err != nil {
		t.Fatalf("counting triggers: %v", err)
	}
	return n
}

var triggeredWorkflowYAML = []byte(`name: triggered-wf
description: workflow with all trigger types

triggers:
  - type: cron
    schedule: "*/5 * * * *"
  - type: webhook
    path: /incoming
    secret: shh
  - type: email
    mailbox: inbox-cred
    folder: INBOX
    filter: unseen
    poll_interval: 30s

steps:
  - name: s
    action: http/request
    params:
      method: GET
      url: "https://example.com"
`)

// TestSave_RegistersTriggers is the regression test for the bug where triggers
// declared in YAML were parsed but never written to workflow_triggers, so they
// never fired. Applying a workflow must populate the table the schedulers read.
func TestSave_RegistersTriggers(t *testing.T) {
	database := setupTestDB(t)
	ctx := context.Background()

	result, err := ParseBytes(triggeredWorkflowYAML)
	if err != nil {
		t.Fatalf("ParseBytes: %v", err)
	}
	version, err := Save(ctx, database, result, triggeredWorkflowYAML)
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	if version != 1 {
		t.Fatalf("version = %d, want 1", version)
	}

	if got := countTriggers(t, database, "triggered-wf", false); got != 3 {
		t.Fatalf("registered triggers = %d, want 3", got)
	}

	// The cron trigger row must carry the schedule and point at the version.
	var (
		schedule string
		ver      int
	)
	if err := database.QueryRowContext(ctx,
		`SELECT COALESCE(schedule, ''), workflow_version FROM workflow_triggers
		 WHERE workflow_name = 'triggered-wf' AND type = 'cron'`,
	).Scan(&schedule, &ver); err != nil {
		t.Fatalf("querying cron trigger: %v", err)
	}
	if schedule != "*/5 * * * *" {
		t.Errorf("cron schedule = %q, want %q", schedule, "*/5 * * * *")
	}
	if ver != 1 {
		t.Errorf("cron trigger version = %d, want 1", ver)
	}

	// The email trigger row must carry its email-specific fields.
	var mailbox, folder, filter, poll string
	if err := database.QueryRowContext(ctx,
		`SELECT COALESCE(mailbox,''), COALESCE(folder,''), COALESCE(filter,''), COALESCE(poll_interval,'')
		 FROM workflow_triggers WHERE workflow_name = 'triggered-wf' AND type = 'email'`,
	).Scan(&mailbox, &folder, &filter, &poll); err != nil {
		t.Fatalf("querying email trigger: %v", err)
	}
	if mailbox != "inbox-cred" || folder != "INBOX" || filter != "unseen" || poll != "30s" {
		t.Errorf("email fields = (%q,%q,%q,%q), want (inbox-cred,INBOX,unseen,30s)",
			mailbox, folder, filter, poll)
	}

	// The webhook trigger row must carry its path and secret.
	var path, secret string
	if err := database.QueryRowContext(ctx,
		`SELECT COALESCE(path,''), COALESCE(secret,'') FROM workflow_triggers
		 WHERE workflow_name = 'triggered-wf' AND type = 'webhook'`,
	).Scan(&path, &secret); err != nil {
		t.Fatalf("querying webhook trigger: %v", err)
	}
	if path != "/incoming" || secret != "shh" {
		t.Errorf("webhook fields = (%q,%q), want (/incoming,shh)", path, secret)
	}
}

// triggerIDs returns the sorted IDs of a workflow's trigger rows.
func triggerIDs(t *testing.T, database *sql.DB, name string) []string {
	t.Helper()
	rows, err := database.QueryContext(context.Background(),
		`SELECT id::text FROM workflow_triggers WHERE workflow_name = $1 ORDER BY id`, name)
	if err != nil {
		t.Fatalf("querying trigger ids: %v", err)
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			t.Fatalf("scanning id: %v", err)
		}
		ids = append(ids, id)
	}
	return ids
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// TestSave_BackfillsTriggersOnUnchangedApply is the regression test for the
// upgrade path: a workflow whose definition already exists but has no
// workflow_triggers rows (applied before trigger registration existed) must
// get its triggers backfilled when identical content is re-applied — without
// churning rows on subsequent no-op applies.
func TestSave_BackfillsTriggersOnUnchangedApply(t *testing.T) {
	database := setupTestDB(t)
	ctx := context.Background()

	result, _ := ParseBytes(triggeredWorkflowYAML)
	if _, err := Save(ctx, database, result, triggeredWorkflowYAML); err != nil {
		t.Fatalf("initial Save: %v", err)
	}

	// Simulate a pre-fix workflow: definition present, trigger rows absent.
	if _, err := database.ExecContext(ctx,
		`DELETE FROM workflow_triggers WHERE workflow_name = 'triggered-wf'`); err != nil {
		t.Fatalf("clearing triggers: %v", err)
	}
	if got := countTriggers(t, database, "triggered-wf", false); got != 0 {
		t.Fatalf("precondition: triggers = %d, want 0", got)
	}

	// Re-applying identical content must backfill the triggers (version stays 0).
	version, err := Save(ctx, database, result, triggeredWorkflowYAML)
	if err != nil {
		t.Fatalf("backfill Save: %v", err)
	}
	if version != 0 {
		t.Errorf("version = %d, want 0 (unchanged content)", version)
	}
	if got := countTriggers(t, database, "triggered-wf", true); got != 3 {
		t.Fatalf("backfilled enabled triggers = %d, want 3", got)
	}

	// A further identical apply must be a true no-op: rows already in sync,
	// so their IDs must not churn (the email poller keys off trigger IDs).
	ids1 := triggerIDs(t, database, "triggered-wf")
	if _, err := Save(ctx, database, result, triggeredWorkflowYAML); err != nil {
		t.Fatalf("no-op Save: %v", err)
	}
	ids2 := triggerIDs(t, database, "triggered-wf")
	if !equalStrings(ids1, ids2) {
		t.Errorf("triggers churned on no-op apply: %v -> %v", ids1, ids2)
	}
}

// TestSave_CronTriggersUniquePerTeam verifies that two teams can each register
// a workflow with the same name and cron schedule. Before the uniqueness index
// was team-scoped, the second team's apply failed with a unique-constraint
// violation on (workflow_name, schedule).
func TestSave_CronTriggersUniquePerTeam(t *testing.T) {
	database := setupTestDB(t)
	ctx := context.Background()

	// A second team — both workflow_definitions.team_id and
	// workflow_triggers.team_id are foreign keys to teams(id).
	teamB := "00000000-0000-0000-0000-0000000000b2"
	if _, err := database.ExecContext(ctx,
		`INSERT INTO teams (id, name) VALUES ($1, 'team-b')`, teamB); err != nil {
		t.Fatalf("creating team-b: %v", err)
	}

	yaml := []byte(`name: shared-name
triggers:
  - type: cron
    schedule: "*/5 * * * *"
steps:
  - name: s
    action: http/request
    params:
      method: GET
      url: "https://example.com"
`)
	result, err := ParseBytes(yaml)
	if err != nil {
		t.Fatalf("ParseBytes: %v", err)
	}

	// Default team.
	if _, err := Save(ctx, database, result, yaml); err != nil {
		t.Fatalf("Save (default team): %v", err)
	}
	// team-b — same workflow name and schedule.
	ctxB := auth.WithUser(ctx, &auth.User{TeamID: teamB})
	if _, err := Save(ctxB, database, result, yaml); err != nil {
		t.Fatalf("Save (team-b): %v", err)
	}

	// Each team owns exactly one cron trigger for the workflow.
	for _, tid := range []string{auth.DefaultTeamID, teamB} {
		var n int
		if err := database.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM workflow_triggers
			 WHERE workflow_name = 'shared-name' AND type = 'cron' AND team_id = $1`, tid,
		).Scan(&n); err != nil {
			t.Fatalf("counting triggers for %s: %v", tid, err)
		}
		if n != 1 {
			t.Errorf("team %s cron triggers = %d, want 1", tid, n)
		}
	}
}

// TestSave_BackfillPreservesDisabledState verifies that backfilling triggers
// for a currently-disabled workflow inserts them disabled, so a pruned
// workflow does not start firing on a re-apply.
func TestSave_BackfillPreservesDisabledState(t *testing.T) {
	database := setupTestDB(t)
	ctx := context.Background()

	result, _ := ParseBytes(triggeredWorkflowYAML)
	if _, err := Save(ctx, database, result, triggeredWorkflowYAML); err != nil {
		t.Fatalf("initial Save: %v", err)
	}
	if err := Disable(ctx, database, "triggered-wf"); err != nil {
		t.Fatalf("Disable: %v", err)
	}
	// Drop the rows to force a backfill on the next apply.
	if _, err := database.ExecContext(ctx,
		`DELETE FROM workflow_triggers WHERE workflow_name = 'triggered-wf'`); err != nil {
		t.Fatalf("clearing triggers: %v", err)
	}

	if _, err := Save(ctx, database, result, triggeredWorkflowYAML); err != nil {
		t.Fatalf("backfill Save: %v", err)
	}
	if got := countTriggers(t, database, "triggered-wf", false); got != 3 {
		t.Errorf("backfilled rows = %d, want 3", got)
	}
	if got := countTriggers(t, database, "triggered-wf", true); got != 0 {
		t.Errorf("enabled backfilled rows = %d, want 0 (workflow is disabled)", got)
	}
}

// TestSave_ReplacesTriggersOnNewVersion verifies a new version supersedes the
// prior triggers rather than accumulating duplicates.
func TestSave_ReplacesTriggersOnNewVersion(t *testing.T) {
	database := setupTestDB(t)
	ctx := context.Background()

	result, _ := ParseBytes(triggeredWorkflowYAML)
	if _, err := Save(ctx, database, result, triggeredWorkflowYAML); err != nil {
		t.Fatalf("first Save: %v", err)
	}

	// New version with a single cron trigger on a different schedule.
	updated := []byte(`name: triggered-wf
description: now with one trigger

triggers:
  - type: cron
    schedule: "0 * * * *"

steps:
  - name: s
    action: http/request
    params:
      method: GET
      url: "https://example.com"
`)
	result2, err := ParseBytes(updated)
	if err != nil {
		t.Fatalf("ParseBytes updated: %v", err)
	}
	version, err := Save(ctx, database, result2, updated)
	if err != nil {
		t.Fatalf("second Save: %v", err)
	}
	if version != 2 {
		t.Fatalf("version = %d, want 2", version)
	}

	if got := countTriggers(t, database, "triggered-wf", false); got != 1 {
		t.Fatalf("triggers after new version = %d, want 1", got)
	}
	var schedule string
	var ver int
	if err := database.QueryRowContext(ctx,
		`SELECT COALESCE(schedule,''), workflow_version FROM workflow_triggers WHERE workflow_name = 'triggered-wf'`,
	).Scan(&schedule, &ver); err != nil {
		t.Fatalf("querying trigger: %v", err)
	}
	if schedule != "0 * * * *" {
		t.Errorf("schedule = %q, want %q", schedule, "0 * * * *")
	}
	if ver != 2 {
		t.Errorf("trigger version = %d, want 2", ver)
	}
}

// TestSave_NoTriggersLeavesTableEmpty verifies a workflow without triggers
// registers none (and doesn't error on the empty set).
func TestSave_NoTriggersLeavesTableEmpty(t *testing.T) {
	database := setupTestDB(t)
	ctx := context.Background()

	result, _ := ParseBytes(testWorkflowYAML) // defined in store_test.go, has no triggers
	if _, err := Save(ctx, database, result, testWorkflowYAML); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if got := countTriggers(t, database, "test-workflow", false); got != 0 {
		t.Errorf("triggers = %d, want 0", got)
	}
}

// TestDisableReenable_TogglesTriggerEnabled verifies that disabling a workflow
// (e.g. a GitOps prune) stops its triggers from firing, and re-enabling
// restores them — including the case where the definition is unchanged so Save
// would not re-register them.
func TestDisableReenable_TogglesTriggerEnabled(t *testing.T) {
	database := setupTestDB(t)
	ctx := context.Background()

	result, _ := ParseBytes(triggeredWorkflowYAML)
	if _, err := Save(ctx, database, result, triggeredWorkflowYAML); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if got := countTriggers(t, database, "triggered-wf", true); got != 3 {
		t.Fatalf("enabled triggers = %d, want 3", got)
	}

	if err := Disable(ctx, database, "triggered-wf"); err != nil {
		t.Fatalf("Disable: %v", err)
	}
	if got := countTriggers(t, database, "triggered-wf", true); got != 0 {
		t.Errorf("enabled triggers after Disable = %d, want 0", got)
	}
	// Rows are retained (disabled), not deleted.
	if got := countTriggers(t, database, "triggered-wf", false); got != 3 {
		t.Errorf("total triggers after Disable = %d, want 3", got)
	}

	if err := Reenable(ctx, database, "triggered-wf"); err != nil {
		t.Fatalf("Reenable: %v", err)
	}
	if got := countTriggers(t, database, "triggered-wf", true); got != 3 {
		t.Errorf("enabled triggers after Reenable = %d, want 3", got)
	}
}
