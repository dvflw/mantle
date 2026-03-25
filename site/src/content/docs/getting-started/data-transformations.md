# Data Transformations

Workflows rarely move data from one place to another without changing its shape. A user record from one API uses camelCase and ISO dates; your database expects snake_case and Unix timestamps. A support ticket arrives as unstructured text; your alerting system needs a priority label and extracted entity names.

Mantle handles both categories: mechanical transforms using CEL expressions, and semantic transforms using the AI connector. This guide covers each pattern and when to reach for each one.

## Pattern 1: Structural Transforms with CEL

Use CEL when the mapping between source and target is known and deterministic -- field renaming, case normalization, type coercion, and filtering. CEL runs in the engine with no external calls, so structural transforms add no latency and no cost.

**Example:** An upstream API returns user records with camelCase fields and a date of birth string. Your database expects snake_case and rejects the original field names.

Source:

```json
{"firstName": "Alice", "lastName": "Smith", "dob": "1995-03-24"}
```

Target:

```json
{"name": "alice smith", "birth_date": "1995-03-24"}
```

Here is a complete workflow that fetches a list of user records, reshapes each one, and writes the results to Postgres:

```yaml
name: normalize-users
description: >
  Fetch user records from the upstream API, normalize field names and
  casing, then insert into the local database.

steps:
  - name: fetch-users
    action: http/request
    params:
      method: GET
      url: "https://api.example.com/users"

  - name: insert-users
    action: http/request
    params:
      method: POST
      url: "https://internal.example.com/db/users/batch"
      headers:
        Content-Type: "application/json"
      body:
        records: >
          {{ steps['fetch-users'].output.json.users.map(u,
               obj(
                 'name',       (u.firstName + ' ' + u.lastName).toLower(),
                 'birth_date', u.dob
               )
             ) }}
```

What the CEL expression does:

- `.map(u, ...)` -- iterates the `users` list, binding each element to `u`
- `obj('name', ..., 'birth_date', ...)` -- constructs a new object with the renamed keys
- `.toLower()` -- normalizes the full name to lowercase (method call on the concatenated string)
- String concatenation (`+`) joins first and last name with a space

The output of the `map()` call is a new list of objects ready for the batch insert. Nothing left the workflow engine.

## Pattern 2: AI-Powered Transforms

Use the AI connector when the transform requires interpretation, classification, or understanding that a deterministic expression cannot provide. Common cases: classifying priority from free-form text, extracting named entities, generating summaries, or translating between domain vocabularies.

**Example:** Raw support tickets arrive as unstructured text. Your team needs each ticket classified by priority, categorized by product area, and tagged with any affected usernames or order IDs mentioned in the body.

Here is a workflow that fetches open tickets, uses the AI connector to extract structured data, and conditionally routes high-priority items to a separate store:

```yaml
name: classify-tickets
description: >
  Fetch open support tickets, classify each one using structured AI output,
  and store high-priority tickets in the escalation queue.

steps:
  - name: fetch-tickets
    action: http/request
    params:
      method: GET
      url: "https://support.example.com/api/tickets?status=open"

  - name: classify
    action: ai/completion
    credential: openai
    params:
      model: gpt-4o-mini
      prompt: >
        Classify the following support ticket. Extract the priority, product
        area, and any entity identifiers (usernames, order IDs) mentioned.

        Ticket:
        {{ steps['fetch-tickets'].output.json.tickets[0].body }}
      output_schema:
        type: object
        properties:
          priority:
            type: string
            enum: [low, medium, high, critical]
          product_area:
            type: string
            enum: [billing, authentication, api, ui, other]
          entities:
            type: array
            items:
              type: object
              properties:
                type:
                  type: string
                value:
                  type: string
        required: [priority, product_area, entities]

  - name: store-escalation
    action: http/request
    if: >
      steps.classify.output.json.priority == 'high' ||
      steps.classify.output.json.priority == 'critical'
    params:
      method: POST
      url: "https://internal.example.com/escalation-queue"
      headers:
        Content-Type: "application/json"
      body:
        ticket_id: "{{ steps['fetch-tickets'].output.json.tickets[0].id }}"
        priority: "{{ steps.classify.output.json.priority }}"
        product_area: "{{ steps.classify.output.json.product_area }}"
        entities: "{{ steps.classify.output.json.entities }}"
```

Key points:

- `output_schema` tells the AI connector to return structured JSON matching the schema, not free-form text. The engine validates the response against the schema before making it available as `output.json`.
- The `if` field on `store-escalation` is a bare CEL expression that reads from the AI step's structured output. CEL works on the schema-validated object directly.
- For bulk processing, wrap this pattern in a `forEach` or a parent workflow that fans out over the ticket list.

See the [AI Workflows guide](/docs/getting-started/ai-workflows) for credential setup and the full `output_schema` reference.

## Pattern 3: Hybrid Transforms

Combine CEL for structural normalization with the AI connector for semantic enrichment. Use CEL first to extract and reshape the fields you need, then pass the cleaned data to the AI step. This keeps prompts concise and keeps AI costs proportional to the semantic work required.

**Example:** A product reviews feed includes raw ratings, dates, and freeform review text mixed with metadata you do not need. You want to store normalized records enriched with sentiment labels and key themes.

```yaml
name: enrich-reviews
description: >
  Fetch product reviews, normalize structure with CEL, enrich each review
  with AI-classified sentiment and themes, then store the enriched records.

steps:
  - name: fetch-reviews
    action: http/request
    params:
      method: GET
      url: "https://api.example.com/products/{{ inputs.product_id }}/reviews"

  - name: normalize
    action: http/request
    params:
      method: POST
      url: "https://internal.example.com/transform/passthrough"
      headers:
        Content-Type: "application/json"
      body:
        reviews: >
          {{ steps['fetch-reviews'].output.json.data.map(r,
               obj(
                 'id',          r.reviewId,
                 'rating',      r.starRating,
                 'reviewed_at', r.submittedAt,
                 'text',        r.body.trim()
               )
             ) }}

  - name: enrich
    action: ai/completion
    credential: openai
    params:
      model: gpt-4o-mini
      prompt: >
        Analyze the following product reviews and classify the sentiment
        and key themes for each one.

        Reviews:
        {{ steps.normalize.output.json.reviews }}
      output_schema:
        type: array
        items:
          type: object
          properties:
            id:
              type: string
            sentiment:
              type: string
              enum: [positive, neutral, negative]
            themes:
              type: array
              items:
                type: string
          required: [id, sentiment, themes]

  - name: store
    action: http/request
    params:
      method: POST
      url: "https://internal.example.com/db/reviews/batch"
      headers:
        Content-Type: "application/json"
      body:
        records: "{{ steps.enrich.output.json }}"

inputs:
  product_id:
    type: string
    description: The product ID to fetch and enrich reviews for
```

The three-stage pattern:

1. **Fetch** -- pull raw data from the source
2. **Normalize (CEL)** -- extract only the fields you need, rename them, trim whitespace, coerce types
3. **Enrich (AI)** -- pass the clean, minimal payload to the AI step; the smaller the input, the lower the token cost and the more reliable the output

The AI step receives already-cleaned data, so the prompt stays focused on the semantic task rather than field mapping instructions.

## When to Use CEL vs AI

| Situation | Use |
|---|---|
| Rename or reorder fields | CEL |
| Change string case | CEL |
| Parse or format dates and timestamps | CEL |
| Filter a list by a field value | CEL |
| Convert types (string to int, etc.) | CEL |
| Compose values from multiple fields | CEL |
| Classify text into known categories | AI |
| Extract named entities from prose | AI |
| Determine sentiment or tone | AI |
| Summarize freeform content | AI |
| Map between domain vocabularies without a fixed rule | AI |
| Structural reshape + semantic enrichment | Hybrid |

The decision is usually straightforward: if you could write the rule as an `if` statement in Go, use CEL. If you would struggle to enumerate all the cases, use AI.

## Available Functions Reference

These are the Mantle CEL extensions available in workflow expressions. For full signatures, examples, and edge cases, see the [Expressions guide](/docs/concepts/expressions).

**List macros**

| Function | Description |
|---|---|
| `.map(var, expr)` | Transform each element, returning a new list |
| `.filter(var, expr)` | Return elements where `expr` evaluates to true |
| `.exists(var, expr)` | True if any element satisfies `expr` |
| `.exists_one(var, expr)` | True if exactly one element satisfies `expr` |
| `.all(var, expr)` | True if every element satisfies `expr` |

**String**

| Function | Example | Description |
|---|---|---|
| `s.toLower()` | `"HELLO".toLower()` | Convert string to lowercase |
| `s.toUpper()` | `"hello".toUpper()` | Convert string to uppercase |
| `s.trim()` | `" a ".trim()` | Remove leading and trailing whitespace |
| `s.replace(old, new)` | `"a-b".replace("-", "_")` | Replace all occurrences of `old` with `new` |
| `s.split(sep)` | `"a,b".split(",")` | Split string into a list on separator |

**Object construction**

| Function | Description |
|---|---|
| `obj(key, val, ...)` | Construct a map from alternating key-value arguments |

**Type coercion**

| Function | Description |
|---|---|
| `parseInt(s)` | Parse string to integer |
| `parseFloat(s)` | Parse string to float |
| `toString(v)` | Convert any value to its string representation |

**JSON**

| Function | Description |
|---|---|
| `jsonEncode(v)` | Serialize a value to a JSON string |
| `jsonDecode(s)` | Parse a JSON string into a CEL value |

**Time**

| Function | Description |
|---|---|
| `parseTimestamp(s)` | Parse a date/time string into a timestamp (accepts RFC 3339, RFC 3339 Nano, bare ISO datetimes, date-only, US dates, and named-month formats) |
| `formatTimestamp(t, layout)` | Format a timestamp using a Go time layout string |

**Utility**

| Function | Description |
|---|---|
| `default(v, fallback)` | Return `v` if it is set and non-null, otherwise `fallback` |
| `flatten(list)` | Flatten a list of lists into a single list |
