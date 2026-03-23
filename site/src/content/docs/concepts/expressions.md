# CEL Expressions

Data flows between steps through [CEL (Common Expression Language)](https://github.com/google/cel-go) expressions. CEL is a small, fast, non-Turing-complete expression language designed by Google for security and policy evaluation.

## Three Namespaces

| Namespace | Example | Description |
|---|---|---|
| `inputs` | `inputs.url` | Values passed when the workflow is triggered. |
| `steps` | `steps.fetch-data.output.body` | Output from a previously completed step. |
| `env` | `env.API_TOKEN` | Environment variables available to the engine. |

## Where CEL Appears

**Conditional execution** -- the `if` field on a step:

```yaml
if: "steps.fetch-data.output.status_code == 200"
```

The step runs only when this expression evaluates to `true`. If the expression evaluates to `false`, the step is skipped.

**Template interpolation** -- double-brace syntax in `params`:

```yaml
params:
  url: "{{ inputs.url }}"
  prompt: "Summarize: {{ steps.fetch-data.output.body }}"
```

Template expressions are evaluated and their results are substituted into the string.

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
      prompt: "Summarize: {{ steps.fetch-data.output.body }}"
```

The data flows like this:

1. The caller provides `url` as an input when triggering the workflow.
2. Step `fetch-data` reads `inputs.url` and makes an HTTP GET request.
3. The HTTP connector returns output with fields like `status_code`, `headers`, and `body`.
4. Step `summarize` reads `steps.fetch-data.output.body` to build its prompt.
5. The AI connector returns the completion result.

Each step can only reference outputs from steps that have completed before it runs. The engine detects these references automatically and treats them as implicit dependencies. When combined with explicit `depends_on` declarations, this enables parallel execution -- see [Execution Model](/docs/concepts/execution).

## CEL Syntax Quick Reference

**Access step output:**

```yaml
url: "{{ steps['step-name'].output.json.field }}"
```

**Access inputs:**

```yaml
url: "{{ inputs.field_name }}"
```

**Bracket notation is required** when step names contain hyphens:

```yaml
# Correct
if: "steps['get-user'].output.status == 200"

# Incorrect in if expressions (hyphen interpreted as subtraction)
if: "steps.get-user.output.status == 200"
```

**Dot notation works** for step names without hyphens:

```yaml
prompt: "{{ steps.summarize.output.json.summary }}"
```

**Template strings** use `{{ }}` delimiters inside `params` values.

## Type Safety

CEL is a strongly typed language. If you compare values of different types, the expression will fail at evaluation time. For example, `inputs.count > "5"` fails because you are comparing a number to a string.

## Expression Examples

Boolean logic:

```yaml
if: "inputs.verbose == true && steps.fetch-data.output.status == 200"
```

String operations:

```yaml
if: "steps.fetch-data.output.body.contains('error') == false"
```

Size checks:

```yaml
if: "steps.summarize.output.key_points.size() > 0"
```

See the [Workflow Reference](/docs/workflow-reference#cel-expressions) for the full expression reference.
