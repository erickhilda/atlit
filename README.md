# jt -- Jira Ticket CLI

A lightweight CLI to pull Jira Cloud tickets into local markdown files.

## Features

- Interactive setup with secure token storage (system keyring or encrypted file)
- Fetch Jira tickets as markdown with full ADF-to-markdown conversion
- Preserves a local "My Notes" section across re-pulls
- Dry-run and comments-only update modes
- Open tickets in your browser directly from the terminal
- Print file paths for easy piping to other tools
- Fetch Bitbucket Cloud pull requests (diff + comments) as markdown for code-review context

## Installation

### Shell installer (macOS/Linux)

```bash
curl -sSfL https://raw.githubusercontent.com/erickhilda/jt/master/install.sh | sh
```

To install to a custom directory or pin a version:

```bash
curl -sSfL https://raw.githubusercontent.com/erickhilda/jt/master/install.sh | sh -s -- -d ~/.local/bin -v v0.1.0
```

### Homebrew (macOS/Linux)

```bash
brew install erickhilda/tap/jt
```

### Binary download

Download a prebuilt binary from the [GitHub Releases](https://github.com/erickhilda/jt/releases) page.

### From source

```bash
git clone https://github.com/erickhilda/jt.git
cd jt
make install
```

Requires Go 1.25+.

## Quick Start

```bash
# 1. Configure your Jira instance, email, and API token
jt init

# 2. Pull a ticket to local markdown
jt pull PROJ-123

# 3. View it
jt view PROJ-123

# 4. Open in browser
jt open PROJ-123
```

## Creating a Jira API Token

jt authenticates using Jira Cloud API tokens (not your account password). To create one:

1. Log in to <https://id.atlassian.com/manage-profile/security/api-tokens>
2. Click **Create API token**
3. Give it a label (e.g. "jt CLI") and click **Create**
4. Copy the token -- you won't be able to see it again

Paste the token when `jt init` prompts for it. The token inherits the permissions of your Atlassian account.

If you need to rotate a token later, create a new one in the same page and update jt:

```bash
jt config set token <new-token>
```

## Commands

### `jt init`

Interactive setup wizard. Prompts for your Jira instance URL, email, API token, and default project key. Tests credentials before saving.

### `jt auth test`

Verify stored credentials against the Jira API. Prints your display name, email, account ID, and timezone on success.

### `jt auth bitbucket`

Set (and verify) a Bitbucket Cloud API token, stored separately from the Jira token. Create the token at <https://id.atlassian.com/manage-profile/security/api-tokens> with scopes `read:pullrequest:bitbucket` and `read:repository:bitbucket`. If `bitbucket_workspace` is configured, the token is verified against it.

### `jt config show`

Display all configuration settings (token is masked).

### `jt config set <key> <value>`

Update a single configuration value.

Valid keys: `instance`, `email`, `default_project`, `tickets_dir`, `fetch_comments`, `token`, `bitbucket_workspace`, `prs_dir`, `bitbucket_token`.

```bash
jt config set instance https://myorg.atlassian.net
jt config set default_project PROJ
jt config set fetch_comments false
```

### `jt pull <TICKET-KEY>`

Fetch a Jira ticket and save it as local markdown.

| Flag | Description |
|------|-------------|
| `--comments-only` | Only update the comments section |
| `--dry-run` | Show a diff of what would change without saving |

The pull command preserves any content you've written under the `## My Notes` section.

### `jt view <TICKET-KEY>`

Print the local ticket markdown to stdout. Useful for piping:

```bash
jt view PROJ-123 | less
```

### `jt open <TICKET-KEY>`

Open the ticket in your default browser.

### `jt path <TICKET-KEY>`

Print the absolute filesystem path to a ticket's markdown file. Useful for scripting:

```bash
cat "$(jt path PROJ-123)"
```

### `jt pr <PR-REF>`

Fetch a Bitbucket Cloud pull request (metadata, diff, comments) and save it as local markdown for code-review context. Requires a Bitbucket token (`jt auth bitbucket`).

Reference forms:

```bash
jt pr 4521                    # infer workspace/repo from the git remote (run inside the repo)
jt pr widget/4521             # repo explicit, workspace from `bitbucket_workspace`
jt pr acme/widget/4521        # fully explicit
```

| Flag | Description |
|------|-------------|
| `--no-diff` | Omit the unified diff (keep diffstat + comments) — useful for very large PRs |
| `--dry-run` | Show a diff of what would change without saving |

PRs are saved to `prs_dir` (default `~/.jt/prs`) as `<workspace>__<repo>__<id>.md`. A `## My Notes` section is preserved across re-fetches, and if the PR's branch/title contains a Jira key (e.g. `PROJ-1234`) it is linked — with a pointer to the local ticket file when one exists.

The full unified diff is embedded by default; on a large diff `jt pr` prints a warning (it never silently truncates) so you can re-run with `--no-diff`.

## Configuration

Configuration is stored in `~/.jt/config.yaml`:

```yaml
instance: https://yourcompany.atlassian.net
email: you@company.com
default_project: PROJ
tickets_dir: ~/.jt/tickets
token_storage: keyring
fetch_comments: true
```

| Key | Description |
|-----|-------------|
| `instance` | Jira Cloud base URL (must start with `https://`) |
| `email` | Jira account email |
| `default_project` | Default project key (optional) |
| `tickets_dir` | Directory for saved tickets (default: `~/.jt/tickets`) |
| `token_storage` | `keyring` (system keyring) or `file` (`~/.jt/credentials`, 0600) |
| `fetch_comments` | Fetch and render the Comments section. Default `true`. Set `false` to skip comments on `pull`, `diff`, and `sync` (smaller payloads; existing `## Comments` blocks in local files are preserved). `jt pull --comments-only` overrides this and always refreshes comments. |
| `bitbucket_workspace` | Default Bitbucket workspace for `jt pr <repo>/<id>` references |
| `prs_dir` | Directory for saved pull requests (default: `~/.jt/prs`) |

API tokens are stored in your system keyring when available, with an automatic fallback to an encrypted credentials file.

## Local File Format

Pulled tickets are saved as `~/.jt/tickets/<KEY>.md`:

```markdown
<!-- jt:meta ticket=PROJ-123 fetched=2026-02-14T10:30:00Z -->
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
