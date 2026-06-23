---
status: implemented
last_reviewed: 2026-06-23
spec_refs:
  - https://github.com/vercel-labs/skills
  - https://www.npmjs.com/package/skills
---

# skillyard Global Skills Spec

## Summary

`skillyard` is a Go CLI for managing global Codex and Amp skills from public or private Git sources.

The core workflow is:

```bash
skillyard discover github:lox/agent-skills
skillyard show github:lox/agent-skills --include check-pr-description
skillyard export --target codex > skillyard.lock.json
skillyard apply skillyard.lock.json --target codex --dry-run
skillyard subscribe github:lox/agent-skills --include '*' --target codex
skillyard subscribe github:lox/agent-skills --include '*' --ref v1.2.3 --target codex
skillyard subscribe github:lox/agent-skills --include '*' --exclude consulting-librarian --target amp
skillyard subscribe git@github.com:org/private-skills.git --include deploy-review --target codex
skillyard subscribe github:lox/slack-cli --dry-run
skillyard sync
skillyard list
skillyard unsubscribe deploy-review --target codex
```

`skillyard` records a subscription from a source to one or more global targets, discovers matching skill directories, validates each selected `SKILL.md`, links the selected skills into the requested agent roots, and records the source-to-link relationship in generated state. Subscriptions are desired state; installed symlinks are realized state.

The design borrows the useful parts of the upstream `skills` CLI: familiar Git source inputs, explicit skill selection, symlink-first installation, list/remove-style management commands, and support for both public and private Git repositories through ordinary Git URLs. `skillyard` keeps the target model explicit: every install is tied to a configured agent target and root.

When a source changes, `skillyard sync` refreshes the managed checkout and reconciles installed links with the subscription. If a subscription includes `'*'`, newly added valid matching skills are linked on sync unless they match an exclusion.

## Problem

Global skill installation is currently a manual symlink workflow:

- find a repository or local directory containing skills
- inspect which directories contain `SKILL.md`
- decide whether each skill belongs in Codex, Amp, or both
- create symlinks under the correct global agent root
- avoid overwriting existing installed skills
- remember where each symlink came from

Manual linking works for a single local repo, but it becomes fragile with multiple public/private sources, repeated installs, removal, and target-specific choices such as installing `consulting-librarian` for Codex but not Amp.

`skillyard` makes that relationship explicit: source, selected skill, target, link path, and resolved Git commit are all tracked.

## Goals

- Manage global Codex and Amp user skills.
- Add skills from GitHub shorthand, HTTPS Git URLs, SSH Git URLs, and local paths.
- Use the system `git` binary so private repositories work with existing SSH agents and credential helpers.
- Inspect source skills without changing subscriptions, installed links, or lockfile state.
- Show one selected skill's instructions without installing it.
- Export and apply portable desired state for sources and subscriptions.
- Pin Git sources to a branch, tag, or commit when requested.
- Install by symlinking selected skill directories into agent roots.
- Preserve Codex and Amp as explicit targets.
- Record ownership state for safe list, unsubscribe, and unlink behavior.
- Preserve subscription intent so broad includes like `'*'` can pick up newly added matching skills during sync.
- Refuse unmanaged conflicts.
- Validate selected skills before linking.
- Keep subscription changes distinct from link-only repair operations.
- Keep command results on stdout and diagnostics/progress on stderr.
- Provide JSON output for automation where useful.

## Paths

`skillyard` state:

```text
~/.config/skillyard/skillyard.lock.json
~/.local/share/skillyard/sources/<source-id>/
~/.cache/skillyard/
```

`skillyard` configuration:

```text
~/.config/skillyard/config.hcl
```

Built-in agent targets are used when no config file exists:

```text
codex -> ${CODEX_HOME:-~/.codex}/skills
amp   -> ~/.config/agents/skills
```

The config file can override, disable, or add agents:

```hcl
agent "codex" {
  enabled    = true
  skills_dir = "~/.codex/skills"
}

agent "amp" {
  enabled    = true
  skills_dir = "~/.config/agents/skills"
}

agent "custom" {
  enabled    = true
  skills_dir = "~/Library/Application Support/custom-agent/skills"
}
```

`skills_dir` supports `~` and environment variable expansion such as `$CODEX_HOME/skills`.

`skillyard subscribe` creates the `skillyard` config/data directories as needed. It creates target skill roots only when linking a skill to that target.

## Source Model

A source is either a Git repository or a local filesystem path containing one or more skill directories.

Supported source inputs:

```text
github:owner/repo
https://github.com/owner/repo.git
git@github.com:owner/repo.git
/local/path/to/skills-repo
```

Source rules:

- `github:owner/repo` normalizes to `https://github.com/owner/repo.git`.
- Git sources clone into `~/.local/share/skillyard/sources/<source-id>/repo`.
- Git source snapshots live under `~/.local/share/skillyard/sources/<source-id>/snapshots/<commit>/`.
- Git sources can set `--ref <branch|tag|commit>` to track a non-default branch, immutable tag, or commit.
- Source IDs include the Git ref when one is set, so the same repository can be subscribed at multiple refs.
- Local path sources link directly to the local path and are marked as mutable development sources.
- Source IDs are stable slugs with a short hash suffix to avoid collisions.
- Git source IDs are derived from the normalized URL.
- Local source IDs are derived from the source path basename plus a hash of the resolved path, so `~/Develop/slack-cli` becomes a readable `slack-cli-<hash>` source id.
- Git source records include the last seen commit after clone or fetch.
- Private repository access is delegated to `git`.

## Skill Discovery

A skill is a directory containing `SKILL.md`.

Discovery rules:

- A source root can itself be a skill directory.
- A source root can contain direct child skill directories.
- The `skills/` child directory is treated as a skill container when present.
- `skills/<category>/` child directories are treated as nested skill containers.
- `.agents/skills/` and `.claude/skills/` are treated as skill containers when present.
- `.claude-plugin/plugin.json`, `.claude-plugin/marketplace.json`, `.codex-plugin/plugin.json`, and `.codex-plugin/marketplace.json` can declare skill paths or skill containers.
- Hidden directories are ignored except the explicit `.agents/skills/` and `.claude/skills/` containers; `.git` is ignored.
- `SKILL.md` must have YAML frontmatter.
- Frontmatter must include `name` and `description`.
- `name` must match the skill directory basename, except when the source root is the skill directory.

Selected skills are blocked when:

- `SKILL.md` is missing.
- YAML frontmatter is missing or invalid.
- `name` or `description` is missing.
- `name` does not match the directory basename.
- an exact include pattern matches no skill.
- the same selected skill name would be installed twice into the same target.

Selected skills warn when they contain:

- `scripts/`
- executable files
- `mcp.json`

Warnings are visible in human output and included in JSON output.

`skillyard discover` reports validation findings and warnings for every candidate it can inspect. Invalid `SKILL.md` files do not block the whole discover command, but the same findings still block install-oriented commands such as `subscribe` and `sync`.

## Desired And Realized State

Subscriptions are the desired state:

```text
source + target + include/exclude patterns
```

Install records are the realized state:

```text
skill + target + source snapshot/local path + symlink path
```

`sync` reconciles realized state to desired state. `unsubscribe` changes desired state so future syncs stop managing a skill or subscription. `unlink` removes a realized symlink only; if a subscription still matches that skill, the next sync may recreate it.

## Reconciliation Model

Every command that changes desired state, realized state, or install records uses the same reconciler. `subscribe`, `unsubscribe`, and `sync` differ in how they build the desired state, but they share one plan/apply/verify/lock path:

1. Load the current lockfile.
2. Apply command-specific desired-state edits in memory.
3. Resolve desired installs from subscriptions.
4. Resolve Git sources and snapshots, or resolve mutable local paths.
5. Validate selected skills.
6. Preflight target conflicts and link removals.
7. Apply filesystem changes, including link creates, link retargets, and managed link removals for installs that no longer match desired state.
8. Verify resulting links.
9. Write the lockfile atomically.

If any preflight step fails, no target links change and no lockfile is written. If filesystem apply fails after a target mutation, the lockfile is left untouched so `list` can report drift. `unlink` is the only command that mutates realized state without changing subscriptions, but it still uses the same preflight, apply, verify, and lockfile write ordering.

Reconciliation is fail-whole-plan by default. One invalid selected skill, unmanaged conflict, or unsafe removal blocks the command instead of partially applying a broad subscription. Output should show the blocked items clearly so the user can narrow includes, add excludes, or repair target paths.

Local path sources take the local-source branch of the reconciler:

- Do not fetch.
- Do not create snapshots.
- Resolve and validate the current local path.
- Link directly to the local skill path.
- Record `type: "local"` with input and resolved paths.
- Always report installed links from local sources as `mutable-source`.

## Materialization

`skillyard` installs by symlink to a managed source snapshot:

```text
<target-root>/<skill-name> -> <managed-source-snapshot>/<skill-name>
```

Symlink installation keeps source ownership visible, matches the existing manual workflow, and makes removal straightforward. Git-backed installs should link to a snapshot for the resolved commit rather than a mutable branch checkout, so the lockfile commit describes what Codex or Amp actually load.

Safety rules:

- Never overwrite an existing non-symlink skill path.
- Never overwrite an unmanaged symlink unless `--force` is passed.
- Treat an existing symlink to the desired source skill as already installed.
- Retarget a managed symlink when it still points at the previously recorded managed source.
- Require `--force` to replace or remove a managed symlink that has drifted away from the recorded source.
- Validate target link paths stay inside the target root.
- Remove only symlinks recorded in `skillyard.lock.json`.
- After linking, verify `<target-root>/<skill-name>/SKILL.md` resolves.

## Lockfile

Generated ownership and source state lives at:

```text
~/.config/skillyard/skillyard.lock.json
```

The lockfile records enough information to list installs, classify link status, remove only managed links, and know which Git commit each installed skill was linked from. It also records subscription intent, such as include/exclude patterns, so sync can discover and install newly matching skills from the same source without the user retyping the original subscribe command.

Shape:

```json
{
  "version": 1,
  "sources": {
    "github-com-lox-agent-skills-a1b2c3d4": {
      "input": "github:lox/agent-skills",
      "type": "git",
      "url": "https://github.com/lox/agent-skills.git",
      "ref": "main",
      "checkout_path": "~/.local/share/skillyard/sources/github-com-lox-agent-skills-a1b2c3d4/repo",
      "last_seen_commit": "0123456789abcdef0123456789abcdef01234567"
    },
    "local-lox-agent-skills-d4c3b2a1": {
      "input": "~/Develop/lox-agent-skills",
      "type": "local",
      "input_path": "~/Develop/lox-agent-skills",
      "resolved_path": "/Users/example/Develop/lox-agent-skills"
    }
  },
  "subscriptions": [
    {
      "source": "github-com-lox-agent-skills-a1b2c3d4",
      "target": "codex",
      "selection": {
        "include": ["*"],
        "exclude": []
      }
    }
  ],
  "installs": [
    {
      "skill": "slack",
      "source": "github-com-lox-agent-skills-a1b2c3d4",
      "source_path": "slack",
      "source_commit": "0123456789abcdef0123456789abcdef01234567",
      "snapshot_path": "~/.local/share/skillyard/sources/github-com-lox-agent-skills-a1b2c3d4/snapshots/0123456789abcdef0123456789abcdef01234567",
      "target": "codex",
      "target_root": "${CODEX_HOME:-~/.codex}/skills",
      "target_root_resolved": "/Users/example/.codex/skills",
      "link_path": "${CODEX_HOME:-~/.codex}/skills/slack",
      "link_path_resolved": "/Users/example/.codex/skills/slack"
    }
  ]
}
```

Lockfile rules:

- Store display expressions for default target roots.
- Store resolved target roots used at install time so `list`, `unsubscribe`, and `unlink` do not change behavior when environment variables change.
- Store requested Git refs on source records; omitted refs track the remote default branch.
- Store each install's resolved source commit and snapshot path; source-level `last_seen_commit` is cache metadata, not proof that every target is installed at that commit.
- Store managed source checkout and snapshot paths under `~/.local/share/skillyard`.
- Store local path sources with both the user-provided path and cleaned absolute path.
- Do not store Git credentials.
- Write atomically by writing a temp file in the same directory and renaming it into place.

## Commands

### `skillyard setup`

Creates the XDG config file and reports detected agent skill directories.

```bash
skillyard setup
skillyard setup --dry-run
skillyard setup --force
skillyard setup --json
```

Rules:

- Write `~/.config/skillyard/config.hcl` when it does not exist.
- Refuse to overwrite an existing config unless `--force` is passed.
- `--dry-run` shows the config that would be written without changing files.
- Detect built-in Codex and Amp agent roots by resolving the same defaults used at runtime.
- Report each detected/configured agent, whether it is enabled, the resolved skills directory, and whether that directory currently exists.
- If a config already exists and `--force` is not passed, load and report the existing configured agents without modifying the file.
- `--json` emits config path, write status, dry-run status, generated content when relevant, and detected agents.

### `skillyard discover`

Inspects a source without changing subscriptions, installed links, or the lockfile. Git sources may be cloned into cache for inspection, but no managed source snapshot or install record is written.

```bash
skillyard discover github:lox/agent-skills
skillyard discover github:lox/slack-cli --json
skillyard discover ~/Develop/lox-agent-skills
skillyard discover ./repo --full-depth
skillyard discover github:lox/agent-skills --ref v1.2.3
```

Human output includes source metadata and candidate skills:

```text
Source
ID                        TYPE  REF     COMMIT        URL                                      ROOT
github-com-lox-skills...  git   v1.2.3  0123456789ab  https://github.com/lox/skills.git        /Users/me/.cache/skillyard/source-.../repo

Skills
NAME   INSTALLABLE  PATH          FINDINGS  WARNINGS     DESCRIPTION
slack  yes          skills/slack  -         has-scripts  Work with Slack messages
```

`discover --json` emits source metadata plus each candidate skill's name, description, path, installability, findings, and warnings.

`discover --ref` inspects a Git branch, tag, or commit instead of the remote default branch.

`discover --full-depth` recursively searches all subdirectories for `SKILL.md`, skipping `.git` and `node_modules`. It is intended for read-only inspection of unusual repositories; subscription reconciliation uses the standard discovery containers so desired state remains predictable.

Plugin manifests are read from `.claude-plugin/` and `.codex-plugin/`. Single-plugin manifests may declare `skills`, and marketplace manifests may declare `metadata.pluginRoot`, `plugins[].source`, and `plugins[].skills`. Manifest-declared paths and containers must stay inside the source root.

### `skillyard show`

Prints one selected skill's `SKILL.md` content to stdout without changing subscriptions, installed links, or the lockfile.

```bash
skillyard show github:lox/agent-skills --include check-pr-description
skillyard show github:lox/agent-skills --include check-pr-description --ref v1.2.3
skillyard show ./skills/review
```

Rules:

- `--ref` reads from a Git branch, tag, or commit instead of the remote default branch.
- If `--include` is omitted and the source has exactly one discovered skill, print that skill.
- If the source has zero skills, multiple skills, no matching include, or an include pattern matches multiple skills, fail with guidance to select exactly one skill.
- If the selected skill has validation findings, fail instead of printing instructions.
- Only the selected `SKILL.md` content is written to stdout.

### `skillyard export`

Writes portable desired state for sources and subscriptions to stdout. Realized install records, snapshot paths, checkout paths, and last-seen commits are omitted. Git refs are preserved.

```bash
skillyard export > skillyard.lock.json
skillyard export --target codex > skillyard.lock.json
```

Rules:

- `--target` filters subscriptions to one configured target.
- Only sources referenced by exported subscriptions are included.
- Git sources keep their input, type, URL, and ref; machine-local checkout and last-seen commit fields are omitted.

### `skillyard apply`

Reconciles the current machine to an exported desired-state file through the same source resolution, validation, preflight, link, and lockfile write path as `sync`.

```bash
skillyard apply skillyard.lock.json --dry-run
skillyard apply skillyard.lock.json --target codex
```

Rules:

- When `--target` is omitted, replace all current subscriptions with the file's subscriptions.
- When `--target` is set, replace only that target's subscriptions; other targets remain unchanged.
- `--dry-run` shows the reconciliation plan without changing links or lockfile state.
- `--json` emits the same machine-readable reconciliation result as `subscribe` and `sync`.
- `--force` has the same conflict-replacement meaning as `sync --force`.

### `skillyard subscribe`

Adds or updates a subscription from one source into one or more global targets, then reconciles that desired state unless `--dry-run` is set.

```bash
skillyard subscribe github:lox/agent-skills --include '*' --target codex
skillyard subscribe github:lox/agent-skills --include '*' --ref v1.2.3 --target codex
skillyard subscribe github:lox/agent-skills --include '*' --exclude consulting-librarian --target amp
skillyard subscribe git@github.com:org/private-skills.git --include deploy-review --target codex
skillyard subscribe github:lox/slack-cli --dry-run
skillyard subscribe ~/Develop/lox-agent-skills --include slack --target amp
```

Flags:

```text
--include <pattern>   include matching skills; repeatable; defaults to the only skill when the source has exactly one
--exclude <pattern>   exclude matching skills after includes; repeatable
--target <name>       install target; repeatable; defaults to all enabled configured agents
--name <source-id>    source id override
--ref <ref>           Git branch, tag, or commit to track
--force               replace unmanaged symlinks and drifted managed links
--dry-run             show clone/link plan without writing target links or lockfile
--json                machine-readable result
```

Rules:

- If `--target` is omitted, default to all enabled configured agents.
- If `--target` is provided, it must name an enabled configured agent.
- `--ref` is valid only for Git sources.
- When `--ref` is set, sync fetches refs and checks out the requested branch, tag, or commit before snapshotting.
- If `--include` is omitted and the source has exactly one discovered skill, subscribe to that skill by name.
- If `--include` is omitted and the source has zero or multiple discovered skills, fail with guidance to pass `--include <skill>` or `--include '*'`.
- Evaluate includes first, then excludes.
- Treat `'*'` as an explicit broad subscription to all current and future matching skills from the source.
- Pattern matching is against skill names, not paths.
- `--dry-run` may use a temporary clone or existing cache to produce an accurate plan.
- `--dry-run` does not create target roots, persist new source state, create symlinks, or update the lockfile.
- On failure, do not record the subscription unless reconciliation succeeds.

### `skillyard list`

Shows recorded subscriptions, managed installs with computed link status, and unmanaged entries currently present in Codex and Amp skill roots.

```text
Subscriptions
TARGET SOURCE                   INCLUDE EXCLUDE
codex  github:lox/agent-skills  *       -
amp    github:lox/agent-skills  *       consulting-librarian

Managed
SKILL   TARGET SOURCE                         STATUS          PATH
slack   codex  github:lox/agent-skills        linked          /Users/me/.codex/skills/slack
slack   amp    github:lox/agent-skills        linked          /Users/me/.config/agents/skills/slack
notion  codex  git@github.com:org/private...  missing-target  /Users/me/.codex/skills/notion

Unmanaged
SKILL          TARGET KIND            PATH                                      LINK_TARGET
local-review   codex  symlink         /Users/me/.codex/skills/local-review      /Users/me/Develop/skills/local-review
old-skill      amp    dir             /Users/me/.config/agents/skills/old-skill -
broken-skill   codex  broken-symlink  /Users/me/.codex/skills/broken-skill      /missing/path
```

Statuses:

- `linked`: symlink exists and resolves to recorded source skill.
- `mutable-source`: symlink resolves to a local path source that can change outside `skillyard`.
- `drifted`: symlink exists but does not match the lockfile's recorded source path.
- `missing-target`: link path does not exist.
- `wrong-target`: link path exists but points elsewhere.
- `missing-source`: source checkout or source skill no longer exists.
- `invalid-skill`: source skill no longer validates.

Unmanaged entries are filesystem entries in a target skill root whose basename is not recorded as a managed install for that target. Hidden entries and `.skillyard` temporary links are ignored. `kind` is one of `symlink`, `broken-symlink`, `dir`, `file`, or `other`.

Empty human-output sections render as `none`. `list --json` emits subscriptions, managed install records with computed status, and unmanaged entries with paths and link targets.

### `skillyard sync`

Refreshes managed Git sources and reconciles installed links with recorded subscriptions.

```bash
skillyard sync
skillyard sync github:lox/agent-skills
skillyard sync --target amp
skillyard sync --dry-run
```

Rules:

- Build a plan from the lockfile's subscriptions before mutating targets.
- Use the shared reconciler without editing subscription records.
- Report Git source commit movement as `source-update` actions with `from` and `to` commit fields.
- Report retargeted Git skill links with `from` and `to` commit fields when the source commit changed.
- Link every valid skill matching a subscription's include/exclude patterns.
- For narrow includes such as `slack`, keep only those matching skills current.
- For broad includes such as `'*'`, link newly discovered valid skills automatically.
- Remove managed links for skills that no longer match any subscription, including skills removed from the upstream source.
- Refuse unmanaged target conflicts.
- `--dry-run` shows added, unchanged, blocked, and missing-source changes without changing links, persisted source state, snapshots, or lockfile.

### `skillyard unsubscribe`

Changes desired state so selected skills stop being managed, then reconciles matching managed links.

```bash
skillyard unsubscribe slack --target codex
skillyard unsubscribe slack --target codex --target amp
```

Rules:

- If the skill is explicitly included in a matching subscription, remove that include.
- If the skill is included by a broader pattern such as `'*'`, add the skill name to that subscription's excludes.
- Remove matching managed symlinks after updating desired state.
- Refuse to remove non-symlink paths.
- Refuse to remove symlinks that no longer match the recorded source unless `--force` is passed.
- Leave source checkouts and snapshots in place.
- Remove empty subscriptions that have no includes left.
- On failure, do not record the desired-state edit unless reconciliation succeeds.

### `skillyard unlink`

Removes realized links without changing subscriptions. This is a repair/debug command; a later `sync` may recreate the link if a subscription still matches it.

```bash
skillyard unlink slack --target codex
```

Rules:

- Remove only lockfile-recorded symlinks.
- Refuse to remove non-symlink paths.
- Refuse to remove symlinks that no longer match the recorded source unless `--force` is passed.
- Remove the install record after the symlink is removed.
- On failure, do not update the lockfile unless the link mutation succeeds.

### `skillyard doctor`

Reports local configuration and obvious state problems:

- Codex global skill root and existence.
- Amp global skill root and existence.
- `skillyard` config directory.
- `skillyard` source directory.
- Lockfile path and existence.
- System `git` path and version.
- Managed install count.

## Package Layout

```text
cmd/skillyard/main.go
internal/cmd/          # Kong command structs and orchestration
internal/agent/        # configured global target registry
internal/config/       # XDG HCL config loading
internal/gitexec/      # system git clone/fetch/rev-parse helpers
internal/skill/        # discovery and SKILL.md parsing
internal/state/        # skillyard.lock.json read/write and install records
internal/materialize/  # symlink create/remove/status
internal/sync/         # reconcile sources and subscriptions
```

Dependencies:

- `github.com/alecthomas/kong` for CLI parsing.
- `github.com/charmbracelet/log` for stderr diagnostics.
- `gopkg.in/yaml.v3` for frontmatter parsing.

## Implementation Plan

Build the global source linker in one PR.

Scope:

- Initialize the Go module.
- Add Kong command parsing and Charmbracelet logging.
- Resolve Codex and Amp global skill roots.
- Resolve XDG config/data/cache paths.
- Implement lockfile read/write with atomic updates.
- Normalize source inputs.
- Clone Git sources with the system `git` binary.
- Create immutable source snapshots for resolved Git commits.
- Support local path sources.
- Discover skill directories from root, direct children, `skills/`, `skills/<category>/`, `.agents/skills/`, `.claude/skills/`, and plugin manifests.
- Parse and validate `SKILL.md` frontmatter.
- Implement `setup`, `subscribe`, `list`, `sync`, `unsubscribe`, `unlink`, and `doctor`.
- Implement `discover` for read-only source inspection.
- Implement `show` for one-off skill instruction output.
- Implement `export` and `apply` for portable desired-state files.
- Implement one reconciler shared by `subscribe`, `sync`, `unsubscribe`, and `unlink`.
- Support repeated `--include` and repeated `--exclude` selection.
- Create and remove managed symlinks.
- Refuse unmanaged conflicts.
- Add `--dry-run` and `--json` for `subscribe`.
- Add `--dry-run` and `--json` for `sync`.
- Add `--json` for `list`.
- Add `--dry-run`, `--force`, and `--json` for `setup`.
- Add fixture Git repositories in tests to exercise Git behavior without network.

Definition of done:

- `skillyard subscribe <local-git-repo> --include valid --target codex --dry-run` shows the planned subscription and link.
- `skillyard setup --dry-run` shows the generated HCL config and detected agents.
- `skillyard setup` creates `config.hcl` when missing and does not overwrite it on repeated runs.
- `skillyard subscribe <single-skill-local-source> --dry-run` defaults to all enabled configured agents and the single discovered skill.
- `skillyard subscribe <local-git-repo> --include valid --target codex` creates a symlink in a test target root.
- `skillyard list --json` reports the managed install record and link status.
- `skillyard sync --dry-run` reports newly available skills for an `--include '*'` subscription.
- `skillyard sync` links newly added valid skills for an `--include '*'` subscription.
- `skillyard unsubscribe valid --target codex` edits desired state and removes the managed symlink.
- `skillyard unlink valid --target codex` removes only the managed symlink.
- Existing unmanaged target paths block `subscribe` and `sync`.
- Re-running `subscribe` for the same source/include/target is idempotent.
- Tests cover public/private-equivalent Git by using system `git` against local repositories.

## Validation Strategy

Unit tests:

- Source input normalization.
- Source ID stability and collision suffixing.
- Agent target root resolution with built-ins, HCL config overrides, disabled agents, custom agents, and `CODEX_HOME`.
- Lockfile read/write and atomic replacement.
- Per-install source commit and snapshot tracking.
- Skill discovery and frontmatter validation.
- Selection with `--include` and `--exclude`.
- Sync reconciliation for broad and narrow includes.
- Sync preflight failure leaves target links unchanged.
- Sync apply failure leaves the lockfile untouched.
- Subscribe and unsubscribe failures leave the lockfile untouched.
- Symlink status classification.
- Mutable local source status.
- `unsubscribe` edits explicit includes or broad-pattern excludes correctly.
- Refusing unmanaged conflicts.

Integration tests:

- Create a local Git repo with valid and invalid skills.
- Clone from local Git repositories to exercise `git clone`.
- Subscribe one skill to a temporary Codex target root.
- Add a broad `--include '*'` subscription to temporary Codex and Amp target roots.
- Unsubscribe managed skills.
- Unlink managed links.
- Verify repeated subscribe is idempotent.
- Add a new skill to a managed test source and verify `sync` installs it for an `--include '*'` subscription.
- Remove a skill from a managed test source and verify `sync` unlinks its managed target link.
- Run `sync --target amp` and verify Codex install records from the same source keep their previous commit and snapshot.
- Verify sync does not write the lockfile when a target mutation fails.
- Verify subscribe and unsubscribe use the same reconciliation failure behavior as sync.
- Verify lockfile state after subscribe/sync/unsubscribe/unlink.

Manual smoke tests:

```bash
go test ./...
go run ./cmd/skillyard doctor
go run ./cmd/skillyard subscribe ~/Develop/lox-agent-skills --include slack --target codex --dry-run
go run ./cmd/skillyard sync --dry-run
go run ./cmd/skillyard list
```

Before running a real non-dry-run smoke test against `~/.codex/skills` or `~/.config/agents/skills`, inspect the target path and confirm it is absent or already managed.

## Upstream Lessons Applied

The upstream `skills` CLI has already validated several useful UX choices:

- GitHub shorthand, full Git URLs, SSH Git URLs, and local paths are worth supporting.
- `subscribe`, `sync`, `list`, and explicit unsubscribe/unlink commands are the right core command shape for desired-state management.
- Symlink-first installation is a good default because it keeps a single source of truth.
- Explicit configured agent targets are clearer than writing to universal locations.
- JSON output matters for non-interactive use.

`skillyard` applies those lessons to a narrower operating model: configured global skill roots, explicit target selection when needed, managed symlink ownership, and generated state under XDG paths.
