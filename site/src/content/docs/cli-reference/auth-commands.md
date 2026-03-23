# Authentication & Team Commands

Commands for managing authentication, teams, users, and roles.

## mantle login

Authenticate with a Mantle server. Supports three authentication methods: OIDC authorization code with PKCE (default), device authorization flow, and API key caching.

Credentials are stored in `~/.mantle/credentials`.

```
Usage:
  mantle login [flags]
```

**Flags:**

| Flag | Description |
|---|---|
| `--api-key` | Authenticate by entering and caching an API key. |
| `--device` | Use the device authorization flow (for headless/SSH environments). |

When neither flag is provided, the default OIDC authorization code + PKCE flow is used. This opens a browser for the identity provider login and listens on a local callback URL.

**Example -- OIDC (default):**

```bash
$ mantle login
Open this URL to authenticate:

  https://auth.example.com/authorize?client_id=...

Waiting for callback...
Login successful! Credentials saved to /home/alice/.mantle/credentials
```

**Example -- device flow:**

```bash
$ mantle login --device
To authenticate, visit:

  https://auth.example.com/device

And enter code: ABCD-1234

Waiting for authorization...
Login successful! Credentials saved to /home/alice/.mantle/credentials
```

**Example -- API key:**

```bash
$ mantle login --api-key
Enter API key: mk_a1b2c3d4e5f6...
API key saved to /home/alice/.mantle/credentials
```

OIDC requires `auth.oidc.issuer_url` and `auth.oidc.client_id` to be configured in `mantle.yaml` or via environment variables.

---

## mantle logout

Remove cached credentials from `~/.mantle/credentials`.

```
Usage:
  mantle logout
```

**Example:**

```bash
$ mantle logout
Credentials removed from /home/alice/.mantle/credentials
```

---

## mantle teams

Manage teams. Teams are the unit of multi-tenancy in Mantle -- each team has its own workflows, credentials, and users.

```
Usage:
  mantle teams create [flags]
  mantle teams list
  mantle teams delete [flags]
```

### mantle teams create

Create a new team.

**Flags:**

| Flag | Required | Description |
|---|---|---|
| `--name` | Yes | Team name. |

**Example:**

```bash
$ mantle teams create --name my-team
Created team my-team (id: a1b2c3d4-e5f6-7890-abcd-ef1234567890)
```

### mantle teams list

List all teams.

```bash
$ mantle teams list
NAME      ID                                    CREATED
my-team   a1b2c3d4-e5f6-7890-abcd-ef1234567890  2026-03-18 14:30:00
default   b2c3d4e5-f6a7-8901-bcde-f12345678901  2026-03-18 10:00:00
```

If no teams exist:

```
(no teams)
```

### mantle teams delete

Delete a team by name.

**Flags:**

| Flag | Required | Description |
|---|---|---|
| `--name` | Yes | Name of the team to delete. |

**Example:**

```bash
$ mantle teams delete --name my-team
Deleted team "my-team"
```

---

## mantle users

Manage users. Users belong to teams and have a role that controls their permissions.

```
Usage:
  mantle users create [flags]
  mantle users list [flags]
  mantle users delete [flags]
  mantle users api-key [flags]
```

### mantle users create

Create a new user and assign them to a team with a role.

**Flags:**

| Flag | Required | Default | Description |
|---|---|---|---|
| `--email` | Yes | -- | User email address. |
| `--name` | Yes | -- | User display name. |
| `--team` | No | `default` | Team to add the user to. |
| `--role` | No | `operator` | Role to assign: `admin`, `team_owner`, `operator`. |

**Example:**

```bash
$ mantle users create --email alice@example.com --name "Alice Smith" --team my-team --role team_owner
Created user alice@example.com (role: team_owner, team: my-team)
```

### mantle users list

List users in a team.

**Flags:**

| Flag | Required | Default | Description |
|---|---|---|---|
| `--team` | No | `default` | Team name to list users for. |

**Example:**

```bash
$ mantle users list --team my-team
EMAIL                NAME          ROLE
alice@example.com    Alice Smith   team_owner
bob@example.com      Bob Jones     operator
```

If no users exist:

```
(no users)
```

### mantle users delete

Delete a user by email.

**Flags:**

| Flag | Required | Default | Description |
|---|---|---|---|
| `--email` | Yes | -- | Email of the user to delete. |
| `--team` | No | `default` | Team name. |

**Example:**

```bash
$ mantle users delete --email bob@example.com
Deleted user "bob@example.com"
```

### mantle users api-key

Generate an API key for a user. The key is displayed once and cannot be retrieved again.

**Flags:**

| Flag | Required | Description |
|---|---|---|
| `--email` | Yes | User email. |
| `--key-name` | Yes | A name for the API key (for identification). |

**Example:**

```bash
$ mantle users api-key --email alice@example.com --key-name ci-deploy

API Key: mk_a1b2c3d4e5f6...

Save this key — it cannot be retrieved again.
Key prefix for reference: mk_a1b2
```

---

## mantle roles

Manage user roles.

```
Usage:
  mantle roles assign [flags]
```

### mantle roles assign

Assign a role to an existing user.

**Flags:**

| Flag | Required | Default | Description |
|---|---|---|---|
| `--email` | Yes | -- | User email. |
| `--role` | Yes | -- | Role to assign: `admin`, `team_owner`, `operator`. |
| `--team` | No | `default` | Team name. |

**Example:**

```bash
$ mantle roles assign --email alice@example.com --role admin
Assigned role "admin" to user "alice@example.com"
```
