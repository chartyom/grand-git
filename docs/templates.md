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

Save YAML in `.gg/templates/<name>.yaml` in the project's **primary working directory**. For global templates, use `$XDG_CONFIG_HOME/grand-git/templates/` or `~/.config/grand-git/templates/`. The name comes from the filename: letters, digits, `_` and `-`, without the extension in CLI commands.

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

Without `--local`, `gg set/reset` changes the global `config.yaml`. Existing `gg config set/reset` commands work the same way; `--project` aliases `--local`. `gg config` shows values and sources. Legacy TOML files are read until YAML exists at their level; the first write migrates all settings at that level while preserving TOML. Reset keeps YAML and does not restore old TOML values.

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

An npm development server with a user-selected port:

```yaml
commands:
  - export PORT=3101
  - npm run dev
```

Prepare the environment using a function from `.zshrc`, such as nvm:

```yaml
commands:
  - nvm use
  - npm run dev
```

Commands run in one interactive `sh`, `bash` or `zsh` after normal shell initialization. `export`, `cd` and functions remain available to subsequent commands. Syntax must match the selected shell (`$SHELL`, otherwise `/bin/sh`). Other shells support `shell`; command templates are rejected before worktree creation.

Each entry is a shell command or block with its own final exit status. Execution stops at the first nonzero entry. Within multiline blocks, use `&&` or explicit error checks where needed. Put long-running commands last: the next entry starts only after the previous one exits. Press T to open another plain terminal for another application.

The environment includes `GG_BRANCH`, `GG_WORKTREE`, `GG_PROJECT_ROOT`. For example:

```yaml
commands:
  - printf 'Branch %s, worktree %s\n' "$GG_BRANCH" "$GG_WORKTREE"
  - codex
```

Values are passed through the environment; Grand Git does not substitute branch names into command text. The selected shell interprets YAML quotes and variables. Ordinary commands return control to the shell; explicit `exit`/`exec` in user scripts changes this shell process as usual.

## Selection and restarting

`--template` overrides `default_template`; without a setting, `shell` is selected. The default applies only to `create`. Legacy records and the source directory use `shell`. Additional terminals always open a shell.

A session retains its template name. Its next start reads the current commands. Editing a template, switching tasks or changing the default does not restart running terminals.

```bash
gg stop feat/backend
gg start feat/backend                    # current backend definition
gg stop feat/backend
gg start feat/backend --template shell   # save another template and start
```

`start --template` is rejected for active sessions; plain `start` creates no duplicates. If a template is removed or malformed, startup reports the reason: fix the file or explicitly choose `shell`.

Creation without `--choose-template` asks no template questions. In the dashboard form, Tab opens the list, arrows select, Enter confirms and Esc restores the previous selection. In an explicit CLI `--choose-template` picker with a branch supplied, Enter creates the session and Esc cancels creation. The flag requires an interactive terminal and conflicts with `--template`.

Commands start after the session is saved in the registry. `create` confirms terminal creation, not application success. Failure, normal completion or Ctrl+C leaves output, exit status and a plain shell; session files remain. There are no automatic retries, dependency installation, port selection or hook trust grants.

For automation, `gg template list --json` exposes `name` and `source`, and `gg list --json` adds `template`. A `built-in` source denotes a built-in template; other sources are paths to the selected YAML files.
