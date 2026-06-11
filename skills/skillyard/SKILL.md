---
name: skillyard
description: Manage global Codex and Amp agent skills with the skillyard CLI. Use when asked to install, subscribe, sync, list, remove, repair, or debug skills managed by skillyard, update this repository, or explain skillyard config, lockfiles, targets, sources, and symlink behavior.
---

# skillyard

Use this skill when working with the `skillyard` CLI or repository.

`skillyard` manages global Codex and Amp Agent Skills from Git or local sources. It records desired subscriptions and realized symlinks in an XDG lockfile so installs can be inspected, synced, and removed without losing source provenance.

## First Checks

From the repository:

```bash
git status --short --branch
go run ./cmd/skillyard --help
```

For the installed CLI:

```bash
skillyard doctor
skillyard list
```

Keep normal command output on stdout and progress or diagnostics on stderr when changing CLI code.

## Core Paths

Resolve the actual paths with:

```bash
skillyard doctor
```

The path resolver respects these environment overrides:

```text
SKILLYARD_CONFIG_DIR
SKILLYARD_DATA_DIR
SKILLYARD_CACHE_DIR
XDG_DATA_HOME
CODEX_HOME
```

Do not assume Linux-style `~/.config` or `~/.cache` paths on every host. `skillyard` derives config and cache paths from Go's user directory APIs, and data paths from `SKILLYARD_DATA_DIR`, `XDG_DATA_HOME`, or `~/.local/share/skillyard`.

Default agent targets:

```text
codex -> ${CODEX_HOME:-~/.codex}/skills
amp   -> ~/.config/agents/skills
```

`config.hcl` can override, disable, or add targets:

```hcl
agent "codex" {
  enabled    = true
  skills_dir = "~/.codex/skills"
}

agent "amp" {
  enabled    = true
  skills_dir = "~/.config/agents/skills"
}
```

If `CODEX_HOME` is set, `skillyard setup` may emit `"$CODEX_HOME/skills"` for Codex. If it is not set, use `"~/.codex/skills"`; `skills_dir = "$CODEX_HOME/skills"` expands to `/skills` when the environment variable is empty.

When `--target` is omitted, `subscribe` uses every enabled configured agent.

## Common Operations

Create or inspect config:

```bash
skillyard setup
skillyard setup --dry-run
skillyard setup --force
```

Subscribe to a single-skill source:

```bash
skillyard subscribe github:lox/slack-cli --dry-run
skillyard subscribe github:lox/slack-cli
```

Subscribe to all skills from a source:

```bash
skillyard subscribe github:buildkite/skills --include '*'
```

Install only selected skills:

```bash
skillyard subscribe github:lox/manager-os \
  --include google-groups \
  --include granola \
  --target amp
```

Exclude a target-specific skill:

```bash
skillyard subscribe github:lox/agent-skills \
  --include '*' \
  --exclude consulting-librarian \
  --target amp
```

Reconcile and inspect state:

```bash
skillyard sync
skillyard sync github:buildkite/skills --target codex
skillyard sync --dry-run
skillyard list
skillyard list --json
```

Remove behavior:

```bash
# Change desired state; future sync will not recreate this skill for the target.
skillyard unsubscribe slack --target codex

# Remove the realized link only; future sync may recreate it.
skillyard unlink slack --target codex
```

## Source And Selection Rules

- Supported Git inputs include `github:owner/repo`, HTTPS Git URLs, SSH Git URLs, and `file://` URLs.
- Local paths are supported and link directly to the local skill directory.
- Git installs link to immutable snapshots under `~/.local/share/skillyard/sources/<source-id>/snapshots/<commit>/`.
- Discovery checks the source root, direct child directories, and `skills/<name>`.
- A valid skill directory contains `SKILL.md` with YAML frontmatter containing `name` and `description`.
- The frontmatter `name` must match the directory basename.
- Omitted `--include` defaults only when the source has exactly one discovered skill.
- `--include '*'` is an explicit broad subscription; new matching skills are installed on the next `sync` unless excluded.

## Conflict Handling

`skillyard` is intentionally conservative around target directories:

- Existing non-symlink skill paths are never overwritten.
- Unmanaged symlinks are only replaced with `--force`.
- Managed links are removed or retargeted only when recorded in `skillyard.lock.json`.
- Failed preflight checks should leave target links and the lockfile unchanged.

When replacing an unmanaged directory with a subscription, compare it to the source first and back it up before moving it aside:

```bash
diff -qr ~/.config/agents/skills/<skill> <source-checkout>/skills/<skill>
mkdir -p ~/.cache/skillyard/replaced-unmanaged/<timestamp>
mv ~/.config/agents/skills/<skill> ~/.cache/skillyard/replaced-unmanaged/<timestamp>/<skill>
skillyard subscribe <source> --include <skill> --target amp
```

Prefer `--dry-run` before a broad subscription or force operation.

## Repository Work

Use the repo's normal Go validation path:

```bash
go test ./...
go vet ./...
mise run check
```

For local install testing:

```bash
mise run install
~/bin/skillyard --version
```

When changing CLI behavior, update README examples and focused tests with the behavior. Keep edits scoped to `cmd/skillyard`, `internal/`, `README.md`, `docs/plans/`, `mise.toml`, or `skills/` as appropriate.
