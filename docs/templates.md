# Grand Git startup templates

A template defines a sequence of commands for one terminal. `gg create` starts it in the background; use `gg dashboard` to view output and interact with the application.

```bash
gg template list
gg template show codex
gg create feat/login --template codex
gg create feat/server --choose-template
```

Built-ins are `shell` (a plain shell), `codex` (`codex`), `claude` (`claude`) and `npm-dev` (`npm run dev`). Install applications and dependencies yourself. `shell` is reserved and always opens a plain shell.

## Custom templates

Save YAML in `.gg/templates/<name>.yaml` in the project's **primary worktree**. For global templates, use `$XDG_CONFIG_HOME/grand-git/templates/` when `XDG_CONFIG_HOME` is an absolute path, otherwise `~/.config/grand-git/templates/`. Outside Git, only global and built-in templates apply.

A name starts with an ASCII letter or digit and can also contain `_` and `-` after the first character. Pass the filename without `.yaml` to the CLI. The YAML file has one field, `commands`, containing a list of nonempty strings. Use `commands: []` for a plain shell.

For example, `.gg/templates/backend.yaml`:

```yaml
commands:
  - npm run prepare
  - codex
```

```bash
gg create feat/backend --template backend
gg set default_template backend --local
```

A local template completely replaces a global or built-in template of the same name. `.gg` is shared by all worktrees, so it does not need updating in every task. It can be committed to Git. Other worktrees' `.gg` versions are not read; ignored `.gg` is not copied as environment data. Other ignored files follow the normal copying rules.

Project settings are in `.gg/config.yaml`:

```yaml
language: en
default_template: backend
```

Without `--local`, `gg set` and `gg reset` change the global `config.yaml`. The same commands work under `gg config`, and `--project` aliases `--local`. Run `gg config` to see effective values and their sources. If a settings layer has only a legacy TOML file, the first write migrates its supported settings to YAML and leaves the TOML file intact. Subsequent reads and resets use that layer's YAML.

## Examples

Codex with an environment variable:

```yaml
commands:
  - export NODE_ENV=development
  - codex
```

Claude:

```yaml
commands:
  - claude
```

Node.js:

```yaml
commands:
  - node server.js
```

Choose a port for an npm development server that reads `PORT`.

```yaml
commands:
  - export PORT=3101
  - npm run dev
```

With zsh as your shell and nvm loaded in `.zshrc`, select the Node.js version from the project's `.nvmrc` before starting the development server.

```yaml
commands:
  - nvm use
  - npm run dev
```

Commands run in one interactive `sh`, `bash` or `zsh` after normal shell initialization. `export`, `cd` and functions remain available to subsequent commands. Grand Git uses the executable named by `$SHELL`, falling back to `/bin/sh` when it cannot find one. Other shells support plain shell templates. Selecting a command template there produces an error before worktree creation.

Each entry is a shell command or block with its own final exit status. Execution stops at the first nonzero entry. Within multiline blocks, use `&&` or explicit error checks where needed. Put long-running commands last because the next entry starts only after the previous one exits. For a second terminal, focus the dashboard list, select the running session and press T to choose a pane position.

Grand Git sets `GG_BRANCH` to the session's branch, `GG_WORKTREE` to the session directory and `GG_PROJECT_ROOT` to the primary worktree or ordinary workspace directory. Plain terminal sessions have an empty `GG_BRANCH`.

```yaml
commands:
  - printf 'Branch %s, worktree %s\n' "$GG_BRANCH" "$GG_WORKTREE"
  - codex
```

The YAML parser reads each entry as a string. The interactive shell then executes the string, including its shell quoting and variable expansions. Quote expansions such as `"$GG_WORKTREE"` to preserve paths with spaces. Explicit `exit` or `exec` in a command ends or replaces the shell as usual.

## Selection and restarting

`gg create` and the dashboard's New session form use `default_template`, which defaults to `shell`. Select a different template with `--template` or the form's picker. Legacy records without a saved template use `shell`. Pressing T on a folder section opens a plain shell at that path, including inside Git. Additional panes in a running session also open plain shells.

A session retains its template name. Its next start reads the current commands. Editing a template, switching tasks or changing the default does not restart running terminals.

```bash
gg stop feat/backend
gg start feat/backend                    # current backend definition
gg stop feat/backend
gg start feat/backend --template shell   # save another template and start
```

`start --template` is rejected for active sessions; plain `start` creates no duplicates. If a template is removed or malformed, startup reports the reason: fix the file or explicitly choose `shell`.

Creation without `--choose-template` asks no template questions. In the dashboard form, Tab opens the list, arrows select, Enter confirms and Esc restores the previous selection. In an explicit CLI `--choose-template` picker with a branch supplied, Enter creates the session and Esc cancels creation. The flag requires an interactive terminal and conflicts with `--template`.

Commands start after the session is saved in the registry. `create` returns as soon as the terminal is ready. Open the session in `gg dashboard` to see the application's output. On normal completion, failure or Ctrl+C, the terminal displays the startup exit status and returns to an interactive shell. Session files remain available.

For automation, `gg template list --json` exposes `name` and `source`, and `gg list --json` adds `template`. A `built-in` source denotes a built-in template; other sources are paths to the selected YAML files.
