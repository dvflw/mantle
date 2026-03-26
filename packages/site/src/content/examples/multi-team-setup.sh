#!/usr/bin/env bash
# Multi-Team Setup Demo
#
# Demonstrates Mantle's Phase 6 multi-tenancy and RBAC features.
# This script creates teams, users with different roles, and API keys,
# then shows how to run a workflow scoped to a team.
#
# Prerequisites:
#   - Mantle binary built (make build)
#   - Postgres running (docker compose up -d)
#   - Migrations applied (./mantle init)
#
# This is a demonstration script — it requires a running database to execute.

set -euo pipefail

MANTLE="./mantle"

echo "==> Creating teams..."
$MANTLE teams create --name engineering
$MANTLE teams create --name data-science

echo ""
echo "==> Creating users with different roles..."

# Team owner — can manage team members and workflows
$MANTLE users create \
  --email alice@example.com \
  --name "Alice Chen" \
  --team engineering \
  --role team_owner

# Operator — can run and monitor workflows
$MANTLE users create \
  --email bob@example.com \
  --name "Bob Martinez" \
  --team engineering \
  --role operator

# Another team with its own owner
$MANTLE users create \
  --email carol@example.com \
  --name "Carol Kim" \
  --team data-science \
  --role team_owner

echo ""
echo "==> Listing users by team..."
$MANTLE users list --team engineering
$MANTLE users list --team data-science

echo ""
echo "==> Creating API keys for programmatic access..."

# Generate keys for CI/CD and automation
$MANTLE users api-key --email alice@example.com --key-name ci-deploy
$MANTLE users api-key --email bob@example.com --key-name monitoring

echo ""
echo "==> Applying a workflow (scoped to team via API key)..."
$MANTLE apply examples/scheduled-health-check.yaml

echo ""
echo "==> Running the workflow..."
$MANTLE run scheduled-health-check

echo ""
echo "========================================"
echo "  Multi-tenant setup complete!"
echo "========================================"
echo ""
echo "Each team's workflows, executions, and secrets are isolated."
echo "Users authenticate via API keys when using the REST API:"
echo ""
echo "  curl -H 'Authorization: Bearer mtl_...' http://localhost:8080/api/v1/workflows"
echo ""
