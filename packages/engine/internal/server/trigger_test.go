package server

import (
	"context"
	"testing"

	"github.com/dvflw/mantle/internal/auth"
)

// TestLookupWebhookTrigger_TeamScoped verifies that webhook path uniqueness and
// lookup are per-team: two teams may register the same path, and a lookup under
// one team's context resolves only that team's trigger — never the other's. The
// /hooks/ endpoint is authenticated, so the caller's team is a tenant boundary;
// a caller must not be able to trigger another team's webhook that happens to
// share a path.
func TestLookupWebhookTrigger_TeamScoped(t *testing.T) {
	db, _ := setupWebhookTest(t)
	ctx := context.Background()

	teamA := auth.DefaultTeamID
	teamB := "00000000-0000-0000-0000-0000000000b2"
	if _, err := db.ExecContext(ctx,
		`INSERT INTO teams (id, name) VALUES ($1, 'team-b')`, teamB); err != nil {
		t.Fatalf("creating team-b: %v", err)
	}

	// Both teams register the same webhook path pointing at their own workflow.
	for _, tc := range []struct{ team, wf string }{
		{teamA, "wf-a"},
		{teamB, "wf-b"},
	} {
		if _, err := db.ExecContext(ctx,
			`INSERT INTO workflow_triggers (workflow_name, workflow_version, type, path, team_id)
			 VALUES ($1, 1, 'webhook', '/hooks/shared', $2)`, tc.wf, tc.team); err != nil {
			t.Fatalf("seeding webhook trigger for %s: %v", tc.team, err)
		}
	}

	// Each caller resolves only its own team's trigger.
	for _, tc := range []struct{ team, wantWF string }{
		{teamA, "wf-a"},
		{teamB, "wf-b"},
	} {
		callerCtx := auth.WithUser(ctx, &auth.User{TeamID: tc.team})
		tr, err := LookupWebhookTrigger(callerCtx, db, "/hooks/shared")
		if err != nil {
			t.Fatalf("lookup for %s: %v", tc.team, err)
		}
		if tr == nil {
			t.Fatalf("lookup for %s: expected trigger, got nil", tc.team)
		}
		if tr.WorkflowName != tc.wantWF {
			t.Errorf("team %s resolved workflow %q, want %q", tc.team, tr.WorkflowName, tc.wantWF)
		}
		if tr.TeamID != tc.team {
			t.Errorf("team %s resolved team_id %q, want %q", tc.team, tr.TeamID, tc.team)
		}
	}

	// A caller from a team with no matching path gets nothing — no cross-team leak.
	teamC := "00000000-0000-0000-0000-0000000000c3"
	if _, err := db.ExecContext(ctx,
		`INSERT INTO teams (id, name) VALUES ($1, 'team-c')`, teamC); err != nil {
		t.Fatalf("creating team-c: %v", err)
	}
	callerCtx := auth.WithUser(ctx, &auth.User{TeamID: teamC})
	tr, err := LookupWebhookTrigger(callerCtx, db, "/hooks/shared")
	if err != nil {
		t.Fatalf("lookup for team-c: %v", err)
	}
	if tr != nil {
		t.Errorf("team-c should resolve no trigger, got %+v", tr)
	}
}
