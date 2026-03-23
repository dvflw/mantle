# Mantle v0.1.0 Marketing Website -- Creative Brief for Stitch

## Project Overview

Generate a single-page marketing website for the v0.1.0 launch of Mantle, a headless AI workflow automation platform. The site targets platform engineers and SREs who already use Terraform and manage Kubernetes clusters. Every design decision should serve that audience: show code, show terminal output, show architecture. No stock photography. No marketing fluff.

---

## Brand Identity

**Product name:** Mantle
**Tagline:** Define AI workflows as YAML. Deploy them like infrastructure. Run them anywhere.
**Positioning:** Terraform for workflow automation, with native AI.
**License:** BSL 1.1 (source-available). Do NOT call this "open source" anywhere on the page. Use "source-available" when referencing the license model.
**GitHub:** https://github.com/dvflw/mantle
**Docs:** https://mantle.dev/docs (placeholder)

### Tone and Voice
- Technical, confident, developer-first
- Declarative statements. No superlatives ("best", "revolutionary", "game-changing")
- Show, don't tell. Every claim is backed by a code example or terminal screenshot
- Write as if the reader has 10 years of infrastructure experience

### Color Scheme
- **Background:** #0a0a0a (near-black)
- **Surface/cards:** #111111 with subtle 1px border #222222
- **Primary accent:** #00ff88 (green) -- used sparingly for CTAs, highlights, active states
- **Secondary accent:** #3b82f6 (blue) -- used for links and secondary elements
- **Text primary:** #e5e5e5
- **Text secondary:** #a3a3a3
- **Code background:** #0d1117 (GitHub dark theme)
- **Syntax highlighting:** Use a dark theme palette (Monokai, One Dark, or GitHub Dark)

### Typography
- **Headlines:** JetBrains Mono or IBM Plex Mono, bold
- **Body:** Inter or system sans-serif, regular
- **Code:** JetBrains Mono or Fira Code, regular
- **Base size:** 16px body, scale up for headers

### Design Inspiration
- Temporal.io (technical credibility, comparison tables)
- Supabase (dark theme, developer focus, code-forward)
- Linear (clean layout, restrained color use, quality typography)
- Vercel (hero section structure, deploy CTA pattern)

---

## Page Structure and Content

### Section 1: Hero

**Layout:** Full-viewport height. Left side has headline, subheadline, and CTAs. Right side has a code block showing a workflow YAML file in a terminal-style window with syntax highlighting.

**Headline:**
```
Headless AI Workflow Automation
```

**Subheadline:**
```
Define workflows as YAML. Deploy them like infrastructure. Run them anywhere.
Single binary. Postgres state. Bring your own keys.
```

**CTA buttons:**
- Primary (green accent, solid): "View on GitHub" -> https://github.com/dvflw/mantle
- Secondary (outlined): "Get Started" -> https://mantle.dev/docs/getting-started

**Code block** (right side, in a faux terminal window with traffic-light dots and title bar showing "fetch-and-summarize.yaml"):

```yaml
name: fetch-and-summarize
description: Fetch data from an API and summarize it with an LLM

inputs:
  url:
    type: string
    description: URL to fetch

steps:
  - name: fetch-data
    action: http/request
    params:
      method: GET
      url: "{{ inputs.url }}"

  - name: summarize
    action: ai/completion
    credential: my-openai-key
    params:
      model: gpt-4o
      prompt: "Summarize this data: {{ steps['fetch-data'].output.body }}"
      output_schema:
        type: object
        properties:
          summary:
            type: string
          key_points:
            type: array
            items:
              type: string
```

**Version badge** (small, subtle, above the headline):
```
v0.1.0 -- Released March 2026
```

---

### Section 2: Key Differentiators

**Layout:** 4-column grid of cards on desktop, 2x2 on tablet, stacked on mobile. Each card has an icon (use simple monoline SVG icons or emoji-style glyphs), a title, and 2-3 lines of description.

**Card 1: IaC Lifecycle**
- Icon: Git branch or diff icon
- Title: IaC Lifecycle
- Body: validate, plan, apply, run. The same workflow you use for Terraform, applied to workflow automation. Pin executions to immutable versions. Diff before you deploy.

**Card 2: Single Binary + Postgres**
- Icon: Box/cube icon
- Title: Single Binary
- Body: One Go binary. One Postgres database. No message queues, no worker fleets, no cluster topology. Deploy anywhere containers run.

**Card 3: Bring Your Own Keys**
- Icon: Key icon
- Title: BYOK
- Body: Your API keys live in your database, encrypted with your encryption key. Mantle never proxies through a hosted service. OpenAI, Anthropic, Bedrock, Azure, or self-hosted -- you choose.

**Card 4: AI Tool Use**
- Icon: Robot/wrench icon
- Title: AI Tool Use
- Body: Multi-turn function calling out of the box. The LLM requests tools, Mantle executes them via connectors, feeds results back. Crash recovery included -- resume mid-conversation.

---

### Section 3: How It Works

**Layout:** Horizontal 4-step flow with connecting lines/arrows between steps. Below the flow diagram, show a terminal window with the actual commands and output.

**Header:** How It Works

**Steps (shown as connected nodes):**

1. **Write** -- Define your workflow as a YAML file
2. **Validate** -- Check structure offline, run in CI
3. **Apply** -- Store an immutable version in Postgres
4. **Run** -- Execute with checkpoint-and-resume

**Terminal block** (faux terminal window, title: "Terminal"):

```
$ mantle validate examples/fetch-and-summarize.yaml
fetch-and-summarize.yaml: valid

$ mantle plan examples/fetch-and-summarize.yaml
+ fetch-and-summarize (new workflow, version 1)

$ mantle apply examples/fetch-and-summarize.yaml
Applied fetch-and-summarize version 1

$ mantle run fetch-and-summarize --input url=https://api.example.com/data
Running fetch-and-summarize (version 1)...
Execution a1b2c3d4: completed
  fetch-data: completed (0.8s)
  summarize:  completed (2.1s)
```

---

### Section 4: AI-Native Workflows

**Layout:** Two-column. Left column has explanatory text. Right column has a large YAML code block.

**Header:** AI-Native Workflows

**Subheader:** Multi-turn tool use with crash recovery

**Body text (left column):**
```
Mantle's AI connector goes beyond simple prompt-response. Declare tools on
an AI step, each mapping to a real connector action. The engine orchestrates
a multi-turn loop:

1. Send prompt and tool definitions to the LLM
2. LLM responds with tool call requests
3. Engine executes tools via connectors, collects results
4. Results are fed back to the LLM
5. Repeat until the LLM produces a final response

If the process crashes mid-loop, checkpoint-and-resume picks up from the
last completed tool call. No lost context, no repeated work.

Safety limits prevent runaway loops: max_rounds caps LLM-tool round trips,
max_tool_calls_per_round caps parallel tool invocations per turn.
```

**Code block (right column)** -- the research assistant example:

```yaml
name: research-assistant
description: AI agent that searches the web and summarizes findings

inputs:
  - name: topic
    type: string
    description: Research topic

steps:
  - name: research
    action: ai/completion
    credential: openai-key
    params:
      model: gpt-4o
      system_prompt: |
        You are a research assistant. Use the search tool
        to find information, then synthesize your findings
        into a concise summary.
      prompt: "Research this topic: {{ inputs.topic }}"
      max_rounds: 5
      tools:
        - name: web_search
          description: Search the web for information
          action: http/request
          input_schema:
            type: object
            properties:
              query:
                type: string
                description: Search query
            required: [query]
          params:
            method: GET
            url: "https://api.duckduckgo.com/"
            query_params:
              q: "{{ inputs.tool_input.query }}"
              format: json
```

---

### Section 5: Built-in Connectors

**Layout:** 3x3 grid of connector tiles. Each tile has an icon, a name, and a one-line description. Below the grid, a single row for the plugin system.

**Header:** 9 Built-in Connectors

**Connector tiles:**

| Icon | Name | Description |
|------|------|-------------|
| Globe icon | HTTP | REST APIs, webhooks, any HTTP endpoint |
| Brain/sparkle icon | AI (OpenAI) | Chat completions, structured output, tool use |
| Cloud icon | AI (Bedrock) | AWS Bedrock models with region routing |
| Chat bubble icon | Slack | Send messages, read channel history |
| Envelope icon | Email | Send via SMTP, plaintext and HTML |
| Database icon | Postgres | Parameterized SQL against external databases |
| Bucket icon | S3 | Put, get, list objects (S3-compatible) |
| Puzzle icon | Plugin System | Extend with custom connectors in any language |

**Note:** The plugin system tile should span the full width below the grid and say:
```
Need something else? Write a plugin. Any executable that reads JSON from stdin
and writes JSON to stdout. Python, Rust, Node, Bash -- your call.
```

---

### Section 6: Comparison Table

**Layout:** Full-width responsive table with horizontal scroll on mobile. Mantle column should be visually highlighted (slightly brighter background or green accent on column header).

**Header:** How Mantle Compares

**Table content:**

| | Mantle | Temporal | n8n / Zapier | LangChain | Prefect / Airflow |
|---|---|---|---|---|---|
| **Primary use case** | AI workflow automation | Distributed transactions | SaaS integration | LLM app development | Data pipelines |
| **Workflow format** | YAML + CEL | Go / Java SDK | Visual canvas | Python code | Python code |
| **Deployment** | Single binary + Postgres | Multi-service cluster | Self-hosted or cloud | Library in your app | Python platform |
| **AI/LLM support** | First-class (built-in) | Build it yourself | Partial (AI nodes) | First-class (library) | Build it yourself |
| **Checkpointing** | Built-in | Built-in | Partial | None | Built-in |
| **Secrets management** | Built-in, encrypted | External | Built-in | External | External |
| **Version control** | Git-native (IaC) | Code in repos | Database-stored | Code in repos | Code in repos |
| **Target user** | Platform engineers | Backend engineers | Non-technical users | Python developers | Data engineers |
| **Operational complexity** | Low | High | Medium | N/A (library) | Medium-High |

**Below the table, add a footnote:**
```
Mantle is early (v0.1.0). Temporal, Airflow, and LangChain have years of
production hardening and larger ecosystems. Choose the tool that fits your
use case and team.
```

---

### Section 7: Enterprise Features

**Layout:** 2-column grid of feature items, each with an icon and short description. Use a slightly different card style than section 2 (perhaps no border, just icon + text).

**Header:** Built for Production

**Features:**

- **Multi-tenancy and RBAC** -- Teams, users, roles (admin / team_owner / operator). API key authentication with hashed storage.

- **OIDC / SSO** -- Token-sniffing middleware supports API keys and OIDC tokens. Integrate with your existing identity provider.

- **Encrypted Credentials** -- AES-256-GCM encryption at rest. Cloud secret backends: AWS Secrets Manager, GCP Secret Manager, Azure Key Vault. Key rotation support.

- **Audit Trail** -- Every state-changing operation emits an immutable audit event to Postgres. Query with `mantle audit`.

- **Prometheus Metrics** -- Workflow execution counts, step durations, connector latencies, active executions gauge. Scrape `/metrics` in server mode.

- **Helm Chart** -- Production-ready Kubernetes deployment. PDB, migration job, startup probes, security contexts, ServiceMonitor for Prometheus Operator.

- **Rate Limiting** -- Protect external APIs and control execution throughput.

- **Data Retention** -- Configure retention policies for execution history and audit events.

- **Multi-arch Docker Images** -- amd64 + arm64. Published to GHCR.

- **CI Security Scanning** -- govulncheck, gosec, and Trivy scanning in the CI pipeline.

---

### Section 8: Getting Started

**Layout:** Numbered steps in a vertical list, each with a terminal code block. Dark background, tight spacing between steps. Include a "copy" button icon on each code block.

**Header:** Get Running in 5 Minutes

**Step 1: Install**
```bash
go install github.com/dvflw/mantle/cmd/mantle@latest
```

**Step 2: Start Postgres and initialize**
```bash
docker compose up -d
mantle init
```

**Step 3: Apply your first workflow**
```bash
mantle apply examples/hello-world.yaml
# Applied hello-world version 1
```

**Step 4: Run it**
```bash
mantle run hello-world
# Running hello-world (version 1)...
# Execution a1b2c3d4: completed
#   fetch: completed (1.0s)
```

**Below the steps:**
```
17 example workflows included. HTTP, AI, Slack, Postgres, S3, parallel
execution, cron triggers, webhooks, and multi-turn tool use.
```

**CTA button:** "Read the full Getting Started guide" -> https://mantle.dev/docs/getting-started

---

### Section 9: Footer

**Layout:** Simple footer with three columns and a bottom bar.

**Column 1: Product**
- GitHub (https://github.com/dvflw/mantle)
- Documentation (https://mantle.dev/docs)
- Examples (https://github.com/dvflw/mantle/tree/main/examples)
- Changelog (https://github.com/dvflw/mantle/blob/main/CHANGELOG.md)

**Column 2: Resources**
- Getting Started
- Workflow Reference
- CLI Reference
- Concepts

**Column 3: Project**
- License: BSL 1.1 (converts to Apache 2.0 on 2030-03-22)
- Security: security@dvflw.dev
- Contact: licensing@dvflw.dev

**Bottom bar:**
```
(c) 2026 dvflw. Licensed under the Business Source License 1.1.
```

---

## Design Specifications

### Layout
- Maximum content width: 1200px, centered
- Section vertical padding: 120px top and bottom
- Desktop-first, but fully responsive down to 375px mobile
- Sticky/fixed top navigation bar with logo (text "mantle" in monospace), section links (scrollspy), and a GitHub button

### Navigation Bar
- Fixed to top, semi-transparent dark background with backdrop blur
- Logo: "mantle" in JetBrains Mono, lowercase, white, with a small green dot or underscore cursor
- Links: How It Works, Connectors, Compare, Get Started
- Right side: GitHub icon link

### Code Blocks
- Dark background (#0d1117)
- Rounded corners (8px)
- Faux terminal window chrome (three dots: red/yellow/green, and a title bar)
- YAML syntax highlighting with color-coded keys, values, strings, and comments
- Line numbers in a subtle gray (#484848) on the left gutter
- Copy button (clipboard icon) on hover in the top-right corner

### Cards
- Background: #111111
- Border: 1px solid #222222
- Border radius: 12px
- Padding: 24px
- Subtle hover state: border color transitions to #333333

### Animations (subtle, performant)
- Hero code block: typing animation on first load, characters appearing left-to-right
- Cards: fade up on scroll (opacity 0 -> 1, translateY 20px -> 0)
- Terminal blocks: sequential line reveal on scroll
- Comparison table: slide in from bottom
- All animations triggered by IntersectionObserver, play once

### Responsive Behavior
- Desktop (1200px+): full layout as described
- Tablet (768px-1199px): 2-column grids become 2x2, code blocks full width
- Mobile (375px-767px): single column, code blocks horizontally scrollable, comparison table horizontally scrollable, nav collapses to hamburger

---

## Technical Details to Feature

These are specific numbers and capabilities to weave into the copy where relevant:

- 9 built-in connector actions (http/request, ai/completion, slack/send, slack/history, postgres/query, email/send, s3/put, s3/get, s3/list)
- 17 example workflows in the repository
- Checkpoint-and-resume with Postgres-backed state (workflow_executions, step_executions tables)
- CEL (Google Common Expression Language) for data passing between steps
- DAG-based parallel step execution with automatic dependency detection from CEL expressions
- SHA-256 content hashing for immutable versioning
- Prometheus metrics: workflow_executions_total, step_executions_total, step_duration_seconds, connector_duration_seconds, active_executions
- Helm chart with Ingress, HPA, PDB, ServiceMonitor, migration job, startup probes, security contexts
- Multi-arch Docker images (amd64 + arm64) published to GHCR
- SKIP LOCKED work distribution for distributed step execution
- Worker/reaper liveness tracking with health check integration
- Plugin protocol: JSON stdin/stdout (simple), with protobuf spec for future gRPC
- Cloud secret backends: AWS Secrets Manager, GCP Secret Manager, Azure Key Vault
- OIDC/SSO with token-sniffing middleware supporting both API keys and OIDC tokens
- `mantle login` with auth code PKCE and device flow
- Shared workflow library with publish/deploy model
- CI: govulncheck, gosec, Trivy scanning
- License converts from BSL 1.1 to Apache 2.0 on 2030-03-22

---

## What NOT to Include

- Do not use the word "open source." The license is BSL 1.1, which is source-available.
- Do not use stock photography of any kind. No people, no office scenes, no abstract gradients.
- Do not use marketing superlatives: "revolutionary", "best-in-class", "game-changing", "cutting-edge."
- Do not add a pricing section. Mantle is free to use (BSL 1.1 permits production use).
- Do not add a newsletter signup or email capture form.
- Do not add testimonials or social proof (the product just launched).
- Do not add a blog section.
- Do not include animations that block content from being readable on load.

---

## Summary

This is a single-page marketing site for a developer tool. The audience evaluates tools by reading code and running commands, not by watching demo videos. Every section should reinforce the core message: Mantle lets you define AI workflows as YAML, deploy them through an IaC lifecycle, and run them with a single binary backed by Postgres. The design should feel like a tool built by infrastructure engineers, for infrastructure engineers.
