# Mantle v0.2.0 Release Review

**Date:** 2026-03-25
**Scope:** v0.1.0...main (114 files, +9,583 / -6,083 lines)
**Reviewers:** Security Engineer, Code Reviewer, Software Architect, Technical Writer, Evidence Collector, Reality Checker

---

## Executive Summary

v0.2.0 is a substantial release adding Docker containers, artifacts, 15 CEL data-transformation functions, AI cost controls, and multiple new connectors (Postgres, S3, Email, Slack history). Architecture is sound -- clean package separation, extensible connector registry, no circular dependencies. However, **the release is not shippable as-is** due to 2 critical security findings, 1 critical dead-code issue, and several broken example workflows.

### Release Verdict: HOLD -- Fix P0 items before tagging

| Category | Critical | High | Medium | Low |
|----------|----------|------|--------|-----|
| Security | 2 | 1 | 4 | 2 |
| Code Quality | 2 | 4 | -- | -- |
| Architecture | -- | 1 | 2 | 2 |
| Documentation | 2 | 1 | 4 | 2 |
| Runtime | 1 | -- | 1 | -- |
| **Total** | **7** | **7** | **11** | **6** |

---

## P0 -- Must Fix Before Release

### SEC-1: Container Escape via Host Network Mode [CRITICAL/Security]

**File:** `internal/connector/docker.go:98-101`

A workflow author can set `network: "host"`, giving the container full access to the host network stack -- including localhost Postgres, cloud metadata endpoints (169.254.169.254), and any internal services.

**Fix:** Add a network mode allowlist. Reject `host` and any unrecognized modes:
```go
var allowedNetworkModes = map[string]bool{"bridge": true, "none": true}
```

### SEC-2: No Container Security Hardening [CRITICAL/Security]

**File:** `internal/connector/docker.go:272-274`

`HostConfig` is created with zero security constraints: no `CapDrop`, no `PidsLimit`, no `SecurityOpt`, no `ReadonlyRootfs`. Containers retain all default Linux capabilities and can fork-bomb the host.

**Fix:** Drop all capabilities, set `no-new-privileges`, limit PIDs:
```go
CapDrop: []string{"ALL"},
SecurityOpt: []string{"no-new-privileges"},
Resources: container.Resources{PidsLimit: int64Ptr(256)},
```

### RT-1: Artifact Reaper is Dead Code [CRITICAL/Runtime]

**File:** `internal/artifact/reaper.go` (never called outside tests)

The artifact `Reaper` struct exists and tests pass, but `NewReaper` / `Sweep` are never called from the server, engine, or any startup path. Artifacts with expired TTLs will accumulate indefinitely in production.

**Fix:** Wire `artifact.Reaper` into the server's `Run` loop with a periodic ticker, similar to `engine.Reaper`.

### DOC-1: `docker-volume-backup.yaml` is Broken [CRITICAL/Docs]

**File:** `examples/docker-volume-backup.yaml`

- Line 35: `{{ date }}` is not a valid CEL variable (no `date` in the environment)
- Lines 43, 53: `steps[...].error` is not exposed in CEL context (only `.output` exists)

**Fix:** Replace `{{ date }}` with a valid expression (e.g., `inputs.date` parameter). Remove `.error` references or mark steps as aspirational with clear comments.

### DOC-2: Multiple Example YAMLs Have Broken Field References [CRITICAL/Docs]

- `ai-tool-use.yaml`: `inputs` as list instead of map -- will fail YAML parsing
- `s3-backup.yaml:35`: `body` param should be `content`
- `db-backup.yaml:78`: `output.keys` should be `output.objects`
- `db-backup.yaml:12`: `env.MANTLE_DATABASE_URL` unreachable (only `MANTLE_ENV_` prefix exposed)
- `webhook-processor.yaml:26-27`: `inputs.trigger.*` should be `trigger.*`

### CQ-1: `limitWriter.Write` Violates io.Writer Contract [CRITICAL/Code]

**File:** `internal/connector/docker.go:174-191`

When the output limit is exceeded, `Write` returns `len(p), nil` even though zero bytes were written. This violates the `io.Writer` contract. While current callers tolerate this, future callers (e.g., `io.Copy`) will behave incorrectly.

**Fix:** Document the intentional "silent discard" semantics with a comment, or return `0, io.ErrShortWrite`.

### CQ-2: Multiple `defer os.RemoveAll` in Retry Loop [CRITICAL/Code]

**File:** `internal/engine/engine.go:447-459`

Each retry iteration pushes a new `defer os.RemoveAll(artifactsDir)`. Deferred cleanups accumulate and all fire on function return. While functionally safe (removing already-removed dirs is a no-op), the pattern is fragile -- the deferred cleanup of the *successful* attempt could race with artifact persistence.

**Fix:** Replace `defer` with explicit cleanup after the retry loop completes.

---

## P1 -- Should Fix Before Release

### SEC-3: Arbitrary Docker Volume Mounts [HIGH/Security]

**File:** `internal/connector/docker.go:277-284`

User-supplied mounts can reference any named Docker volume and target any container path (including `/proc`, `/sys`, `/dev`). No validation or restriction.

**Fix:** Reject dangerous container mount targets (`/proc`, `/sys`, `/dev`, `/var/run/docker.sock`).

### CQ-3: `rows.Err()` Missing Error Wrapping [Code]

**File:** `internal/artifact/store.go:77, 123`

`ListByExecution` and `ListExpired` return `rows.Err()` unwrapped. Callers see raw driver errors with no operation context.

### CQ-4: `GetByName` Swallows `sql.ErrNoRows` [Code]

**File:** `internal/artifact/store.go:49`

Returns a formatted error string instead of wrapping `sql.ErrNoRows`. Callers cannot distinguish "not found" from "database error" without string matching.

### CQ-5: Docker Connector Uses `log.Printf` Instead of `slog` [Code]

**File:** `internal/connector/docker.go:179, 186, 325, 342, 365, 407`

Inconsistent with the rest of the codebase which uses `log/slog`. Structured logging context (container ID, step name) is lost.

### CQ-6: Missing `%w` Error Wrapping in Engine [Code]

**File:** `internal/engine/engine.go:390-397`

Uses `%v` instead of `%w`, breaking the error chain for `errors.Is`/`errors.As`.

### ARCH-1: Orphaned Blob Recovery [HIGH/Architecture]

If the engine crashes between `TmpStorage.Put` and `ArtifactStore.Create`, blobs exist in storage with no metadata. The Reaper only cleans artifacts with metadata records, so orphaned blobs accumulate indefinitely.

**Fix:** Add a periodic storage reconciliation job that lists keys in TmpStorage and deletes any lacking corresponding metadata rows.

### DOC-3: Slack Channel ID vs Name Inconsistency [HIGH/Docs]

Connector reference says "Use the channel ID, not the channel name." Every example uses `#channel-name`. Either update the connector to accept names, or fix all examples to use IDs.

---

## P2 -- Should Fix (Next Sprint)

### SEC-4: CEL `split()` Unbounded Allocation [MEDIUM/Security]

**File:** `internal/cel/functions.go:75-86`

`strings.Split` with no limit on a 1MB string can produce ~1M allocations.
**Fix:** Use `strings.SplitN` with a 10,000-part cap.

### SEC-5: CEL `flatten()` Unbounded Allocation [MEDIUM/Security]

**File:** `internal/cel/functions.go:171-193`

No limit on total elements in flattened result. **Fix:** Cap at 100,000 elements.

### SEC-6: `jsonDecode` No Input Size Limit [MEDIUM/Security]

**File:** `internal/cel/functions.go:272-293`

Deeply nested JSON can cause stack overflow in recursive `normalizeJSONNumbers`.
**Fix:** Add 1MB input size limit; convert recursion to iterative.

### SEC-7: Artifact Bind Mount Missing Lstat Defense [MEDIUM/Security]

**File:** `internal/engine/engine.go:538`

Uses `os.Stat` (follows symlinks) instead of `os.Lstat`. The `TmpStorage.Put` has a Lstat check, but defense-in-depth says check earlier too.

### ARCH-2: Add `team_id` to `execution_artifacts` [MEDIUM/Architecture]

Denormalize now to avoid a painful migration during Phase 6 multi-tenancy. A nullable column with a backfill migration is trivial today.

### ARCH-3: Reaper Skip-on-Failure [MEDIUM/Architecture]

**File:** `internal/artifact/reaper.go:46`

When file deletion fails, metadata is not deleted, and the artifact is retried on every sweep with no backoff. Permanently unreadable files cause log spam forever.

### DOC-4: Missing `tmp` Block in Config Full Example [MEDIUM/Docs]

**File:** `site/src/content/docs/configuration.md`

The full config example omits the `tmp` storage block. Users scanning the example will miss artifact storage configuration.

### DOC-5: No Cross-Links Between Artifacts and Docker Docs [MEDIUM/Docs]

`concepts/artifacts.md` doesn't link to `docker-workflows.md`. Docker guide doesn't mention artifacts. These are closely related features with no cross-referencing.

### DOC-6: `registry_credential` Not in Connector Params Table [MEDIUM/Docs]

**File:** `site/src/content/docs/workflow-reference/connectors.md`

It's a step-level field, not a param. Needs a clarifying note in the `docker/run` section.

### DOC-7: `toString` vs `string()` Clarification Needed [MEDIUM/Docs]

**File:** `site/src/content/docs/concepts/expressions.md`

Both are used but the relationship isn't explained.

### EV-1: CI Integration Tests May Silently Skip [MEDIUM/Evidence]

Only `internal/artifact/test_helpers_test.go` has the `os.Getenv("CI")` guard that fatals when Postgres containers can't start. The other 6 packages with testcontainers will silently `t.Skipf`, potentially masking failures in CI.

**Fix:** Add the CI-aware fatal pattern to all testcontainers helpers.

---

## P3 -- Nice to Have

### SEC-8: Docker Output 20MB Memory Per Step [LOW/Security]

Two 10MB `limitWriter` buffers per docker step. Many parallel steps could cause memory pressure.

### SEC-9: CEL Program Cache Unbounded Growth [LOW/Security]

`sync.Map` with no eviction. Long-running servers will leak memory.

### ARCH-4: Docker Build Tag [LOW/Architecture]

Consider `//go:build !nodocker` for environments that don't need Docker, trimming the Moby dependency tree.

### ARCH-5: Document `obj()` 5-Pair Limit [LOW/Architecture]

Non-obvious constraint that should be more prominent in docs.

### EV-2: Makefile `test` Target Missing `-race` Flag [LOW/Evidence]

CI uses `go test -race`, but `make test` does not. Local testing misses race conditions.

### EV-3: `MigrateDown` Test Skipped [LOW/Evidence]

Down-migration rollback across all 13 migrations is untested. At minimum, test that migration 013 rolls back individually.

---

## Positive Findings

### Security Passes
- **SQL injection:** All queries use parameterized `$1, $2` placeholders. No string concatenation in SQL.
- **Secret leakage:** CEL environment only exposes `MANTLE_ENV_` prefixed vars. Credentials resolved after CEL evaluation, never exposed to expressions. AES-256-GCM encryption with random nonces correctly implemented.
- **Path traversal:** `FilesystemTmpStorage` validates all paths stay within `BasePath` using `filepath.Rel` and rejects `..` prefixes. Symlinks rejected via `os.Lstat`.
- **Dependencies:** No known vulnerable versions in `go.mod`. Clean -- no `replace` directives.

### Architecture Grades
| Area | Grade |
|------|-------|
| Docker connector coupling | A |
| CEL environment design | A |
| Migration 013 safety | A |
| Connector extensibility | A |
| Package separation | A |
| Artifact lifecycle | B+ |
| Single binary principle | A- |

### Test Coverage
- **469 test functions** across 63 test files
- All new packages have tests with both happy-path and error coverage
- Extensive table-driven tests (CEL functions, validation, Docker params)
- 8 packages use testcontainers for real Postgres integration testing
- CI exceeds CLAUDE.md requirements: runs `govulncheck` and `gosec` in addition to `go test -race`, `go vet`, `golangci-lint`

### Documentation Quality
- CEL function names/signatures match code exactly (all 15 verified)
- Getting-started flow is logical and progressively complex
- Navigation entries correct in `docs-nav.ts`
- Internal cross-references verified (valid link targets)

---

## Changelog Material for v0.2.0

### New Features
- **Docker connector (`docker/run`)** -- Run containers as workflow steps with stdin piping, env vars, volume mounts, resource limits, exit code branching, and private registry support
- **Artifacts** -- Large file passing between steps with configurable retention and automatic TTL cleanup
- **15 CEL functions** -- String (`toLower`, `toUpper`, `trim`, `replace`, `split`), type coercion (`parseInt`, `parseFloat`, `toString`), collections (`default`, `flatten`, `obj`), JSON (`jsonEncode`, `jsonDecode`), date/time (`parseTimestamp`, `formatTimestamp`)
- **Docker credential type** -- TLS-based daemon authentication with optional `host`, `ca_cert`, `client_cert`, `client_key`
- **AI cost controls** -- Per-provider, per-team, per-workflow token budgets
- **Init connection recovery** -- Docker auto-provisioning on `mantle init`
- **Postgres connector (`postgres/query`)** -- Parameterized SQL against external databases
- **Email connector (`email/send`)** -- SMTP with HTML support
- **S3 connectors (`s3/put`, `s3/get`, `s3/list`)** -- S3-compatible storage with custom endpoints
- **Slack history connector (`slack/history`)** -- Read channel messages
- **Cloud secret backends** -- AWS Secrets Manager, GCP Secret Manager, Azure Key Vault

### New Documentation
- Artifacts concept guide, Docker Workflows guide, Data Transformations guide
- Updated CEL Expressions reference, Connector Reference, Configuration guide, Secrets guide
- 4 new example workflows

---

*Report generated by 6 parallel review agents. Individual agent transcripts available in the task output files.*
