# Current project context

## Purpose

Grand Git (`gg`) manages parallel tasks across Git projects and ordinary directories through a shared dashboard. Git tasks use independent worktrees; ordinary directory sessions use terminals without creating branches or copying files.

## Architecture

- The standalone Go CLI uses Cobra for commands, Bubble Tea v2 for forms and lists, and tmux for interactive terminals.
- A global catalog in `$XDG_STATE_HOME/grand-git` or `~/.local/state/grand-git` identifies Git groups by canonical common directory and ordinary groups by canonical directory. Public IDs use `group:session`; exact branch names also address the current Git project.
- Git registries live in `grand-git/` inside the Git common directory. Ordinary terminal records live in user state. Registry and catalog writes use flock and atomic replacement.
- New worktrees use `<project>__sessions/<branch>`. Existing registered paths and internal IDs remain supported. Source and external protected worktrees cannot be deleted through gg, including with `--force`.
- New dashboard invocations keep one gg TUI process and open the saved section list without a session or terminal. The calling cwd is only the folder browser's starting directory. Project tmux servers own explicitly created terminals; cross-project transitions preserve the TUI process and terminal PIDs. Q and disconnection preserve persistent sessions. Already attached legacy dashboards retain their owned temporary cleanup until exit.
- A versioned sections catalog shares the global workspace lock. Sections have stable internal IDs, workspace references and exact canonical paths; task records store section_id. Existing catalogs migrate once to root sections. New CLI creates use their exact cwd section, dashboard creates preserve their selected section, and removal requires an empty section under catalog-before-registry locks. Public IDs and JSON workspace identity remain unchanged.
- Managed and ordinary session records outlive terminals. Git discovery does not create source sessions; inactive legacy source records are retired under the registry lock after a reliable tmux query. Running legacy sources retain their IDs until stopped. stop terminates managed terminal processes while preserving records and files; start opens a new terminal using the current saved template. Detached service workers finish operations that close their calling terminal.
- Global YAML settings and primary-worktree `.gg/config.yaml` provide language and default template selection. Other worktrees share the primary `.gg`. Ordinary groups use only global settings and templates. Supported TOML migration remains part of current behavior; obsolete navigation values are ignored and dropped on write.
- Built-in and user-defined templates run sequential commands in one interactive sh/bash/zsh after registry commit. Additional terminals are plain shells. No dependency installation, port selection or hook trust is automatic.
- Ignored environment files are copied independently on Git task creation, excluding shared `.gg`. Linux uses reflink with ordinary copying as fallback; macOS uses ordinary copying. Symlinks are preserved without dereferencing.
- Dashboard groups sessions alphabetically without counts beside group names, uses one row per session, selected-session details and an actions menu, supports search only above ten activity-filtered rows, search-only editing controls, mouse and Ctrl+W panel cycling, and uses cached per-project Git state. Selectable folder sections mix worktrees and terminals with type marks; + opens a path/directory browser and X removes an empty section; single clicks open sessions, hover previews details, and target-bound menus and bulk actions operate on stable identities. Section actions create persistent plain shells at exact paths inside or outside Git and scope bulk actions to that section. The left arrow identifies the open terminal independently of selection. Its responsive sidebar hides in compact windows without restarting user terminals. The TUI renders tmux pane titles and a green project/activity status line around terminal contents, with bounded mouse-enabled dialog overlays.
- Project-local Codex hooks track one primary agent per session. Initialization preserves other settings and requires explicit user trust. English machine codes and legacy Russian status normalization remain supported.
- Repository prose uses English except `README.ru.md`. The interface defaults to `en` and supports `ru`; language changes do not restart user terminals.

## Local design prototype

`design/index.html` provides an offline interactive HTML model of the terminal dashboard with fictional session data, repeatable scenarios, English/Russian presentation and stable unique `data-testid` attributes on every DOM element. The prototype combines worktrees and terminals inside exact-path sections, models a fictional folder browser and empty-section removal, opens sessions on a single click, previews hovered details, and supports selectable folders, target-bound context menus, bulk lifecycle actions and dedicated search hints. Its demonstration controls are separate from the terminal layout; actions do not execute Git, tmux or shell commands. See `design/README.md` for controls and browser-test setup.

## Environment

Building requires Go 1.26+. For Git projects, Grand Git v1.0.1 supports Git 2.31+. The v1.0.0 release requires Git 2.36+. Ordinary directory sessions can run without Git. Terminal sessions require tmux 3.4+. Runtime targets are Linux/WSL2 and macOS on amd64/arm64. Native Windows and bare repositories are unsupported. Python 3 supports integration tests. OpenSpec validation requires Node.js 22.12+ and npx.

The canonical repository is https://github.com/chartyom/grand-git and the Go module is github.com/chartyom/grand-git. GoReleaser targets that repository, and the installer uses chartyom/grand-git unless GG_REPOSITORY overrides it. The release configuration builds four Linux/macOS archives, SHA-256 checksums and an installer.
