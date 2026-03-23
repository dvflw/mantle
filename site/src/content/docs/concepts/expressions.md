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

### Workflow inputs

Inputs are declared at the top of the workflow file and passed at runtime:

```yaml
inputs:
  name:
    type: string
  count:
    type: integer

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

```yaml
# Convert number to string for concatenation
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

## Limitations

- **`env.*` is restricted** — only environment variables with the `MANTLE_ENV_` prefix are available. This prevents accidental exposure of system secrets through CEL.
- **Secrets are NOT available in CEL** — credentials are resolved as opaque handles at connector invocation time and are never exposed as raw values in expressions. See [Secrets Management](/docs/concepts/secrets).
- **Resource limits** — CEL evaluation is time-bounded and output-size-limited to prevent runaway expressions from affecting engine performance.
- **Not Turing-complete** — CEL intentionally lacks loops and general recursion. It is an expression language, not a programming language.
