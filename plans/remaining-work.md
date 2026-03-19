# Remaining Work Plan

> Compiled 2026-03-19 from comprehensive review findings + Linear backlog.
> Priority: P1 items first, then P2, then Linear backlog.

---

## P1: Review Findings — Must Address

### 1. Team scoping not enforced in application code

**Source:** Software Architect review
**Risk:** Any team can read/write other teams' workflows, secrets, and executions in multi-tenant mode.

**Files to modify:**
- `internal/workflow/store.go` — `Save`, `GetLatestVersion`, `GetLatestHash`, `GetLatestContent` need `AND team_id = $N`
- `internal/secret/store.go` — `Get`, `List`, `Delete`, `Create` need team_id scoping
- `internal/engine/engine.go` — `loadWorkflow` needs team_id filter
- `internal/server/api.go` — all handlers need to extract team_id from auth context
- `internal/server/server.go` — `getLatestVersion` needs team_id

**Approach:** Add a `TeamScope` context helper that all DB queries use. The auth middleware already sets the authenticated user/team in context — extract team_id from there and pass it through. This is a systematic fix across ~15 query sites.

**Estimate:** Medium — touches many files but each change is mechanical (add `AND team_id = $N`).

---

### 2. Unbounded tool-loop message growth

**Source:** AI Engineer + Optimization Architect reviews
**Risk:** Quadratic token cost. With max_tool_rounds=50 and max_tool_calls_per_round=25, conversation can grow to 1250+ messages, each round sending the full history.

**Files to modify:**
- `internal/connector/tools.go` — `ToolLoop` struct and `Run` method

**Approach:**
- Add `MaxMessageBytes int` field to `ToolLoop` (default 128KB)
- Track cumulative message size across rounds
- When approaching limit, truncate older tool results to summaries
- Add `MaxTokenBudget int` field — track `usage.total_tokens` across rounds, abort if exceeded
- Truncate individual tool results beyond a configurable byte limit (default 32KB)

**Estimate:** Small-medium — contained to one file.

---

### 3. Worker/reaper liveness in health checks

**Source:** SRE review
**Risk:** A node appears healthy to Kubernetes but its worker/reaper has silently died. No restart triggered.

**Files to modify:**
- `internal/engine/worker.go` — add `LastPollAt` atomic timestamp
- `internal/engine/reaper.go` — add `LastRunAt` atomic timestamp
- `internal/api/health/health.go` — extend `/readyz` to check worker/reaper liveness
- `internal/server/server.go` — pass worker/reaper refs to health check

**Approach:**
- Worker updates `LastPollAt` on every poll cycle (successful or not)
- Reaper updates `LastRunAt` on every reap cycle
- `/readyz` checks: if `time.Since(LastPollAt) > 3 * PollInterval`, return 503
- Wrap goroutines with panic recovery that logs and sets a degraded flag

**Estimate:** Small — straightforward atomic timestamp + health check extension.

---

### 4. PodDisruptionBudget in Helm chart

**Source:** SRE + DevOps reviews
**Risk:** Cluster drain can kill all replicas simultaneously, interrupting all in-flight executions.

**Files to create:**
- `charts/mantle/templates/pdb.yaml`

**Approach:**
```yaml
apiVersion: policy/v1
kind: PodDisruptionBudget
metadata:
  name: {{ include "mantle.fullname" . }}
spec:
  minAvailable: 1
  selector:
    matchLabels:
      {{- include "mantle.selectorLabels" . | nindent 6 }}
```
Add `pdb.enabled` to values.yaml (default true when replicaCount > 1).

**Estimate:** Trivial — one template file.

---

### 5. Migration init container races

**Source:** DevOps review
**Risk:** Multiple replicas race to run migrations simultaneously on startup.

**Files to modify:**
- `charts/mantle/templates/deployment.yaml` — replace init container with a Helm hook Job

**Approach:** Create a `charts/mantle/templates/migration-job.yaml` with `helm.sh/hook: pre-install,pre-upgrade` annotation. Remove the init container from the deployment. The Job runs once per deploy, before any pods start.

**Estimate:** Small — one new template, one edit.

---

## P2: Review Findings — Should Address

### 6. `io.LimitReader` on HTTP/AI connector responses

**Source:** Software Architect review
**Risk:** OOM from large response bodies (malicious or misconfigured endpoints).

**Files:** `internal/connector/http.go`, `internal/connector/ai.go`, `internal/connector/slack.go`, `internal/connector/s3.go`
**Fix:** Replace `io.ReadAll(resp.Body)` with `io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))`. Default 10MB.

---

### 7. Security scanning in CI

**Source:** DevOps review

**Files:** `.github/workflows/ci.yml`
**Fix:** Add jobs for `govulncheck`, `gosec`, and container image scanning (Trivy).

---

### 8. Startup probe in Helm chart

**Source:** SRE review

**Files:** `charts/mantle/templates/deployment.yaml`
**Fix:** Add `startupProbe` with `failureThreshold: 30`, `periodSeconds: 2`.

---

### 9. Pin golangci-lint version in CI

**Source:** DevOps review

**Files:** `.github/workflows/ci.yml`
**Fix:** Change `version: latest` to `version: v1.62` (or current stable).

---

### 10. CEL program caching

**Source:** Software Architect review
**Risk:** CEL compiles on every `Eval` call. CPU-bound at scale.

**Files:** `internal/cel/cel.go`
**Fix:** Add `sync.Map` cache keyed by expression string. Cache compiled `cel.Program` objects.

---

### 11. NodeID generation should include randomness

**Source:** SRE review
**Risk:** `hostname:pid` not unique across Kubernetes container restarts (PID 1 common).

**Files:** `internal/config/config.go`
**Fix:** Append a random suffix: `hostname:pid:random8chars`.

---

### 12. Pod security context in Helm chart

**Source:** DevOps review

**Files:** `charts/mantle/templates/deployment.yaml`
**Fix:** Add `securityContext` with `runAsNonRoot: true`, `readOnlyRootFilesystem: true`, `allowPrivilegeEscalation: false`.

---

### 13. Docker image build/push in CI

**Source:** DevOps review

**Files:** `.github/workflows/release.yml`
**Fix:** Add Docker build + push to `ghcr.io/dvflw/mantle` on tag push.

---

## Linear Backlog — Remaining Issues

### DVFLW-230: Workflow definition API: list and get versions

**Phase:** 1 (Core Engine)
**Status:** Partially implemented — execution API exists, workflow definition endpoints missing.

**What to build:**
- `GET /api/v1/workflows` — list all workflow definitions (team-scoped)
- `GET /api/v1/workflows/{name}` — get latest version
- `GET /api/v1/workflows/{name}/versions` — list all versions
- `GET /api/v1/workflows/{name}/versions/{version}` — get specific version

**Files:** `internal/server/api.go`, `internal/server/server.go`
**Estimate:** Small — 4 handler functions, straightforward DB queries.

---

### DVFLW-265: SSO/OIDC integration

**Phase:** 10 (Enterprise Hardening)
**Status:** Not implemented — only API key auth exists.

**What to build:**
- OIDC configuration in `mantle.yaml` (issuer URL, client ID, allowed audiences)
- OIDC token validation middleware (alongside existing API key middleware)
- Map OIDC claims (email/groups) to Mantle users/teams
- RBAC identical for SSO and API key auth

**Files:** New `internal/auth/oidc.go`, modify `internal/auth/middleware.go`, `internal/config/config.go`
**Estimate:** Medium — requires JWT/OIDC library, token validation, claims mapping.

---

### DVFLW-269: AWS IAM authentication for Bedrock

**Phase:** 11 (Cloud Secret Stores)
**Status:** Not implemented — no Bedrock connector.

**What to build:**
- AWS IAM-based authentication for the AI connector when targeting Bedrock
- Use AWS SDK's default credential chain (instance role, env vars, config file)
- No explicit API key needed — IAM handles auth
- Bedrock uses a different API format than OpenAI — needs a Bedrock-specific provider in the `LLMProvider` interface

**Files:** New `internal/connector/bedrock.go`, modify `internal/connector/ai.go` for provider routing
**Estimate:** Medium-large — new provider implementation + IAM credential chain.

---

## Recommended Execution Order

**Tomorrow — Swarm Batch 1 (5 parallel agents, all P1):**
1. Team scoping enforcement (largest, most important)
2. Unbounded tool-loop message growth
3. Worker/reaper liveness health checks
4. PDB + migration job in Helm (combine into one agent)
5. DVFLW-230 workflow definition API (quick win)

**Tomorrow — Swarm Batch 2 (P2 quick wins, parallel):**
6. `io.LimitReader` on all connectors
7. Security scanning + golangci-lint pin in CI
8. Startup probe + pod security context in Helm
9. NodeID randomness
10. CEL program caching

**Future (separate sessions):**
11. Docker image build in CI/release
12. DVFLW-265 SSO/OIDC (needs design discussion)
13. DVFLW-269 Bedrock provider (needs design discussion)
