# Authentication & RBAC Guide

Mantle supports two authentication methods: API keys and OIDC/SSO (JWT). Both resolve to the same User model with team scoping. This guide covers setting up authentication, configuring roles, and managing teams.

For CLI flag details, see the [CLI Reference](cli-reference.md). For the server endpoints that require authentication, see the [Server Guide](server-guide.md).

## Authentication Methods at a Glance

| Method | Format | Best For |
|---|---|---|
| API Key | `mk_` + 64 hex chars | CI pipelines, scripts, programmatic access |
| OIDC/SSO | JWT from your identity provider | Interactive users, browser-based access, SSO enforcement |

Both methods produce the same result: a resolved user with a team, a role, and an audit trail. Every state-changing API request is logged with the authenticated user's identity.

## API Key Authentication

API keys are the simplest way to authenticate with Mantle. Each key is tied to a specific user and inherits that user's role and team membership.

### How Keys Work

When you create an API key, Mantle generates a random key with the `mk_` prefix and displays it once. The raw key is never stored -- Mantle keeps only a SHA-256 hash in the database. On each request, the server hashes the provided key and looks up the matching record.

### Creating an API Key

Create a key for an existing user:

```bash
mantle users api-key --email alice@example.com --key-name prod-key
```

```
API Key: mk_a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6e7f8a9b0c1d2e3f4a5b6c7d8e9f0a1b2

Save this key — it cannot be retrieved again.
Key prefix for reference: mk_a1b2c3
```

The `--key-name` flag is a human-readable label for identifying the key later (e.g., in audit logs or when revoking).

### Using an API Key

Pass the key in one of three ways, listed by precedence:

**HTTP header (recommended for API calls):**

```bash
curl -H "Authorization: Bearer mk_a1b2c3..." http://localhost:8080/api/v1/executions
```

**CLI flag:**

```bash
mantle run my-workflow --api-key mk_a1b2c3...
```

**Environment variable:**

```bash
export MANTLE_API_KEY="mk_a1b2c3..."
mantle run my-workflow
```

The CLI flag overrides the environment variable. The HTTP header is the only option for REST API calls.

### Key Lifecycle

**Expiration.** Keys have an optional `expires_at` timestamp. Create a key with an expiration:

```bash
mantle users api-key --email alice@example.com --key-name temp-key --expires 90d
```

Expired keys are rejected at authentication time. The key remains in the database for audit purposes.

**Last-used tracking.** Mantle updates the `last_used_at` timestamp on every successful authentication. Use this to identify stale keys:

```bash
mantle users list-keys --email alice@example.com
```

```
NAME       PREFIX     CREATED              LAST USED            EXPIRES
prod-key   mk_a1b2c3  2026-03-18 14:30:00  2026-03-22 09:15:00  never
temp-key   mk_d4e5f6  2026-03-20 10:00:00  never                2026-06-18 10:00:00
```

**Revocation.** Revoke a key immediately:

```bash
mantle auth revoke-key --email alice@example.com --key-name prod-key
```

```
Revoked key "prod-key" for alice@example.com
```

Revoked keys are rejected on the next request. There is no grace period.

### Key Format

All Mantle API keys follow the format `mk_` followed by 64 hexadecimal characters. The `mk_` prefix makes keys easy to identify in logs and secret scanners.

## OIDC/SSO Authentication

Mantle supports OpenID Connect for single sign-on with identity providers like Okta, Auth0, Azure AD, and Google Workspace.

### Configuration

Configure OIDC in `mantle.yaml`:

```yaml
auth:
  oidc:
    issuer_url: "https://company.okta.com/oauth2/default"
    client_id: "0oa1234..."
    client_secret: ""  # optional for public clients
    audience: "mantle"
    allowed_domains: ["company.com"]
```

Or use environment variables:

```bash
export MANTLE_AUTH_OIDC_ISSUER_URL="https://company.okta.com/oauth2/default"
export MANTLE_AUTH_OIDC_CLIENT_ID="0oa1234..."
export MANTLE_AUTH_OIDC_CLIENT_SECRET=""
export MANTLE_AUTH_OIDC_AUDIENCE="mantle"
export MANTLE_AUTH_OIDC_ALLOWED_DOMAINS="company.com"
```

For multiple allowed domains, separate them with commas in the environment variable: `MANTLE_AUTH_OIDC_ALLOWED_DOMAINS="company.com,subsidiary.com"`.

### How It Works

1. The user authenticates with your identity provider and obtains a JWT
2. The JWT is sent to Mantle in the `Authorization: Bearer <jwt>` header
3. Mantle validates the JWT signature against the provider's JWKS endpoint (fetched from `issuer_url/.well-known/openid-configuration`)
4. Mantle verifies the `aud` (audience), `iss` (issuer), and `exp` (expiration) claims
5. The `email` claim is extracted and matched against a pre-provisioned user in the database
6. The `email_verified` claim must be `true` -- unverified emails are rejected
7. The email domain is checked against `allowed_domains` if configured

### User Pre-Provisioning

Users must be created in Mantle before they can authenticate via OIDC. There is no just-in-time (JIT) provisioning -- this is intentional to prevent unauthorized access from anyone with a valid identity provider account.

Create users before they log in:

```bash
mantle users create --email alice@company.com --name "Alice Chen" --team engineering --role operator
```

If a valid JWT arrives for an email address that does not exist in Mantle, the request is rejected with a 403 Forbidden response.

### CLI Login Flow

For interactive CLI use, Mantle supports two OIDC login flows:

**Browser-based (authorization code with PKCE):**

```bash
mantle login
```

This opens your default browser to the identity provider's login page. After authentication, the browser redirects to a local callback server and Mantle stores the token.

**Device flow (headless environments):**

```bash
mantle login --device
```

This prints a URL and a code. Open the URL on any device, enter the code, and authenticate. Useful for SSH sessions and CI environments.

**Logout:**

```bash
mantle logout
```

This removes the cached credentials.

### Credential Caching

After a successful `mantle login`, the JWT and refresh token are stored at `~/.mantle/credentials`. The CLI automatically refreshes expired tokens using the refresh token. If the refresh token is also expired, you are prompted to log in again.

The credentials file is created with `0600` permissions (owner read/write only).

## RBAC

Mantle uses role-based access control with three roles. Roles are hierarchical -- each role includes all permissions of the roles below it.

### Roles and Permissions

| Permission | `operator` | `team_owner` | `admin` |
|---|---|---|---|
| Run workflows | Yes | Yes | Yes |
| View executions and logs | Yes | Yes | Yes |
| Cancel executions | Yes | Yes | Yes |
| Apply workflow definitions | No | Yes | Yes |
| Create and delete credentials | No | Yes | Yes |
| Manage team members | No | Yes | Yes |
| Assign roles within team | No | Yes (operator only) | Yes |
| Create and delete teams | No | No | Yes |
| Manage all users | No | No | Yes |
| Revoke API keys for any user | No | No | Yes |
| View audit logs | No | No | Yes |
| Access `/metrics` endpoint | No | No | Yes |

**`operator`** -- the default role. Can run workflows, view results, and cancel executions. Cannot modify workflow definitions, credentials, or team membership.

**`team_owner`** -- manages a team's workflows and credentials. Can apply workflow definitions, create credentials, and add or remove team members. Can assign the `operator` role to team members.

**`admin`** -- full access. Can create and delete teams, manage all users across all teams, view audit logs, and access Prometheus metrics.

### Assigning Roles

Assign a role to an existing user:

```bash
mantle roles assign --email alice@example.com --role team_owner
```

```
Assigned role "team_owner" to alice@example.com
```

Only admins can assign any role. Team owners can assign the `operator` role to members of their own team.

### Viewing a User's Role

```bash
mantle users get --email alice@example.com
```

```
Email:   alice@example.com
Name:    Alice Chen
Team:    engineering
Role:    team_owner
Created: 2026-03-18 14:30:00
```

## Teams

Teams provide data isolation in multi-tenant environments. All workflows, executions, credentials, and audit events are scoped to a team. Users in one team cannot see or interact with another team's data.

### Creating a Team

```bash
mantle teams create --name engineering
```

```
Created team engineering (id: f6a7b8c9-d0e1-2345-fabc-456789012345)
```

### Listing Teams

```bash
mantle teams list
```

```
NAME           MEMBERS  CREATED
engineering    3        2026-03-18 14:30:00
data-science   2        2026-03-19 10:00:00
```

### Adding Users to a Team

Specify the team when creating a user:

```bash
mantle users create --email bob@example.com --name "Bob Park" --team engineering --role operator
```

### Data Isolation

Every database query is scoped by `team_id`. This means:

- Workflows applied by team A are not visible to team B
- Executions, logs, and step outputs are team-scoped
- Credentials created by team A cannot be referenced by team B's workflows
- Webhook paths are globally unique but only trigger workflows owned by the team that created them

### Default Team (Single-Tenant Mode)

When Mantle starts without any teams configured, it operates in single-tenant mode. A `default` team is created automatically during `mantle init`. All users and data belong to this team unless you explicitly create additional teams.

Single-tenant mode is the starting point described in the [Getting Started](getting-started.md) guide. You can migrate to multi-tenant mode at any time by creating teams and reassigning users.

## REST API Authentication

All REST API endpoints require a Bearer token in the `Authorization` header. The token can be either an API key or an OIDC JWT:

```bash
# With an API key
curl -H "Authorization: Bearer mk_a1b2c3..." http://localhost:8080/api/v1/executions

# With an OIDC JWT
curl -H "Authorization: Bearer eyJhbGciOiJSUzI1NiIs..." http://localhost:8080/api/v1/executions
```

Mantle distinguishes between the two by checking the `mk_` prefix. Tokens starting with `mk_` are treated as API keys; all others are validated as JWTs.

### Endpoints That Bypass Authentication

The following endpoints do not require authentication:

| Endpoint | Purpose |
|---|---|
| `GET /healthz` | Liveness probe |
| `GET /readyz` | Readiness probe |

### Endpoints That Require Authentication

All other endpoints require a valid Bearer token:

| Endpoint | Minimum Role |
|---|---|
| `POST /api/v1/run/{workflow}` | `operator` |
| `GET /api/v1/executions` | `operator` |
| `GET /api/v1/executions/{id}` | `operator` |
| `POST /api/v1/cancel/{id}` | `operator` |
| `POST /hooks/{path}` | `operator` (or unauthenticated if webhook auth is disabled) |
| `GET /metrics` | `admin` |

Requests with missing, expired, or revoked tokens receive a 401 Unauthorized response. Requests with valid tokens but insufficient role permissions receive a 403 Forbidden response.

## Further Reading

- [CLI Reference](cli-reference.md) -- full flag documentation for auth, users, teams, and roles commands
- [Server Guide](server-guide.md) -- production deployment and REST API
- [Configuration](configuration.md) -- all auth-related configuration options and environment variables
- [Secrets Guide](secrets-guide.md) -- credential storage and encryption (separate from authentication)
- [Observability Guide](observability-guide.md) -- audit trail for authentication events
