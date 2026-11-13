# Grand Git reference

[Quick start in Russian](../README.ru.md) · [Concise reference](usage.md) · [Installation](installation.md)

`gg` is a CLI for parallel tasks across Git projects and ordinary directories. A Git task gets a separate worktree; an ordinary session gets a terminal in its directory. A dashboard invocation keeps the same gg process while switching projects. Dashboard opens the saved section list without creating a session or opening a terminal; Q preserves persistent sessions.

Linux/WSL2 or macOS and tmux 3.4+ are required; Git 2.31+ is required for Git projects, while ordinary directories work without Git. Building requires Go 1.26+. Node.js is not needed to run the binary. Bare repositories are not supported.

```bash
go build -o bin/gg ./cmd/gg
```

Use the built binary by its path or a configured `gg` command. You do not need to start tmux separately. `gg` without arguments shows help.

## Sessions

In a Git project's terminal:

```bash
gg create feat/login
gg create fix/release --base release/1.0
gg create feat/clean --no-copy
gg list
gg list --json
gg dashboard
```

Creation starts a terminal **in the background** and returns control. A new dashboard task opens immediately. The default is a plain shell; `--template codex`, another template or `default_template` configures startup commands. The create result confirms session creation, not application success.

Inside Git, interactive `create` without a branch asks only for its name. Optional parameters use flags. `--choose-template` opens an explicit picker; the dashboard form exposes it through Tab. `description` has been removed. New branches default to the calling directory's HEAD; `--base` selects another branch or commit. Existing free branches attach without `--base`. Branches used by another worktree are not switched.

New directories sit beside the primary directory: `<project>__sessions/<branch>`, for example `scom-web-vcs__sessions/feat/feat1`. Old worktrees keep their paths and running processes. Duplicate session names are rejected even when stopped.

Commands accept the positional public ID **group:session**, for example `gg delete 3:7 --yes`, from any directory. Exact branch names address the current Git project: `gg delete 1` means branch `1`. `gg list` and JSON show all known groups with IDs.

Outside Git, `gg create [name]` creates a persistent terminal in the exact current directory with no branch prompt. Names are optional, default to the directory name and may repeat. Deletion stops processes and removes the record while preserving every file, even if Git was later initialized there. `--base` and `--no-copy` are rejected outside Git.

The group catalog lives in `$XDG_STATE_HOME/grand-git` or `~/.local/state/grand-git`. Git worktrees share a group; ordinary directories have separate groups. An old project registers on its first working invocation of the updated gg; there is no disk scan.

### Environment

Creation copies untracked Git-ignored files: `.env`, `.agents`, `node_modules` and others, respecting nested ignore rules. If an explicitly selected local base branch has a worktree, that is the environment source; otherwise the calling worktree is used. `--no-copy` disables copying.

Regular files are independent: Linux uses reflink when supported, otherwise a full copy; macOS uses a full copy. Permissions and mtime are preserved. Symlinks are preserved without dereferencing. Target branch files are not overwritten; `.git`, shared ignored `.gg`, nested repositories and special files are excluded. Environments do not synchronize afterward; npm dependencies are not installed or checked automatically.

### Starting, stopping and deleting

```bash
gg stop feat/login                 # stop processes; keep files and record
gg start feat/login                # current template; no duplicate active terminal
gg stop --all --yes                # stop everything in the current project
gg delete feat/login --yes         # remove worktree and record; keep Git branch
gg delete --all --yes              # stop everything and remove managed tasks
gg delete feat/login --force --yes # explicitly discard worktree changes
```

Stopping terminates shells, agents and servers in managed terminals. Restarting runs the current saved template in a new terminal. This is a new process, not a resumed agent. `gg start feat/login --template shell` changes a stopped session's template; active sessions reject the change. Opening a stopped session in the dashboard also starts it.

The source directory and registered existing worktrees are protected from deletion even with `--force`. Bulk stop includes their terminals; bulk deletion preserves their directories and branches while retiring stopped legacy source records. Source sessions are no longer automatically registered; only running legacy sources remain until stopped. The list remains available without active terminals. Branch arguments and --all address the current Git project; outside Git, --all is limited to the current directory. A composite ID addresses one session in any group.

Deletion and bulk operations require confirmation; without a TTY, use `--yes`. This flag does not authorize file loss: modified and ordinary untracked files remain protected without `--force`. Bulk deletion checks every target first; a dirty task cancels it before processes stop. Ignored environment files and caches are removed with the worktree. Unmodified integration files created by the tool may be removed; user edits to them are protected.

An operation invoked from a terminal it closes finishes in a separate service process; its result is available in the dashboard and `list`. An external deletion failure may leave an explicitly reported partial result. Git branches survive every ordinary session deletion.

## Dashboard

The dashboard follows the terminal layout and interactions in [the local design](../design/README.md). See [Dashboard controls](usage.md#dashboard-controls) for the complete behavior.

Press + to add an existing directory through the path field or folder browser; Ctrl+S confirms and Esc cancels. Relative paths use the original calling directory and ~/ expands home. Browsing and cancellation create nothing. Sections retain exact canonical paths: a Git root and its subdirectory can be separate sections with a shared workspace, settings and public IDs. Adding a duplicate selects the existing section. X removes only an empty section, including no stopped sessions, and preserves files and the workspace record.

Folders are selectable and contain a mixed list of worktrees (⎇) and plain terminals (>_) in creation order. Empty sections stay available without filters. A single session click or Enter opens/starts it; arrows and J/K browse without opening. Hover previews details without moving selection or terminal focus. Right-click opens actions beside the pointer; ? and Shift+F10 open actions for the selected item. Menus and confirmations retain their original target identities.

N creates a session from the selected directory and retains its section. T on a folder opens a persistent shell at that exact path, including inside Git; T on an active session asks Below/Right for another shell. S and D confirm session or section lifecycle actions. Section confirmations include only that section's full target set regardless of filtering, skip vanished targets and exclude later records. CLI --all remains workspace-wide. Dirty managed worktrees block deletion before any process stops; Git branches, protected directories and ordinary files remain.

CLI creation assigns each session to its exact calling directory's section, adding it when needed. Existing catalogs migrate once to root sections without changing session IDs, worktree paths or processes. Removing an empty section persists across dashboard restarts.

Search is available above ten activity-filtered session rows. Up above the first item or / focuses it. During editing, only Clear and Down-to-list are shown. Clear retains input focus; Down/Enter applies the query. Esc clears search, reveals and selects the open terminal and keeps list focus. At ten or fewer candidates search clears and disappears.

New dashboards display mouse-enabled forms and dialogs over the terminal. Settings apply to the open project, or global scope before opening one. Ctrl+W cycles list and terminal panes; ordinary application keys pass through with terminal focus. The sidebar uses 25% width within 28–56 columns, hiding below 100 columns or 16 rows. Compact list access uses Ctrl+W; Esc closes it after canceling any active interaction only when a terminal is open. Resizing preserves terminal processes and relative splits.

The default interface is English; language=ru localizes controls and messages. Focus and activity remain visible with NO_COLOR. Each pane has a title and a green project/activity line stays visible below the terminal. Successful operation notices expire five seconds after completion; errors remain visible.

Launching dashboard never adds its calling directory or creates a temporary shell. Q and disconnection preserve persistent records and processes. Existing legacy temporary rows remain visible; their original clients retain invocation-owned cleanup. Legacy clients retain their existing attachment path; upgrades replace service panes without restarting user processes.

## Settings

```bash
gg config
gg set language ru
gg set default_template codex --local
```

| Setting | Values | Default |
| --- | --- | --- |
| language | en, ru | en |
| default_template | template name | shell |

Ctrl+W moves to the next panel; ordinary arrows and Tab remain available to terminal applications.

Precedence: built-ins → global → local. The global file is `$XDG_CONFIG_HOME/grand-git/config.yaml` or `~/.config/grand-git/config.yaml`. The local file is `.gg/config.yaml` in the primary working directory, shared by all worktrees. `gg set/reset` and legacy `gg config set/reset` default to global scope; `--local` and compatible `--project` select local scope. `gg config` shows effective values and sources.

Legacy TOML is read until YAML exists at that level. The first save migrates active settings at that level to YAML while retaining the old file; obsolete navigation values are omitted. Reset removes only the selected key and keeps YAML, so old TOML values never return. The registry, hooks and service data remain in the Git common directory.

In an ordinary directory, .gg is ignored: only global settings and templates apply, and --local is rejected. Dashboard settings belong to the open context, not a selected row from another group.

The dashboard comma form selects language, default_template and Global/Local save scope. Enter saves all values in one update; Esc cancels. Settings changes do not restart terminals. JSON keeps English keys and codes regardless of language.

## Startup templates

```bash
gg template list
gg template show codex
gg create feat/agent --template codex
gg create feat/server --choose-template
```

Built-ins are `shell`, `codex`, `claude`, `npm-dev`. Custom YAML lives in the primary directory's `.gg/templates/` or global `grand-git/templates/`. A local template completely replaces the same global or built-in name. `shell` is reserved and always opens a shell.

For example, `.gg/templates/backend.yaml`:

```yaml
commands:
  - npm run prepare
  - codex
```

A session remembers its template name and reads current commands on its next start. The default affects only new create calls; legacy and source sessions use shell. Additional terminals are plain shells. Commands run sequentially after session registration; the first failure stops the sequence. Completion or Ctrl+C leaves output, exit status and a shell. sh/bash/zsh are required; users install applications and dependencies beforehand.

[Codex, Claude, Node and npm examples, environment variables and selection details](templates.md). For automation, `gg list --json` includes `template`, while `gg template list --json` returns `name` and `source`.

## Codex statuses

`gg init` merges project `.codex/hooks.json` and `.codex/config.toml` with existing settings, backing them up under `grand-git/hook-backups/` in the Git common directory. Repeated runs do not duplicate handlers; new tasks receive integration. Global settings and trust are unchanged.

After installation, open **`/hooks`** in Codex, inspect the handlers and grant trust. Without trust the terminal works, but exact statuses are not guaranteed. One primary Codex is tracked per session; other CLIs without hooks show shell or unknown state.

Machine codes: `ready`, `working`, `waiting`, `done`, `interrupted`, `shell`, `stopped`, `unknown`. `done` means the agent's turn has ended, not the entire task. Legacy saved Russian statuses remain supported.

## Git and the skill

Ordinary Git commands work in the worktree terminal. After committing, the first `git push -u origin feat/login` publishes the branch; subsequent changes use `git push`. Create PRs/MRs with your hosting provider's tools.

The [Grand Git usage skill](skills/grand-git/SKILL.md) is for agents using gg in user projects. Reference it by path or install it through your agent's standard mechanism. There is no automatic global installation.

The project Codex `skill-maintainer` profile reviews the skill after public interface changes. [AGENTS.md](../AGENTS.md) requires a separate review invocation and a recorded result.

## Checks and specifications

```bash
sh scripts/test.sh
sh scripts/check-specs.sh
```

The first script builds the binary, runs Go tests, go vet and integration scenarios using real Git/tmux/PTY in temporary projects. The race detector runs when CGO is available. The second validates OpenSpec through a pinned npx package and requires Node.js 22.12+.

Current requirements live in [openspec/specs](../openspec/specs), with context in [openspec/project.md](../openspec/project.md). Behavior changes update code, tests, help, skill and specifications; verification.md records the evidence.

## Global JSON

Each list --json record includes `id` as a string such as `3:7`, `name`, `kind` (`source`, `worktree`, `terminal`, `temporary`) and `workspace` with `id`, `type`, `name`, `path`, plus existing activity, template and path fields. `branch` and `base` are empty for ordinary/temporary terminals; description is absent. Group and session IDs persist across invocations. Machine codes remain English.
