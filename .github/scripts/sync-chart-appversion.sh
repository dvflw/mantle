#!/usr/bin/env bash
set -euo pipefail

# Usage: sync-chart-appversion.sh <engine-version>
# Example: sync-chart-appversion.sh 0.4.1

if [[ -z "${1-}" ]]; then
  echo "Usage: sync-chart-appversion.sh <engine_version>" >&2
  exit 1
fi

ENGINE_VERSION="$1"
CHART_FILE="packages/helm-chart/Chart.yaml"

if [[ ! -f "$CHART_FILE" ]]; then
  echo "ERROR: $CHART_FILE not found"
  exit 1
fi

sed -i "s/^appVersion:.*/appVersion: \"${ENGINE_VERSION}\"/" "$CHART_FILE"
echo "Updated $CHART_FILE appVersion to $ENGINE_VERSION"
