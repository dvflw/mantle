package server

import (
	"context"
	"testing"

	"github.com/dvflw/mantle/internal/auth"
)

// TestLookupWebhookTrigger_GlobalPathReturnsOwningTeam verifies that a webhook
// registered by a non-default team is found by an unauthenticated inbound
// lookup (which carries no team identity) and reports the owning team, so the
// handler can run the workflow under the correct tenant. Previously the lookup
// was team-scoped to the default team, so non-default-team webhooks never
// routed.
func TestLookupWebhookTrigger_GlobalPathReturnsOwningTeam(t *testing.T) {
	db, _ := setupWebhookTest(t)
	ctx := context.Background()

	teamB := "00000000-0000-0000-0000-0000000000b2"
	if _, err := db.ExecContext(ctx,
		`INSERT INTO teams (id, name) VALUES ($1, 'team-b')`, teamB); err != nil {
		t.Fatalf("creating team-b: %v", err)
	}
	if _, err := db.ExecContext(ctx,
		`INSERT INTO workflow_triggers (workflow_name, workflow_version, type, path, team_id)
		 VALUES ('wf', 1, 'webhook', '/hooks/tb', $1)`, teamB); err != nil {
		t.Fatalf("seeding webhook trigger: %v", err)
	}

	// No team in context, as an inbound webhook request would be.
	tr, err := LookupWebhookTrigger(ctx, db, "/hooks/tb")
	if err != nil {
		t.Fatalf("LookupWebhookTrigger: %v", err)
	}
	if tr == nil {
		t.Fatal("expected trigger, got nil")
	}
	if tr.TeamID != teamB {
		t.Errorf("TeamID = %q, want %q", tr.TeamID, teamB)
	}
	if tr.WorkflowName != "wf" {
		t.Errorf("WorkflowName = %q, want wf", tr.WorkflowName)
	}

	// A default-team context must not change the result — the path is a global
	// routing key, not team-scoped.
	ctxDefault := auth.WithUser(ctx, &auth.User{TeamID: auth.DefaultTeamID})
	tr2, err := LookupWebhookTrigger(ctxDefault, db, "/hooks/tb")
	if err != nil {
		t.Fatalf("LookupWebhookTrigger (default ctx): %v", err)
	}
	if tr2 == nil || tr2.TeamID != teamB {
		t.Errorf("global lookup should be team-agnostic; got %+v", tr2)
	}
}
