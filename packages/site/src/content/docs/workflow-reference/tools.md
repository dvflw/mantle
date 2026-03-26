# AI Tool Use Reference

Tools let the LLM call back into Mantle connectors during a completion. When you declare `tools` on an `ai/completion` step, the engine runs a multi-turn loop: it sends the prompt to the LLM, the LLM may request tool calls, the engine executes those calls using connector actions, feeds the results back to the LLM, and repeats until the LLM produces a final text response or the configured limits are reached.

## Tool Schema

Each tool in the `tools` list has the following schema:

| Field | Type | Required | Description |
|---|---|---|---|
| `name` | string | Yes | Tool name exposed to the LLM. |
| `description` | string | No | Human-readable description of what the tool does. Helps the LLM decide when to use it. |
| `input_schema` | object | No | JSON Schema describing the tool's input parameters. |
| `action` | string | Yes | Connector action to invoke when the LLM calls this tool (e.g., `http/request`, `postgres/query`). |
| `params` | map | No | Static parameters merged with the LLM-provided arguments when the tool is invoked. |

## How Tool Use Works

1. The engine sends the prompt (and tool definitions) to the LLM.
2. The LLM responds with either a text completion or one or more tool call requests.
3. For each tool call, the engine executes the corresponding connector action and collects the result.
4. The tool results are appended to the conversation and sent back to the LLM.
5. Steps 2-4 repeat until the LLM produces a final text response, or the configured round limit is reached.

## Safety Limits

| Parameter | Default | Description |
|---|---|---|
| `max_tool_rounds` | `10` | Maximum number of LLM-tool round trips. Configurable via `engine.default_max_tool_rounds`. |
| `max_tool_calls_per_round` | `10` | Maximum tool calls the LLM can make in a single turn. Configurable via `engine.default_max_tool_calls_per_round`. |

If the LLM exhausts all rounds without producing a final text response, the engine makes one last call asking the LLM to summarize with the information gathered so far.

## Error Handling

If a tool execution fails, the error message is sent back to the LLM as the tool result rather than crashing the workflow. This gives the LLM the opportunity to retry with different arguments or proceed without that tool's output.

## Example -- Tool Use with Web Search

```yaml
- name: research-assistant
  action: ai/completion
  credential: openai
  params:
    model: gpt-4o
    prompt: "Find the current population of Seattle and summarize the top 3 industries."
    max_tool_rounds: 5
    tools:
      - name: web_search
        description: "Search the web for current information"
        input_schema:
          type: object
          properties:
            query:
              type: string
              description: "Search query"
          required:
            - query
        action: http/request
        params:
          method: GET
          url: "https://api.search.example.com/search"
```

The LLM sees `web_search` as an available function. When it decides to call `web_search(query="Seattle population 2026")`, the engine executes the `http/request` action with the merged parameters and returns the result to the LLM. This continues for up to `max_tool_rounds` rounds.

## Example -- Database Lookup Tool

```yaml
- name: data-analyst
  action: ai/completion
  credential: openai
  params:
    model: gpt-4o
    prompt: "What are the top 5 customers by revenue this quarter?"
    tools:
      - name: query_database
        description: "Run a read-only SQL query against the analytics database"
        input_schema:
          type: object
          properties:
            query:
              type: string
              description: "SQL SELECT query"
          required:
            - query
        action: postgres/query
        params:
          credential: analytics-db
```

The LLM can compose SQL queries and the engine executes them via the `postgres/query` connector, returning the results for the LLM to analyze and summarize.
