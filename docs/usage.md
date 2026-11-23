# Grand Git usage reference

[Quick start](../README.md) · [Quick start in Russian](../README.ru.md) · [Installation](installation.md)

## Session identity and creation

`gg list` and `gg list --json` show registered sessions across known projects and directories, including stopped tasks. Each session has a global ID such as `3:7` (group:session). Use this ID with `gg start`, `gg stop` or `gg delete` from any directory. In a Git project, these commands also accept an exact branch name. For example, `gg stop 1` addresses a branch named `1`. A stopped session keeps its branch name reserved. A fresh project has no sessions until you create one.

Run the creation examples in a Git worktree with at least one commit. The `--base` example assumes a branch named `release/1.0`; substitute a branch or commit from your project.

```sh
gg create feat/login
gg create fix/release --base release/1.0
gg create feat/clean --no-copy
gg create feat/agent --template codex
gg create feat/server --choose-template
gg list --json
```

Without `--base`, new branches start at the calling worktree's HEAD. An existing free branch can be attached without `--base`; a branch checked out elsewhere cannot be taken over. In an interactive terminal, `gg create` prompts for a missing Git branch name. Scripts must supply the branch name. `--choose-template` requires interactive input and output and cannot be combined with `--template`.

New folders use `<project>__sessions/<branch>`, preserving slashes. Existing paths are not moved. Creation starts the terminal in the background and returns without waiting for application success. Bare repositories are unsupported.

Git checkout hooks run during worktree creation. A hook can use `gg list --json` to inspect committed sessions. Once the hook exits, a background process it started can keep running while `gg create` finishes setting up the new terminal.

Outside Git, `gg create [name]` creates a persistent terminal in the current directory. No name or branch prompt is required; the default name is the directory name and duplicate names are allowed. `--base` and `--no-copy` require Git. Deleting a directory session only removes processes and registration, preserving all its files, even if the folder later becomes a Git repository.

Groups are recorded under `$XDG_STATE_HOME/grand-git` when `XDG_STATE_HOME` is an absolute path, or `~/.local/state/grand-git` otherwise. Git worktrees share one group; ordinary directories have separate groups. An existing project with older gg session records joins the catalog when you run a workspace command such as `gg list` there.

## Environment copying

Worktree creation copies untracked files covered by Git ignore rules. For example, `.env`, `.agents` and `node_modules` are copied if your ignore rules cover them. Use `--no-copy` to skip this step. If an explicit local base branch has a worktree, that worktree supplies the files. Otherwise gg copies them from the calling worktree.

Copies are independent: Linux attempts reflink with ordinary-copy fallback; macOS uses ordinary copying. File permissions and mtime are preserved. Symlinks are copied without following them. Files already present in the destination branch are not replaced. `.git`, shared ignored `.gg`, nested repositories and special files are excluded. Environments are not synchronized afterward, and npm dependencies are not automatically checked or installed.

## Lifecycle and safeguards

```sh
gg stop feat/login
gg start feat/login --template shell
gg delete feat/login
```

`stop` ends shells, agents, servers and background jobs in the session's terminals. It preserves files and registration. `start` creates a new terminal and runs the current definition of the saved template. It does not resume a stopped agent process. Starting an active session is idempotent; changing its template requires stopping first.

`delete` removes a managed worktree and record, preserving its Git branch. Without `--force`, changed and ordinary untracked files block deletion. Ignored files are removed with the worktree. Untouched integration-generated files may be removed; user modifications remain protected.

Use `gg stop --all` or `gg delete --all` to act on all sessions in the current Git project, across its folder sections. Outside Git, `--all` targets only the current directory group. Bulk deletion checks every target before stopping any processes. Deletion and bulk stop ask for confirmation. Pass `--yes` to confirm in a script. Discarding modified or ordinary untracked worktree files also requires `--force`.

Source and registered external worktrees are protected from deletion even with `--force`. Both bulk commands stop their terminals and preserve their directories and branches. Inactive legacy source records are retired. Ordinary and managed stopped sessions stay registered until deleted.

Stopping or deleting a background task preserves the current window and focus. Closing the currently open task returns to the service list. Operations that close their invoking terminal finish in a detached worker. External failures may leave a partial result, reported explicitly.

## Folder sections and dashboard startup

`gg dashboard` opens the saved section list. On first use, press + to add a directory. Type or paste a path, or navigate folders with the mouse, arrows and Enter. With the folder list focused, Backspace or Left opens the parent directory. Tab moves between the path, folder list and buttons. Ctrl+S confirms and Esc cancels. Relative paths use the original calling directory, `~/` expands the home directory, and hidden directories are listed. Invalid or inaccessible paths retain the form for correction.

Sections keep exact canonical paths. A Git root and its subdirectory can have separate sections while sharing the same repository, settings, tmux server and public workspace ID. Symlink aliases select the same existing section. Adding a section creates no terminal; T on it opens a persistent plain shell in its exact directory. N creates a worktree from that directory's HEAD, or an ordinary directory session. Worktree paths remain `<project>__sessions/<branch>`. Creation from a selected session retains its section and uses that session's directory as the base context.

`gg create` assigns new sessions to the section for the exact calling directory, creating that section when needed. Existing catalog entries migrate once to root sections; old session records keep their public IDs. X removes a section only when it has no registered sessions, including stopped records. Removal preserves files and the underlying workspace; removed empty sections stay removed on restart.

Q and connection loss close the dashboard without stopping persistent terminals. The same gg process continues across project transitions. Already running legacy clients retain their owned temporary-session cleanup, including token checks that prevent stale workers from cleaning another invocation. New dashboard launches have no temporary session.

## Dashboard controls

Ctrl+W cycles through the session list and terminal panes in screen order, wrapping to the list after the last pane. Ordinary arrows, Tab, Shift+Tab, Ctrl+G and Ctrl+P belong to the terminal application. Esc and Ctrl+C inside a user terminal retain their ordinary behavior.

The terminal area shows each tmux pane title above its contents. Application colors, including full-row backgrounds, stay inside their terminal pane when split or resized. A green status line remains visible below the panes with the current project, active session count and Ctrl+W hint, including in compact mode.

Folders are selectable and stay visible when empty in the unfiltered list. Within each section, worktrees and plain terminals share a list in creation order, marked with ⎇ and >_. Existing legacy source and temporary records remain accessible. Duplicate folder names include paths. Each session uses one row: ●/○ indicates live terminal activity, ▸ marks the open terminal, and › plus highlighting identifies another selected row. Details below the list show the path, terminal count, agent, Git and open state; hovering previews another session or folder without changing selection or terminal focus. Leaving the list restores selected-item details.

A single session click or Enter opens it, starting a stopped session with its saved template. Arrows/J/K browse folders and sessions without opening them. Right-click opens target actions beside the pointer; ? or Shift+F10 opens the selected item's actions. Menus accept arrows/Enter, shortcuts and mouse clicks; Esc closes them, and clicking outside dismisses a pointer menu. Actions stay bound to their original group/session identity across refresh. Ctrl+W cycles the list and terminal panes. Forms and confirmations take input before the list or terminal.

| With list focus | Action |
| --- | --- |
| + / X | Add a folder section / remove the selected empty section |
| N | Create from the selected session or section directory, retaining that section |
| T on a session | Start a stopped session, or choose Below/Right for an additional plain shell |
| T on a folder | Open a persistent plain shell in its exact directory, including inside Git, using the reserved shell template |
| S | Start/confirm stop for a session, or confirm Stop all running sessions for a folder |
| D | Confirm deletion of an unprotected session, or Delete all sessions in a folder |
| A | Toggle All/Active filtering |
| , | Settings for the open project |
| Q | Exit while persistent sessions keep running |

Section confirmations capture that section's targets regardless of the current filter. Disappeared targets are skipped and sessions created later are excluded. Delete all checks every surviving managed worktree before stopping any process. Modified or ordinary untracked worktree files block deletion. Protected directories, ordinary directory files and Git branches are preserved. Use the public ID to start, stop or delete a persistent plain terminal in Git. Deleting it removes its processes and record and preserves repository files.

Creation, template selection, placement, settings and confirmations appear as bounded overlays in new dashboards. Controls accept mouse and keyboard input, and creation errors preserve entered values. Git creation requires a branch and uses HEAD at the selected directory; ordinary creation offers an optional prefilled name. Tab opens the real template catalog; Esc returns without changing the name or saved choice. Settings belong to the open project, even when another section is selected; before opening a project, settings use global scope.

Search is available only above ten session rows after All/Active filtering, counted across groups before text filtering or viewport clipping. Section headings do not count; existing legacy temporary session rows do. At ten or fewer candidates the query clears and the control disappears. / or Up above the first item focuses available search. While editing, only Clear and Down-to-list appear in the footer: Clear keeps input focus; Down, Enter or Down-to-list applies the query and restores list controls. Clicking an applied field resumes editing. Esc clears the query, reveals and selects the open terminal, and keeps list focus; it retains activity filtering unless that hides the open row. In compact mode a subsequent Esc closes the list only when a terminal is open.

Successful background stop and delete operations show a green notice for five seconds after completion. Refresh, resize and returning to the list do not renew the deadline; newer notifications have independent deadlines. Errors remain visible. Agent `done` is a completed turn, not an expiring operation notice. `NO_COLOR` disables colors.

The list uses 25% of the window, bounded to 28–56 columns. Below 100 columns or 16 rows it hides; Ctrl+W opens a full-screen list after the last terminal pane, and Ctrl+W or Esc closes it when a terminal is open. Without an open terminal the list remains accessible. Resizing preserves running processes and adjusts pane sizes to fit the window. `Active <active>/<total>` counts session records and active sessions across available groups, including running legacy source records and excluding service panes. An unavailable group is reported without hiding healthy groups. `list --json` still prints valid healthy records but exits nonzero and reports the error on stderr.

## Settings and templates

```sh
gg config
gg set language ru
gg set default_template codex --local
gg template list --json
gg template show codex
```

`gg config` shows effective values and their sources. Project settings in the primary worktree's `.gg/config.yaml` override globals, and global settings override built-ins. Globals use `$XDG_CONFIG_HOME/grand-git/config.yaml` when `XDG_CONFIG_HOME` is an absolute path, or `~/.config/grand-git/config.yaml` otherwise. `set/reset` and compatible `config set/reset` default to globals. `--local` and `--project` select the project. Changes take effect without restarting terminals. Legacy TOML is read until that layer has YAML. Its first write migrates supported settings and keeps the old file. Resetting the last key leaves the YAML mapping `{}` so old TOML values stay inactive.

Outside Git, only global settings and templates apply; a directory’s `.gg` is ignored, and `--local` is rejected. Dashboard settings belong to its open group, rather than the highlighted foreign row.

Defaults: `language=en`, `default_template=shell`. Dashboard settings save all selected fields to Global or Local with Enter; Esc cancels.

Built-ins are `shell`, `codex`, `claude` and `npm-dev`. Definitions contain a `commands` list. The `templates/` directory beside the global config file can override built-ins, and the primary worktree's `.gg/templates/` can override globals. The name `shell` is reserved. `gg template list --json` returns names and sources, and `gg template show NAME` prints the selected definition as YAML. A session saves the template name and reads its current definition when restarted. If the definition goes missing, restore it or run `gg start ID --template shell` with the session ID from `gg list`. Default changes affect only future sessions. Extra terminals open plain shells.

Configured commands run sequentially in one interactive sh/bash/zsh context, after registration succeeds. A nonzero command skips the remaining commands. On return or Ctrl+C, gg displays the exit status and opens an interactive shell. An explicit shell `exit` or `exec` retains its usual meaning. Each terminal receives `GG_BRANCH` (empty for plain terminals), `GG_WORKTREE` (session directory) and `GG_PROJECT_ROOT` (primary worktree or ordinary group directory). Install any programs and dependencies your template invokes. See [template examples](templates.md).

## Codex hooks and automation

`gg init` merges project-local `.codex/hooks.json` and `.codex/config.toml`, preserving unrelated content. Backups of changed, nonempty originals are stored under `grand-git/hook-backups/` in the Git common directory. Future worktrees created by gg receive the hooks too. Repeat initialization does not duplicate handlers. In Codex, inspect and trust handlers through `/hooks`. One primary Codex session is tracked per task. Other agents without integrated hooks show shell/unknown status.

`gg list --json` returns public `id` (a string such as `3:7`), `name`, `kind` (`source`, `worktree`, `terminal`, `temporary`), and `workspace` (`id`, `type`, `name`, `path`), plus `branch`, `base`, `path`, `created_at`, `protected`, `active`, `temporary`, `terminals`, `agent_status` and `template`. Copy metadata is optional. When zero files were copied, `copied_files` is omitted even if `copy_source` is present. The branch and base are empty for persistent plain and temporary terminal records. A plain terminal created in a Git section has `workspace.type` equal to `git`. Machine keys and status codes stay in English for every language setting. Agent codes are `ready`, `working`, `waiting`, `done`, `interrupted`, `shell`, `stopped` and `unknown`.

Use ordinary Git commands inside a worktree for commits and pushes, and your hosting provider's tools to create pull or merge requests. The source checkout includes the agent usage skill at `docs/skills/grand-git/SKILL.md`.
