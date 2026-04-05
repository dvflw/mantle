# OpenAPI Specification Design

**Issue:** [dvflw/mantle#55](https://github.com/dvflw/mantle/issues/55)
**Milestone:** v0.5.0 — The GitOps Update

## Goal

Generate an OpenAPI 3.0 spec from Go code using swaggo/swag annotations. The spec becomes the stable contract for SDK generation in v1.0.0 and gives operators a machine-readable description of the API.

## Approach

swaggo/swag: annotation comments on existing handlers, `swag init` at build time. No router or handler signature changes. Generates `swagger.json` + `swagger.yaml` into `packages/engine/docs/`, checked into git. Spec served at `GET /api/v1/openapi.json` via `embed.FS`.

OpenAPI 3.0 (not 3.1) — `oapi-codegen` and all major SDK gen tools handle 3.0 without issue.

## API Surface Covered

### REST API — authenticated (Bearer)

| Method | Path | Description |
|--------|------|-------------|
| POST | `/api/v1/run/{workflow}` | Trigger a workflow execution |
| POST | `/api/v1/cancel/{execution}` | Cancel a running execution |
| GET | `/api/v1/executions` | List executions (workflow, status, since, limit) |
| GET | `/api/v1/executions/{id}` | Get execution detail with steps |
| GET | `/api/v1/workflows` | List workflow definitions |
| GET | `/api/v1/workflows/{name}` | Get latest workflow definition |
| GET | `/api/v1/workflows/{name}/versions` | List all versions of a workflow |
| GET | `/api/v1/workflows/{name}/versions/{version}` | Get a specific workflow version |
| GET | `/api/v1/budgets` | List AI provider budgets |
| PUT | `/api/v1/budgets/{provider}` | Set budget for a provider |
| DELETE | `/api/v1/budgets/{provider}` | Delete budget for a provider |
| GET | `/api/v1/budgets/usage` | Get token usage for the current period |

### System endpoints — unauthenticated

| Method | Path | Description |
|--------|------|-------------|
| GET | `/healthz` | Liveness probe |
| GET | `/readyz` | Readiness probe (DB + worker/reaper) |

### Excluded from spec

- `GET /metrics` — Prometheus scrape endpoint, not a consumer API
- `POST /hooks/{path}` — Dynamic webhook paths, not a typed contract

## File Changes

| Action | File | Purpose |
|--------|------|---------|
| Create | `packages/engine/internal/server/docs.go` | Global swag annotations (title, version, auth schemes) |
| Modify | `packages/engine/internal/server/api.go` | Export response types; add per-handler annotations |
| Modify | `packages/engine/internal/server/server.go` | Add per-handler annotations to handleRun, handleCancel; add `GET /api/v1/openapi.json` route |
| Create | `packages/engine/internal/server/docs/` | swag-generated output (swagger.json, swagger.yaml, docs.go) |
| Modify | `packages/engine/go.mod` | Add swaggo/swag, swaggo/files dependencies |
| Modify | `packages/engine/Makefile` | Add `spec` target |
| Modify | `.github/workflows/engine-ci.yml` | Add spec freshness check step |

## Response Type Exports

The following unexported types in `api.go` are public API contracts and must be exported for swag to generate accurate schemas:

| Before | After |
|--------|-------|
| `executionSummary` | `ExecutionSummary` |
| `executionDetail` | `ExecutionDetail` |
| `stepSummary` | `StepSummary` |

`TeamBudget` in `internal/budget/store.go` is already exported.

## Auth Schemes

Two security schemes documented in the spec:

- **ApiKeyAuth** — `http` bearer scheme. API key issued by `mantle secrets create` or the auth CLI. Format: `Authorization: Bearer mk_<key>`.
- **OIDCAuth** — `http` bearer scheme. OIDC JWT from a configured identity provider. Format: `Authorization: Bearer <jwt>`.

All `/api/v1/` endpoints require one of the two. Health endpoints are unauthenticated.

## Global Annotations (docs.go)

```go
// @title          Mantle API
// @version        1.0
// @description    Headless AI workflow automation — BYOK, IaC-first, enterprise-grade.
// @contact.name   Mantle
// @contact.url    https://github.com/dvflw/mantle
// @license.name   BSL/SSPL
// @license.url    https://github.com/dvflw/mantle/blob/main/LICENSE
//
// @host            localhost:8080
// @basePath        /
//
// @securityDefinitions.apikey ApiKeyAuth
// @in header
// @name Authorization
// @description Bearer API key. Format: "Bearer mk_..."
//
// @securityDefinitions.apikey OIDCAuth
// @in header
// @name Authorization
// @description Bearer OIDC JWT. Format: "Bearer <jwt>"
```

## Spec Serving

A new `GET /api/v1/openapi.json` endpoint serves the embedded spec:

```go
//go:embed docs/swagger.json
var swaggerJSON []byte

mux.HandleFunc("GET /api/v1/openapi.json", func(w http.ResponseWriter, r *http.Request) {
    w.Header().Set("Content-Type", "application/json")
    w.Write(swaggerJSON)
})
```

The embed lives in `packages/engine/internal/server/` alongside the generated `docs/` directory.

## Makefile Target

```makefile
.PHONY: spec
spec: ## Regenerate OpenAPI spec
	swag init -g internal/server/docs.go \
	  -d internal/server \
	  --output internal/server/docs \
	  --outputTypes json,yaml \
	  --parseDependency \
	  --parseInternal
```

Run from `packages/engine/`.

## CI Freshness Check

Added to the engine CI workflow after `go build`:

```yaml
- name: Check OpenAPI spec is up to date
  run: |
    cd packages/engine
    swag init -g internal/server/docs.go \
      -d internal/server \
      --output internal/server/docs \
      --outputTypes json,yaml \
      --parseDependency \
      --parseInternal
    git diff --exit-code internal/server/docs/
```

Fails the build if a handler was changed without regenerating the spec.

## Example Annotations

### POST /api/v1/run/{workflow}

```go
// handleRun triggers a workflow execution via the API.
//
// @Summary      Trigger a workflow execution
// @Tags         executions
// @Param        workflow  path  string  true  "Workflow name"
// @Success      202  {object}  RunResponse
// @Failure      400  {object}  ErrorResponse
// @Failure      404  {object}  ErrorResponse
// @Failure      500  {object}  ErrorResponse
// @Security     ApiKeyAuth
// @Security     OIDCAuth
// @Router       /api/v1/run/{workflow} [post]
```

### GET /api/v1/executions

```go
// handleListExecutions handles GET /api/v1/executions with query param filters.
//
// @Summary      List executions
// @Tags         executions
// @Param        workflow  query  string  false  "Filter by workflow name"
// @Param        status    query  string  false  "Filter by status"  Enums(pending,running,completed,failed,cancelled)
// @Param        since     query  string  false  "Filter by age (e.g. 1h, 7d)"
// @Param        limit     query  integer false  "Max results (default 20)"
// @Success      200  {object}  ExecutionListResponse
// @Failure      400  {object}  ErrorResponse
// @Failure      500  {object}  ErrorResponse
// @Security     ApiKeyAuth
// @Security     OIDCAuth
// @Router       /api/v1/executions [get]
```

## Additional Response Types

Inline response wrapper types to define in `api.go`:

```go
// RunResponse is returned when a workflow execution is accepted.
type RunResponse struct {
    ExecutionID string `json:"execution_id"`
    Workflow    string `json:"workflow"`
    Version     int    `json:"version"`
}

// CancelResponse is returned when an execution is cancelled.
type CancelResponse struct {
    ExecutionID string `json:"execution_id"`
    Status      string `json:"status"`
}

// ExecutionListResponse wraps a list of executions.
type ExecutionListResponse struct {
    Executions []ExecutionSummary `json:"executions"`
}

// WorkflowListResponse wraps a list of workflow summaries.
type WorkflowListResponse struct {
    Workflows []workflow.WorkflowSummary `json:"workflows"`
}

// WorkflowDetailResponse is returned for GET /api/v1/workflows/{name}.
type WorkflowDetailResponse struct {
    Name       string          `json:"name"`
    Version    int             `json:"version"`
    Definition json.RawMessage `json:"definition"`
}

// WorkflowVersionListResponse wraps a list of workflow versions.
type WorkflowVersionListResponse struct {
    Name     string                   `json:"name"`
    Versions []workflow.VersionSummary `json:"versions"`
}

// ErrorResponse is the standard error envelope.
type ErrorResponse struct {
    Error string `json:"error"`
}

// UsageResponse is returned for GET /api/v1/budgets/usage.
type UsageResponse struct {
    PeriodStart      string `json:"period_start"`
    Provider         string `json:"provider"`
    PromptTokens     int64  `json:"prompt_tokens"`
    CompletionTokens int64  `json:"completion_tokens"`
    TotalTokens      int64  `json:"total_tokens"`
}
```

## Testing

- `go build ./...` must pass after type renames
- `swag init` must complete without errors
- `GET /api/v1/openapi.json` returns valid JSON containing `"openapi":"3.0.0"` (swag v2 generates OpenAPI 3.0)
- `git diff --exit-code` on generated files in CI
