# Releasing Mantle

Mantle uses [Changesets](https://github.com/changesets/changesets) for versioning and automated releases.
The CI pipeline is fully automated once the Version PR is merged — no manual tagging or publishing required.

## Prerequisites

Before performing a manual release (bootstrap or re-tag), confirm you have:

- **Push access** to `main` and tags on `github.com/dvflw/mantle`
- **`gh` CLI** installed and authenticated (`gh auth status`)
- **GHCR write access** — your GitHub account must have write access to `ghcr.io/dvflw` (granted via org membership)

The automated CI path (Version PR → merge) requires no local tooling beyond git.

## How It Works

```
1. changeset file added during dev
         ↓
2. push to main → release-please.yml creates/updates "Version PR"
         ↓
3. merge Version PR
         ↓
4. release-tags.yml detects version bumps, pushes package-scoped tags
         ↓
5a. release-engine.yml  → goreleaser: binaries + Docker + GitHub Release + Trivy
5b. release-helm.yml    → helm push OCI + GitHub Release
```

> **Note on workflow naming:** `release-please.yml` is named after a tool we evaluated but didn't adopt — it actually runs the Changesets CLI. A rename to `changeset-version.yml` is tracked in [the relevant issue].

## Step 1: Add a Changeset (During Development)

Every PR that changes user-visible behavior needs a changeset. Run this from the repo root:

```bash
bun run changeset
```

The interactive CLI asks:
1. **Which packages changed?** Select from `@mantle/engine`, `@mantle/helm-chart`, `@mantle/site`
2. **Bump type per package:** `major` / `minor` / `patch`
3. **Summary:** One-line description of the change (appears in CHANGELOG)

This creates a file like `.changeset/fuzzy-lions-dance.md`. Commit it alongside your code.

> **`@mantle/site`:** changesets version the docs site but do not trigger a separate release workflow. The `site/v*` tag is pushed automatically but there is no equivalent `release-site.yml` — site deploys happen via the Astro/hosting pipeline.

**When to use each bump type:**
- `patch` — bug fixes, internal refactors, docs updates
- `minor` — new features, new CLI commands, new config fields
- `major` — breaking changes (workflow YAML format, CLI flags, API shape)

If a PR touches only CI, tests, or tooling with no user impact, skip the changeset.

## Step 2: The Version PR

After any push to `main` that contains pending changesets, the `release-please.yml` workflow automatically creates (or updates) a PR titled **"ci: version packages"**.

This PR:
- Bumps `version` in each affected `package.json` (taking the highest bump across all pending changesets)
- Generates / appends to `packages/*/CHANGELOG.md`
- Deletes the consumed changeset files

Review the PR to confirm the version bumps and changelog entries are correct. If more changesets land on `main` before you merge, CI updates the PR automatically.

> **Loop guard:** the workflow has an `if` condition that skips runs where the triggering commit message starts with `"ci: version packages"`. This prevents the workflow from retriggering on its own Version PR merge commit. If you ever rename the commit message in the workflow, update the guard condition in `release-please.yml` to match.

## Step 3: Merge the Version PR

When you're ready to cut a release, **merge the Version PR**. That's the release trigger.

## Step 4: Tags and Artifacts (Automated)

After the Version PR merges, `release-tags.yml` fires and:

1. Detects which `package.json` files changed version
2. If the engine version bumped, syncs `Chart.yaml appVersion` and commits it back to `main`
3. Pushes package-scoped tags: `engine/v<version>`, `helm-chart/v<version>`, `site/v<version>`

Those tags trigger the package-specific release workflows:

| Tag pattern | Workflow | What it produces |
|---|---|---|
| `engine/v*` | `release-engine.yml` | goreleaser builds Linux/macOS amd64/arm64 binaries + checksums + LICENSE, pushes multi-arch `ghcr.io/dvflw/mantle:<version>` Docker image (+ floating tags on stable), creates GitHub Release, runs Trivy CVE scan |
| `helm-chart/v*` | `release-helm.yml` | Packages and pushes `oci://ghcr.io/dvflw/helm-charts/mantle:<version>`, creates GitHub Release |

**How the engine tag translates to a clean version:** the workflow strips the `engine/` prefix, sets `GORELEASER_CURRENT_TAG=v<version>`, and creates a local `git tag v<version>` alias so goreleaser can validate the tag against the current commit. Only the namespaced tag (`engine/v<version>`) is permanent in the remote.

**Floating tags** (`major.minor`, `major`, `latest`) are only pushed for stable releases. For pre-release versions (e.g. `0.5.0-rc.1`) the versioned image is pushed but floating tags are skipped.

**Platform-specific image tags:** the pipeline no longer publishes per-arch tags like `ghcr.io/dvflw/mantle:<version>-amd64`. Only the multi-arch manifest tag (`ghcr.io/dvflw/mantle:<version>`) is pushed. Update any CI or deploy configs that reference the old suffixed tags.

**Trivy scan policy:** the `trivy` job runs after `release` with `exit-code: 1` on `CRITICAL` or `HIGH` CVEs. A failure marks the workflow run as failed but does not retract the already-published release. If a CVE scan fails post-release, open a patch release issue immediately.

**Partial-failure recovery:** if `release-tags.yml` fails after pushing some but not all tags, push the missing tags manually:

```bash
git tag helm-chart/v<version>
git push origin helm-chart/v<version>
```

The tag-triggered workflows are safe to retrigger — goreleaser will fail cleanly if the GitHub Release already exists.

## First Release of a New Version (No Pending Changesets)

This applies when `package.json` is already at the target version but `.changeset/` contains no pending changeset files — for example, the first-ever release of a newly bootstrapped repo, or after manually editing `package.json` outside the changeset flow.

In this state the `release-please.yml` workflow has nothing to process and will not create a Version PR. Push the tags directly:

```bash
git tag engine/v<version>
git push origin engine/v<version>

# If also releasing the Helm chart at the same version:
git tag helm-chart/v<version>
git push origin helm-chart/v<version>
```

After the initial tag, use the full changeset flow for all subsequent releases.

## Verifying a Release

After the workflows complete, substitute your version for `<version>`:

```bash
# Check GitHub Releases
gh release list

# Verify Docker image
docker pull ghcr.io/dvflw/mantle:<version>
docker run --rm ghcr.io/dvflw/mantle:<version> mantle version

# Verify Helm chart
helm show chart oci://ghcr.io/dvflw/helm-charts/mantle --version <version>
```

## Pre-releases

To cut a pre-release (e.g. `0.5.0-rc.1`), no changeset is needed — bump the version manually and tag directly:

```bash
# 1. Set the pre-release version in package.json
#    Edit packages/engine/package.json: "version": "0.5.0-rc.1"
git add packages/engine/package.json
git commit -m "chore: bump engine to 0.5.0-rc.1"
git push origin main

# 2. Tag and push
git tag engine/v0.5.0-rc.1
git push origin engine/v0.5.0-rc.1
```

goreleaser's `prerelease: auto` publishes the GitHub Release as a pre-release when the version contains a pre-release identifier. The versioned Docker image (`0.5.0-rc.1`) is pushed; floating tags (`latest`, `major`, `major.minor`) are not.

## Rollback

Releases are immutable — GitHub Releases and pushed OCI images cannot be unpublished retroactively for users who have already pulled them. Rolling back means directing users to the last known-good version and cutting a patch.

**1. Communicate immediately** with the last known-good version and the broken version to avoid.

**2. Optionally hide the broken release from the GitHub Releases UI:**

```bash
# Mark as pre-release so it no longer shows as the "latest" release
gh release edit engine/v<broken-version> --prerelease

# Or move to draft (still accessible by direct URL/tag, just hidden from listing)
gh release edit engine/v<broken-version> --draft
```

> These do not retract published Docker images or Helm charts from GHCR. Downstream users who have already pulled the image are not affected.

**3. Cut a patch release** as the authoritative fix via the normal changeset flow.
