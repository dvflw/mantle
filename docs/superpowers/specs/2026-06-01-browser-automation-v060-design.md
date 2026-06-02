# Browser Automation v0.6.0 Design

**Date:** 2026-06-01
**Issues:** #32 (custom Playwright images), #33 (declarative browser actions)
**Milestone:** v0.6.0

---

## Overview

Two independent tracks ship under the browser automation umbrella:

- **#33 — Declarative browser actions:** Seven new connectors (`browser/navigate`, `browser/click`, `browser/fill`, `browser/extract`, `browser/screenshot`, `browser/wait`, `browser/evaluate`) built on top of the existing `browser/run` connector. Session state is carried between steps as a serializable blob threaded via CEL expressions.
- **#32 — Custom Playwright images:** Opt-in Docker images (`ghcr.io/dvflw/mantle-playwright-node` and `ghcr.io/dvflw/mantle-playwright-python`) that pre-install the Playwright package, eliminating the bootstrap `npm install` / `pip install` step for `browser/run` power users.

The tracks are independent: declarative actions always use the official Microsoft Playwright images. Custom images remain opt-in for `browser/run`.

---

## Track 1: Declarative Browser Actions (#33)

### Architecture

All new code lives in `packages/engine/internal/connector/browser_declarative.go`. The existing `browser.go` and `BrowserRunConnector` are untouched.

The new file introduces:

1. **`BrowserSession` struct** — the serializable handoff type:
   ```go
   type BrowserSession struct {
       Cookies      []map[string]any             `json:"cookies"`
       LocalStorage map[string]map[string]string `json:"local_storage"`
       URL          string                       `json:"url"`
   }
   ```

2. **`buildDeclarativeScript(actionSnippet string, session *BrowserSession, timeoutMs int) string`** — shared JS generator. Produces a complete, self-contained Playwright script:
   - Launch Chromium (headless)
   - Create browser context
   - If `session` is non-nil: restore cookies and localStorage
   - Set `page.setDefaultTimeout(timeoutMs)`
   - Navigate to `session.URL` if session is being restored
   - Execute the action-specific snippet (injected verbatim)
   - Serialize cookies + localStorage + current URL into `session_state`
   - `console.log(JSON.stringify({ session_state, data }))` then exit 0
   - On any unhandled error: print to stderr, exit 1

3. **Seven connector structs** — each implements `Execute(ctx, params)` by:
   - Parsing and validating its own params
   - Calling `buildDeclarativeScript` with its action snippet
   - Delegating execution to `DockerRunConnector` (same path as `BrowserRunConnector`)
   - Parsing stdout JSON and returning `session_state` + action-specific fields

4. **Registration** — all seven connectors registered in `connector.go` alongside `browser/run`:
   ```
   "browser/navigate"   → BrowserNavigateConnector
   "browser/click"      → BrowserClickConnector
   "browser/fill"       → BrowserFillConnector
   "browser/extract"    → BrowserExtractConnector
   "browser/screenshot" → BrowserScreenshotConnector
   "browser/wait"       → BrowserWaitConnector
   "browser/evaluate"   → BrowserEvaluateConnector
   ```

Language support is JavaScript-only for this iteration. Python support is deferred.

### Session State

```json
{
  "cookies": [
    { "name": "session", "value": "abc123", "domain": "example.com", ... }
  ],
  "local_storage": {
    "https://example.com": { "token": "xyz", "user_id": "42" }
  },
  "url": "https://example.com/dashboard"
}
```

When `session_state` input is absent or empty, the script skips restoration and starts a fresh browser context. This makes every connector usable as a standalone step.

### Connector Reference

All connectors share three common optional params:

| Param | Type | Default | Description |
|---|---|---|---|
| `session_state` | object | — | Handoff blob from a prior browser step |
| `timeout_ms` | int | 30000 | Playwright default timeout (ms) |
| `memory` | string | `"1g"` | Container memory limit |

#### `browser/navigate`

| Param | Type | Required | Description |
|---|---|---|---|
| `url` | string | yes | URL to navigate to |
| `wait_until` | string | no | `"load"` (default) \| `"networkidle"` \| `"domcontentloaded"` |

Outputs: `session_state`, `title` (string — page title after navigation)

#### `browser/click`

| Param | Type | Required | Description |
|---|---|---|---|
| `selector` | string | yes | CSS selector of element to click |
| `wait_for` | string | no | CSS selector to wait for after click |

Outputs: `session_state`

#### `browser/fill`

| Param | Type | Required | Description |
|---|---|---|---|
| `fields` | map[string]string | yes | CSS selector → value pairs |

Outputs: `session_state`

#### `browser/extract`

| Param | Type | Required | Description |
|---|---|---|---|
| `selectors` | map[string]string | yes | Name → CSS selector pairs |
| `attribute` | string | no | Extract attribute value instead of text (e.g. `"href"`) |

Outputs: `session_state`, `data` (map[string]string — name → extracted text/attribute)

#### `browser/screenshot`

| Param | Type | Required | Description |
|---|---|---|---|
| `full_page` | bool | no | Capture full scrollable page (default false) |
| `path` | string | no | Filename in output (default `"screenshot.png"`) |

Outputs: `session_state`, `base64` (string — base64-encoded PNG), `path` (string)

#### `browser/wait`

Exactly one of `selector`, `url_pattern`, or `duration_ms` must be provided.

| Param | Type | Description |
|---|---|---|
| `selector` | string | Wait for CSS selector to be visible |
| `url_pattern` | string | Wait for page URL to match glob pattern |
| `duration_ms` | int | Wait unconditionally for N ms (distinct from the shared `timeout_ms` Playwright timeout) |

Outputs: `session_state`

#### `browser/evaluate`

| Param | Type | Required | Description |
|---|---|---|---|
| `expression` | string | yes | JS expression evaluated in page context |

Outputs: `session_state`, `result` (any — return value of the expression)

### Error Handling

**Script contract:** Every script prints exactly one JSON line to stdout on success and exits 0. On failure it writes to stderr and exits non-zero. The connector maps non-zero exits to Go errors with stderr included in the message.

**Selector/navigation timeouts:** Playwright's `TimeoutError` propagates as a non-zero exit. The step fails like any other connector failure — users handle it with `retry` or `continue_on_error` in the workflow YAML.

**Session corruption:** If the `session_state` input is present but unparseable (malformed JSON), the connector returns an error before spawning a container.

### Example Workflow Fragment

```yaml
steps:
  - name: go_to_login
    action: browser/navigate
    params:
      url: https://example.com/login

  - name: submit_credentials
    action: browser/fill
    params:
      session_state: "{{ steps.go_to_login.outputs.session_state }}"
      fields:
        "#email": "{{ inputs.email }}"
        "#password": "{{ secrets.password }}"

  - name: click_submit
    action: browser/click
    params:
      session_state: "{{ steps.submit_credentials.outputs.session_state }}"
      selector: "button[type=submit]"
      wait_for: ".dashboard"

  - name: extract_profile
    action: browser/extract
    params:
      session_state: "{{ steps.click_submit.outputs.session_state }}"
      selectors:
        username: ".profile-name"
        plan: ".subscription-tier"
```

### Testing

**Unit tests** (`browser_declarative_test.go`):
- `buildDeclarativeScript` outputs correct Playwright calls per action
- Session restore code present when `session_state` provided, absent when not
- Malformed params return errors before container spawn
- `browser/wait` rejects invocations where zero or multiple of its exclusive params are set

**Integration tests** (using `testcontainers` + `httptest.NewServer`):
- `navigate` → `extract` session chain: login page, fill form, extract dashboard value
- `fill` + `click` submits a form correctly
- `screenshot` produces non-empty base64
- `evaluate` returns a computed value from page context
- `wait` times out and surfaces error on missing selector

---

## Track 2: Custom Playwright Images (#32)

### Overview

Two Docker images that pre-install the `playwright` npm/pip package on top of the official Microsoft images, eliminating the `npm install --no-save playwright@X.Y.Z` bootstrap step that `browser/run` currently performs on every invocation.

These images are opt-in: users point `browser/run` at them via the `image` param. Declarative actions (#33) continue using the official images.

### Directory Structure

```
packages/playwright/
  PLAYWRIGHT_VERSION        # single line: "1.52.0"
  Dockerfile.node           # FROM mcr.microsoft.com/playwright:vX.Y.Z-noble + npm install
  Dockerfile.python         # FROM mcr.microsoft.com/playwright/python:vX.Y.Z-noble + pip install
```

### Dockerfiles

**Dockerfile.node:**
```dockerfile
ARG PLAYWRIGHT_VERSION
FROM mcr.microsoft.com/playwright:v${PLAYWRIGHT_VERSION}-noble
RUN npm install --global --no-save playwright@${PLAYWRIGHT_VERSION}
```

**Dockerfile.python:**
```dockerfile
ARG PLAYWRIGHT_VERSION
FROM mcr.microsoft.com/playwright/python:v${PLAYWRIGHT_VERSION}-noble
RUN pip install --quiet playwright==${PLAYWRIGHT_VERSION}
```

### Image Tags

- `ghcr.io/dvflw/mantle-playwright-node:<version>` (e.g. `1.52.0`)
- `ghcr.io/dvflw/mantle-playwright-python:<version>`
- `ghcr.io/dvflw/mantle-playwright-node:latest` (only on stable releases, not pre-release)

### CI Workflow

New workflow `build-playwright-images.yml`:
- **Trigger:** push to `main` when `packages/playwright/PLAYWRIGHT_VERSION` changes, or `workflow_dispatch` for manual rebuilds
- **Steps:** checkout → QEMU → buildx → GHCR login → build + push both images (multi-arch: amd64, arm64)
- **Smoke test:** `docker run --rm ghcr.io/dvflw/mantle-playwright-node:<version> node -e "require('playwright'); console.log('ok')"`

### Testing

A `docker build` smoke test in CI verifies both images build successfully. No additional runtime tests — declarative action integration tests cover runtime behavior via official images.

---

## Out of Scope for v0.6.0

- Python language support for declarative actions
- Browser/tab management (multiple pages per session)
- File upload/download via browser actions
- Proxy configuration
- Custom browser launch args
- Authenticated GHCR pulls for private registries in `browser/run`
