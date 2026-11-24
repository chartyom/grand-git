# CLI Specification

## Purpose

Define Grand Git's (`gg`) interface, worktree identity, session lifecycle and locale-independent automation.

## Requirements

### Requirement: Help-first entry point
CLI SHALL show help for `gg`, `gg --help`, `gg help` and describe create, list, start, stop, delete, dashboard, config, set, reset, template and init. Help and gg --version SHALL work without a Git repository, TTY, Git or tmux. Version SHALL include build version, commit and date; local builds SHALL be dev. The product SHALL be named Grand Git; the command SHALL remain gg.

#### Scenario: Bare command outside a repository
- **WHEN** the user runs gg in an ordinary directory
- **THEN** help appears without opening the dashboard or creating project state

#### Scenario: Command-specific help
- **WHEN** gg create --help is called
- **THEN** help explains branch, base, no-copy, template, choose-template, background creation and the absence of description

### Requirement: Required-only creation prompts
In Git, create SHALL prompt only for a missing required worktree name in a TTY and reject missing names without a TTY. Outside Git, create [name] SHALL create an ordinary terminal session without branch questions; names SHALL default to the current directory name. Description and --description SHALL be absent. branch, base and no-copy SHALL use flags without prompts and be rejected outside Git. --choose-template SHALL explicitly open a picker, require a TTY and conflict with --template.

#### Scenario: Name supplied
- **WHEN** a worktree name is supplied as an argument
- **THEN** creation asks no questions without --choose-template

#### Scenario: Missing worktree name in a terminal
- **WHEN** create runs in a TTY without a name
- **THEN** the form requests a worktree name and submission immediately completes it

#### Scenario: Explicit optional values
- **WHEN** base or no-copy is supplied
- **THEN** parameters apply without interactive questions

#### Scenario: Missing worktree name without a terminal
- **WHEN** both a name and --branch are missing without a TTY
- **THEN** the command fails without a session

#### Scenario: Cancel creation
- **WHEN** Esc is pressed in the form
- **THEN** no new session is created

### Requirement: Background creation result
create SHALL assign the session to the section for its exact calling directory, adding that section when needed. It SHALL start an active background terminal, print the session name, path and selected template and return control. Copying SHALL report its source and file count. shell SHALL be the built-in default; explicit or configured templates SHALL run their commands after registration. Successful create SHALL mean terminal creation, not application success.

#### Scenario: Explicit branch and optional matching name
- **WHEN** create receives --branch feat/login
- **THEN** it creates or attaches the branch and uses feat/login as the worktree name unless the caller supplied a separate name

#### Scenario: Successful creation
- **WHEN** create succeeds
- **THEN** the terminal runs in the background and the result suggests gg dashboard
- **AND** the public result contains no session number

#### Scenario: Copying is enabled
- **WHEN** no-copy is absent
- **THEN** the result includes the source and copied file count

### Requirement: Global grouped listing
list SHALL show active and stopped sessions from all known groups from any directory, including running legacy source sessions, without registering source worktrees automatically. The table SHALL include group, public ID, name, current branch, type, activity, terminals, agent and path. Unavailable groups SHALL be reported to stderr with nonzero exit status; stdout SHALL preserve healthy records and valid JSON.

#### Scenario: Same global list from another directory
- **WHEN** list runs from another directory or worktree
- **THEN** all registered groups appear with the same IDs

#### Scenario: No managed tasks
- **WHEN** a known Git project has no created tasks
- **THEN** no source record is invented; JSON may be an empty array

#### Scenario: Partial unavailable group
- **WHEN** one group is unavailable
- **THEN** other records appear, the error is reported separately and exit status is nonzero

### Requirement: Worktree name and global ID lifecycle commands
start, stop and delete SHALL accept positional public group:session IDs from any directory. Inside Git they SHALL also accept exact permanent worktree names in the current project. Numeric names SHALL address a worktree by name. stop/delete --all SHALL affect only the current Git group or ordinary directory and conflict with a session argument. Confirmations, --yes and worktree protection SHALL remain.

#### Scenario: Numeric worktree name and composite ID
- **WHEN** delete receives 1 inside Git or 3:7 from another directory
- **THEN** the first addresses current-project worktree named 1 and the second a specific record in group 3

#### Scenario: Noninteractive bulk operation
- **WHEN** stop --all or delete runs without a TTY and without --yes
- **THEN** the operation is rejected; --yes does not bypass file protection

#### Scenario: Directory bulk operation
- **WHEN** delete --all runs in an ordinary directory
- **THEN** only that group's records are removed; user directories and files remain

### Requirement: Unregistered source guidance
start SHALL NOT implicitly create a source session. A missing session SHALL fail with guidance to open gg dashboard, add the desired directory with + and press T on its section. Existing session IDs and JSON fields SHALL remain compatible; dashboard section IDs SHALL remain internal.

#### Scenario: Start main without a record
- **WHEN** start main runs in a new project without a session named main
- **THEN** it fails with gg dashboard guidance and does not create a record or terminal

### Requirement: Template inspection and startup override
template list [--json] and template show <name> SHALL work inside and outside Git. list SHALL show unique names and sources; JSON SHALL include name and source. show SHALL print editable YAML. start --template <name> SHALL change only a stopped session's template; active sessions SHALL reject changes without restarting. Plain start on active sessions SHALL remain idempotent.

#### Scenario: Inspect a preset
- **WHEN** template list --json and template show codex run
- **THEN** the definition source and commands YAML are available without launching an application

#### Scenario: Explicit recovery template
- **WHEN** a saved template is unavailable and start <name> --template shell runs
- **THEN** the stopped session saves shell and starts as a plain shell

### Requirement: Actionable runtime dependency diagnostics
Git operations SHALL check Git 2.31+; terminal creation/opening SHALL check tmux 3.4+. Ordinary terminals outside Git SHALL work without Git installed. Missing or outdated required dependencies SHALL produce minimum-version and installation diagnostics without a new session/worktree. Broken or bare Git contexts SHALL NOT silently become ordinary directories. Help/version SHALL work without dependencies.

#### Scenario: Missing or old tmux
- **WHEN** create or dashboard runs without tmux 3.4+
- **THEN** the command reports the cause and installation commands without a new task/worktree

#### Scenario: Missing Git outside a repository
- **WHEN** --help or --version is requested without Git in PATH
- **THEN** it succeeds without project state

### Requirement: Global public session JSON
JSON SHALL add id (a group:session string), name, kind and workspace to existing session fields. kind SHALL be source/worktree/terminal/temporary; workspace SHALL contain id/type/name/path. branch/base SHALL be empty strings for persistent plain terminals and temporary records, including kind=terminal in a Git workspace. temporary SHALL denote a live temporary terminal. description SHALL be absent; machine fields and codes SHALL remain English.

#### Scenario: Checkout changes
- **WHEN** list --json reads a named worktree after git switch
- **THEN** name and base retain the worktree name and original commit, and branch reports the new checkout or an empty string for detached HEAD

#### Scenario: Public record shape
- **WHEN** list --json prints a record
- **THEN** it includes id, name, kind, workspace, branch, base, path, created_at, protected, active, temporary, terminals, agent_status, template and optional copy_source/copied_files
- **AND** workspace contains id, type (git/directory), name and path
