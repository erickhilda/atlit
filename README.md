# atlit -- Atlassian context CLI

A lightweight CLI that pulls Atlassian content -- Jira tickets, Bitbucket PRs, and Confluence pages -- into local markdown files.

> **Renamed from `jt` to `atlit` in v0.4.0.** If you're upgrading from a pre-v0.4.0 (`jt`) install, run `atlit migrate` once after installing -- see [Migrating from `jt`](#migrating-from-jt-pre-v040).

## Features

- Interactive setup with secure token storage (system keyring or encrypted file)
- Fetch Jira tickets as markdown with full ADF-to-markdown conversion
- Preserves a local "My Notes" section across re-pulls
- Dry-run and comments-only update modes
- Open tickets in your browser directly from the terminal
- Print file paths for easy piping to other tools
- Search Jira with preset filters (status, assignee, mine) or raw JQL, listed as a stdout table
- Fetch Bitbucket Cloud pull requests (diff + comments) as markdown for code-review context
- Fetch Confluence Cloud pages as markdown (ADF-to-markdown) for offline reading and LLM context

## Installation

### Shell installer (macOS/Linux)

```bash
curl -sSfL https://raw.githubusercontent.com/erickhilda/atlit/master/install.sh | sh
```

To install to a custom directory or pin a version:

```bash
curl -sSfL https://raw.githubusercontent.com/erickhilda/atlit/master/install.sh | sh -s -- -d ~/.local/bin -v v0.4.0
```

### Binary download

Download a prebuilt binary from the [GitHub Releases](https://github.com/erickhilda/atlit/releases) page.

### From source

```bash
git clone https://github.com/erickhilda/atlit.git
cd atlit
make install
```

Requires Go 1.25+.

## Migrating from `jt` (pre-v0.4.0)

This tool was called `jt` (a Jira-only ticket CLI) through v0.3.x. As of **v0.4.0** it is **`atlit`**, reflecting its broader scope across Jira, Bitbucket, and Confluence.

If you install v0.4.0+ over an existing `jt` setup, `atlit` already reads your old state — the legacy `~/.jt` config directory, `jt-cli` keyring tokens, and the `<!-- jt:meta -->` header in previously-pulled files all still work — so nothing breaks immediately. To move that state onto the new names, run:

```bash
atlit migrate --dry-run   # preview what would change; modifies nothing
atlit migrate             # apply
```

`atlit migrate` is idempotent (safe to re-run) and:

- moves `~/.jt` → `~/.atlit` (config + credentials)
- copies keyring tokens from the `jt-cli` service to `atlit-cli`
- rewrites the `<!-- jt:meta ... -->` header in your pulled files to `atlit:meta`

Then remove the old binary (e.g. `rm "$(command -v jt)"`). If you skip migration, `atlit` keeps working through its legacy fallbacks — running `migrate` just keeps your on-disk state consistent with the new name.

## Quick Start

```bash
# 1. Configure your Jira instance, email, and API token
atlit init

# 2. Pull a ticket to local markdown
atlit pull PROJ-123

# 3. View it
atlit view PROJ-123

# 4. Open in browser
atlit open PROJ-123
```

## Creating a Jira API Token

atlit authenticates using Jira Cloud API tokens (not your account password). To create one:

1. Log in to <https://id.atlassian.com/manage-profile/security/api-tokens>
2. Click **Create API token**
3. Give it a label (e.g. "atlit CLI") and click **Create**
4. Copy the token -- you won't be able to see it again

Paste the token when `atlit init` prompts for it. The token inherits the permissions of your Atlassian account.

If you need to rotate a token later, create a new one in the same page and update atlit:

```bash
atlit config set token <new-token>
```

## Commands

### `atlit init`

Interactive setup wizard. Prompts for your Jira instance URL, email, API token, and default project key. Tests credentials before saving.

### `atlit auth test`

Verify stored credentials against the Jira API. Prints your display name, email, account ID, and timezone on success.

### `atlit auth bitbucket`

Set (and verify) a Bitbucket Cloud API token, stored separately from the Jira token. Create the token at <https://id.atlassian.com/manage-profile/security/api-tokens> with scopes `read:pullrequest:bitbucket` and `read:repository:bitbucket`. If `bitbucket_workspace` is configured, the token is verified against it.

### `atlit config show`

Display all configuration settings (token is masked).

### `atlit config set <key> <value>`

Update a single configuration value.

Valid keys: `instance`, `email`, `default_project`, `tickets_dir`, `fetch_comments`, `fetch_pull_requests`, `token`, `bitbucket_workspace`, `prs_dir`, `bitbucket_token`.

```bash
atlit config set instance https://myorg.atlassian.net
atlit config set default_project PROJ
atlit config set fetch_comments false
```

### `atlit pull <TICKET-KEY>`

Fetch a Jira ticket and save it as local markdown.

| Flag | Description |
|------|-------------|
| `--comments-only` | Only update the comments section |
| `--dry-run` | Show a diff of what would change without saving |

The pull command preserves any content you've written under the `## My Notes` section.

### `atlit view <TICKET-KEY>`

Print the local ticket markdown to stdout. Useful for piping:

```bash
atlit view PROJ-123 | less
```

### `atlit open <TICKET-KEY>`

Open the ticket in your default browser.

### `atlit path <TICKET-KEY>`

Print the absolute filesystem path to a ticket's markdown file. Useful for scripting:

```bash
cat "$(atlit path PROJ-123)"
```

### `atlit search`

Search Jira and list matching tickets as a table on stdout (newest-updated first). Nothing is written to disk — use it to find a ticket, then run `atlit pull <KEY>` to fetch it.

Preset filters are composed with `AND` and scoped to `default_project` unless you override the scope. At least one filter is required.

```bash
atlit search --status "code review"                 # one status
atlit search --status "code review,stage test"      # comma-separated -> status in (...)
atlit search --assignee alice                        # name/email resolved to a Jira account
atlit search --mine                                  # assignee = currentUser()
atlit search --mine --active                          # exclude done-category statuses
atlit search --status "stage test" --project FOO     # override the project scope
atlit search --mine --all-projects                   # drop the project scope
```

`--assignee` resolves a human-friendly name or email to a Jira account via the user-search API; if it matches no one (or several people) it errors and lists the candidates so you can refine. Use `--mine` for yourself (no lookup needed).

For anything the presets can't express, `--jql` takes a raw query. It is a standalone escape hatch and cannot be combined with the preset/scope flags:

```bash
atlit search --jql "project = FOO AND sprint in openSprints() ORDER BY updated DESC"
```

| Flag | Description |
|------|-------------|
| `--status` | Filter by status name; comma-separate for multiple (`status in (...)`) |
| `--assignee` | Filter by assignee; a name or email resolved to a Jira account |
| `--mine` | Filter to tickets assigned to you (`assignee = currentUser()`) |
| `--active` | Exclude done-category statuses (`statusCategory != Done`) |
| `--jql` | Raw JQL query (advanced; cannot be combined with the preset filters) |
| `--project` | Restrict to this project key (overrides `default_project`) |
| `--all-projects` | Do not restrict to a project |
| `--limit` | Maximum number of tickets to list (default 30; counts rows shown, not the query total) |

The effective JQL is printed above the table for transparency. The table shows the key, summary, status, assignee, and a relative "updated" age.

### `atlit pr <PR-REF>`

Fetch a Bitbucket Cloud pull request (metadata, diff, comments) and save it as local markdown for code-review context. Requires a Bitbucket token (`atlit auth bitbucket`).

Reference forms:

```bash
atlit pr 4521                    # infer workspace/repo from the git remote (run inside the repo)
atlit pr widget/4521             # repo explicit, workspace from `bitbucket_workspace`
atlit pr acme/widget/4521        # fully explicit
```

| Flag | Description |
|------|-------------|
| `--no-diff` | Omit the unified diff (keep diffstat + comments) — useful for very large PRs |
| `--dry-run` | Show a diff of what would change without saving |

PRs are saved to `prs_dir` (default `~/.atlit/prs`) as `<workspace>__<repo>__<id>.md`. A `## My Notes` section is preserved across re-fetches, and if the PR's branch/title contains a Jira key (e.g. `PROJ-1234`) it is linked — with a pointer to the local ticket file when one exists.

The full unified diff is embedded by default; on a large diff `atlit pr` prints a warning (it never silently truncates) so you can re-run with `--no-diff`.

### `atlit pr list [REPO-REF]`

List a repository's pull requests as a table on stdout (open by default, newest-updated first). Nothing is written to disk — use it to find a PR, then run `atlit pr <id>` to fetch its diff and comments.

Reference forms:

```bash
atlit pr list                    # infer workspace/repo from the git remote (run inside the repo)
atlit pr list widget             # repo explicit, workspace from `bitbucket_workspace`
atlit pr list acme/widget        # fully explicit
```

| Flag | Description |
|------|-------------|
| `--state` | Filter by state: `open` (default), `merged`, `declined`, or `all` |
| `--limit` | Maximum number of PRs to list (default 30; counts rows shown, not the repo total) |

The table shows the PR id, title, linked Jira key (from the branch/title, `-` when absent), author, and a relative "updated" age. The `--limit` count caps the rows fetched, so the header count reflects what was shown rather than the repository's full PR total.

### `atlit page <PAGE-ID | URL>`

Fetch a Confluence Cloud page (title, metadata, body) and save it as local markdown for offline reading and LLM context. The page body is converted from Atlassian Document Format to markdown using the same converter as `atlit pull`.

This reuses your existing Jira API token — Confluence lives on the same Atlassian site (`<instance>/wiki`) and uses the same authentication, so no separate login is needed as long as the token has Confluence access (unscoped API tokens do; a scoped token needs a Confluence read scope).

Reference forms:

```bash
atlit page 12345                                                       # numeric page ID
atlit page https://acme.atlassian.net/wiki/spaces/ENG/pages/12345/Title   # full page URL
```

| Flag | Description |
|------|-------------|
| `--dry-run` | Show a diff of what would change without saving |

Pages are saved to `pages_dir` (default `~/.atlit/pages`) as `<space>__<id>__<slug>.md`. A `## My Notes` section is preserved across re-fetches.

## Configuration

Configuration is stored in `~/.atlit/config.yaml`:

```yaml
instance: https://yourcompany.atlassian.net
email: you@company.com
default_project: PROJ
tickets_dir: ~/.atlit/tickets
token_storage: keyring
fetch_comments: true
```

| Key | Description |
|-----|-------------|
| `instance` | Jira Cloud base URL (must start with `https://`) |
| `email` | Jira account email |
| `default_project` | Default project key (optional) |
| `tickets_dir` | Directory for saved tickets (default: `~/.atlit/tickets`) |
| `token_storage` | `keyring` (system keyring) or `file` (`~/.atlit/credentials`, 0600) |
| `fetch_comments` | Fetch and render the Comments section. Default `true`. Set `false` to skip comments on `pull`, `diff`, and `sync` (smaller payloads; existing `## Comments` blocks in local files are preserved). `atlit pull --comments-only` overrides this and always refreshes comments. |
| `fetch_pull_requests` | Fetch and render the development panel's linked pull requests (a `## Pull Requests` section) on `pull` and `sync`. Default `true`. Uses Jira's dev-status API, so PRs only appear when Jira is connected to your Git host (Bitbucket/GitHub) and the branch/commit/PR references the issue key. Failures are non-fatal: `pull` warns and keeps any existing `## Pull Requests` block. Set `false` to skip the lookup. |
| `bitbucket_workspace` | Default Bitbucket workspace for `atlit pr <repo>/<id>` references |
| `prs_dir` | Directory for saved pull requests (default: `~/.atlit/prs`) |
| `pages_dir` | Directory for saved Confluence pages (default: `~/.atlit/pages`) |

API tokens are stored in your system keyring when available, with an automatic fallback to an encrypted credentials file.

## Local File Format

Pulled tickets are saved as `~/.atlit/tickets/<KEY>.md`:

```markdown
<!-- atlit:meta ticket=PROJ-123 fetched=2026-02-14T10:30:00Z -->
# PROJ-123: Implement OAuth2 flow

| Field       | Value              |
|-------------|--------------------|
| Status      | In Progress        |
| Type        | Story              |
| Priority    | High               |
| Assignee    | you@company.com    |
| Reporter    | pm@company.com     |
| Labels      | backend, security  |
| Created     | 2026-02-01         |
| Updated     | 2026-02-13         |

## Description

The ticket description converted from Atlassian Document Format to markdown.

## Subtasks

- [x] PROJ-124: Research OAuth2 libraries (Done)
- [ ] PROJ-125: Implement token storage (In Progress)

## Linked Issues

- blocks PROJ-130: Protected API endpoints

## Pull Requests (1)

- [MERGED] [PROJ-123: implement OAuth2 flow](https://bitbucket.org/acme/repo/pull-requests/42) (#42)
  - Branch: feature/PROJ-123 -> develop
  - Author: Alice
  - Approved by: Bob

## Comments (2)

### Alice -- 2026-02-10

We should use PKCE for the mobile app flow.

## My Notes

Your local notes are preserved across re-pulls.
```

## Development

```bash
make build     # build the binary
make install   # install to $GOPATH/bin
make test      # run all tests
make lint      # run golangci-lint
make clean     # remove built binary
```

## Roadmap

See [ROADMAP.md](ROADMAP.md) for planned features including search, sync, shell completions, and more.
