package workflow

import (
	"context"
	"database/sql"
	"testing"
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
