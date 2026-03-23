# Mantle v0.1.0 Growth Strategy

Solo developer playbook for the first 12 months. No budget, no team, high leverage.

---

## 1. Viral Loops and Network Effects

### Why Developer Tools Spread Organically

Developer tools spread through three mechanisms: visible adoption (badges, configs checked into repos), knowledge sharing (blog posts, conference talks, Stack Overflow answers), and workflow integration (once a tool is embedded in CI/CD, it creates lock-in and visibility for every team member). Mantle has a structural advantage here: workflow YAML files are checked into git repositories, making Mantle visible to every engineer who touches the repo.

### Shareable Workflow Definitions

Mantle's YAML format is its most underrated growth asset. Unlike Python-based tools where workflows are tangled with application code, a Mantle YAML file is self-contained, readable, and copy-pastable. This creates natural shareability.

**Actions:**

- **Example gallery on the website.** Ship 17 example workflows (already in `examples/`) as a browsable gallery at `mantle.dev/examples`. Each example gets its own page with the YAML, a plain-English description, and a one-line `mantle apply` command. This is the single highest-leverage content investment because each example page becomes an SEO landing page.
- **`mantle init --template` command.** Let users scaffold from templates: `mantle init --template=slack-summarizer`. Templates live in a public GitHub repo. Community members can submit templates via PR, creating a contribution flywheel.
- **Workflow-of-the-week series.** Publish one practical workflow every week for the first 12 weeks on the blog and cross-post to dev.to, Hashnode, and r/devops. Each post walks through a real problem, shows the YAML, and ends with `mantle apply`. Topics:
  1. Summarize Slack channels daily with GPT-4o
  2. Monitor uptime and page on-call via webhook
  3. Auto-triage GitHub issues with AI classification
  4. Weekly cost report from AWS billing API
  5. Rotate API keys and notify the team
  6. Pull request summary bot
  7. Database backup verification workflow
  8. Incident postmortem generator
  9. Dependency vulnerability scanner with AI analysis
  10. On-call handoff report automation
  11. SLA breach detection and escalation
  12. AI-powered changelog generator from git history

### GitHub Stars Strategy

Stars are social proof. They are the single most important vanity metric for developer tool discovery because GitHub's trending algorithm, awesome-lists, and developer newsletters all use star count as a filter.

**Actions:**

- **Star the repo from personal/work accounts on launch day.** Get to 10-20 stars before any public announcement so the repo does not look abandoned.
- **Hacker News launch post.** Write a "Show HN" post titled "Mantle: Define AI workflows as YAML, deploy like Terraform" with a concise description and link to the repo. Post between 8-10am ET on a Tuesday or Wednesday. Respond to every comment within 2 hours. A successful HN post can yield 200-500 stars in 48 hours.
- **r/golang, r/devops, r/selfhosted posts.** These subreddits are receptive to source-available tools. Frame posts as "I built this to solve my own problem" rather than marketing launches.
- **awesome-go PR.** Submit to the awesome-go list under "Workflow" or "Automation." Requires 100+ stars, so time this after the HN launch.
- **Dev.to and Hashnode launch articles.** Cross-post the origin story and technical deep-dive.
- **README quality.** The README is the landing page for GitHub visitors. It already has strong structure. Add an animated GIF or asciicast showing `mantle apply` and `mantle run` in action -- this alone increases star conversion by 2-3x based on observed patterns in trending repos.

### Network Effects

Mantle does not have strong direct network effects (one person's use does not inherently make it more valuable for another). However, there are indirect network effects worth cultivating:

- **Plugin ecosystem.** Every community-contributed plugin increases the platform's value for all users. Prioritize making the plugin protocol dead simple and well-documented. The easier it is to write a plugin, the faster the ecosystem grows.
- **Shared workflow library.** The `mantle library` command enables a publish/deploy model. If this becomes a community registry of reusable workflows, it creates a reason to stay on Mantle rather than switch to alternatives.
- **Team adoption within organizations.** Once one platform engineer adopts Mantle and checks workflow YAMLs into the team's infrastructure repo, every team member who touches that repo encounters Mantle. This is the most important organic growth vector.

---

## 2. SEO and Content Marketing

### High-Value Keyword Targets

Platform engineers and SREs search for solutions to specific problems, not product categories. The keyword strategy should target problem-aware searches, not product-aware ones.

**Tier 1 -- High intent, moderate volume:**
- "yaml workflow automation" (low competition, directly describes Mantle)
- "ai workflow orchestration" (growing search volume with AI adoption)
- "terraform for workflows" (exact positioning, long-tail)
- "llm workflow engine" (emerging category)
- "self-hosted workflow automation" (strong in r/selfhosted audience)

**Tier 2 -- Comparison and alternative searches:**
- "temporal alternative simpler" / "temporal too complex"
- "n8n alternative yaml" / "n8n git workflow"
- "langchain workflow orchestration" / "langchain production deployment"
- "airflow alternative lightweight"
- "prefect vs airflow vs" (insert Mantle into comparison searches)

**Tier 3 -- Problem-aware, long-tail:**
- "automate slack notifications yaml"
- "ai workflow checkpoint recovery"
- "llm tool use crash recovery"
- "orchestrate api calls with yaml"
- "openai workflow automation self-hosted"
- "bring your own api keys workflow"
- "schedule ai tasks cron yaml"
- "structured output openai workflow"

### Content Pieces That Rank

Each of these doubles as a trust-building technical resource and an SEO landing page.

**Comparison pages (highest leverage for SEO):**
1. `/compare/temporal` -- "Mantle vs Temporal: When you need workflows without cluster topology"
2. `/compare/n8n` -- "Mantle vs n8n: YAML-native vs visual workflow automation"
3. `/compare/langchain` -- "Mantle vs LangChain: Platform vs library for AI workflows"
4. `/compare/airflow` -- "Mantle vs Airflow: AI-first vs data-first workflow orchestration"
5. `/compare/prefect` -- "Mantle vs Prefect: YAML declarations vs Python decorators"

These pages already have strong source material in `docs/comparison.md`. Each comparison page should be 1500-2000 words, honest about tradeoffs (the existing comparison doc sets the right tone), and include a summary table.

**Tutorial and how-to content:**
1. "How to build an AI-powered Slack summarizer in 5 minutes"
2. "Automate incident response with YAML workflows"
3. "Self-hosted AI workflow automation with Docker Compose"
4. "How to version-control your automation workflows like infrastructure"
5. "Building a research agent with tool use and crash recovery"
6. "Replace your Python automation scripts with declarative YAML"

**Technical blog posts (build trust with the target audience):**
1. "Why we chose CEL over Jinja2 for workflow expressions" (technical depth signals credibility)
2. "Checkpoint-and-resume: how Mantle handles crashes mid-AI-conversation" (unique differentiator)
3. "Single binary, zero dependencies: the case against microservice workflow engines"
4. "BYOK architecture: why your AI keys should never leave your infrastructure"
5. "How Mantle's DAG scheduler detects parallelism from CEL expressions"
6. "Building a plugin system with JSON over stdin/stdout"

### Content Calendar (First 90 Days)

| Week | Content | Channel |
|------|---------|---------|
| 1 | Launch post: "I built Terraform for AI workflows" | HN, Reddit, dev.to |
| 2 | Comparison: Mantle vs Temporal | Blog, cross-post |
| 3 | Tutorial: AI Slack summarizer | Blog, dev.to |
| 4 | Technical: Why CEL over Jinja2 | Blog, Hashnode |
| 5 | Comparison: Mantle vs n8n | Blog |
| 6 | Workflow of the week: GitHub issue triage | Blog, dev.to |
| 7 | Technical: Checkpoint-and-resume deep dive | Blog, HN |
| 8 | Comparison: Mantle vs LangChain | Blog |
| 9 | Tutorial: Self-hosted AI workflows with Docker | Blog, r/selfhosted |
| 10 | Technical: Single binary architecture | Blog |
| 11 | Comparison: Mantle vs Airflow | Blog |
| 12 | Tutorial: Building a plugin in 50 lines of Python | Blog, dev.to |

---

## 3. Developer Tool Distribution Channels

### Package Manager Distribution

Every additional installation method removes friction. Prioritize by audience overlap.

| Channel | Priority | Effort | Rationale |
|---------|----------|--------|-----------|
| `go install` | P0 (done) | None | Core audience uses Go |
| Docker Hub / GHCR | P0 (done) | None | Multi-arch images already built |
| Helm chart | P0 (done) | None | K8s is the deployment target |
| Binary releases (GitHub Releases) | P0 | Low | goreleaser config, covers Linux/macOS/Windows |
| Homebrew tap | P1 | Low | macOS developers, easy to maintain a tap |
| npm wrapper | P2 | Low | Enables `npx mantle` for polyglot teams |
| Nix package | P3 | Medium | Nix users are vocal advocates, good word-of-mouth |
| AUR (Arch Linux) | P3 | Low | Small but passionate community |

### Marketplace and Directory Listings

These are one-time submissions that generate ongoing discovery traffic.

| Listing | When | Prerequisite |
|---------|------|-------------|
| Artifact Hub (Helm chart) | Week 1 | Helm chart published |
| awesome-go | After 100 stars | GitHub stars threshold |
| awesome-selfhosted | After 50 stars | Working Docker setup |
| awesome-sysadmin | After 50 stars | Relevant category |
| GitHub topic tags | Week 1 | Add topics: workflow, automation, ai, yaml, devops |
| AlternativeTo.net | Week 1 | List as alternative to Temporal, n8n, Airflow |
| Product Hunt | Month 2-3 | After initial traction, save for a second wave |

### Integration Directories

These are longer-term plays that require building specific integrations.

| Directory | Effort | Impact |
|-----------|--------|--------|
| GitHub Actions (mantle-action) | Medium | `mantle validate` in CI pipelines -- every user's CI logs mention Mantle |
| Slack App Directory | High | Requires OAuth flow, not worth it until user demand is clear |
| Terraform Registry (provider) | High | "Manage Mantle workflows from Terraform" -- strong narrative fit but significant engineering |

**Recommended priority:** Build the GitHub Action first. It is the only integration that creates organic visibility (Mantle appears in CI logs and workflow files of adopters' repos, which other engineers see).

---

## 4. Conversion Funnel

### Funnel Map

```
Discovery --> Evaluation --> First Use --> Adoption --> Advocacy
```

### Stage Details

**Stage 1: Discovery**
- *How they find us:* HN post, Reddit, search engine, awesome-list, colleague recommendation, seeing a `mantle.yaml` in a repo
- *Conversion rate assumption:* 5-10% of people who see a mention will visit the website or GitHub repo
- *Key metric:* Website unique visitors, GitHub repo views
- *Biggest drop-off risk:* The product description does not resonate. "Why do I need this?" is unanswered.
- *Optimization:* The README and website hero section must answer "what is this" and "why should I care" in under 10 seconds. The YAML code example in the hero does this well. Add a 30-second asciicast to the README.

**Stage 2: Evaluation**
- *What they do:* Read the README, scan the comparison page, look at star count, check last commit date, skim issues/PRs
- *Conversion rate assumption:* 15-25% of visitors will proceed to try installation
- *Key metric:* Time on site, comparison page views, GitHub clone count
- *Biggest drop-off risk:* Low star count signals "not ready." Stale commits signal "abandoned." No documentation signals "not serious."
- *Optimization:* Keep the repo active with regular commits. Maintain comprehensive docs. The 17 example workflows are a strong evaluation aid. Add a "quick start" section that takes under 2 minutes to reach a working state.

**Stage 3: First Use**
- *What they do:* Install, run `mantle init`, apply an example workflow, run it
- *Conversion rate assumption:* 40-60% of people who install will complete the quick start
- *Key metric:* Docker pulls, `go install` downloads, first `mantle run` (if telemetry exists)
- *Biggest drop-off risk:* Installation fails. Postgres setup is confusing. The example workflow does not work. Error messages are unhelpful.
- *Optimization:* The docker-compose setup must work flawlessly on macOS and Linux. Error messages must be specific and actionable. Add a `mantle doctor` command that checks prerequisites. Include a "hello world" workflow that requires zero external dependencies (no API keys needed).

**Stage 4: Adoption**
- *What they do:* Write their own workflow, integrate into their infrastructure, share with team
- *Conversion rate assumption:* 20-30% of first-use users will write their own workflow
- *Key metric:* Number of custom workflows applied, repeat `mantle run` usage, server mode adoption
- *Biggest drop-off risk:* The user hits a limitation or bug. Documentation does not cover their use case. No way to get help.
- *Optimization:* Comprehensive reference docs for every connector, every CLI flag, every config option. A Discord or GitHub Discussions community where questions get answered within 24 hours.

**Stage 5: Advocacy**
- *What they do:* Star the repo, write a blog post, recommend to colleagues, submit a plugin or example
- *Conversion rate assumption:* 10-15% of adopters will become advocates
- *Key metric:* GitHub stars from non-launch sources, inbound "how did you hear about us" mentions, community contributions
- *Biggest drop-off risk:* No channel for advocacy. No recognition for contributions.
- *Optimization:* Thank every contributor publicly. Feature community workflows in the gallery. Add a "Contributors" section to the README.

### Funnel Math (Realistic Month 3 Projection)

```
Website visitors:     3,000/month
  --> Install:          450 (15%)
  --> Complete quickstart: 225 (50% of installs)
  --> Write own workflow:   56 (25% of quickstarts)
  --> Advocate:              8 (15% of adopters)
```

These 8 advocates per month are the growth engine. Each one generates 2-5 new discovery events through blog posts, recommendations, and visible adoption, creating a compounding loop.

---

## 5. Referral and Community Strategies

### Building Community as a Solo Developer

The constraint is response time. A solo developer cannot monitor Discord 16 hours a day. Choose community channels that tolerate asynchronous interaction.

**Recommended stack:**
1. **GitHub Discussions** (primary). Lowest friction for developers. Searchable. Indexed by Google. Does not require maintaining a separate platform. Categories: Q&A, Show & Tell, Ideas, Announcements.
2. **Discord** (secondary). Create a server but set expectations: "I check Discord a few times per day. For urgent issues, open a GitHub issue." Keep channels minimal: general, help, showcase, announcements.
3. **Monthly "office hours" livestream.** 30-minute screen-share where you build a workflow live, answer questions, and discuss the roadmap. Record and post to YouTube. This builds personal connection without requiring daily community management.

### Ambassador / Champion Program

Too early for a formal program at v0.1.0. Instead, identify and cultivate organic champions:

- **Watch for repeat contributors.** Anyone who opens 3+ issues or submits a PR is a potential champion. Reach out personally via email or DM. Ask what they are building and what they need.
- **Feature community work.** If someone writes a blog post about Mantle or builds a plugin, amplify it on every channel. Retweet, share in Discord, add to a "Community" section on the website.
- **Early access to features.** Give active community members early access to pre-release builds. Their feedback is valuable and the exclusivity creates loyalty.
- **At 500+ stars, formalize.** Create a "Mantle Champions" program with a private Discord channel, direct access to the maintainer, and a "Champion" badge on their GitHub Discussions profile.

### "Built with Mantle" Badge

Create a simple badge that users can add to their README files:

```markdown
[![Built with Mantle](https://img.shields.io/badge/built%20with-Mantle-00ff88)](https://mantle.dev)
```

This serves two purposes: it signals adoption (social proof for the project) and it creates discovery events (anyone reading that README sees Mantle). Mention the badge in the "Getting Started" docs and in the post-quickstart success message.

### Example Workflow Gallery

Host at `mantle.dev/examples` with:
- All 17 built-in examples, categorized (AI, HTTP, Slack, Data, DevOps)
- A "Community" section for user-submitted workflows
- Each workflow page has: description, full YAML, required connectors/credentials, one-line apply command
- Submission process: open a PR to the `examples/community/` directory with a YAML file and a frontmatter description

The gallery is both a discovery tool (SEO for long-tail searches) and a retention tool (users find solutions to new problems without leaving the ecosystem).

---

## 6. Competitive Wedge Campaigns

### Campaign 1: Temporal Refugees

**Target audience:** Engineers who have evaluated or used Temporal and found the operational complexity disproportionate to their needs.

**Wedge message:** "Temporal is built for distributed transactions across microservices. If your workflows are AI tasks, API calls, and notifications, you do not need a multi-service cluster."

**Content:**
- Blog post: "You probably do not need Temporal" -- an honest assessment of when Temporal is overkill
- Comparison page with specific operational differences: "Temporal requires 4 services + Cassandra/MySQL. Mantle requires 1 binary + Postgres."
- Show the same workflow implemented in Temporal Go SDK (50+ lines of Go, worker setup, activity definitions) vs Mantle YAML (20 lines)
- Target keywords: "temporal alternative," "temporal simpler alternative," "temporal too complex"

**Distribution:** r/golang, HN, dev.to. Temporal's community forum (be respectful -- frame as "different tool for different use cases," not "Temporal is bad").

### Campaign 2: LangChain Production Gap

**Target audience:** Teams that prototyped with LangChain but struggle to productionize AI workflows.

**Wedge message:** "LangChain is a great prototyping library. But when your AI workflow crashes at 3am, who restarts it? Mantle checkpoints every step to Postgres. Crash recovery is built in."

**Content:**
- Blog post: "From LangChain prototype to production AI workflow"
- Tutorial: "Migrate a LangChain chain to a Mantle workflow" with side-by-side code
- Focus on: crash recovery, secrets management, audit trails, deployment simplicity
- Target keywords: "langchain production deployment," "langchain crash recovery," "langchain workflow orchestration"

**Distribution:** r/MachineLearning, r/LangChain, Python community forums. These audiences are Python-centric, so emphasize that Mantle workflows are language-agnostic YAML -- no Python required.

### Campaign 3: n8n Git Workflow Gap

**Target audience:** DevOps and platform engineers who tried n8n but found the GUI-first approach incompatible with their git-based workflow.

**Wedge message:** "n8n stores workflows in a database. Mantle stores them in your git repo. Same code review, same CI/CD, same IaC practices you already use."

**Content:**
- Blog post: "Why your workflow definitions belong in git"
- Side-by-side: n8n workflow JSON export (opaque, auto-generated) vs Mantle YAML (human-authored, readable)
- Focus on: version control, code review, CI validation, rollback via git revert
- Target keywords: "n8n git workflow," "n8n version control," "n8n alternative for devops"

**Distribution:** r/selfhosted, r/devops, n8n community forum (again, respectful framing).

### Campaign 4: Airflow/Prefect for Non-Python Teams

**Target audience:** Teams that need workflow orchestration but are not Python shops.

**Wedge message:** "Airflow and Prefect are Python platforms. If your team writes Go, Rust, or TypeScript, you do not need to adopt Python just to orchestrate workflows."

**Content:**
- Blog post: "Workflow orchestration without Python"
- Emphasize: YAML + CEL (language-agnostic), Go binary (no Python runtime), plugin system (any language)
- Target keywords: "airflow alternative non-python," "workflow orchestration without python," "yaml workflow engine"

**Distribution:** r/golang, r/rust, r/devops.

---

## 7. Growth Experiments Roadmap (First 90 Days)

Sorted by expected impact relative to effort. Run 2-3 experiments concurrently.

### Experiment 1: Hacker News Launch Post

- **Hypothesis:** A well-crafted Show HN post will generate 200+ stars and 1000+ website visitors in 48 hours.
- **Metric:** GitHub stars, website traffic, Docker pulls in the 72 hours post-launch.
- **Effort:** Low (2-3 hours writing the post, 4-6 hours responding to comments).
- **Expected impact:** High. HN is the single highest-ROI channel for developer tool launches.
- **How to measure:** GitHub star graph, Cloudflare analytics, Docker Hub pull count.
- **Kill criteria:** If the post does not reach the front page within 2 hours, it is dead. Repost with a different title on a different day (HN allows resubmission).
- **Scale criteria:** If it hits front page, immediately engage with every comment. Write a follow-up technical post for the next week.

### Experiment 2: Comparison Pages SEO

- **Hypothesis:** Five comparison pages targeting "[competitor] alternative" keywords will generate 500+ organic monthly visitors within 60 days.
- **Metric:** Organic search impressions, clicks, and rankings for target keywords (Google Search Console).
- **Effort:** Medium (8-10 hours to write five 1500-word comparison pages).
- **Expected impact:** High. Comparison searches have high intent -- these visitors are actively evaluating tools.
- **How to measure:** Google Search Console, Cloudflare analytics filtered by /compare/ pages.
- **Kill criteria:** If no impressions after 60 days, the domain authority is too low and content alone will not rank. Pivot to guest posting on higher-DA sites.
- **Scale criteria:** If any page ranks on page 1, create more comparison and "vs" content.

### Experiment 3: Example Workflow Gallery

- **Hypothesis:** A browsable gallery of 17+ workflow examples will increase time-on-site by 40% and quickstart completion rate by 20%.
- **Metric:** Pages per session, average session duration, quickstart completion (proxied by Docker pull-to-star ratio).
- **Effort:** Medium (6-8 hours to build gallery pages, content already exists in `examples/`).
- **Expected impact:** Medium-high. Examples are the best sales pitch for a declarative tool.
- **How to measure:** Cloudflare analytics, page-level metrics.
- **Kill criteria:** N/A -- this is a permanent asset. Build it regardless.
- **Scale criteria:** If gallery pages rank for long-tail keywords, invest in more examples and community submissions.

### Experiment 4: GitHub Action for CI Validation

- **Hypothesis:** A `mantle-action` GitHub Action will create organic visibility in adopters' CI pipelines and generate 50+ new discovery events per month.
- **Metric:** GitHub Action installs, referral traffic from GitHub to mantle.dev.
- **Effort:** Medium (4-6 hours to build and publish the action).
- **Expected impact:** Medium. Slow burn but compounds over time.
- **How to measure:** GitHub Marketplace install count, referral analytics.
- **Kill criteria:** If fewer than 10 installs after 30 days, the user base is too small for this to matter yet. Revisit at 500+ stars.
- **Scale criteria:** If installs grow steadily, add features (plan output as PR comment, validate on PR).

### Experiment 5: Reddit Community Engagement

- **Hypothesis:** Weekly participation in r/devops, r/golang, r/selfhosted (helpful answers, not self-promotion) will generate 20+ stars per month from Reddit traffic.
- **Metric:** Referral traffic from Reddit, stars correlated with Reddit post timing.
- **Effort:** Low-medium (3-4 hours per week answering questions and sharing relevant content).
- **Expected impact:** Medium. Builds reputation and creates multiple touchpoints.
- **How to measure:** Reddit post analytics, Cloudflare referral data.
- **Kill criteria:** If no measurable traffic after 30 days of consistent engagement, the subreddits are too noisy. Focus on HN and dev.to instead.
- **Scale criteria:** If specific subreddits consistently drive traffic, increase posting frequency there.

### Experiment 6: Dev.to / Hashnode Cross-Posting

- **Hypothesis:** Cross-posting technical blog content to dev.to and Hashnode will generate 2000+ additional impressions per post with near-zero marginal effort.
- **Metric:** Dev.to views, reactions, and referral traffic to mantle.dev.
- **Effort:** Low (30 minutes per post to cross-post with canonical URL).
- **Expected impact:** Low-medium per post, but compounds over 12 weeks of content.
- **How to measure:** Dev.to analytics, Hashnode analytics, Cloudflare referrals.
- **Kill criteria:** If average views per post are under 200 after 4 posts, the content is not resonating with the platform's audience. Adjust topics.
- **Scale criteria:** If any post exceeds 1000 views, double down on that topic/format.

### Experiment 7: README Asciicast / Demo GIF

- **Hypothesis:** Adding an animated terminal demo to the README will increase the GitHub visitor-to-star conversion rate by 30%.
- **Metric:** Star conversion rate (stars / unique visitors, available in GitHub traffic insights).
- **Effort:** Low (2-3 hours to record and edit).
- **Expected impact:** Medium. Every single GitHub visitor sees the README.
- **How to measure:** Compare star conversion rate week-over-week before and after adding the demo.
- **Kill criteria:** N/A -- this is a one-time permanent improvement.
- **Scale criteria:** If conversion improves, add demo GIFs to comparison pages and blog posts.

### Experiment 8: awesome-selfhosted Submission

- **Hypothesis:** Listing on awesome-selfhosted will generate 100+ stars and sustained discovery traffic (50+ visitors/month).
- **Metric:** Referral traffic from GitHub awesome-selfhosted, star spikes correlated with listing.
- **Effort:** Very low (1 hour to submit PR).
- **Expected impact:** Medium. awesome-selfhosted is one of the most-visited awesome lists.
- **How to measure:** GitHub referral traffic, star timing.
- **Kill criteria:** N/A -- submit and forget.
- **Scale criteria:** If accepted, submit to every relevant awesome-list (awesome-go, awesome-devops, awesome-ai).

### Experiment 9: Monthly Office Hours Livestream

- **Hypothesis:** A monthly 30-minute livestream will convert 5-10 lurkers into active community members per session.
- **Metric:** Livestream attendees, Discord/Discussions activity in the 48 hours post-stream.
- **Effort:** Low (30 minutes live + 30 minutes prep).
- **Expected impact:** Low in absolute numbers but high in community quality. The people who attend a livestream are high-intent.
- **How to measure:** YouTube views, Discord member count, Discussions post count.
- **Kill criteria:** If fewer than 5 attendees after 3 sessions, pause and wait for a larger community.
- **Scale criteria:** If attendance grows, increase to biweekly.

### Experiment 10: Product Hunt Launch

- **Hypothesis:** A Product Hunt launch will generate 300+ website visitors and 50+ stars in 24 hours, reaching an audience outside the typical HN/Reddit developer sphere.
- **Metric:** PH upvotes, referral traffic, stars.
- **Effort:** Medium (3-4 hours to prepare assets, 8 hours to engage on launch day).
- **Expected impact:** Medium. PH audience skews more product/startup than infrastructure engineering, so conversion quality may be lower.
- **How to measure:** PH analytics, Cloudflare referrals, star timing.
- **Kill criteria:** If fewer than 50 upvotes, the PH audience is not the right fit. Do not repeat.
- **Scale criteria:** If top-5 in the day's rankings, the product has broader appeal than expected. Consider expanding the positioning beyond platform engineers.

### Experiment Sequencing

| Week | Experiments Running |
|------|-------------------|
| 1-2 | #1 (HN launch), #7 (README demo), #8 (awesome-list submission) |
| 3-4 | #2 (comparison pages), #5 (Reddit engagement begins), #6 (cross-posting begins) |
| 5-6 | #3 (example gallery), #4 (GitHub Action) |
| 7-8 | #9 (first livestream), #5 and #6 continue |
| 9-10 | #10 (Product Hunt), #2 continues (more comparison pages) |
| 11-12 | Analyze all results, double down on winners, kill losers |

---

## 8. Realistic Projections

### Month 1 (Launch)

| Metric | Target | Notes |
|--------|--------|-------|
| GitHub stars | 150-400 | Dependent on HN post success. 400 if front page, 150 if not. |
| Docker pulls | 200-500 | Early adopters testing the product. |
| Website visitors | 2,000-5,000 | Spike from launch, then drop to baseline. |
| Weekly active users | 10-20 | Measured by unique IPs in Docker pull logs or opt-in telemetry. |
| Blog posts published | 3-4 | Launch post + 2-3 technical/comparison posts. |
| Community members | 20-40 | GitHub Discussions + Discord combined. |

**What success looks like:** The HN post reaches the front page. Star count crosses 200. At least 5 people you have never met open a GitHub issue or ask a question in Discussions. One person writes about Mantle without being asked.

### Month 3

| Metric | Target | Notes |
|--------|--------|-------|
| GitHub stars | 400-800 | Sustained growth from SEO and community. |
| Docker pulls | 1,000-2,000 | Includes CI pipelines pulling regularly. |
| Website visitors | 3,000-5,000/month | Baseline from SEO, steady from content. |
| Weekly active users | 30-60 | Some teams adopting for real workflows. |
| Blog posts published | 10-12 total | Workflow-of-the-week + technical content. |
| Community members | 60-100 | |
| Comparison page organic traffic | 200-500/month | SEO takes 60-90 days to kick in. |

**What success looks like:** You have a small but real community. People answer each other's questions in GitHub Discussions. At least one company is using Mantle in a non-trivial way. Organic search traffic is growing week over week. You are getting inbound "how do I do X with Mantle" questions.

### Month 6

| Metric | Target | Notes |
|--------|--------|-------|
| GitHub stars | 800-1,500 | Approaching the threshold where the project looks "real." |
| Docker pulls | 5,000-10,000 | Cumulative. Steady weekly pulls. |
| Website visitors | 5,000-10,000/month | SEO becoming a meaningful channel. |
| Weekly active users | 80-150 | |
| Community members | 150-300 | |
| Community-contributed plugins | 3-5 | |
| Community-contributed examples | 5-10 | |

**What success looks like:** Mantle appears in "awesome" lists and third-party blog posts you did not write. The example gallery has community submissions. At least one integration or plugin was built by someone outside the project. You are spending more time reviewing community PRs than writing marketing content. SEO drives more traffic than any single launch event.

### Month 12

| Metric | Target | Notes |
|--------|--------|-------|
| GitHub stars | 2,000-4,000 | "Credible project" territory. |
| Docker pulls | 20,000-50,000 | Cumulative. |
| Website visitors | 10,000-20,000/month | |
| Weekly active users | 200-500 | |
| Community members | 500-1,000 | |

**What success looks like:** Mantle is the default answer when someone asks "how do I orchestrate AI workflows with YAML" on Reddit or Stack Overflow. You are getting inbound interest from companies asking about enterprise features. The community is self-sustaining -- questions get answered by other users, not just the maintainer.

### When to Consider Funding or Hiring

**Do not raise funding or hire until all of these are true:**

1. **Product-market fit signal:** At least 5 companies are using Mantle in production, and at least 2 of them reached out to you (not the other way around).
2. **Organic growth:** Monthly star growth is 200+ without active marketing pushes.
3. **Community health:** The ratio of community-answered questions to maintainer-answered questions is at least 1:3.
4. **Revenue signal:** At least 2 companies have asked about paid features, support contracts, or a hosted version.
5. **Personal capacity:** You are turning down feature requests and bug fixes because you do not have time, not because there are not enough of them.

If all five are true, you have something worth investing in. At that point, the first hire should be a developer advocate / community manager, not another engineer. The bottleneck for a solo developer with traction is community and content, not code.

If fewer than three are true after 12 months, the product may need repositioning rather than investment. Revisit the target audience and core value proposition before spending money.

---

## Appendix: Solo Developer Time Allocation

Time is the only resource. Budget it deliberately.

| Activity | Hours/Week | Notes |
|----------|-----------|-------|
| Product development | 20-25 | Core priority. Nothing else matters if the product does not work. |
| Content creation | 5-8 | One blog post per week, including cross-posting. |
| Community management | 3-5 | GitHub Discussions, Discord, issue triage. |
| Distribution/marketing | 2-3 | Reddit engagement, submissions, outreach. |
| Analytics/measurement | 1 | Weekly review of metrics, experiment evaluation. |

**Total: 31-42 hours/week.** This is unsustainable long-term. As the community grows, content and community will need to be delegated first -- either through community contributions (others write blog posts, answer questions) or through hiring.

The most important thing a solo developer can do for growth is ship a product that works reliably. Every hour spent on marketing for a product that crashes on install is wasted. Every hour spent fixing bugs for a product nobody knows about is also wasted. The balance shifts over time: month 1 is 80% product / 20% marketing. By month 6, it should be 60% product / 40% marketing and community.
