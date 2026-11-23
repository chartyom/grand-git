# Grand Git

[Russian](README.ru.md)

**A simple terminal workspace for parallel tasks: Git worktrees, persistent terminals, one dashboard.**

Grand Git (`gg`) keeps the everyday workflow small: create a task, work in its terminal, switch to another task, and stop or remove it when finished. Each Git task has its own worktree, so unfinished changes can stay where they are. Ordinary folders work too, with terminals in place.

Use it when you want a shared view of your projects and running tasks with little setup. Shells work out of the box; startup templates and Codex status are optional. You manage commits, pushes and merges with your usual Git tools.

![Grand Git demo: create a task, edit files, open a second worktree and switch back to the original terminal](docs/assets/gg-demo.gif)

Create a task, open a second worktree, and switch back with your edits and terminal intact. [Static screenshot](docs/assets/gg-demo.png).

## Install

Runs on Linux, macOS and Linux inside WSL2. Terminal sessions require **tmux 3.4+**; Git projects also require **Git 2.31+**. Use an interactive terminal. Bare repositories and native Windows are unsupported.

From an existing source checkout, with **Go 1.26+**:

```sh
go build -o bin/gg ./cmd/gg
export PATH="$PWD/bin:$PATH"
gg --version
```

The repository is [chartyom/grand-git](https://github.com/chartyom/grand-git). The [installation guide](docs/installation.md) covers dependencies, permanent installation, archives, the release installer, updates and removal. Go and Node.js are not needed to run a built binary.

If `gg` opens Git GUI or reports a missing `gui` command, check for a [shell alias named `gg`](docs/installation.md#if-gg-runs-git-gui).

## Quick start

```sh
gg dashboard                 # open saved folder sections and sessions
```

Press +, choose a directory and confirm with Ctrl+S. On that section, press T for a plain shell or N to create a session. In Git, enter a branch name and press Enter to open the new task in its own worktree. Ctrl+W cycles through the list and terminal panes; Enter opens the selected session. The same gg dashboard process stays open when switching projects.

You can also create tasks from the command line:

```sh
gg create feat/login         # worktree + background shell
gg create fix/release --base main
gg list
```

New worktrees live at `<project>__sessions/<branch>`. Ignored local files such as `.env` and `node_modules` are copied independently; `--no-copy` skips copying. Outside Git, `gg create [name]` creates a terminal in the current folder. `gg` without a command shows help.

## Dashboard

The sidebar groups sessions under selectable folder sections. Worktrees (`⎇`) and plain terminals (`>_`) share one list in creation order. A single click opens or starts a session; hovering previews details without switching terminals. `●` / `○` indicates running / stopped terminals; `▸` marks the open terminal independently of the highlighted selection. Details show the path, terminal count, agent and Git state.

| In the session list | Key |
| --- | --- |
| Select / open | ↑↓ or J/K / Enter or single click |
| Add a folder section / remove an empty section | + / X |
| New session | N |
| Actions and shortcuts | Right-click, ? or Shift+F10 |
| Search, when the list has more than 10 sessions | / |
| All / active sessions | A |
| Next panel / cancel | Ctrl+W / Esc |
| Quit this dashboard | Q |

Select a folder and press T to open a persistent plain shell there, including inside Git. Section S/D confirm Stop all/Delete all within that section; confirmations freeze their targets and deletion checks every worktree before stopping processes. The footer shows Delete for an unprotected selected session. The actions menu also exposes T for another terminal, S to start/stop, D to delete and comma for settings. Use its shortcuts, arrows/Enter or the mouse. Search counts rows after the activity filter, before the text query; at 10 or fewer rows it clears and disappears. Search editing shows only Clear and Down-to-list; Down/Enter applies the query, and Esc clears it and selects the open terminal. In a narrow terminal, Ctrl+W opens the compact list after the last terminal pane.

Dashboard starts with list focus and no terminal open. It creates no session and does not add the calling directory. Use **+ Add section**, browse folders or enter a path, then confirm with **Ctrl+S**. Relative paths use the calling directory; `~/` and paths with spaces are supported. Browsing and canceling create nothing. Adding an existing section selects it.

Each section keeps its exact directory: `/project` and `/project/src` can coexist in one Git repository. T creates a persistent shell in that directory. N creates a worktree or ordinary session; creation from a session retains its section. `gg create` uses the section for its exact calling directory, adding it when needed. Existing sessions retain their root section, public IDs and worktree paths. X removes only an empty section, including no stopped records; files and workspace records remain.

Q, detach and connection loss close the dashboard while persistent terminals keep running. With no open terminal, the compact list stays accessible. Existing legacy dashboards and their owned temporary shells retain their cleanup behavior; new dashboards never create a temporary shell.

## Stop and remove

```sh
gg stop feat/login           # end processes; keep the worktree and record
gg start feat/login          # start a new terminal
gg delete feat/login --yes   # remove the worktree; keep the Git branch
```

Modified and ordinary untracked files block deletion without explicit `--force`; ignored files are removed with the task worktree. Source and external worktrees are never removed by `gg`. Deleting an ordinary folder session preserves all its files. See [lifecycle safeguards](docs/usage.md#lifecycle-and-safeguards) before bulk operations.

## Alternatives

| Tool | Choose it for |
| --- | --- |
| [Git worktree](https://git-scm.com/docs/git-worktree) + [tmux](https://github.com/tmux/tmux) | Direct control of worktrees and terminals, arranged manually. |
| [Worktrunk](https://worktrunk.dev/) | A worktree-oriented CLI with branch switching, hooks and merge workflows. |
| [workmux](https://workmux.dev/guide/) | Worktrees paired with terminal windows, setup automation and merge/cleanup workflows. |

Grand Git focuses on a small session workflow and a shared dashboard across Git projects and ordinary folders. Start with a shell and add optional startup commands only when useful.

## More

- [Usage reference](docs/usage.md): global session IDs, settings, navigation, JSON and optional Codex hooks.
- [Startup templates](docs/templates.md): shell, Codex, Claude and custom commands.
- [Grand Git agent skill](docs/skills/grand-git/SKILL.md).
- [Development instructions](AGENTS.md) and [current specifications](openspec/README.md).

Development checks: `sh scripts/test.sh` and `sh scripts/check-specs.sh`. Tests use temporary repositories and isolated tmux servers; specification validation uses Node.js 22.12+ and pinned npx.
