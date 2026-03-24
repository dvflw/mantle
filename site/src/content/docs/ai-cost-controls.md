---
title: AI Cost Controls
description: Configure token budgets to control AI usage at the workflow, team, and global level.
---

# AI Cost Controls

Mantle provides token-based budgets at three levels to help you control AI usage costs. Budgets are denominated in **tokens** (not dollars) to stay provider-agnostic and avoid stale pricing data.

## Budget Levels

All three budget levels compose as **AND gates** — every applicable level must pass before an AI step is dispatched. If any level is exceeded, the step is blocked (or warned, depending on configuration).

| Level | Scope | Configured In | Enforcement | Reset |
|-------|-------|---------------|-------------|-------|
| **Global** | All teams, all providers | `mantle.yaml` | Hard block only | Monthly |
| **Team + Provider** | One team, one provider | API / CLI | Configurable (hard or warn) | Monthly |
| **Workflow** | Single execution | Workflow YAML | Configurable (hard or warn) | Per execution |

### Enforcement Behavior

- **Hard block (default):** The AI step fails with a budget error. The execution stops at that step. Tokens consumed by prior steps in the execution are preserved — Mantle never cancels a step mid-execution.
- **Warn only:** The AI step proceeds, but a warning is logged and an audit event is emitted. The `mantle_budget_check_total` Prometheus metric is incremented with `result="warning"`.

## Configuration

### Global Budget (mantle.yaml)

```yaml
engine:
  budget:
    global_monthly_token_limit: 10000000  # 0 = unlimited
    default_team_monthly_token_limit: 1000000  # applied to teams without explicit budgets
    reset_mode: calendar  # "calendar" or "rolling"
    reset_day: 1          # 1-28, only used when reset_mode is "rolling"
```

**Environment variables:**

| Variable | Description |
|----------|-------------|
| `MANTLE_ENGINE_BUDGET_GLOBAL_MONTHLY_TOKEN_LIMIT` | Global token cap (hard block) |
| `MANTLE_ENGINE_BUDGET_DEFAULT_TEAM_MONTHLY_TOKEN_LIMIT` | Default per-team cap |
| `MANTLE_ENGINE_BUDGET_RESET_MODE` | `calendar` or `rolling` |
| `MANTLE_ENGINE_BUDGET_RESET_DAY` | Start day for rolling periods (1-28) |

### Reset Windows

- **Calendar month:** Budget resets on the 1st of each month (UTC).
- **Rolling period:** Budget resets on your configured `reset_day` each month. For example, `reset_day: 15` means the current period runs from the 15th of the current month to the 14th of the next. The maximum `reset_day` is 28 to avoid February edge cases.

### Team + Provider Budget (API / CLI)

Set per-provider budgets for your team:

```bash
# Set a hard budget of 1M tokens/month for OpenAI
mantle budget set openai 1000000

# Set a warn-only budget for Bedrock
mantle budget set bedrock 500000 --enforcement warn

# Set a budget for all providers
mantle budget set '*' 2000000

# View current budgets
mantle budget get

# View current usage
mantle budget usage
mantle budget usage --provider openai

# Remove a budget
mantle budget delete openai
```

**API:**

```bash
# Set budget
curl -X PUT /api/v1/budgets/openai \
  -d '{"monthly_token_limit": 1000000, "enforcement": "hard"}'

# List budgets
curl /api/v1/budgets

# Get usage
curl /api/v1/budgets/usage?provider=openai

# Delete budget
curl -X DELETE /api/v1/budgets/openai
```

### Workflow Budget (YAML)

Add a `token_budget` field at the workflow level to cap total tokens consumed in a single execution:

```yaml
name: my-analysis-workflow
token_budget: 500000  # max tokens across all AI steps in one execution

steps:
  - name: summarize
    action: ai/completion
    params:
      model: gpt-4o
      prompt: "Summarize this document: {{ inputs.document }}"
      max_token_budget: 100000  # per-step limit (existing feature)

  - name: analyze
    action: ai/completion
    params:
      model: gpt-4o
      prompt: "Analyze the summary: {{ steps.summarize.output.text }}"
```

The workflow `token_budget` is checked before each AI step. If the cumulative tokens from prior steps exceed the budget, the next AI step is blocked. Non-AI steps (HTTP, etc.) are unaffected.

The existing per-step `max_token_budget` param continues to work independently — it caps tokens within a single step's tool-use loop.

## Monitoring

### Prometheus Metrics

| Metric | Labels | Description |
|--------|--------|-------------|
| `mantle_budget_check_total` | `team_id`, `provider`, `result` | Budget checks (result: pass/blocked/warning) |
| `mantle_budget_usage_tokens` | `team_id`, `provider` | Current token usage in the budget period |
| `mantle_ai_tokens_total` | `workflow`, `step`, `model`, `provider`, `token_type` | Raw token consumption (existing) |

### Audit Events

| Action | When |
|--------|------|
| `budget.exceeded` | AI step blocked by budget |
| `budget.warning` | AI step proceeded with warn-only budget exceeded |
| `budget.updated` | Team budget created, updated, or deleted |

## Provider Pricing Reference

Token budgets are provider-agnostic. To estimate costs, refer to your provider's pricing page:

- **OpenAI:** [https://openai.com/api/pricing](https://openai.com/api/pricing)
- **AWS Bedrock:** [https://aws.amazon.com/bedrock/pricing](https://aws.amazon.com/bedrock/pricing)
- **Anthropic:** [https://www.anthropic.com/pricing](https://www.anthropic.com/pricing)
- **Google AI:** [https://ai.google.dev/pricing](https://ai.google.dev/pricing)

> **Tip:** Most providers charge differently for prompt (input) vs. completion (output) tokens. Mantle tracks both separately in the `ai_token_usage` table and Prometheus metrics, so you can compute costs externally using your provider's rate card.
