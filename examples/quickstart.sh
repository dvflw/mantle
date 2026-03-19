#!/usr/bin/env bash
set -euo pipefail

# Mantle Quickstart
# Builds Mantle, starts Postgres, and runs your first workflow in under 3 minutes.

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

echo "==> Checking prerequisites..."
command -v go >/dev/null 2>&1 || { echo "Error: Go is not installed. Install from https://go.dev/dl/"; exit 1; }
command -v docker >/dev/null 2>&1 || { echo "Error: Docker is not installed. Install from https://docker.com"; exit 1; }

echo "==> Building mantle..."
make build

echo "==> Starting Postgres..."
docker compose up -d --wait 2>/dev/null || docker-compose up -d 2>/dev/null
sleep 2

echo "==> Running migrations..."
./mantle init

echo ""
echo "==> Validating hello-world workflow..."
./mantle validate examples/hello-world.yaml

echo ""
echo "==> Applying hello-world workflow..."
./mantle apply examples/hello-world.yaml

echo ""
echo "==> Running hello-world workflow..."
./mantle run hello-world

echo ""
echo "========================================"
echo "  Mantle is working!"
echo "========================================"
echo ""
echo "Try these next:"
echo ""
echo "  # Two-step workflow with data passing between steps"
echo "  ./mantle apply examples/chained-requests.yaml"
echo "  ./mantle run chained-requests"
echo ""
echo "  # Conditional workflow with inputs, retry, and timeout"
echo "  ./mantle apply examples/conditional-workflow.yaml"
echo "  ./mantle run conditional-workflow --input user_id=3"
echo ""
echo "  # See what changed before applying"
echo "  ./mantle plan examples/hello-world.yaml"
echo ""
echo "  # View execution history"
echo "  ./mantle logs <execution-id>"
echo ""
