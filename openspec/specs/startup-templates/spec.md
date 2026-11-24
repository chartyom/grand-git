# Startup Templates Specification

## Purpose

Define named startup commands, template selection, execution after registration and compatibility with existing sessions.

## Requirements

### Requirement: Named single-terminal definitions
Templates SHALL contain only commands, a list of nonempty shell strings; an empty list MAY open a shell. Names SHALL start with a letter/digit, followed by letters/digits/underscores/hyphens, matching the YAML filename stem. Built-ins SHALL be shell, codex, claude and npm-dev. Invalid selected definitions SHALL be rejected before creating a worktree or terminal.

#### Scenario: Whole-template override
- **WHEN** global and local files define codex
- **THEN** only the primary directory's local commands run

#### Scenario: Invalid selected YAML
- **WHEN** commands contains invalid types, unknown fields, duplicate keys or multiple documents
- **THEN** creation is rejected without a new branch, worktree or session

### Requirement: Creation selection and stable template identity
create SHALL select --template → default_template → shell. The registry SHALL retain the name. Legacy/protected records without a name SHALL mean shell. Default changes SHALL affect only future create calls. Restarting stopped sessions SHALL read the current definition; unavailable definitions SHALL be rejected without substitution. Switching and starting active sessions SHALL NOT repeat commands.

#### Scenario: Edited definition on restart
- **WHEN** a stopped session's template is edited before start
- **THEN** new commands run while the selected name remains

#### Scenario: Template removed
- **WHEN** the saved definition is no longer available
- **THEN** start reports the template and allows explicit recovery with --template shell

### Requirement: Explicit interactive selection
--choose-template SHALL require TTY stdin/stdout and conflict with --template. Git worktrees SHALL require a name, inferred from an explicit --branch when omitted; outside Git, names SHALL be optional and default to the directory name. Confirming an explicit picker with a supplied name SHALL proceed with creation; Esc SHALL cancel creation. Ordinary create with complete arguments SHALL ask no questions. Dashboard Tab SHALL open optional selection; Esc SHALL restore the previous template.

#### Scenario: Automated creation
- **WHEN** create receives a worktree name and --template without a TTY
- **THEN** the template applies without interactive input

#### Scenario: Picker cancellation
- **WHEN** explicit selection is canceled
- **THEN** no branch, worktree or session is created

### Requirement: Post-commit command authorization
Configured commands SHALL run only after successful registry persistence. A per-launch token SHALL bind workers to saved session identity; invalid or stale workers SHALL NOT execute commands. Preparation errors SHALL preserve creation rollback. Once commands may have run, application failure SHALL preserve files and records. create SHALL return without awaiting application success. Plain shells MAY launch directly to preserve immediate input.

#### Scenario: Registry persistence failure
- **WHEN** a terminal is prepared but registry persistence fails
- **THEN** configured commands do not run and new worktree/branch creation rolls back

#### Scenario: Command inspects registration
- **WHEN** the first command reads the shared registry
- **THEN** its session is already present with the selected template

### Requirement: Interactive sequential execution and fallback
Entries SHALL run sequentially in one shell context after interactive initialization, starting in the session directory. Nonzero status SHALL skip remaining entries. Completion, failure and Ctrl+C SHALL leave a shell, output and visible exit status without retries. Explicit exit/exec SHALL retain shell semantics.

#### Scenario: Shared shell context and failure
- **WHEN** entries export a variable, change directory and then return nonzero
- **THEN** later entries do not run and the resulting shell retains the directory and environment

#### Scenario: Foreground interruption
- **WHEN** Ctrl+C interrupts a template command
- **THEN** remaining entries do not run and the terminal accepts ordinary shell input

### Requirement: Literal startup environment
User terminals SHALL receive GG_BRANCH, GG_WORKTREE and GG_PROJECT_ROOT through the environment. GG_BRANCH SHALL be the actual branch at the launch of each terminal, including restarts and additional panes, or empty at detached HEAD. Branch names and paths SHALL NOT be interpolated into generated shell text. Paths with spaces and valid branches containing shell metacharacters SHALL remain literal data.

#### Scenario: Branch with shell metacharacters
- **WHEN** a valid branch contains dollar expressions or parentheses
- **THEN** the environment contains the exact name and its implied command is not executed

### Requirement: Whole-template resolution and reserved shell
Whole definitions SHALL resolve in order: built-ins → global grand-git/templates → primary .gg/templates. Reserved shell SHALL always mean a shell; the catalog SHALL report attempted override files.

#### Scenario: Local replacement and reserved name
- **WHEN** local codex overrides global and shell.yaml attempts to define commands
- **THEN** codex uses only local commands; shell remains a shell and the catalog reports the reserved name

### Requirement: Supported runtimes and explicit execution boundaries
Command templates SHALL support sh/bash/zsh, including dash as sh; other shells SHALL allow plain shell and reject command templates before preparation. Additional panes SHALL be plain shells without repeating templates. Applications, dependencies, ports and hook trust SHALL NOT be installed or configured automatically.

#### Scenario: Unsupported shell before creation
- **WHEN** a command template is selected with another shell
- **THEN** creation is rejected before a new branch/worktree with a suggestion to select shell

#### Scenario: Additional terminal after startup
- **WHEN** another pane is added after a template runs
- **THEN** it opens a plain shell without repeating commands

### Requirement: Directory templates and explicit folder shells
Ordinary persistent terminals SHALL support --template, --choose-template, global default_template and global definitions without reading the directory's .gg. The section New terminal here action and existing legacy temporary terminals SHALL use shell, including inside Git. New dashboard launches SHALL run no template or shell. For terminal/temporary, GG_BRANCH SHALL be empty, GG_WORKTREE SHALL be the actual cwd and GG_PROJECT_ROOT SHALL be the group root; values SHALL be passed literally.

#### Scenario: Global template in an ordinary directory
- **WHEN** create --template mark runs outside Git
- **THEN** global mark commands run after the record is committed

#### Scenario: Dashboard launch runs no commands
- **WHEN** default_template specifies a command template and dashboard opens
- **THEN** no shell or template command runs until the user explicitly creates or opens a session
