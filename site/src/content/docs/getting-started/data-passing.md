# Data Passing Between Steps

Workflows become powerful when steps pass data to each other. This page covers CEL expressions, conditional execution, and step chaining.

## Chaining Step Outputs

Look at `examples/chained-requests.yaml`:

```yaml
name: chained-requests
description: >
  Fetch a user from a public API, then fetch their posts using the user's ID.
  Demonstrates CEL data passing between steps via steps.<name>.output.

steps:
  - name: get-user
    action: http/request
    params:
      method: GET
      url: "https://jsonplaceholder.typicode.com/users/1"

  - name: get-user-posts
    action: http/request
    params:
      method: GET
      url: "https://jsonplaceholder.typicode.com/posts?userId={{ steps['get-user'].output.json.id }}"
```

The key line is the second step's URL. The expression `{{ steps['get-user'].output.json.id }}` reads the JSON response from the `get-user` step and extracts the `id` field.

Apply and run it:

```bash
mantle apply examples/chained-requests.yaml
mantle run chained-requests
```

```
Running chained-requests (version 1)...
Execution b2c3d4e5-f6a7-8901-bcde-f12345678901: completed
  get-user: completed
  get-user-posts: completed
```

## CEL Expression Syntax

Mantle uses [CEL (Common Expression Language)](https://github.com/google/cel-go) for data passing and conditional logic. The essentials:

- **Access step output:** `steps['step-name'].output.json.field`
- **Access inputs:** `inputs.field_name`
- **Bracket notation is required** when step names contain hyphens: `steps['get-user']` (not `steps.get-user`)
- **Dot notation works** for step names without hyphens: `steps.summarize.output.json.summary`
- **Template strings** use `{{ }}` delimiters inside `params` values

## Conditional Execution

Steps can run conditionally based on the output of previous steps. Look at `examples/conditional-workflow.yaml`:

```yaml
name: conditional-workflow
description: >
  Fetch todos for a user, then conditionally post a summary only if there are
  incomplete todos. Demonstrates conditional execution with if: and retry policies.

inputs:
  user_id:
    type: string
    description: JSONPlaceholder user ID (1-10)

steps:
  - name: get-todos
    action: http/request
    timeout: "10s"
    retry:
      max_attempts: 3
      backoff: exponential
    params:
      method: GET
      url: "https://jsonplaceholder.typicode.com/todos?userId={{ inputs.user_id }}"

  - name: post-summary
    action: http/request
    if: "steps['get-todos'].output.status == 200"
    params:
      method: POST
      url: "https://jsonplaceholder.typicode.com/posts"
      headers:
        Content-Type: "application/json"
      body:
        title: "Todo summary"
        body: "Fetched todos for user {{ inputs.user_id }}"
        userId: "{{ inputs.user_id }}"
```

This workflow introduces three features:

- **`inputs`** -- the workflow declares a `user_id` input, passed at runtime with `--input`
- **`if`** -- the `post-summary` step only runs when the CEL expression evaluates to true
- **`retry` and `timeout`** -- the `get-todos` step retries up to 3 times with exponential backoff and times out after 10 seconds

Apply and run it:

```bash
mantle apply examples/conditional-workflow.yaml
mantle run conditional-workflow --input user_id=3
```

```
Running conditional-workflow (version 1)...
Execution c3d4e5f6-a7b8-9012-cdef-123456789012: completed
  get-todos: completed
  post-summary: completed
```

You can pass multiple inputs by repeating the `--input` flag:

```bash
mantle run my-workflow --input key1=value1 --input key2=value2
```
