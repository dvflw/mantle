#!/usr/bin/env bash
set -euo pipefail

# Shared Workflow Library Demo
#
# Demonstrates Mantle's shared workflow library for publishing, discovering,
# and deploying reusable workflow templates across teams.
#
# Prerequisites:
#   - Mantle binary built (make build)
#   - Postgres running (docker compose up -d)
#   - Migrations applied (./mantle init)
#   - At least one workflow applied (e.g., ./mantle apply examples/hello-world.yaml)
#
# This is a demonstration script — it requires a running database to execute.

MANTLE="./mantle"

echo "==> Publishing workflows as library templates..."
echo ""
echo "Publish a workflow so other teams can discover and reuse it."
$MANTLE library publish --workflow hello-world
$MANTLE library publish --workflow scheduled-health-check

echo ""
echo "==> Listing available templates..."
echo ""
echo "Any team can browse the shared library."
$MANTLE library list

echo ""
echo "==> Deploying a template to a team..."
echo ""
echo "Deploy a library template into a team's workspace. The team gets"
echo "their own copy that they can customize."
$MANTLE library deploy --template hello-world --team engineering
$MANTLE library deploy --template scheduled-health-check --team data-science

echo ""
echo "==> Verifying deployed workflows..."
$MANTLE validate examples/hello-world.yaml
$MANTLE validate examples/scheduled-health-check.yaml

echo ""
echo "========================================"
echo "  Shared library demo complete!"
echo "========================================"
echo ""
echo "Library workflow:"
echo ""
echo "  1. Author a workflow and apply it:    ./mantle apply my-workflow.yaml"
echo "  2. Publish it to the shared library:  ./mantle library publish --workflow my-workflow"
echo "  3. Other teams discover it:           ./mantle library list"
echo "  4. Teams deploy their own copy:       ./mantle library deploy --template my-workflow --team my-team"
echo ""
