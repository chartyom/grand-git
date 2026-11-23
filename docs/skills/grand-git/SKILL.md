---
name: grand-git
description: Use Grand Git (gg) to operate sessions in exact-path dashboard sections across Git projects and ordinary directories, startup templates, persistent terminals, shared YAML settings, and local Codex status hooks. Apply to using gg rather than developing Grand Git itself.
---

# Grand Git

Use the available `gg` executable from a Git worktree or an ordinary directory. Grand Git runs on Linux and macOS on amd64/arm64, and on Windows through the Linux build inside WSL2. Terminals require tmux 3.4+. Source builds containing the worktree discovery fix (`fcf9d51`) support Git 2.31+ for worktrees. When using v1.0.0, install Git 2.36+ because its worktree discovery requires `git worktree list -z`. Ordinary directory sessions can run without Git installed. Bare repositories are unsupported. Use an existing binary or the user's supplied executable path.

Start with `gg list --json` when identifying existing sessions. Listing is global: it includes active and stopped managed and ordinary sessions, plus retained legacy temporary and source records, from all known Git projects and ordinary directories irrespective of the calling directory. A new project can have an empty list; listing does not invent a `main` or other source session. Select the intended workspace before acting; a branch name is only local to its Git project, while the public `id` is global. Use `gg --help` and command-specific help to check the installed interface. `gg` without a command shows help; open the terminal interface explicitly with `gg dashboard`. Help and `gg --version` work outside a repository, without a TTY, Git or tmux. Version output includes version, commit and build date; local builds report `dev`. Dashboard requires an interactive terminal and opens the saved section list without creating or opening a session. Missing or old runtime dependencies produce minimum-version and installation hints before a new terminal/worktree is created.

## Installation when requested

A working executable is sufficient; Go, Node.js and Python are not runtime requirements for `gg`. Startup applications such as Codex or npm can have their own dependencies. Install or upgrade Git/tmux separately when the user requests setup, using the platform's package manager and checking the resulting versions.

Download the installer or the matching platform archive from the [latest stable Grand Git release](https://github.com/chartyom/grand-git/releases/latest). From a source checkout, building `./cmd/gg` requires Go 1.26+.

The POSIX installer defaults to `chartyom/grand-git`, the latest stable published release and `~/.local/bin`. `GG_REPOSITORY` accepts a validated owner/name override; an unset or empty value keeps the default. `GG_DOWNLOAD_BASE_URL` selects a release mirror or local test fixture, using HTTPS except for localhost fixtures. Use `sh install.sh --version v1.0.0` to pin the release, `--bin-dir /absolute/path` to select the destination, and `--force` to replace an existing executable. The installer checks both Git and tmux, verifies the archive's SHA-256 and installs atomically without sudo or shell configuration edits. Updating reruns the installer with `--force`. Removing the executable leaves project files and live sessions intact.

## Identity and creation

Every session has a global public `id` in `group:session` form, for example `3:7`. Read this string from `gg list --json` and pass it positionally to `start`, `stop` or `delete` from any directory. Use this ID for plain terminals, including those created in Git folder sections; their names are not branches. The group component identifies a workspace, not a dashboard section. Never substitute just the internal numeric component. The example IDs below must be replaced with the actual listed ID.

Inside a Git project, commands also accept its exact branch name, including slashes. A numeric-looking argument such as `1` still means that branch in the current Git project, not a record ID. The same branch can exist independently in multiple projects; use its global ID when targeting another project. Legacy protected records may retain `HEAD@<short-commit>` labels for a detached HEAD. There is no `--description` flag or description field.

```sh
gg create feat/login
gg create fix/release --base release/1.0
gg create feat/clean --no-copy
gg create feat/agent --template codex
gg create feat/server --choose-template
gg list
gg list --json
gg dashboard
```

- Inside Git, `create [branch]` creates a worktree and a terminal running the selected startup template in the background, prints the branch, path and template, and returns control. The default is `shell`; configured templates can launch agents or other applications. Use dashboard to reach the terminal. Success means the terminal was created, not that its application succeeded.
- During worktree creation, a Git `post-checkout` hook can use `gg list --json` to read previously registered sessions. The new session appears after checkout and terminal setup succeed. Creation waits for the hook itself to finish and can return while its background processes continue. When using v1.0.0, run the list query after `gg create` returns because that version holds the catalog lock during checkout.
- Inside Git, only an omitted branch is required in a terminal form; provide it explicitly without a TTY. Tab optionally opens the template picker; Esc outside the picker cancels the form. Supplying a branch avoids questions unless `--choose-template` is requested. That flag requires both stdin and stdout TTY, is incompatible with `--template`, and opens the picker explicitly. With a supplied branch, Enter confirms and creates without another form; Esc cancels creation. Without a branch, it remains required after template selection.
- A new branch starts at the calling worktree's HEAD, or at the explicit `--base` branch/commit. An existing free branch can be attached without `--base`; combining an existing branch with `--base` is rejected. A branch checked out elsewhere cannot be taken over.
- A registered branch cannot be created again, even when stopped. Use `start` or open it in dashboard.
- New worktrees live beside the main project at `<root>__sessions/<branch>`; `feat/login` preserves the slash, for example `project__sessions/feat/login`. Existing sessions keep their paths. Read `path` from the list instead of constructing or moving an existing session's directory.
- Outside Git, `gg create [name]` creates a terminal in the current directory, without a worktree, branch question or file copying. Omit the name to use the directory name; repeated names are allowed, so use the public ID to address these sessions. Global `default_template`, `--template` and the explicit `--choose-template` picker apply. `--base` and `--no-copy` require Git and are rejected outside it.
- CLI creation assigns the session to the dashboard section for the exact canonical calling directory, adding that section when needed. This includes Git subdirectories and managed worktree directories. A section's directory and a newly created worktree's path can differ; the worktree path still follows `<root>__sessions/<branch>`.
- The Git registry is shared across a project's worktrees. Discovering, listing, creating tasks, initializing hooks or opening dashboard does not automatically register the main or an external worktree as a source session. Dashboard launch creates no shell or session and does not add the calling directory as a workspace or section. Running legacy protected sources remain addressable with their existing IDs until their terminals stop; inactive source records are retired without removing directories, branches or renumbering other records. Unknown tmux state preserves the records.
- Git worktrees form one group per canonical Git common directory; ordinary sessions form one group per canonical calling directory. Groups receive stable numbers in the user state catalog under absolute `$XDG_STATE_HOME/grand-git`, otherwise `~/.local/state/grand-git`. Existing Git registries, internal IDs, sockets and worktree paths are retained. No disk-wide repository discovery occurs: older projects become known when the updated `gg` is first used there. Help and version do not register groups.
- If a known group is unavailable, `list` still prints healthy records (including a valid JSON array with `--json`), reports the failure on stderr and returns a nonzero status. Do not interpret partial output as a complete inventory.

For example, from an existing directory outside Git:

```sh
gg create
gg create scratch --template shell
gg list --json
```

Both created sessions use that directory; neither creates a worktree or copies its contents.

### Ignored environment files

Git worktree creation copies untracked Git-ignored files by default, including `.env`, ignored skills, and `node_modules`. It uses the worktree of an explicitly named local base branch when available; otherwise it uses the calling worktree, including when invoked from a subdirectory. `--no-copy` disables copying. The result reports the source and copied-file count.

Copies are independent ordinary files: Linux attempts reflink with ordinary-copy fallback, while macOS uses ordinary copying; permissions and mtime are retained. Symlinks are preserved without following them. Tracked files, ordinary untracked files, `.git`, nested repositories and special files are not copied; existing destination files and links are not overwritten. Ignored `.gg` is also excluded: settings and templates are shared from the primary worktree. Copied environments and ignored skills subsequently evolve independently. Copying does not install or validate dependencies, allocate ports, or synchronize environments.

## Startup templates

Inspect definitions without running their commands:

```sh
gg template list
gg template list --json
gg template show codex
```

These commands work inside and outside Git projects; outside Git, only global and built-in definitions apply. The catalog contains one entry per name. Its JSON fields are `name` and `source`: `built-in` or the selected YAML file path. `show <name>` prints editable YAML with `commands`.

| Built-in name | Startup commands |
| --- | --- |
| `shell` | Empty sequence; ordinary shell. Reserved and cannot be overridden. |
| `codex` | `codex` |
| `claude` | `claude` |
| `npm-dev` | `npm run dev` |

Definitions resolve as whole templates in increasing precedence: built-in, global `grand-git/templates/<name>.yaml` beside the global config, then `<primary-worktree>/.gg/templates/<name>.yaml`. Local definitions replace the complete lower definition; command arrays are not merged. Files in another worktree's `.gg` are not used. The primary `.gg` may be tracked in Git. A `shell.yaml` override is reported as an error by listing, while selecting `shell` still means the reserved ordinary shell.

Names match `[A-Za-z0-9][A-Za-z0-9_-]*`, without the `.yaml` suffix when passed to CLI. A template has exactly one `commands` sequence of nonempty strings; `commands: []` opens a plain shell. Unknown fields, wrong types, duplicate keys and multiple YAML documents are rejected. An invalid or missing selected definition fails before creation provisions a worktree or terminal. Inspect resolved commands before choosing a shared definition; they run as ordinary user shell code.

For example, save this as `<primary-worktree>/.gg/templates/backend.yaml` when a custom template is requested:

```yaml
commands:
  - export NODE_ENV=development
  - printf 'Branch %s\n' "$GG_BRANCH"
  - codex
```

Select it with `gg create feat/backend --template backend`. Selection is explicit `--template`, otherwise the effective `default_template`, whose built-in value is `shell`. Changing the default affects only future creation. Sessions save the template **name**: every cold start, including opening a stopped session in dashboard, resolves its current definition. Editing templates, switching live sessions, or an ordinary `start` of an active session does not rerun commands. Additional panes are always plain shells.

Command templates use `$SHELL` when executable, otherwise `/bin/sh`. They support interactive `sh` (including dash), `bash` and `zsh`; other shells support plain shell templates but reject command templates before provisioning. Normal interactive initialization runs first, making functions such as an rc-defined `nvm` available. Entries then run sequentially in one shell context, starting in the session working directory: exports, directory changes and functions carry to later entries. A nonzero entry skips all remaining entries. Each entry or multiline block has its own final exit status; use `&&` or explicit checks inside a block when intermediate failures must stop it. Put a long-running foreground application last, or open an extra pane for another program.

Startup terminals, ordinary shells and extra panes receive `GG_BRANCH` (exact worktree-session branch, empty for persistent plain terminals and temporary sessions, including inside Git), `GG_WORKTREE` (session working directory), and `GG_PROJECT_ROOT` (primary worktree for Git groups, or the ordinary group directory) through literal environment values. Quote expansions such as `"$GG_WORKTREE"` in commands. Grand Git does not interpolate branch/path values into generated shell code; spaces and valid shell metacharacters in those values remain data.

Configured commands run only after the session registry is committed; uncommitted or obsolete launch workers do not execute them. Preparation or registry-write failure rolls back new creation. Once startup commands can run, their completion, failure or Ctrl+C leaves the output, a visible exit status and an interactive shell; directory changes and exported variables remain available. Application failure does not roll back session files or registration, and there is no automatic retry. Explicit `exit` or `exec` in a template keeps ordinary shell semantics. Programs and dependencies must already be available; Grand Git does not install packages, allocate ports, grant hook trust, change global agent settings or call a network API for template startup.

## Lifecycle and deletion

| Command | Effect |
| --- | --- |
| `gg start feat/login` | Start an inactive session using its current named template, without attaching dashboard; an already active session keeps its terminals. |
| `gg start feat/login --template shell` | Save a new template name and start an inactive session; rejected while active. |
| `gg stop feat/login` | Stop that session's terminals and their processes; preserve files, branch and registry record. |
| `gg stop --all --yes` | Stop every registered session in the current Git project or ordinary directory group, including legacy sources and temporary sessions; preserve files and managed/persistent plain records, retiring stopped legacy source and temporary records. |
| `gg delete feat/login --yes` | Stop and remove a managed task's worktree and record; preserve its Git branch. |
| `gg delete --all --yes` | Stop sessions in the current Git project or ordinary directory group; remove managed worktrees and persistent plain/temporary records, retire stopped legacy source records, and preserve plain-terminal/source directories and every Git branch. |
| `gg start 3:7` / `gg stop 3:7` | Start or stop the exact globally identified session, irrespective of the calling directory. |
| `gg delete 3:7 --yes` | Delete the identified session; for persistent plain or temporary terminals, including those in Git groups, close processes and remove the record without deleting the working directory or its contents. |

Use bulk commands only for a request covering the current project or ordinary directory group; `--all` never means every group in the global list. CLI `--all` covers all sections of that workspace, while dashboard section S/D affects only the selected section. Switching sessions, quitting dashboard or disconnecting its terminal preserves running persistent terminals, agents and background jobs. **Stopping ends processes.** Starting or opening a stopped session runs its current named template in a fresh terminal; it does not resume an agent process. Managed legacy records without a saved template name use `shell`. Stopping a legacy protected source retires its record, so it cannot then be restarted by its former ID. `gg start main` without a registered session fails and suggests opening `gg dashboard`, adding the desired directory with `+` and pressing T on its section for a shell. If a saved template is missing or invalid, startup fails without silently substituting `shell`; fix the definition or explicitly recover with `gg start <branch|id> --template shell` while inactive.

Stopping or deleting a background session preserves the displayed session and focus. When an open session is closed, its dashboards return to the session list and remain available.

Deletion and bulk operations require confirmation; without a TTY pass `--yes`. Single-session CLI stop does not prompt. `--all` and a branch/ID argument are mutually exclusive. `--yes` skips confirmation only; it does not authorize discarding dirty files.

For managed Git worktrees, modified or ordinary untracked user files block deletion without `--force`. Commit or preserve the work, or use `--force` only when the user explicitly authorizes losing those files. Git-ignored files, including copied environments, are not protected by the dirty-file check and disappear with the removed worktree. Unchanged integration-generated files may also be removed. Source and external protected worktrees cannot be individually deleted, even with `--force --yes`. Use stop to end a running legacy source's processes and retire its record, preserving its worktree and branch.

A stop/delete invoked from an affected terminal can continue in a detached worker after that terminal closes. Check the result from dashboard or another worktree with `gg list --json`. After a failure, inspect the remaining sessions; a bulk operation can have partial results. Never substitute manual directory removal, branch deletion, or killing a shared tmux server for these lifecycle commands.

### Legacy temporary dashboard launchers

New `gg dashboard` invocations create no temporary record or shell, run no startup template, and claim no ownership of existing legacy temporary sessions. Already attached legacy dashboards keep their earlier tmux-client workflow until they exit. Their temporary rows remain visible as `dashboard`, with directory and temporary lifetime in details when space permits. CLI/JSON retain the directory-derived `name` and `kind=temporary`; the TUI label is not a CLI identifier.

Q or disconnection from a legacy dashboard closes only that invocation's temporary shells and descendant jobs and removes their records, including after switching projects. Exiting the last legacy temporary shell also removes its record. Files, persistent sessions, other invocations' temporary terminals and running legacy source sessions remain. Quitting a new dashboard preserves these legacy terminals. Older stopped temporary records may still have `temporary=false` and `kind=temporary` until explicitly removed.

## Dashboard

`gg dashboard` runs in the calling interactive terminal, including inside an existing tmux pane. It starts with list focus and saved folder sections, without creating a session, opening a terminal or registering the calling directory. An empty dashboard starts no project tmux server. The calling directory only supplies the folder browser's initial path. Explicitly create or open a session to display its terminal. One dashboard process keeps its list state across workspace switches and preserves existing user terminal processes. Each tmux pane title appears above its terminal content. A green status line below the panes shows the current project, active/total session count and Ctrl+W hint, including in compact windows.

The saved list is usable while Git status updates. A detached worktree sidebar resumes Git status polling when a client displays its window.

Folder sections refer to exact canonical directories. A Git project root and its subdirectories can have separate sections while sharing the same workspace, registry, server and public IDs. Existing workspace catalogs migrate once to root sections; existing records keep their paths, IDs and processes. Section IDs are internal and do not replace the workspace IDs from `gg list --json`.

Use `+ Add section` to open the folder dialog, including from an empty or compact list. Enter a path or browse directories, including hidden ones. Relative paths resolve from the dashboard's launch directory even after browsing elsewhere; `~/` expands the user's home. Enter in the path field browses that directory; Enter or a mouse click in the directory list descends into it. In the list, Backspace/Left or `..` goes to the parent. Tab/Shift+Tab cycles the path, directory list, Add and Cancel. Ctrl+S or Add confirms the path; Esc cancels. Browsing starts nothing, errors retain the input, and adding a duplicate canonical path within the same workspace selects its existing section. Adding a section opens no terminal.

X or Remove section removes only an empty section. Stopped registered sessions count as nonempty, even if a filter hides them. Removal preserves files and the workspace record and persists across dashboard restarts; a missing or replaced directory prevents removal when emptiness cannot be checked reliably. Deleting the last session leaves its section available for new sessions until explicitly removed.

The session list shows selectable, non-collapsible section headings alphabetically by folder name and then path. Folder names have no counts or trailing markers; equal names include their paths. Worktree and terminal rows share creation order, with `⎇` and `>_` type marks respectively. Unfiltered empty sections remain selectable. Each session takes one row with its name and activity circle. Rows hide public IDs; use `list` when an ID is needed. `▸` marks the terminal open to the current dashboard; a highlight and a separate `›` mark distinguish a different selected row. Up/Down visits sections and sessions, keeping the selected item and its section heading visible. Refresh preserves the selected section/session, or chooses the nearest remaining item when it disappears. Unavailable sections report errors without hiding healthy sections.

Enter or a single left-click opens a session, including on another workspace's isolated tmux server, without restarting the dashboard or its running user processes. Clicking a section selects it; Enter opens its actions. Hover previews a session's path, live terminal count, agent status, Git status and open state, or a section's path, session/running counts and type when space permits. Hover does not change selection, focus or the open terminal; leaving the list restores selected-item details. When focus enters the list from a terminal, selection moves to that terminal's row, clearing search and the active-only filter only if needed to reveal it. There is no product/focus title or all-sessions count above the list; an active-only filter shows `Active` (`Активные` in Russian) without a count.

Press Ctrl+W to move from the session list to the first terminal pane, through later panes in screen order by top edge and then left edge, and back to the list. Each press advances once; Ctrl+W is consumed before it reaches a terminal application. Ctrl+G, plain arrows, Tab, Shift+Tab, Ctrl+P and the former direct Ctrl/Alt navigation keys remain available to terminal applications; Shift+Tab is forwarded as BackTab. Esc in the list closes the actions menu, cancels a form or confirmation, or clears editing or applied search. In the sidebar, with no interaction to cancel, Esc leaves list focus; press Ctrl+W to reach a terminal. Outside list/forms, ordinary input, Ctrl+C and Esc reach the terminal application. Clicking a terminal area restores terminal focus; clicking the sidebar focuses the list. The wheel over the sidebar scrolls its rows. Terminal mouse events reach applications that request them.

The sidebar occupies 25% of the window, within 28–56 columns. At fewer than 100 columns **or** fewer than 16 rows, the list uses the full screen when focused; Ctrl+W or Esc returns to an open terminal after search or other interactions are cleared. Without an open terminal, Ctrl+W and Esc retain list access. Once the window is at least 100 columns wide and 16 rows tall again, the sidebar returns as a full-height column on the left. Terminal panes keep their running processes and their positions relative to each other. A selected session's footer shows Open, New, Actions and Quit, plus Delete when unprotected. Section footers offer creation, available bulk actions and removal when empty. Search editing replaces these with Clear and Down-to-list. Add section remains available above the list.

Right-click selects a section or session and opens its actions beside the pointer without opening or starting it. `?` or Shift+F10 opens the selected item's actions; these keyboard menus also offer available search, filtering, settings and quit. Menus stay bound to the original section/session across refresh or selection changes. Arrows/J/K and Enter, shortcuts or mouse clicks choose an action; menus scroll in short windows. Esc closes the menu without clearing applied search; clicking outside a pointer menu dismisses it. New dashboards present menus, folder selection, creation, template selection, placement, settings and confirmations as bounded overlays above terminal content. They support keyboard and visible mouse controls, retain the project status line and consume modal input, including Ctrl+W. Creation errors retain the name and template for correction. The same list actions work in the sidebar and full-screen list:

| Input while the list is focused | Action |
| --- | --- |
| + | Open Add section to choose an existing directory; Ctrl+S or Add saves it without a session. |
| Up/Down or J/K | Select a section or session. Up above the first item focuses search when available. |
| ? or Shift+F10 | Open contextual actions; protected-session deletion and unavailable search are omitted. |
| Enter or single left-click on a session | Open the session; run its saved template if stopped. Reopening the current session keeps its focused terminal pane. Enter on a section opens its actions. |
| N | Create using the selected section's exact directory or the selected session's directory as the base, retaining membership in that section. Git requires a branch and creates a worktree from that directory's HEAD; outside Git, an optional name defaults to the directory name and no worktree is created. The form offers the target workspace's default template and Tab picker, then opens the new session. Without a selection, add or select a section first. |
| Tab in the creation form | Open the optional template list; arrows select, Enter accepts, Esc preserves the previous template and branch/name. Enter back in the form creates; Esc cancels creation. |
| T | On a section, create and open a persistent plain shell at its exact path, including a Git subdirectory, without a branch, worktree or file copy; this always uses `shell`. On an active session, ask Below or Right before adding a plain shell in its working directory, then focus the new pane. B/R or arrows and Enter choose; Esc cancels. On a stopped session, start its saved template without asking placement. |
| S | Start a stopped session, or confirm stopping an active one. On a section, confirm stopping its running sessions. |
| D or Delete | Confirm deletion of an unprotected session, or all sessions in the selected section. Persistent plain and legacy temporary terminals lose only their processes and records; protected source rows offer no individual deletion. |
| X | Remove a selected empty section; stopped registered sessions prevent removal. Files and workspace identity remain. |
| Y; N/Esc in confirmation | Confirm the fixed stop/delete target set; or cancel. Section actions capture IDs when confirmation opens, independently of search and activity filtering. Missing targets are skipped and newly created sessions are excluded. |
| /; Down/Enter; Esc | With more than ten candidates, search branches, session names and sections while typing. Down/Enter applies the query and returns to list controls. Esc clears editing or applied search, reveals and selects the open terminal when present, and retains list focus; the activity filter remains unless it hides that terminal. Another Esc closes a compact list only when a terminal is open. |
| A | Toggle all versus active sessions across every section; sections without matches are hidden. |
| , | Open the current open workspace's language/default-template settings, independently of list selection. Global/Local scope is available in Git; ordinary workspaces and a dashboard with no workspace opened yet use Global only. Enter saves, Esc cancels. |
| Q | Exit this dashboard, preserving persistent sessions and their processes, including after a workspace switch. Existing legacy temporary records remain owned by their original clients. |

Section S/D covers that section's fixed target set, including hidden matches, and leaves other sections in the same workspace alone. CLI `stop/delete --all` still covers the whole current workspace. Dashboard deletion has no force option: every surviving managed worktree is checked before any target is stopped, and modified or ordinary untracked files block the operation. Protected directories, plain-terminal directories and Git branches remain. Use these lifecycle actions only within the user's requested scope; Q itself stops no persistent session.

Search eligibility counts session rows, including legacy temporary records, across accessible sections after All/Active filtering, before the text query and viewport clipping. Section headings and Add section do not count. At ten or fewer candidates the control is hidden and `/` does nothing. Shrinking to ten clears any query and ends search editing while retaining list focus; growing to eleven restores availability with no old query. A query with few or zero matches does not disable search. Clicking an applied field resumes editing. Clear empties the query while retaining input focus; Down-to-list applies it like Down or Enter.

`●` means at least one live **user terminal**, and `○` means stopped, independently of focus, Git cleanliness or agent status. Details spell out the number of live terminals and the agent and Git statuses; service/sidebar panes never make a session active. They also say `Open: yes/no` (`Открыта: да/нет` in Russian) for the previewed or selected session. Persistent plain and temporary sessions show Git as `—`, including inside Git groups. Stopped managed and persistent plain sessions remain listed unless filtered; newly ended temporary records and inactive legacy sources are retired.

Successful asynchronous stop/delete feedback (`Completed: ...`, or its translation) is green for five seconds from operation completion. Refresh, resize and returning to the list do not renew its deadline. Errors remain visible, and a newer notice has its own deadline. Colors follow terminal capabilities and `NO_COLOR`. This feedback is separate from the agent's `done` status, which still means a completed turn rather than completion of the task.

## Settings

```sh
gg config
gg set language ru --local
gg set default_template codex --local
gg reset language --local
gg reset default_template --local
```

`gg config` reports each effective value and its source without creating configuration. `gg set <key> <value>` and `gg reset <key>` target the global user layer by default, including outside Git; add `--local` for the shared project layer, which requires a Git project. Existing `gg config set/reset` commands remain compatible, and `--project` aliases `--local` on both forms. Change settings only within the requested scope; omitting `--local` changes the user's global Grand Git preferences.

| Key | Default | Accepted values |
| --- | --- | --- |
| `language` | `en` | `en`, `ru` |
| `default_template` | `shell` | Valid template name; availability is checked at creation. Existing sessions keep their saved names. |

The default interface is English. Selecting `ru` translates Grand Git's own help, forms, confirmations and messages without restarting user terminals. Public JSON keys and status codes remain English; legacy Russian status values are normalized when read.

Precedence is built-in defaults, then global, then local. Global settings are `$XDG_CONFIG_HOME/grand-git/config.yaml` when XDG_CONFIG_HOME is absolute, otherwise `~/.config/grand-git/config.yaml`. Local settings are `<primary-worktree>/.gg/config.yaml`, shared across worktrees; a task's own `.gg` does not override them. Git registries and service data use `<git-common-dir>/grand-git/`. Outside Git, a directory's `.gg` is not a local settings/template layer; only global and built-in definitions apply. Ordinary-session metadata and group discovery use the user state directory, separately from settings. Invalid values or malformed YAML are rejected without overwriting existing configuration; use one mapping of supported string keys, without duplicate keys or multiple documents.

When a layer's YAML is absent, Grand Git reads that layer's legacy TOML: global `grand-git/config.toml` beside the new global YAML, or local `<git-common-dir>/grand-git/config.toml`. The first write migrates the supported values of the selected layer, applies the requested edit and atomically writes YAML, leaving TOML intact. The former `navigation` field is accepted from old YAML or TOML files but has no effect and is omitted on the next write. `gg set/reset navigation` is no longer available. Once YAML exists, it replaces the whole legacy layer. Reset removes only the chosen key, exposing the next applicable value; resetting the last key leaves `{}` so old TOML values do not reappear. Global edits do not defeat local overrides.

Dashboard settings use Up/Down or Tab to select a field and Left/Right or Space to change it; clicking a field selects and advances its value. This includes Global/Local scope for an open Git workspace (initially Global; ordinary workspaces and a dashboard with no workspace opened yet stay Global). Selecting another section does not switch the settings context. Enter or Save writes language and template together in one atomic update; Esc or Cancel cancels. Settings apply without restarting user terminals. Changing `default_template` does not alter existing sessions' saved names or running processes; section T still creates a plain `shell`.

## JSON and agent status

`gg list --json` returns an array of public records from all known groups. Use the `workspace` object and global `id` to identify the target; `branch` alone is not globally unique. Use it for automation instead of parsing translated table columns or reading internal registry files.

| Field | Meaning |
| --- | --- |
| `id` | Global string ID, `group:session`; accepted positionally by start/stop/delete from any directory. |
| `name` | CLI/JSON name: Git branch for legacy source/worktree rows, directory-derived or supplied name for terminal rows. Legacy temporary TUI rows are labeled dashboard without changing this field. Names can repeat. |
| `kind` | `source` for retained legacy protected records, or `worktree`, `terminal`, `temporary`. Section T can create `terminal` in a `git` workspace as well as a `directory` workspace. New dashboards create neither source nor temporary records. |
| `workspace` | Object with numeric `id`, `type` (`git` or `directory`), `name`, and canonical `path`. This identifies the underlying workspace; several dashboard sections can share it. |
| `branch` | Exact branch for source/worktree records; empty for persistent plain/temporary terminals, including inside Git groups. |
| `base` | Resolved base commit for source/worktree records; empty for persistent plain/temporary terminals, including inside Git groups. |
| `path` | Actual worktree path or terminal working directory; legacy temporary launchers retain the exact calling directory. |
| `template` | Saved template name; `shell` for legacy/source records without a saved name. Commands are resolved again on cold start. |
| `created_at` | Creation timestamp. |
| `protected` | Whether this is a protected source worktree. |
| `copy_source`, `copied_files` | Copy source and file count when present; omitted values are not evidence of live activity. Zero copied files may omit the count. |
| `active` | Boolean: at least one live user terminal. |
| `temporary` | Boolean: a live temporary launcher; false when stopped. Use `kind` to distinguish a stopped temporary record. Legacy marked source terminals may also report true. |
| `terminals` | Number of live user terminals. |
| `agent_status` | English status code, independent of the selected UI language. |

Public JSON has no `description` or section identifier. Do not confuse its composite string `id` with a private registry's numeric ID. Status codes are `ready`, `working`, `waiting`, `done`, `interrupted`, `shell`, `stopped`, and `unknown`.

For Codex status integration inside a Git project, `gg init` merges local `.codex/config.toml` and `.codex/hooks.json` in the primary, calling and registered real Git worktrees without creating source sessions, preserving other settings/hooks and backing up changed existing files under `<git-common-dir>/grand-git/hook-backups/`. Managed handlers use `statusMessage: grand-git`. Repeated initialization avoids duplicate handlers; subsequent created sessions receive the integration. It does not change global agent configuration or grant hook trust. In Codex, the user must inspect the local hooks and manually trust them through `/hooks`. Sessions work without trust; do not promise exact agent status until trusted events are available.

For one primary Codex session per worktree, event-derived codes mean:

| Events | Code and interpretation |
| --- | --- |
| SessionStart | `ready` — agent session started. |
| UserPromptSubmit, PreToolUse, PostToolUse | `working` — agent activity. |
| PermissionRequest | `waiting` — waiting for permission. |
| Stop | `done` — the agent completed a turn, not the whole task. |
| Interrupt | `interrupted` — interrupted turn. |
| SessionEnd | `shell` — agent session ended. |

With no live user terminals the displayed code is `stopped`. A normal shell gives `shell`; a non-shell command without usable hook evidence gives `unknown`. Treat terminal activity, agent status and Git cleanliness as separate signals. Do not infer task completion, successful tests, or the state of another agent from them.

## Git inside a session

Use ordinary Git in the worktree reported by `path`: inspect changes, commit and push as the task requires. For example, after the user's requested changes and commit, `git push -u origin feat/login` publishes the branch when publishing is authorized. Grand Git has no pull-request integration; do not invent `gg push` or PR commands. Removing a Grand Git task keeps the branch available for ordinary Git use or a later `gg create` with that branch.
