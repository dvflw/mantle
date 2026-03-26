# CEL Expressions

Mantle uses [CEL (Common Expression Language)](https://cel.dev) for data flow and conditional logic in workflows. CEL is a small, fast, non-Turing-complete expression language designed by Google for security and policy evaluation. It is strongly typed, sandboxed, and evaluates in nanoseconds — making it ideal for workflow orchestration. See the [CEL language spec](https://cel.dev) for the full reference.

## How Mantle Uses CEL

CEL expressions appear in two contexts inside a workflow YAML file:

- **Template interpolation** in `params` values — wrapped in `{{ }}` delimiters, can be mixed with literal text.
- **Bare expressions** in the `if` field — no `{{ }}` wrapper, evaluated as a boolean to decide whether a step runs.

```yaml
steps:
  - name: notify
    action: http/request
    # Bare CEL — evaluated as boolean, no {{ }} needed
    if: "steps.check.output.status == 200"
    params:
      method: POST
      # Template CEL — embedded in a string with {{ }}
      url: "https://api.example.com/{{ steps.lookup.output.id }}/notify"
      body:
        message: "Hello {{ inputs.name }}, your request is ready."
```

## Available Variables

Every CEL expression has access to four namespaces:

| Namespace | Example | Description |
|---|---|---|
| `steps.<name>.output` | `steps.fetch.output.json.title` | Output from a previously completed step. |
| `steps.<name>.error` | `steps.fetch.error` | Error message from a failed step (null if successful or skipped). |
| `inputs.<name>` | `inputs.url` | Values passed when the workflow is triggered. |
| `env.<name>` | `env.API_BASE_URL` | Environment variables (restricted to `MANTLE_ENV_*` prefix). |
| `trigger.payload` | `trigger.payload.repository.full_name` | Webhook trigger data (server mode only). |

### Step outputs

Each connector populates output fields. For the HTTP connector, common fields are `status`, `headers`, `body`, and `json` (the parsed JSON body). Access them with dot or bracket notation:

```yaml
# Dot notation
summary: "{{ steps.fetch.output.json.title }}"

# Bracket notation — required when step names contain hyphens
url: "{{ steps['get-user'].output.json.profile_url }}"
```

### Step errors

Every step exposes an `error` field:

- **`null`** — The step succeeded or was skipped (its `if` condition was false).
- **String message** — The step failed. The error message is populated from the connector.

The `error` field is always present in the CEL context for any step. However, it is only practically useful when the referenced step has `continue_on_error: true`. Without that flag, a step failure halts the entire workflow before any downstream step can run — so there is no opportunity to check the error. Use `continue_on_error: true` on any step whose failure you want to handle in subsequent steps:

```yaml
steps:
  - name: try-primary-api
    action: http/request
    continue_on_error: true
    params:
      method: GET
      url: https://primary-api.example.com/data

  - name: handle-error
    action: slack/send
    credential: slack-token
    if: "steps['try-primary-api'].error != null"
    params:
      channel: "#ops-alerts"
      text: "Primary API failed: {{ steps['try-primary-api'].error }}"
```

You can also use error checking in template expressions:

```yaml
steps:
  - name: process
    action: http/request
    params:
      method: POST
      url: https://api.example.com/process
      body:
        # Include error details if the previous step failed
        error_info: "{{ steps.backup.error }}"
```

### Workflow inputs

Inputs are declared at the top of the workflow file and passed at runtime:

```yaml
inputs:
  name:
    type: string
  count:
    type: number

steps:
  - name: greet
    action: http/request
    params:
      url: "https://api.example.com/greet"
      body:
        greeting: "Hello {{ inputs.name }}"
```

### Environment variables

Environment variables are available under `env.*`, but only those with the `MANTLE_ENV_` prefix are exposed. The prefix is stripped in the expression:

```yaml
# If MANTLE_ENV_API_BASE_URL is set:
url: "{{ env.API_BASE_URL }}/v1/resource"
```

### Trigger data (webhooks)

In server mode, workflows triggered by webhooks can access the incoming payload:

```yaml
repo: "{{ trigger.payload.repository.full_name }}"
action: "{{ trigger.payload.action }}"
```

## Common Expressions

### String operations

```yaml
prompt: "Hello {{ inputs.name }}"
url: "https://api.example.com/{{ steps.lookup.output.id }}"
message: "Status: {{ steps.check.output.json.status }}"
```

### Accessing nested data

```yaml
# JSON response fields
summary: "{{ steps.fetch.output.json.title }}"

# Nested objects
city: "{{ steps.fetch.output.json.address.city }}"

# Array access
first_item: "{{ steps.list.output.json.items[0] }}"
```

### Conditional execution (if field)

The `if` field uses bare CEL expressions — no `{{ }}` wrapper:

```yaml
# Status code check
if: "steps.check.output.status == 200"

# Numeric comparison
if: "steps.analyze.output.json.score > 0.8"

# Check list length
if: "size(steps.fetch.output.json.items) > 0"

# Check field existence
if: "has(steps.prev.output.json.email)"

# Boolean logic
if: "inputs.verbose == true && steps.fetch.output.status == 200"

# Negation
if: "steps.fetch.output.body.contains('error') == false"
```

### String functions

```yaml
if: "steps.fetch.output.json.status.startsWith('2')"
if: "steps.data.output.json.email.contains('@company.com')"
if: "steps.input.output.json.name.endsWith('.pdf')"
```

### Size checks

```yaml
# String length
if: "size(steps.response.output.body) < 10000"

# List length
if: "size(steps.search.output.json.results) > 0"
```

### Type conversions

CEL provides a built-in `string()` constructor that works on primitive types (int, double, bool). Mantle also provides a `toString()` function that handles any value type including maps and lists. Either works for simple conversions; use `toString()` when dealing with dynamic or complex types.

```yaml
# Convert number to string for concatenation (built-in string() or custom toString() both work)
timeout: "{{ string(inputs.timeout_seconds) + 's' }}"

# Boolean checks
if: "steps.validate.output.json.valid == true"
```

## Template vs Bare Expressions

This distinction is important and a common source of confusion:

| Context | Syntax | Example |
|---|---|---|
| `params` values | `{{ expression }}` | `"Hello {{ inputs.name }}"` |
| `if` field | bare expression | `"steps.check.output.status == 200"` |

Template expressions (`{{ }}`) can be mixed with literal text and are substituted into the string. You can have multiple templates in a single string:

```yaml
url: "https://{{ env.API_HOST }}/users/{{ steps.lookup.output.id }}/profile"
```

Bare expressions in `if` must evaluate to a boolean. Do not wrap them in `{{ }}`:

```yaml
# Correct
if: "steps.check.output.status == 200"

# Wrong — do not use {{ }} in if
if: "{{ steps.check.output.status == 200 }}"
```

## Bracket vs Dot Notation

**Bracket notation is required** when step names contain hyphens, because CEL interprets `-` as subtraction:

```yaml
# Correct — bracket notation for hyphenated names
if: "steps['get-user'].output.status == 200"

# Wrong — CEL reads this as steps.get minus user.output...
if: "steps.get-user.output.status == 200"
```

**Dot notation works** for step names without hyphens:

```yaml
prompt: "{{ steps.summarize.output.json.summary }}"
```

## Type Safety

CEL is a strongly typed language. Comparing values of different types produces an evaluation error at runtime rather than silent coercion.

**Common type errors and fixes:**

| Expression | Problem | Fix |
|---|---|---|
| `inputs.count > "5"` | Comparing int to string | `inputs.count > 5` |
| `steps.a.output.status + " OK"` | Adding int to string | `string(steps.a.output.status) + " OK"` |
| `steps.a.output.json.missing.field` | Field may not exist | `has(steps.a.output.json.missing) ? steps.a.output.json.missing.field : "default"` |

**Use `has()` to guard optional fields:**

```yaml
if: "has(steps.fetch.output.json.email) && steps.fetch.output.json.email.contains('@')"
```

The `has()` macro checks whether a field exists without triggering a type error. Use it when a previous step might not include a field in its output.

## Data Flow Example

Consider this workflow:

```yaml
inputs:
  url:
    type: string

steps:
  - name: fetch-data
    action: http/request
    params:
      method: GET
      url: "{{ inputs.url }}"

  - name: summarize
    action: ai/completion
    params:
      provider: openai
      model: gpt-4o
      prompt: "Summarize: {{ steps['fetch-data'].output.body }}"
```

The data flows like this:

1. The caller provides `url` as an input when triggering the workflow.
2. Step `fetch-data` reads `inputs.url` and makes an HTTP GET request.
3. The HTTP connector returns output with fields like `status`, `headers`, `body`, and `json`.
4. Step `summarize` reads `steps['fetch-data'].output.body` to build its prompt.
5. The AI connector returns the completion result.

Each step can only reference outputs from steps that have completed before it runs. The engine detects these references automatically and treats them as implicit dependencies. When combined with explicit `depends_on` declarations, this enables parallel execution — see [Execution Model](/docs/concepts/execution).

## List Macros

CEL provides built-in macros for working with lists. These operate on any list value — step output arrays, input arrays, or lists constructed inline.

### `.map(item, expr)`

Transforms each element in a list by evaluating `expr` for every `item`.

```yaml
steps:
  - name: extract-titles
    action: http/request
    params:
      method: POST
      url: "https://api.example.com/batch"
      body:
        # Produce a list of title strings from a list of article objects
        titles: "{{ steps.fetch.output.json.articles.map(a, a.title) }}"
```

### `.filter(item, expr)`

Returns a new list containing only the elements for which `expr` is true.

```yaml
steps:
  - name: notify-failures
    action: http/request
    if: "size(steps.results.output.json.jobs.filter(j, j.status == 'failed')) > 0"
    params:
      method: POST
      url: "https://hooks.example.com/alert"
      body:
        failed_jobs: "{{ steps.results.output.json.jobs.filter(j, j.status == 'failed') }}"
```

### `.exists(item, expr)`

Returns `true` if at least one element satisfies `expr`.

```yaml
steps:
  - name: escalate
    action: http/request
    # Run this step only if any result has a critical severity
    if: "steps.scan.output.json.findings.exists(f, f.severity == 'critical')"
    params:
      method: POST
      url: "https://api.example.com/escalate"
```

### `.all(item, expr)`

Returns `true` if every element satisfies `expr`.

```yaml
steps:
  - name: mark-complete
    action: http/request
    # Only mark complete when every task is done
    if: "steps.fetch.output.json.tasks.all(t, t.done == true)"
    params:
      method: PATCH
      url: "https://api.example.com/projects/{{ inputs.project_id }}"
      body:
        status: "complete"
```

### `.exists_one(item, expr)`

Returns `true` if exactly one element satisfies `expr`.

```yaml
steps:
  - name: assign-owner
    action: http/request
    # Assign only when there is exactly one eligible owner
    if: "steps.fetch.output.json.members.exists_one(m, m.role == 'lead')"
    params:
      method: POST
      url: "https://api.example.com/assignments"
```

### Chaining `.filter()` and `.map()`

Filter and map can be chained to first narrow a list and then reshape it.

```yaml
steps:
  - name: summarize-errors
    action: ai/completion
    params:
      provider: openai
      model: gpt-4o
      prompt: >
        Summarize these error messages:
        {{ steps.logs.output.json.entries
             .filter(e, e.level == 'error')
             .map(e, e.message) }}
```

## String Functions

Mantle registers the following string functions on top of CEL's built-in string methods.

### `toLower()`

Converts a string to lowercase.

```yaml
steps:
  - name: normalize-tag
    action: http/request
    params:
      method: POST
      url: "https://api.example.com/tags"
      body:
        tag: "{{ steps.input.output.json.label.toLower() }}"
```

### `toUpper()`

Converts a string to uppercase.

```yaml
steps:
  - name: set-env-key
    action: http/request
    params:
      method: POST
      url: "https://api.example.com/config"
      body:
        key: "{{ inputs.variable_name.toUpper() }}"
```

### `trim()`

Removes leading and trailing whitespace.

```yaml
steps:
  - name: clean-input
    action: http/request
    params:
      method: POST
      url: "https://api.example.com/search"
      body:
        query: "{{ inputs.search_term.trim() }}"
```

### `replace(old, new)`

Replaces all occurrences of `old` with `new`.

```yaml
steps:
  - name: slugify
    action: http/request
    params:
      method: POST
      url: "https://api.example.com/pages"
      body:
        slug: "{{ inputs.title.toLower().replace(' ', '-') }}"
```

### `split(delimiter)`

Splits a string into a list of strings at each occurrence of `delimiter`.

```yaml
steps:
  - name: process-tags
    action: http/request
    params:
      method: POST
      url: "https://api.example.com/items"
      body:
        # Convert "a,b,c" to ["a", "b", "c"]
        tags: "{{ inputs.tag_string.split(',') }}"
```

## Type Coercion

These functions parse and convert values between types. They produce an evaluation error on invalid input — use `default()` to handle failure gracefully.

### `parseInt(string)`

Parses a decimal string to an integer. Errors if the string is not a valid integer.

```yaml
steps:
  - name: paginate
    action: http/request
    params:
      method: GET
      url: "https://api.example.com/results"
      body:
        page: "{{ parseInt(inputs.page_string) }}"
```

### `parseFloat(string)`

Parses a string to a floating-point number. Errors if the string is not a valid float.

```yaml
steps:
  - name: apply-threshold
    action: http/request
    if: "parseFloat(steps.score.output.body) > 0.75"
    params:
      method: POST
      url: "https://api.example.com/approve"
```

### `toString(value)`

Converts any value to its string representation.

```yaml
steps:
  - name: build-message
    action: http/request
    params:
      method: POST
      url: "https://hooks.example.com/notify"
      body:
        text: "Processed {{ toString(steps.count.output.json.total) }} records."
```

## Object Construction

### `obj(key, value, ...)`

Builds a map from alternating key-value arguments. Supports up to 5 key-value pairs (10 arguments) due to cel-go's fixed-arity overload requirement — CEL does not support true variadic functions without macros. For maps with more than 5 pairs, use nested `obj()` calls or construct the value with `jsonDecode`.

```yaml
steps:
  - name: create-record
    action: http/request
    params:
      method: POST
      url: "https://api.example.com/records"
      body:
        record: "{{ obj('name', inputs.name, 'status', 'pending', 'source', 'mantle') }}"
```

`obj()` is particularly useful combined with `.map()` to reshape a list of objects into a different structure:

```yaml
steps:
  - name: reformat-users
    action: http/request
    params:
      method: POST
      url: "https://api.example.com/import"
      body:
        # Reshape each user to only include id and display_name
        users: >
          {{ steps.fetch.output.json.users.map(u,
               obj('id', u.id, 'display_name', u.first_name + ' ' + u.last_name)) }}
```

## Utility Functions

### `default(value, fallback)`

Returns `value` if it is non-null and does not produce an error; returns `fallback` otherwise. Use this to handle optional fields without a `has()` guard.

```yaml
steps:
  - name: notify
    action: http/request
    params:
      method: POST
      url: "https://hooks.example.com/notify"
      body:
        # Use a default region when the field is absent from the response
        region: "{{ default(steps.fetch.output.json.region, 'us-east-1') }}"
```

### `flatten(list)`

Flattens one level of nesting from a list of lists.

```yaml
steps:
  - name: collect-all-items
    action: http/request
    params:
      method: POST
      url: "https://api.example.com/process"
      body:
        # Each page returns a list; flatten to get a single list of items
        items: "{{ flatten(steps.paginate.output.json.pages.map(p, p.items)) }}"
```

## JSON Functions

### `jsonEncode(value)`

Serializes any value to a JSON string. Useful when a downstream API expects a JSON-encoded string field rather than a structured object.

```yaml
steps:
  - name: store-metadata
    action: http/request
    params:
      method: PUT
      url: "https://api.example.com/records/{{ inputs.id }}"
      body:
        # The target API expects metadata as a JSON string, not an object
        metadata_json: "{{ jsonEncode(steps.fetch.output.json.metadata) }}"
```

### `jsonDecode(string)`

Parses a JSON string to a structured value. Use this when a step returns a JSON-encoded string inside a field rather than a parsed object.

```yaml
steps:
  - name: parse-config
    action: http/request
    params:
      method: POST
      url: "https://api.example.com/apply"
      body:
        # steps.load.output.json.config_str is a JSON string — decode it first
        settings: "{{ jsonDecode(steps.load.output.json.config_str).settings }}"
```

## Date/Time Functions

### `parseTimestamp(string)`

Parses an ISO 8601 / RFC 3339 string to a CEL timestamp value. Named `parseTimestamp` rather than `timestamp` to avoid collision with CEL's built-in `timestamp()` constructor.

```yaml
steps:
  - name: check-expiry
    action: http/request
    if: "parseTimestamp(steps.fetch.output.json.expires_at) < parseTimestamp(\"2026-12-31T00:00:00Z\")"
    params:
      method: POST
      url: "https://api.example.com/renew"
      body:
        resource_id: "{{ inputs.resource_id }}"
```

### `formatTimestamp(timestamp, layout)`

Formats a timestamp value to a string using a [Go time layout](https://pkg.go.dev/time#Layout). The reference time for Go layouts is `Mon Jan 2 15:04:05 MST 2006`.

```yaml
steps:
  - name: create-report
    action: http/request
    params:
      method: POST
      url: "https://api.example.com/reports"
      body:
        # Format as "2006-01-02" (Go layout for YYYY-MM-DD)
        report_date: "{{ formatTimestamp(parseTimestamp(steps.fetch.output.json.created_at), '2006-01-02') }}"
        # Format with time for a human-readable label
        label: "Report for {{ formatTimestamp(parseTimestamp(steps.fetch.output.json.created_at), 'Jan 2, 2006') }}"
```

## Limitations

- **`env.*` is restricted** — only environment variables with the `MANTLE_ENV_` prefix are available. This prevents accidental exposure of system secrets through CEL.
- **Secrets are NOT available in CEL** — credentials are resolved as opaque handles at connector invocation time and are never exposed as raw values in expressions. See [Secrets Management](/docs/concepts/secrets).
- **Resource limits** — CEL evaluation is time-bounded and output-size-limited to prevent runaway expressions from affecting engine performance.
- **Not Turing-complete** — CEL intentionally lacks loops and general recursion. It is an expression language, not a programming language.
