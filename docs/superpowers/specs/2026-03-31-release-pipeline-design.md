# Automated Release Pipeline Design

> **Issue:** [#70 — Automated release pipeline (goreleaser, Helm publish, site deploy)](https://github.com/dvflw/mantle/issues/70)

## Overview

Automate the full release lifecycle for all three monorepo packages (engine, helm-chart, site) using changesets for version management, goreleaser for engine builds, OCI registry for Helm chart distribution, and Cloudflare Pages for site deployment.

## Release Trigger Flow

Changesets manages versioning across all packages. The flow:

1. Developer runs `bunx changeset` to describe changes and semver bump level
2. Changeset `.md` file is committed with the PR
3. On merge to main, `release-please.yml` runs `changeset version`, bumps versions, updates changelogs, and opens/updates a "Version Packages" PR
4. Merging the Version PR triggers `release-tags.yml`, which diffs the merge commit to detect version bumps and creates per-package git tags (`engine/v0.4.1`, `helm-chart/v0.2.0`, etc.)
5. Per-package tag creation triggers the corresponding release workflow

The site is an exception — it deploys on every push to main that touches `packages/site/**`, not gated by version tags.

## Workflow File Structure

| Workflow | Trigger | Purpose |
|---|---|---|
| `release-please.yml` | push to main | Run changesets, open/update Version PR |
| `release-tags.yml` | push to main (filters to commits from changesets bot via commit message check) | Detect bumped packages, create per-package git tags, sync `Chart.yaml` `appVersion` |
| `release-engine.yml` | `engine/v*` tag | goreleaser: binaries, Docker image, GitHub Release |
| `release-helm.yml` | `helm-chart/v*` tag | `helm package` + `helm push` to GHCR OCI registry |
| `site-deploy.yml` | push to main (paths: `packages/site/**`) | Build Astro + deploy to Cloudflare Pages |

Existing `engine-ci.yml` and `helm-ci.yml` remain unchanged for PR checks. The current `release.yml` is deleted — its responsibilities move to `release-engine.yml`.

## Engine Release (goreleaser)

A `.goreleaser.yaml` at `packages/engine/.goreleaser.yaml` replaces the hand-rolled build script.

- **Builds:** Single `mantle` binary, `CGO_ENABLED=0`, 4 targets: `linux/amd64`, `linux/arm64`, `darwin/amd64`, `darwin/arm64`. Ldflags inject version, commit, and date.
- **Archives:** Tarballs named `mantle-{os}-{arch}.tar.gz` (matches existing convention)
- **Checksum:** `checksums.txt` (SHA256)
- **Changelog:** Auto-generated from conventional commits, grouped by type
- **Docker:** Multi-platform images (`linux/amd64`, `linux/arm64`) pushed to `ghcr.io/dvflw/mantle`. Tags: `{{.Version}}`, `{{.Major}}.{{.Minor}}`, `{{.Major}}`, `latest`. Uses existing `Dockerfile`.
- **Release:** Creates GitHub Release with changelog and artifacts
- **Trivy:** Runs as a separate workflow step after Docker push (not in goreleaser config)

The workflow sets `GORELEASER_CURRENT_TAG` from the `engine/v*` tag with the `engine/` prefix stripped.

## Helm Chart OCI Publish

When a `helm-chart/v*` tag is created, `release-helm.yml`:

1. Checks out the repo
2. Sets up Helm
3. Logs in to GHCR via `helm registry login ghcr.io`
4. Runs `helm package packages/helm-chart/`
5. Runs `helm push mantle-*.tgz oci://ghcr.io/dvflw/helm-charts`
6. Creates a lightweight GitHub Release for the tag with a link to the OCI artifact

Users install with:
```bash
helm install mantle oci://ghcr.io/dvflw/helm-charts/mantle --version 0.2.0
```

### appVersion Sync

`Chart.yaml` has two version fields:
- `version` — the chart's own version, bumped by changesets when chart templates change
- `appVersion` — informational label for which engine version the chart targets

`release-tags.yml` auto-syncs `appVersion` whenever an engine tag is created: it updates `Chart.yaml`, commits, and pushes. This eliminates manual tracking of the engine version in two places.

## Site Deploy

`site-deploy.yml` triggers on push to main when `packages/site/**` changes.

1. Checks out the repo
2. Installs bun
3. Runs `bun install` and `bun run build` in `packages/site/`
4. Uses `cloudflare/wrangler-action` to deploy `packages/site/dist/` to Cloudflare Pages

No version tagging — docs deploy immediately on merge. Requires `CLOUDFLARE_API_TOKEN` and `CLOUDFLARE_ACCOUNT_ID` repository secrets.

## Changesets Integration

**Package registration:** Each package needs a `package.json` with `name` and `version` for changesets to track. The site already has one. Lightweight `package.json` files are added to engine and helm-chart (just `name` + `version`, no dependencies).

**Chart.yaml sync:** Rather than using a changesets `versionScript`, the `appVersion` sync is handled at tag-creation time in `release-tags.yml`. When an engine tag is created, the workflow updates `Chart.yaml`'s `appVersion`, commits, and pushes. This is cleaner than syncing during version PR preparation because it's driven by actual engine releases. The engine version at build time comes from the git tag via ldflags.

**Tag format:** `engine/v0.4.1`, `helm-chart/v0.2.0`, `site/v1.1.0`. The `release-tags.yml` workflow diffs the merge commit to detect which `package.json` versions changed and creates tags accordingly.

## File Changes

### New files
- `.github/workflows/release-please.yml`
- `.github/workflows/release-tags.yml`
- `.github/workflows/release-engine.yml`
- `.github/workflows/release-helm.yml`
- `.github/workflows/site-deploy.yml`
- `packages/engine/.goreleaser.yaml`
- `packages/engine/package.json` (version anchor for changesets)
- `packages/helm-chart/package.json` (version anchor for changesets)

### Modified files
- Root `package.json` — add `workspaces` field to register all packages

### Deleted files
- `.github/workflows/release.yml` — replaced by `release-engine.yml`

## Required Secrets

| Secret | Purpose |
|---|---|
| `GITHUB_TOKEN` | Already available. Used by changesets, goreleaser, Helm push, tag creation |
| `CLOUDFLARE_API_TOKEN` | Site deploy via wrangler |
| `CLOUDFLARE_ACCOUNT_ID` | Site deploy via wrangler |
