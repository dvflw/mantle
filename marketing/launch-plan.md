# Mantle v0.1.0 Launch Marketing Plan

## Executive Summary

Mantle is a headless AI workflow automation platform targeting platform engineers, SREs, and DevOps engineers who already use Terraform and Kubernetes. The positioning is "Terraform for workflow automation, with native AI." This plan covers a 10-week campaign window: 2 weeks pre-launch, launch day, and 8 weeks of sustained momentum.

The primary goal is awareness among infrastructure engineers. The secondary goal is GitHub stars and early adopters. Every piece of content must demonstrate code, terminal output, or architecture. No marketing fluff.

**Key constraints:**
- BSL 1.1 license. Never say "open source." Always say "source-available."
- v0.1.0 is an early release. Be honest about maturity. Credibility is the brand.
- The audience detects and punishes hype. Lead with substance.

---

## 1. Launch Timeline

### Week -2: Groundwork (March 9-15)

| Day | Activity | Platform |
|-----|----------|----------|
| Mon | Set up all social accounts (Twitter/X, LinkedIn company page, Dev.to, Product Hunt "coming soon") | All |
| Mon | Publish GitHub repo as public (if not already), ensure README, CHANGELOG, examples/ are polished | GitHub |
| Tue | Personal tweet from founder: "Been building something for the last few months. Infrastructure-native workflow automation with first-class AI. Shipping soon." | Twitter/X |
| Wed | First teaser: terminal recording of `mantle validate` / `mantle plan` / `mantle apply` / `mantle run` cycle (30 sec) | Twitter/X |
| Thu | LinkedIn post from founder's personal account: "Why we built another workflow tool" (short version, 3 paragraphs) | LinkedIn |
| Fri | Second teaser: screenshot of the AI tool-use YAML with caption "Multi-turn function calling, defined in YAML, with crash recovery. Shipping next week." | Twitter/X |
| Sat-Sun | Prep all launch day assets: blog post, HN submission, Twitter thread, Reddit posts, Dev.to article |  |

### Week -1: Build Anticipation (March 16-21)

| Day | Activity | Platform |
|-----|----------|----------|
| Mon | Teaser: comparison table image (Mantle vs Temporal vs n8n vs LangChain vs Airflow) | Twitter/X, LinkedIn |
| Tue | Short video/GIF: `mantle run` executing a workflow with checkpoint output | Twitter/X |
| Wed | "Ask me anything about building a workflow engine in Go" -- engage with Go community | Twitter/X |
| Thu | Teaser: "9 built-in connectors. HTTP, AI (OpenAI + Bedrock), Slack, Email, Postgres, S3. One binary." | Twitter/X |
| Fri | Final teaser: "Tomorrow." with a screenshot of the website hero section | Twitter/X |
| Fri | Queue all launch day posts. Brief 3-5 people who will amplify on launch day. | -- |

### Launch Day: March 22 (Saturday)

Saturday launch is intentional -- Hacker News traffic is high on weekends, and the audience (engineers) browses on Saturday mornings.

| Time (ET) | Activity | Platform |
|-----------|----------|----------|
| 8:00 AM | Submit Show HN post | Hacker News |
| 8:15 AM | Publish launch announcement thread (8-10 tweets) | Twitter/X |
| 8:30 AM | Publish LinkedIn announcement (founder personal + company page) | LinkedIn |
| 9:00 AM | Post to r/golang, r/selfhosted, r/devops | Reddit |
| 9:30 AM | Publish "Introducing Mantle" article | Dev.to |
| 10:00 AM | Monitor and respond to every HN comment within 30 minutes | Hacker News |
| All day | Engage with every reply, retweet, and question across all platforms | All |
| Evening | Post a "launch day recap" tweet: "Launched Mantle this morning. X stars on GitHub, Y comments on HN. Thank you." | Twitter/X |

### Week +1: Amplify (March 23-29)

| Day | Activity | Platform |
|-----|----------|----------|
| Mon | "Why we built Mantle" narrative thread | Twitter/X |
| Tue | Technical deep-dive post: IaC lifecycle for workflows | LinkedIn |
| Wed | Tutorial: "Build an AI research assistant in 5 minutes with Mantle" | Dev.to |
| Thu | Comparison thread: "Mantle vs Temporal -- when to use which" | Twitter/X |
| Fri | Community roundup: highlight interesting questions from HN/Reddit, answer them publicly | Twitter/X |

### Weeks +2 through +4: Sustain

| Week | Content Theme | Key Posts |
|------|--------------|-----------|
| +2 | AI tool use deep-dive | Twitter thread on multi-turn function calling, Dev.to tutorial on building a Slack bot with AI tool use |
| +3 | Infrastructure story | "Single binary + Postgres: why we chose operational simplicity," Helm chart walkthrough |
| +4 | Use cases | 3 concrete use case threads (content pipeline, incident response, data enrichment), first community showcase |

### Weeks +5 through +8: Grow

| Week | Content Theme | Key Posts |
|------|--------------|-----------|
| +5 | Contributor onboarding | "Good first issues" campaign, contributor guide highlight, first external PR celebration |
| +6 | Competitive depth | Detailed "Mantle vs LangChain" article on Dev.to, comparison for Python teams considering Go alternatives |
| +7 | Enterprise angle | Multi-tenancy and RBAC walkthrough, OIDC/SSO setup guide, audit trail demo |
| +8 | Roadmap and community | Public roadmap share, "What should we build next?" engagement post, Month 2 retrospective |

---

## 2. Platform Strategy

### Twitter/X

**Role:** Primary real-time channel. Build personal brand of founder as an infrastructure-minded builder.

**Content types:**
- Terminal screenshots and recordings (highest engagement for dev tools)
- Code snippet images (YAML workflows with syntax highlighting)
- Threads (technical deep-dives, narratives, comparisons)
- Quick takes on industry news (AI agents, workflow automation, IaC trends)
- Engagement replies to relevant conversations in the DevOps/platform engineering space

**Posting frequency:** 5-7 tweets per week. 1 thread per week. Daily engagement replies.

**Tone:** Direct, technical, occasionally dry humor. Write like a staff engineer, not a marketer. No hashtag spam. Use #golang and #devops sparingly and only when genuinely relevant.

**Format guidelines:**
- Lead with the insight, not the product name
- Always include a visual (screenshot, code block, terminal recording)
- Threads should be self-contained -- each tweet readable on its own
- End threads with a link to the repo, not a "please star" ask

**Example posts:**

Teaser tweet:
```
Workflow automation tools make you choose: visual canvas (n8n) or write
a Go/Python SDK (Temporal, Airflow).

What if workflows were just YAML files with an IaC lifecycle?

validate -> plan -> apply -> run

Shipping soon.
```

Quick insight tweet:
```
The moment you need to diff a workflow change before deploying it,
you've outgrown tools that store definitions in a database.

mantle plan shows you exactly what will change. Same mental model
as terraform plan.
```

Terminal screenshot tweet:
```
9 connectors. 17 example workflows. One binary.

$ mantle apply examples/research-assistant.yaml
Applied research-assistant version 1

$ mantle run research-assistant --input topic="kubernetes cost optimization"
Running research-assistant (version 1)...
  research: completed (4.2s) [3 tool calls, 2 rounds]

[screenshot of terminal output]
```

### LinkedIn

**Role:** Professional credibility. Reach engineering managers, directors of platform engineering, and DevOps leads who evaluate tools for their teams.

**Content types:**
- Founder personal posts (primary -- gets 5-10x the reach of company page posts)
- Company page posts (mirror key announcements, maintain presence)
- LinkedIn articles (long-form technical content, 1-2 per month)
- Comment engagement on posts by DevOps thought leaders

**Posting frequency:** 3-4 posts per week on founder's personal account. 2 posts per week on company page.

**Tone:** Professional but not corporate. Write as an engineer solving a real problem, not as a startup founder selling a vision. Avoid LinkedIn-speak ("thrilled to announce," "excited to share"). State facts.

**Format guidelines:**
- First line must hook. LinkedIn truncates after ~210 characters
- Use line breaks aggressively -- LinkedIn's algorithm rewards readability
- Include a code block or screenshot in every post
- End with a question to drive comments (algorithm signal)

**Content pillars:**
1. **Builder's perspective** -- What decisions you made and why (Go, Postgres-only, YAML, CEL)
2. **IaC philosophy** -- Why workflow definitions should be treated like infrastructure code
3. **AI in production** -- Practical takes on deploying AI workflows with proper ops (checkpoints, secrets, audit)
4. **Competitive landscape** -- Honest comparisons, acknowledging trade-offs

**Example post (founder personal):**

```
We spent 6 months building a workflow automation tool.

Here is the unpopular decision that defines it:
no visual workflow builder.

Workflows are YAML files. They live in git. Changes go through
code review. You run `mantle plan` to see the diff before deploying.

The same lifecycle you already use for Terraform.

This is not for everyone. If your team wants a drag-and-drop canvas,
use n8n -- it is excellent.

But if your team already manages infrastructure as code and wants
the same rigor for workflow automation, this is what we built.

9 built-in connectors. First-class AI tool use. Single Go binary
+ Postgres. Source-available under BSL 1.1.

github.com/dvflw/mantle

What is the workflow automation tool your team actually uses in
production? Curious to hear what is working and what is not.
```

**Example post (company page):**

```
Mantle v0.1.0 is out.

Headless AI workflow automation. Define workflows as YAML, deploy
through an IaC lifecycle, run with a single binary backed by Postgres.

What shipped:
- validate / plan / apply / run lifecycle
- AI tool use with multi-turn function calling and crash recovery
- 9 built-in connectors (HTTP, AI, Slack, Email, Postgres, S3)
- Checkpoint-and-resume execution
- Multi-tenancy, RBAC, OIDC/SSO
- Helm chart, Prometheus metrics, audit trail
- 17 example workflows

Source-available under BSL 1.1. Read the docs, clone the repo,
run `mantle apply` on your first workflow.

github.com/dvflw/mantle
```

### Hacker News

See dedicated section below (Section 4).

### Reddit

**Role:** Community discovery. Reach engineers in their natural habitats.

**Target subreddits:**
- r/golang (primary -- Go community will care about architecture decisions)
- r/selfhosted (self-hosted single binary is a strong angle)
- r/devops (IaC lifecycle resonates here)
- r/MachineLearning or r/LocalLLaMA (BYOK angle for AI workflows)
- r/sre (operational simplicity angle)

**Posting frequency:** Launch day posts, then 1-2 posts per month. Primarily participate in existing threads.

**Tone:** Casual, honest, self-aware. Reddit punishes self-promotion harder than any other platform. Lead with the problem you solved, not the product. Acknowledge limitations.

**Format guidelines:**
- Title should describe what the thing does, not what it is called
- Body should explain the motivation, show code, and invite feedback
- Always disclose you are the creator
- Answer every question thoroughly, even critical ones
- Never delete negative comments or get defensive

**Example Reddit titles:**
- r/golang: `I built a workflow automation engine in Go with an IaC lifecycle (validate/plan/apply/run) and first-class AI tool use`
- r/selfhosted: `Mantle: single-binary workflow automation with AI -- Go + Postgres, no external dependencies`
- r/devops: `What if workflow definitions went through the same lifecycle as Terraform? validate -> plan -> apply -> run`

**Example r/golang post body:**

```
I have been building Mantle for the past 6 months. It is a workflow
automation engine written in Go that treats workflow definitions like
infrastructure code.

The core idea: workflows are YAML files. You validate them offline,
plan to see the diff, apply to store an immutable version, and run
to execute. Same mental model as Terraform.

Some technical decisions I am happy to discuss:

- Single binary + Postgres (no message queues, no worker fleets)
- CEL (Google Common Expression Language) for expressions between steps
- SKIP LOCKED for distributed step execution
- DAG-based parallel execution with dependency detection from CEL refs
- AI tool-use loop with checkpoint-and-resume for crash recovery
- AES-256-GCM credential encryption with cloud backend support

9 built-in connectors, plugin system for custom ones (JSON over
stdin/stdout -- any language).

Source-available under BSL 1.1 (not open source -- wanted to be
upfront about that).

GitHub: github.com/dvflw/mantle

Happy to answer questions about the Go architecture, the design
decisions, or anything else. What would you have done differently?
```

### Dev.to / Hashnode

**Role:** Long-form technical content. SEO. Reach developers who discover tools through tutorial content.

**Content types:**
- Launch announcement article
- Technical tutorials (step-by-step with code)
- Architecture decision records ("Why we chose X")
- Comparison articles

**Posting frequency:** 1 article per week for first 4 weeks, then biweekly.

**Tone:** Tutorial-friendly, thorough, code-heavy. Every article should be usable as a standalone guide.

**Article pipeline:**

| Week | Title | Angle |
|------|-------|-------|
| Launch | "Introducing Mantle: Terraform for Workflow Automation, with Native AI" | Launch announcement with full walkthrough |
| +1 | "Build an AI Research Assistant in 10 Minutes with Mantle" | Step-by-step tutorial, beginner-friendly |
| +2 | "Multi-Turn AI Tool Use with Crash Recovery: How Mantle's AI Connector Works" | Technical deep-dive |
| +3 | "Why We Chose Go, Postgres, and YAML for a Workflow Engine" | Architecture decisions |
| +4 | "Mantle vs Temporal: Choosing the Right Workflow Engine" | Honest comparison |
| +6 | "Setting Up Mantle on Kubernetes with Helm" | Operations tutorial |
| +8 | "Building a Custom Mantle Connector in Python" | Plugin system tutorial |

### Product Hunt

**Role:** One-day visibility spike. Secondary channel.

**Timing:** Launch on a Tuesday or Wednesday, 1-2 weeks after the primary launch (to have HN/Reddit traction as social proof).

**Listing details:**
- Tagline: "Define AI workflows as YAML. Deploy like infrastructure."
- Description: Focus on the 3 key differentiators (IaC lifecycle, single binary, AI tool use)
- Media: Terminal recording GIF, comparison table image, YAML code screenshot
- Maker comment: Explain the motivation, link to GitHub and docs

### GitHub

**Role:** The product itself. The README is the most important marketing asset.

**Optimization:**
- README should work as a standalone landing page (it already does -- see current README)
- Topics/tags: `workflow-automation`, `ai`, `llm`, `infrastructure-as-code`, `golang`, `yaml`, `devops`, `platform-engineering`
- Pin key discussions in GitHub Discussions (if enabled)
- Maintain a "good first issues" label for contributor onboarding
- Respond to every issue within 24 hours during the first month
- Star the repo from personal accounts of all team members (this is normal and expected)

---

## 3. Content Calendar

### Pre-Launch: Week -2 (March 9-15)

| Day | Platform | Content | Format |
|-----|----------|---------|--------|
| Mon 3/9 | Twitter/X | "Been building something. Infrastructure-native workflow automation with first-class AI. Details soon." | Text + screenshot of YAML |
| Tue 3/10 | Twitter/X | Terminal recording: validate/plan/apply/run cycle, 30 seconds | Video/GIF |
| Wed 3/11 | LinkedIn | "Why we built another workflow tool" -- 3-paragraph founder post | Text |
| Thu 3/12 | Twitter/X | Screenshot: AI tool-use YAML example. "Multi-turn function calling, defined in YAML. Crash recovery included." | Image |
| Fri 3/13 | Twitter/X | Screenshot: comparison table. "Where Mantle fits." | Image |

### Pre-Launch: Week -1 (March 16-21)

| Day | Platform | Content | Format |
|-----|----------|---------|--------|
| Mon 3/16 | Twitter/X | "9 built-in connectors. HTTP, AI (OpenAI + Bedrock), Slack, Email, Postgres, S3. One binary. One database." | Text + connector grid image |
| Tue 3/17 | Twitter/X | GIF: `mantle run` with step-by-step checkpoint output | Video/GIF |
| Wed 3/18 | Twitter/X | "Ask me anything about building a workflow engine in Go from scratch" | Text (engagement) |
| Thu 3/19 | LinkedIn | "The case for treating workflow definitions as infrastructure code" -- founder post | Text |
| Fri 3/21 | Twitter/X | "Tomorrow." + hero section screenshot | Image |

### Launch Day: March 22

| Time | Platform | Content | Format |
|------|----------|---------|--------|
| 8:00 AM | Hacker News | Show HN submission (see Section 4) | Link post |
| 8:15 AM | Twitter/X | Launch announcement thread (see Section 5) | Thread (8-10 tweets) |
| 8:30 AM | LinkedIn | Founder personal launch post + company page post | Text + images |
| 9:00 AM | Reddit | r/golang, r/selfhosted, r/devops posts | Text posts |
| 9:30 AM | Dev.to | "Introducing Mantle" article | Long-form article |
| Evening | Twitter/X | "Launch day recap: X stars, Y HN comments. Thank you." | Text |

### Week +1 (March 23-29)

| Day | Platform | Content | Format |
|-----|----------|---------|--------|
| Mon | Twitter/X | "Why we built Mantle" narrative thread (6-8 tweets) | Thread |
| Tue | LinkedIn | "IaC lifecycle for workflows: validate, plan, apply, run" -- technical post | Text + terminal screenshot |
| Wed | Dev.to | "Build an AI Research Assistant in 10 Minutes with Mantle" | Tutorial article |
| Thu | Twitter/X | "Mantle vs Temporal" comparison thread | Thread |
| Fri | Twitter/X | Community roundup: "Great questions from HN and Reddit this week. Here are answers." | Thread |

### Week +2 (March 30 - April 5)

| Day | Platform | Content | Format |
|-----|----------|---------|--------|
| Mon | Twitter/X | "How Mantle's AI tool-use loop works" -- technical thread with diagrams | Thread |
| Wed | Dev.to | "Multi-Turn AI Tool Use with Crash Recovery" | Technical article |
| Thu | LinkedIn | "BYOK: why your AI keys should live in your database, not ours" | Founder post |
| Fri | Twitter/X | Quick tip: "You can define output_schema on ai/completion to get structured JSON back. No parsing, no regex." | Text + code screenshot |

### Week +3 (April 6-12)

| Day | Platform | Content | Format |
|-----|----------|---------|--------|
| Mon | Twitter/X | "Single binary + Postgres. Here is why we did not add Redis, RabbitMQ, or Kafka." | Text |
| Wed | Dev.to | "Why We Chose Go, Postgres, and YAML for a Workflow Engine" | Architecture article |
| Thu | LinkedIn | Helm chart walkthrough for Kubernetes deployment | Text + code |
| Fri | Twitter/X | "17 example workflows ship with Mantle. Here are the 5 most interesting ones." | Thread |

### Week +4 (April 13-19)

| Day | Platform | Content | Format |
|-----|----------|---------|--------|
| Mon | Twitter/X | Use case: "Automated content pipeline -- fetch RSS, summarize with AI, post to Slack" | Code screenshot |
| Tue | Product Hunt | Product Hunt launch | Listing |
| Wed | Twitter/X | Use case: "Incident enrichment -- on PagerDuty webhook, query logs, summarize with AI, update ticket" | Code screenshot |
| Thu | Dev.to | "Mantle vs Temporal: Choosing the Right Workflow Engine" | Comparison article |
| Fri | LinkedIn | First community showcase: highlight an early adopter or interesting use case | Text |

---

## 4. Hacker News Strategy

### The Show HN Post

**Title options (pick one):**

1. `Show HN: Mantle -- Define AI workflows as YAML, deploy like Terraform`
2. `Show HN: Mantle -- Headless workflow automation with IaC lifecycle and AI tool use`
3. `Show HN: Mantle -- Single-binary workflow engine with validate/plan/apply/run`

Recommended: **Option 1.** It communicates the two key ideas (YAML workflows + Terraform mental model) in the shortest form. "AI" is a draw. "Terraform" signals the audience.

**What to link to:** The GitHub repo (github.com/dvflw/mantle), not the marketing website. HN strongly prefers linking to the repo for Show HN posts. The README serves as the landing page.

**Post body:**

```
Hey HN,

I have been building Mantle for the past 6 months. It is a workflow
automation engine that treats workflow definitions like infrastructure
code.

The core idea: workflows are YAML files with a Terraform-style lifecycle.
`mantle validate` checks syntax offline. `mantle plan` shows you the diff.
`mantle apply` stores an immutable version. `mantle run` executes it with
checkpoint-and-resume.

It has first-class AI support: the ai/completion connector handles
OpenAI and AWS Bedrock, supports structured output schemas, and can do
multi-turn tool use -- where the LLM requests tools, Mantle executes
them via connectors, and feeds results back. If the process crashes
mid-loop, it resumes from the last checkpoint.

What ships in v0.1.0:
- 9 built-in connectors (HTTP, AI, Slack, Email, Postgres, S3)
- DAG-based parallel execution
- AES-256-GCM encrypted secrets with cloud backend support
- Multi-tenancy and RBAC
- Helm chart, Prometheus metrics, audit trail
- 17 example workflows
- Plugin system for custom connectors (JSON stdin/stdout)

Single Go binary. Postgres for state. No message queues, no worker
fleets.

Source-available under BSL 1.1 (not open source -- converts to
Apache 2.0 in 2030). Production use is fine; commercial resale is not.

Would love feedback on the design, the YAML format, the connector
model, or anything else.

GitHub: https://github.com/dvflw/mantle
Docs: https://mantle.dvflw.co/docs
```

### Timing

Post between **8:00-9:00 AM ET on Saturday** (March 22). Weekend mornings have lower competition on HN, and the dev audience is browsing. Saturday is better than Sunday. Avoid Friday afternoon (dies before Monday) and Monday (highest competition).

### Comment Engagement Rules

**Do:**
- Respond to every comment within 30 minutes for the first 4 hours
- Be genuinely grateful for feedback, even harsh feedback
- Answer technical questions with depth -- show you know the codebase
- Acknowledge limitations honestly: "You are right, the connector ecosystem is small compared to n8n. We ship 9 today and have a plugin system for extension."
- When someone suggests a feature, say whether it is on the roadmap or explain the trade-off
- Engage with competing-tool fans respectfully: "Temporal is excellent for distributed transactions. Mantle targets a different use case -- declarative AI workflows for teams that want IaC semantics."

**Do not:**
- Ask people to star the repo
- Use marketing language ("we are excited," "game-changing")
- Get defensive about the BSL license -- state the facts calmly and move on
- Argue with trolls -- one factual response, then disengage
- Post from multiple accounts or ask friends to upvote (HN detects and penalizes this)
- Edit the post title after submission (HN penalizes this)

### Handling the BSL License Question

This will come up. Have this response ready:

```
Mantle is source-available under BSL 1.1, not open source. We chose BSL
because it lets us keep the project sustainable while allowing production
use. The restriction is narrow: you cannot resell Mantle as a hosted
workflow-as-a-service. Everything else -- running it in production,
modifying it, contributing to it -- is permitted. The license converts
to Apache 2.0 on 2030-03-22.

We considered AGPLv3 and went with BSL instead because it is clearer
about what is and is not permitted. Reasonable people disagree on
licensing. We chose what we think gives us the best chance of maintaining
the project long-term.
```

---

## 5. Twitter/X Thread Templates

### Thread 1: Launch Announcement

```
Tweet 1:
Introducing Mantle -- headless AI workflow automation.

Define workflows as YAML. Deploy them with an IaC lifecycle.
Run them with a single binary backed by Postgres.

v0.1.0 is out today. Here is what it does and why we built it.

github.com/dvflw/mantle

[terminal screenshot: validate/plan/apply/run cycle]

Tweet 2:
The problem: every workflow tool makes you choose.

Visual canvas (n8n, Zapier) -- great for non-technical users,
but workflows live in a database, not git.

SDK-based (Temporal, Airflow) -- powerful, but you are writing
and deploying application code.

We wanted a third option.

Tweet 3:
Mantle workflows are YAML files.

They go through the same lifecycle as Terraform:

  mantle validate  -- check syntax offline, run in CI
  mantle plan      -- see the diff before deploying
  mantle apply     -- store an immutable version
  mantle run       -- execute with checkpoint-and-resume

[screenshot of plan output]

Tweet 4:
AI is a first-class citizen.

The ai/completion connector supports OpenAI and AWS Bedrock,
structured output schemas, and multi-turn tool use.

The LLM requests tools. Mantle executes them via connectors.
Results feed back to the LLM. Crash recovery included.

[screenshot of AI tool-use YAML]

Tweet 5:
One Go binary. One Postgres database.

No Redis. No RabbitMQ. No Kafka. No worker fleets.
No cluster topology.

Deploy anywhere containers run.

go install github.com/dvflw/mantle/cmd/mantle@latest
docker compose up -d
mantle init
mantle apply examples/hello-world.yaml
mantle run hello-world

Tweet 6:
9 built-in connectors ship today:

- HTTP (REST APIs, webhooks)
- AI / OpenAI (completions, structured output, tool use)
- AI / Bedrock (AWS models with region routing)
- Slack (send messages, read history)
- Email (SMTP)
- Postgres (parameterized SQL)
- S3 (put, get, list -- S3-compatible)

Need more? Write a plugin in any language.

Tweet 7:
Production features in v0.1.0:

- Multi-tenancy with RBAC
- OIDC/SSO authentication
- AES-256-GCM encrypted credentials
- Cloud secret backends (AWS, GCP, Azure)
- Prometheus metrics
- Audit trail
- Helm chart with PDB, probes, security contexts
- CI scanning (govulncheck, gosec, Trivy)

Tweet 8:
17 example workflows ship in the repo.

HTTP requests, AI completions, Slack bots, parallel execution,
cron triggers, webhooks, multi-turn tool use, S3 operations,
and more.

Clone the repo, run `mantle apply`, and try them.

github.com/dvflw/mantle/tree/main/examples

Tweet 9:
Source-available under BSL 1.1. Production use is permitted.
Commercial resale as a hosted service is not. Converts to
Apache 2.0 in 2030.

We think this is the right balance between sustainability
and accessibility. Read the license in the repo.

Tweet 10:
If you build infrastructure and want workflow automation
that fits your existing practices -- git, code review, CI/CD,
IaC -- give Mantle a look.

GitHub: github.com/dvflw/mantle
Docs: mantle.dvflw.co/docs

Feedback, issues, and contributions are welcome.
```

### Thread 2: "Why We Built This"

```
Tweet 1:
Why we built another workflow automation tool.

Short version: we wanted Terraform semantics for workflow
definitions, and no existing tool gave us that.

Longer version (thread):

Tweet 2:
We were a platform team running AI workflows in production.

Our options were:
- Airflow (Python, heavy, not designed for AI)
- Temporal (powerful, but operational overhead of a cluster)
- n8n (visual builder, workflows live in a DB)
- LangChain (library, not a platform -- we own all the ops)

Tweet 3:
What we actually wanted:

1. Workflow definitions in git, reviewed like code
2. A diff before deploying (terraform plan for workflows)
3. AI as a first-class concept, not bolted on
4. A single binary we could deploy without a PhD in distributed systems

None of the existing tools gave us all four.

Tweet 4:
So we built Mantle.

YAML for workflow definitions. CEL for expressions.
Postgres for state. Go for the binary.

No SDK to learn. No visual editor to click through.
Write YAML, commit to git, deploy through CI.

Tweet 5:
The IaC lifecycle was non-negotiable.

mantle validate -- catches errors before they hit production
mantle plan -- shows exactly what will change
mantle apply -- stores an immutable, versioned definition
mantle run -- executes against a pinned version

Same muscle memory as Terraform.

Tweet 6:
AI tool use was the feature that convinced us this needed to exist.

Most workflow tools treat AI as "call an API, get text back."

We built a multi-turn loop: the LLM requests tools, the engine
executes them via real connectors, feeds results back. With
checkpointing at every step.

Tweet 7:
We are not trying to replace Temporal or Airflow.

Temporal is better for distributed transactions.
Airflow is better for Python data pipelines.

Mantle is for teams that want declarative, version-controlled
AI workflow automation with minimal operational overhead.

Tweet 8:
v0.1.0 is early. The connector ecosystem is small.
The community is just getting started.

But the core is solid: engine, checkpointing, IaC lifecycle,
AI tool use, secrets, RBAC, Helm chart.

If this resonates, give it a try.
github.com/dvflw/mantle
```

### Thread 3: Technical Deep-Dive (IaC Lifecycle + AI Tool Use)

```
Tweet 1:
How Mantle's IaC lifecycle works under the hood.

validate -> plan -> apply -> run

Each step does something specific. Here is the technical detail.

Tweet 2:
`mantle validate` runs offline. No database connection needed.

It checks:
- YAML structure against JSON Schema
- CEL expression syntax
- Connector action references
- Input/output type compatibility

Run it in CI. Catch errors before merge.

Tweet 3:
`mantle plan` connects to Postgres and computes a diff.

It SHA-256 hashes the workflow definition and compares against
the currently applied version. Shows you exactly what changed:
new steps, removed steps, modified parameters.

Same experience as terraform plan.

Tweet 4:
`mantle apply` stores an immutable version.

The definition is content-addressed (SHA-256 hash). Each apply
creates a new version number. Old versions are never modified.
Executions are pinned to the version that was current at trigger time.

Tweet 5:
`mantle run` executes with checkpoint-and-resume.

Each step writes its result to Postgres on completion. If the
process crashes, restart picks up from the last completed step.

Steps with no dependencies on each other run in parallel via
DAG scheduling. Dependencies are automatically detected from
CEL expression references.

Tweet 6:
AI tool use is where it gets interesting.

When you define `tools` on an ai/completion step, the engine
enters a multi-turn loop:

1. Send prompt + tool schemas to LLM
2. LLM returns tool_calls
3. Engine executes each tool via its connector
4. Results fed back to LLM
5. Repeat until final response or max_rounds

Tweet 7:
Every tool execution is checkpointed.

If the process crashes after tool call 3 of 5 in round 2,
restart resumes from tool call 4. The LLM conversation
context is preserved.

max_rounds and max_tool_calls_per_round prevent runaway loops.

Tweet 8:
All of this is defined in YAML:

[screenshot of research-assistant.yaml with tools defined]

No SDK. No application code. Define the tools, map them to
connectors, set safety limits. The engine handles the rest.
```

### Thread 4: Comparison Thread ("Mantle vs X")

```
Tweet 1:
"How is Mantle different from [Temporal / n8n / LangChain / Airflow]?"

Honest comparison. No shade -- these are good tools for different
use cases.

Tweet 2:
Mantle vs Temporal:

Temporal: multi-service cluster, Go/Java SDK, battle-tested
distributed transactions, saga patterns.

Mantle: single binary, YAML definitions, IaC lifecycle,
first-class AI. Lower operational complexity, narrower scope.

Use Temporal for microservice orchestration.
Use Mantle for declarative AI workflow automation.

Tweet 3:
Mantle vs n8n:

n8n: visual canvas, 400+ integrations, great for non-technical
users, self-hostable.

Mantle: YAML in git, IaC lifecycle, 9 connectors + plugin system,
built for platform engineers.

Use n8n when your team wants drag-and-drop.
Use Mantle when your team wants code review.

Tweet 4:
Mantle vs LangChain:

LangChain: Python library, massive ecosystem, fine-grained
control over prompting and retrieval.

Mantle: execution platform, handles checkpointing/secrets/audit,
language-agnostic YAML, no Python required.

Use LangChain for custom AI applications.
Use Mantle for operational AI workflows.

Tweet 5:
Mantle vs Airflow/Prefect:

Airflow/Prefect: Python DAG engines, mature scheduling,
huge plugin ecosystem, built for data pipelines.

Mantle: YAML + CEL, single binary, AI-native, IaC lifecycle.
Not a data pipeline tool.

Use Airflow for ETL.
Use Mantle for AI workflow automation.

Tweet 6:
Summary table:

[screenshot of comparison table from docs/comparison.md]

Every tool has trade-offs. Mantle is early (v0.1.0) and has a
smaller ecosystem than all of these. We are honest about that.

Choose what fits your team and use case.
```

---

## 6. LinkedIn Strategy

### Company Page vs Personal Posting

**Personal account (founder) is the primary channel.** LinkedIn's algorithm heavily favors personal posts over company page posts. Engagement rates for personal posts are typically 5-10x higher.

**Allocation:**
- Founder personal: 70% of LinkedIn effort. All narrative content, opinions, technical insights, launch announcement.
- Company page: 30%. Mirror key announcements, post job openings (when relevant), share articles and tutorials.

**Founder profile optimization:**
- Headline: "Building Mantle -- headless AI workflow automation. Terraform for workflows, with native AI."
- About: Technical background, what you are building and why, link to GitHub
- Featured: Pin the launch post and a key technical article

### Content Pillars for Platform Engineers

**Pillar 1: IaC Philosophy (40% of posts)**
Why workflow definitions should be infrastructure code. Stories about what goes wrong when they are not. Comparisons to Terraform/Pulumi practices that the audience already uses.

Example: "Your Terraform modules go through code review. Your Kubernetes manifests go through code review. Why do your workflow definitions live in a database UI that no one audits?"

**Pillar 2: AI in Production (30% of posts)**
Practical, grounded takes on running AI workflows with proper operational practices. Anti-hype. Focus on the boring but important stuff: secrets management, checkpointing, audit trails, cost control.

Example: "Everyone is talking about AI agents. Nobody is talking about what happens when your agent crashes mid-conversation with 4 tool calls completed. That is the problem checkpoint-and-resume solves."

**Pillar 3: Builder's Log (20% of posts)**
Technical decisions, architecture trade-offs, lessons learned from building Mantle. The Go community on LinkedIn is engaged and will amplify good technical content.

Example: "We use SKIP LOCKED in Postgres for distributed step execution. No Redis, no RabbitMQ. Here is why that works and where it breaks down."

**Pillar 4: Industry Commentary (10% of posts)**
Thoughtful takes on workflow automation trends, AI tooling landscape, source-available licensing. Always tie back to a broader insight, not just Mantle.

### Leveraging the "Built by Engineers" Angle

The target audience respects technical depth and authenticity. Every post should demonstrate that the person writing it actually builds infrastructure.

**Tactics:**
- Share actual architecture decisions with trade-off analysis
- Post code snippets and terminal output, not diagrams made in Figma
- Acknowledge what Mantle does not do well yet
- Reference specific technologies the audience uses (Terraform, Kubernetes, Prometheus, Helm)
- Use precise technical language (not "scalable cloud-native solution" but "SKIP LOCKED work distribution on Postgres")

---

## 7. Community Seeding

### Subreddits

| Subreddit | Approach | When |
|-----------|----------|------|
| r/golang | Launch post (technical, architecture-focused). Ongoing participation in Go discussions. | Launch day + ongoing |
| r/selfhosted | Launch post (single binary, Postgres, self-hosted angle). | Launch day |
| r/devops | Launch post (IaC lifecycle angle). Participate in workflow tool discussions. | Launch day + ongoing |
| r/sre | Only if there is a relevant thread. Do not force it. | Opportunistic |
| r/LocalLLaMA | Post when there is a relevant thread about running AI workflows locally (BYOK angle). | Opportunistic |

**Rule:** Never post to more than 3 subreddits on the same day. Space them out by 1-2 hours. Each post should be written specifically for that subreddit's audience, not copy-pasted.

### Discord Servers

| Server | Approach |
|--------|----------|
| Gophers Discord | Share in #showcase channel on launch day. Participate in #general ongoing. |
| r/selfhosted Discord | Share in appropriate channel. |
| CNCF Slack | Only if there is a relevant channel for workflow tools. Do not spam. |
| AI Engineer Discord/Slack | Share when relevant to AI tooling discussions. |

**Rule:** Join these communities weeks before launch. Participate genuinely. Answer questions. Help people. Then when you share your project, you are a community member, not a drive-by promoter.

### Dev.to / Hashnode Article Strategy

**Dev.to is the primary platform** (larger developer audience, better SEO, cross-posting to HN is common).

**Article cadence:**

| Week | Article | SEO Target Keywords |
|------|---------|-------------------|
| Launch | "Introducing Mantle: Terraform for Workflow Automation, with Native AI" | workflow automation, AI workflow, IaC |
| +1 | "Build an AI Research Assistant in 10 Minutes with Mantle" | AI agent tutorial, LLM tool use |
| +2 | "Multi-Turn AI Tool Use with Crash Recovery" | AI function calling, tool use |
| +3 | "Why We Chose Go, Postgres, and YAML for a Workflow Engine" | Go workflow engine, Postgres |
| +4 | "Mantle vs Temporal: Choosing the Right Workflow Engine" | Temporal alternative, workflow comparison |
| +6 | "Deploy Mantle on Kubernetes with Helm" | Kubernetes workflow, Helm chart |
| +8 | "Building Custom Connectors for Mantle in Python" | workflow plugin, custom connector |

**Article format:**
- Every article starts with a complete, working code example
- Include terminal output showing the commands and results
- End with a "Next steps" section linking to docs and GitHub
- Cross-post to Hashnode 2-3 days after Dev.to publication
- Tag articles with: #workflow, #ai, #go, #devops

### How to Contribute Value Without Being Spammy

1. **Answer questions first.** When someone on r/devops asks "what workflow tool should I use?", give a thorough answer covering multiple options. Mention Mantle as one option with honest trade-offs. Do not just say "try Mantle."

2. **Share knowledge, not product.** Post about Go patterns, Postgres techniques, CEL usage, IaC practices. These establish credibility. The profile link does the product marketing.

3. **Be the most helpful person in the thread.** If someone has a Temporal question and you know the answer, answer it. Even if it means recommending a competitor. This builds trust faster than any product post.

4. **Disclose always.** "I am the creator of Mantle, so I am biased, but here is my honest take..."

5. **One product post per community per month maximum.** Everything else should be value-add participation.

---

## 8. Metrics and Goals

### Week 1 Targets

| Metric | Target | Stretch |
|--------|--------|---------|
| GitHub stars | 200 | 500 |
| Website unique visitors | 3,000 | 8,000 |
| Twitter/X followers | 150 | 300 |
| HN points | 50 | 150 |
| Dev.to article views | 1,000 | 3,000 |
| GitHub issues opened (external) | 5 | 15 |

### Month 1 Targets (end of Week +4)

| Metric | Target | Stretch |
|--------|--------|---------|
| GitHub stars | 500 | 1,500 |
| Website unique visitors (cumulative) | 8,000 | 20,000 |
| Twitter/X followers | 400 | 800 |
| LinkedIn followers (company page) | 100 | 250 |
| Discord/community members | 30 | 80 |
| Dev.to total article views | 5,000 | 15,000 |
| GitHub forks | 30 | 80 |
| External contributors (PRs) | 2 | 5 |
| `go install` downloads (if trackable) | 100 | 300 |

### Month 3 Targets (end of Week +12)

| Metric | Target | Stretch |
|--------|--------|---------|
| GitHub stars | 1,500 | 4,000 |
| Twitter/X followers | 800 | 2,000 |
| LinkedIn followers (company page) | 300 | 600 |
| Discord/community members | 100 | 300 |
| Monthly website visitors | 5,000 | 15,000 |
| External contributors | 10 | 25 |
| Dev.to total article views | 15,000 | 40,000 |
| Inbound "how do I use Mantle for X" questions | 20/month | 50/month |
| Conference/podcast invitations | 1 | 3 |

### What to Track and Optimize

**Weekly review metrics:**
- GitHub stars trend (daily)
- GitHub traffic (clones, unique visitors, referral sources -- available in repo Insights)
- Twitter impressions and engagement rate per post
- LinkedIn post engagement (reactions, comments, shares)
- Website traffic by source (direct, HN, Twitter, Reddit, Dev.to, Google)
- Dev.to article views and reactions

**Monthly review metrics:**
- GitHub community health (issues response time, PR merge time, contributor count)
- Content performance by type (which formats get the most engagement?)
- Referral source analysis (where are stars coming from?)
- Conversion funnel: website visit -> GitHub visit -> star/clone/install
- Keyword rankings for target terms (workflow automation, AI workflow, etc.)

**Optimization actions:**
- Double down on content formats that perform (if threads outperform single tweets, write more threads)
- If HN is the top referral source, prioritize content that gets HN traction (technical depth, honest trade-offs)
- If a specific use case generates questions, write a tutorial about it
- Track which comparison angle resonates most (vs Temporal? vs LangChain?) and produce more content on that axis
- A/B test tweet formats: code screenshot vs terminal recording vs text-only

---

## Appendix A: Asset Checklist

Prepare these before launch day:

- [ ] GitHub repo public with polished README, CHANGELOG, examples/, docs/
- [ ] Marketing website live at mantle.dvflw.co
- [ ] Twitter/X account created and profile optimized
- [ ] LinkedIn company page created
- [ ] Dev.to account created
- [ ] Product Hunt "coming soon" page (optional, for Week +4 launch)
- [ ] Terminal recording: validate/plan/apply/run cycle (30 sec)
- [ ] Terminal recording: AI tool-use execution with multi-turn output (30 sec)
- [ ] Comparison table image (for social sharing)
- [ ] Connector grid image (for social sharing)
- [ ] YAML code screenshots with syntax highlighting (3-4 examples)
- [ ] OG image for website (for link previews on social)
- [ ] Launch announcement blog post / Dev.to article drafted
- [ ] All launch day posts pre-written and queued
- [ ] 3-5 people briefed for launch day amplification
- [ ] HN post body drafted and reviewed
- [ ] Reddit posts drafted (one per subreddit, each tailored)

## Appendix B: Voice and Tone Quick Reference

| Do | Do Not |
|----|--------|
| "Source-available under BSL 1.1" | "Open source" |
| "v0.1.0 is early" | "Production-ready" (unless specifically about a feature) |
| "Mantle targets a different use case" | "Mantle is better than X" |
| "Single binary + Postgres" | "Cloud-native scalable platform" |
| "Here is the YAML" | "Revolutionary new approach" |
| "We chose Go because..." | "Built with cutting-edge technology" |
| Acknowledge limitations | Overpromise |
| Technical precision | Marketing buzzwords |
| Show terminal output | Show mockups or diagrams |
| "What would you do differently?" | "Please star the repo" |

## Appendix C: Hashtag and Tag Strategy

**Twitter/X hashtags (use 0-2 per tweet, never more):**
- #golang (when the content is about Go specifically)
- #devops (when the content is about operations/deployment)
- No branded hashtag yet -- too early, no one will search for it

**Dev.to tags:**
- #go, #ai, #devops, #workflow (max 4 per article)

**LinkedIn:**
- No hashtags on personal posts (they reduce reach on LinkedIn)
- Company page: #workflowautomation #platformengineering (sparingly)

**GitHub topics:**
- workflow-automation, ai, llm, infrastructure-as-code, golang, yaml, devops, platform-engineering, cli
