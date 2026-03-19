# Design: Workflow Apply with Versioning — `mantle apply`

> Linear issue: [DVFLW-229](https://linear.app/dvflw/issue/DVFLW-229/workflow-apply-with-versioning-mantle-apply)
> Date: 2026-03-18

## Goal

Add `mantle apply` command that stores validated workflow definitions as immutable versioned records in Postgres, with change detection via content hashing.

## Acceptance Criteria

- `mantle apply workflow.yaml` stores a new versioned definition
- Each apply creates a new version; definitions are immutable once stored
- Applying an unchanged workflow reports "no changes" and does not create a new version
- Single-tenant in Phase 1 (no team scoping — comes in Phase 6)

## Package Structure

```
internal/
  workflow/
    store.go           # Database operations — Save, GetLatestVersion, GetLatestHash
    store_test.go      # Integration tests with testcontainers
  cli/
    apply.go           # mantle apply command
```

## Database Operations — `internal/workflow/store.go`

Direct SQL against Postgres. No ORM, no Store interface.

```go
// Save validates, hashes, and stores a workflow definition.
// Returns the new version number. Returns 0 if content is unchanged.
func Save(ctx context.Context, db *sql.DB, result *ParseResult, rawContent []byte) (int, error)

// GetLatestVersion returns the latest version number for a workflow name.
// Returns 0 if no versions exist.
func GetLatestVersion(ctx context.Context, db *sql.DB, name string) (int, error)

// GetLatestHash returns the content_hash of the latest version for a workflow name.
// Returns empty string if no versions exist.
func GetLatestHash(ctx context.Context, db *sql.DB, name string) (string, error)
```

### Save Flow

1. Validate the workflow via `Validate(result)` — return error if validation fails
2. Compute SHA-256 hash of raw YAML bytes
3. Query latest content_hash for this workflow name
4. If hash matches latest → return 0 (no changes)
5. Query latest version number, increment by 1 (first version is 1)
6. Insert new row into `workflow_definitions`:
   - `name`: workflow name
   - `version`: incremented version number
   - `content`: parsed Workflow struct serialized as JSONB
   - `content_hash`: SHA-256 hex string of raw YAML
7. Return new version number

### Content Storage

The `content` JSONB column stores the parsed workflow struct as JSON (not raw YAML). This makes the content queryable via Postgres JSON operators and avoids re-parsing YAML at read time. The raw YAML is hashed for change detection but not stored separately.

## CLI Command — `internal/cli/apply.go`

`mantle apply <file>` — requires DB connection.

- Config loaded by root's `PersistentPreRunE`
- Opens DB connection in `RunE`, closes with `defer`
- Reads raw file bytes for hashing
- Calls `workflow.Parse()` for parsing
- Calls `workflow.Save()` for validation + storage
- Reports result

Output on new version:
```
Applied fetch-and-summarize version 1
```

Output on no changes:
```
No changes — fetch-and-summarize is already at version 1
```

## Testing

`store_test.go` — testcontainers integration tests (reuses `setupTestDB` pattern from `internal/db/migrate_test.go`):

- Save a workflow → verify returns version 1
- Query `workflow_definitions` table → verify row exists with correct name, version, hash
- Save identical content again → verify returns 0 (no changes)
- Modify content, save again → verify returns version 2
- Verify content_hash is correct SHA-256 of raw bytes

Tests require running migrations first via `db.Migrate()`.

## Dependencies

No new dependencies. Uses `crypto/sha256` and `encoding/hex` from stdlib.

## What's NOT Included

- Team scoping (Phase 6)
- `mantle plan` diff output (DVFLW-228)
- Workflow deletion or listing
- Content validation beyond structural checks (CEL, connector params)
