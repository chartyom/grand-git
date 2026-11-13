# Sessions Specification

## Purpose

Define parallel task isolation, base branch selection, terminal persistence and file protection when deleting sessions in one repository.

## Requirements

### Requirement: Independent worktrees and bases
In Git, every create task SHALL have a separate worktree and an active background terminal using the selected template. New branches SHALL start from the current HEAD or an explicit base. Existing free branches SHALL attach without moving their commit; combining an existing branch with --base SHALL be rejected.

#### Scenario: Tasks from main and release
- **WHEN** tasks use `--base main` and `--base release/demo` pointing to different commits
- **THEN** each task's worktree contents match its own base

#### Scenario: Branch already checked out
- **WHEN** the requested branch is used by another worktree
- **THEN** creation fails without switching that worktree

### Requirement: Stable project registry
Git tasks SHALL use <git-common-dir>/grand-git for their registry and service data and SHALL have unique branches and stable internal IDs. New records SHALL use increasing positive IDs; legacy IDs, including source 0, SHALL persist while registered. Ordinary/temporary names SHALL not reserve branches. Discovery SHALL NOT register source worktrees. Concurrent creates SHALL allocate unique IDs. Failure before startup commands SHALL roll back new records and created Git objects.

#### Scenario: Concurrent creation
- **WHEN** several processes create different tasks concurrently
- **THEN** all successful tasks have unique IDs and appear in list

#### Scenario: Failed terminal provisioning
- **WHEN** worktree creation succeeds but tmux is unavailable
- **THEN** the new branch and worktree roll back and no task enters the registry

#### Scenario: Existing registry without an origin row
- **WHEN** a registry without a source record is opened
- **THEN** no source record is added and existing IDs and processes remain unchanged

### Requirement: Persistent terminals
Switching and detach SHALL preserve user processes and the dashboard PID. Q and disconnection SHALL preserve persistent records and processes. Legacy clients SHALL clean only their own temporary records. Stopped managed and ordinary records SHALL remain registered; reopening SHALL run the saved template as a new process, not resume the former agent.

#### Scenario: Background process survives
- **WHEN** the user switches to another task and exits the dashboard
- **THEN** the background process started in the first task continues running

#### Scenario: Server stopped
- **WHEN** the tmux server stops
- **THEN** list retains tasks with stopped status and the dashboard can reopen them

### Requirement: Protected deletion with branch preservation
`delete <branch>` SHALL stop terminals and remove the worktree while preserving the Git branch. Without `--force`, user-modified and untracked files SHALL block deletion. Unmodified integration-generated files MAY be removed; this check does not protect Git-ignored files.

#### Scenario: Dirty worktree
- **WHEN** a task has user changes and delete runs without `--force`
- **THEN** files and the task remain and the CLI explains the refusal

#### Scenario: Explicit destructive deletion
- **WHEN** the user confirms deletion with `--force --yes`
- **THEN** the worktree and uncommitted files are removed but the branch remains

#### Scenario: Noninteractive deletion
- **WHEN** delete runs without a TTY and without `--yes`
- **THEN** the command refuses deletion without explicit confirmation

#### Scenario: Delete from the task being removed
- **WHEN** delete runs inside the target task's terminal
- **THEN** an independent process finishes directory and record removal after the window closes

### Requirement: Ignored environment discovery and source
Creation SHALL copy untracked Git-ignored files and directories, including dotfiles and node_modules but excluding shared .gg, respecting nested rules and negations. The source SHALL be the explicitly selected local base branch's worktree if available, otherwise the calling worktree. --no-copy SHALL disable copying without prompts.

#### Scenario: Hidden files and nested rules
- **WHEN** Git ignores .env, .agents, node_modules and files via nested .gitignore
- **THEN** those files appear in the new task
- **AND** ordinary untracked files, ignore-rule negations and tracked source code are not copied

#### Scenario: Release environment
- **WHEN** a task is created with --base release/demo and that branch has a worktree
- **THEN** the environment is copied from the release worktree rather than main

#### Scenario: Calling worktree and opt-out
- **WHEN** no explicit base worktree exists or no base is supplied
- **THEN** the calling worktree is the source, including invocations from its subdirectories
- **AND** --no-copy leaves the new environment without these copies

### Requirement: Independent and non-destructive environment copies
Copied regular files SHALL have independent inodes and retain source permissions and mtime; reflink MAY be used with ordinary copying as fallback. Symlinks SHALL be preserved without dereferencing. Copying SHALL NOT overwrite target branch files/links, traverse its symlinks or copy .git, nested repositories or special files. Failure SHALL roll back creation.

#### Scenario: Dependency edits stay local
- **WHEN** a regular file in copied node_modules is modified
- **THEN** the source stays unchanged; executable permissions and mtime are initially preserved

#### Scenario: Target conflicts and special files
- **WHEN** the target branch contains a file or link to an external directory, or the source contains a FIFO
- **THEN** existing files and the external directory remain untouched and the FIFO is skipped

#### Scenario: Copy error
- **WHEN** a required source file cannot be read
- **THEN** the task, new worktree and created branch roll back

### Requirement: Protected origin worktrees
Source and external worktree directories SHALL never be removed through gg, including with --force. Merely discovering, listing, creating tasks, initializing hooks or launching dashboard SHALL NOT register an extra source session. Running legacy protected records SHALL support opening, additional terminals and stopping; their directories and branches SHALL remain protected.

#### Scenario: Dashboard without duplicate source
- **WHEN** dashboard launches in a new project with no tasks
- **THEN** no task, source record or temporary shell is created

#### Scenario: Existing external worktree
- **WHEN** dashboard opens from an existing unmanaged worktree
- **THEN** no terminal is added automatically, and the external worktree and its files stay in place

#### Scenario: Protected deletion attempt
- **WHEN** the user calls delete on a source record with --force --yes
- **THEN** the command refuses to remove its directory, branch or terminals

### Requirement: Branch-named worktree paths
New worktrees SHALL use <project>__sessions/<branch>, retaining branch separators. Existing paths SHALL persist, including legacy numeric directories. Occupied directories, symlinks in parent paths and duplicate session names SHALL be rejected without overwriting.

#### Scenario: Nested branch and duplicate
- **WHEN** feat/feat1 is created and the same name is requested again
- **THEN** the first directory is at __sessions/feat/feat1 and repeated creation is rejected even after stop

#### Scenario: Legacy active worktree
- **WHEN** a project with a legacy numeric path is upgraded
- **THEN** its path, internal ID and running process remain

#### Scenario: Symlink or occupied path
- **WHEN** a parent directory is a symlink or the target directory already exists
- **THEN** creation preserves existing files and stays inside the sessions directory

### Requirement: Independent session activity
Activity SHALL mean at least one live user terminal; service panes, row selection and agent status SHALL NOT count. stop SHALL terminate terminal processes and retain files and branches. Managed/ordinary records SHALL remain restartable; inactive legacy source records SHALL be retired. start SHALL open a new shell for a stopped record and avoid duplicating active terminals.

#### Scenario: Stop and restart
- **WHEN** an active managed or ordinary session is stopped and started again
- **THEN** files and record remain and a new terminal runs the saved template; the previous agent process is not resumed

#### Scenario: All user terminals stopped
- **WHEN** no user terminals are running
- **THEN** the dashboard list remains accessible and shows zero activity

### Requirement: Project-wide lifecycle operations
stop --all SHALL stop all selected project sessions, including source sessions. delete --all SHALL stop selected sessions, remove managed tasks and retire stopped legacy source records while retaining protected worktree directories and every Git branch. All targets SHALL be checked for removability before processes stop. Records created after confirmation SHALL NOT enter the fixed target set.

#### Scenario: Dirty bulk deletion
- **WHEN** at least one deletion target has user changes and force is absent
- **THEN** deletion is rejected before stopping processes or modifying files

#### Scenario: Protected source during bulk deletion
- **WHEN** delete --all is confirmed and tasks can be deleted
- **THEN** their worktrees are removed, source directories remain, user terminals stop and inactive source records are retired

#### Scenario: Operation from its own terminal
- **WHEN** a bulk operation closes its calling terminal
- **THEN** an independent service process finishes it and publishes the result in the dashboard

#### Scenario: Partial external failure
- **WHEN** deletion of one target fails after preflight checks
- **THEN** successful removals stay recorded, the error is reported and remaining files are not declared deleted

### Requirement: Legacy temporary session cleanup
New dashboards SHALL NOT create temporary records or shells. Legacy dashboards SHALL retain invocation tokens across transitions. Their Q/disconnection SHALL remove only their own temporary records and processes, preserving directories. Exit from the last legacy temporary shell SHALL remove its record while keeping the dashboard available. Stale workers SHALL NOT affect another invocation.

#### Scenario: Exact directory and independent invocation
- **WHEN** two new dashboards launch from a Git subdirectory or ordinary directory
- **THEN** neither creates a session or opens an existing terminal, and both show list focus

#### Scenario: Quit after project transition
- **WHEN** a legacy client switches projects and presses Q
- **THEN** that invocation's original temporary terminal is removed; persistent and other clients' sessions remain

#### Scenario: Disconnect without quit
- **WHEN** a legacy client disconnects without Q
- **THEN** its temporary record and processes are removed; persistent sessions and other dashboard invocations remain

#### Scenario: Last temporary shell exits
- **WHEN** the last shell of a temporary session exits while its client views another task
- **THEN** that temporary record is removed and the dashboard remains available

#### Scenario: Stale cleanup
- **WHEN** a worker belongs to a previous client invocation token
- **THEN** the current client and its temporary records are unaffected

### Requirement: Global workspace catalog
Groups SHALL live in XDG_STATE_HOME/grand-git or ~/.local/state/grand-git with atomic writes under flock and stable increasing IDs. Groups SHALL use canonical common directories for Git and canonical paths for ordinary directories. Public IDs SHALL be group:session. Git registries, sockets and paths SHALL persist. Corrupt catalogs SHALL NOT be overwritten. Legacy projects SHALL register on a working invocation of updated gg without scanning disks.

#### Scenario: Concurrent group registration
- **WHEN** processes concurrently register different groups and the same group again
- **THEN** each group has one unique stable ID

#### Scenario: Symlink and worktree identity
- **WHEN** a project opens through a symlink or another worktree
- **THEN** no duplicate group is created

#### Scenario: Corrupt catalog
- **WHEN** the catalog contains corrupt records
- **THEN** an error is reported and original bytes are preserved

### Requirement: Ordinary directory terminals
Outside Git, create SHALL register a persistent terminal in the current directory without creating directories, branches, worktrees or file copies. Names SHALL default to the directory name and duplicates SHALL be allowed. stop SHALL retain the record and files; start SHALL create a new terminal. delete SHALL terminate processes and remove only the record, always preserving the directory and all contents, even if it later becomes a Git project.

#### Scenario: Duplicate directory names
- **WHEN** create runs twice without a name in one ordinary directory
- **THEN** two sessions have the same name and distinct public IDs

#### Scenario: Safe deletion
- **WHEN** an ordinary session is deleted by ID
- **THEN** its processes and record are removed; all directory files remain

### Requirement: Safe legacy source retirement
Inactive legacy protected records SHALL be retired under the registry lock without removing files, branches or resetting IDs. Running records SHALL remain until their terminals stop. Unknown or malformed tmux responses SHALL preserve records; a confirmed absent server SHALL permit retirement. Unchanged reads SHALL NOT rewrite the registry.

#### Scenario: Mixed legacy registry
- **WHEN** a registry contains running and stopped sources alongside managed tasks
- **THEN** only inactive source records are retired; running PIDs, managed records and the next ID remain unchanged

#### Scenario: Uncertain server state
- **WHEN** tmux access fails or its reply cannot be parsed
- **THEN** source records remain and the failed query is not treated as proof of inactivity

#### Scenario: Explicit source stop
- **WHEN** a running legacy source is explicitly stopped
- **THEN** its processes stop and its record is retired while its worktree and Git branch remain

### Requirement: Persistent plain terminals in Git groups
Dashboard section actions SHALL create persistent kind=terminal records in the exact section directory using shell, including Git subdirectories. These records SHALL have empty branch/base, distinct stable IDs and no invocation ownership token. Start/stop/delete by public ID SHALL work normally; deletion SHALL remove only terminal processes and the record, preserving every directory file. Existing worktree and ordinary-directory records SHALL remain compatible.

#### Scenario: Git folder shell outlives dashboard
- **WHEN** a section shell is created in Git and the dashboard exits
- **THEN** its persistent terminal remains running and list JSON reports kind=terminal with workspace.type=git
- **AND** later deletion preserves the repository and its files

### Requirement: Persistent path sections and session membership
Sections SHALL have stable increasing internal IDs, workspace references and canonical paths. Git root and nested sections SHALL share the registry, server and public IDs while retaining separate session membership. CLI creation SHALL use its exact cwd section, adding it as needed; dashboard creation SHALL retain the selected section. Creation and empty-section removal SHALL use catalog-before-registry locks. Section removal SHALL preserve files and workspace records.

#### Scenario: Existing catalog migration
- **WHEN** a catalog without section metadata is opened
- **THEN** its known workspaces gain root sections once and existing records retain their IDs, paths and processes
- **AND** removing an empty migrated section does not cause it to reappear on later reads

#### Scenario: CLI creation from a subdirectory
- **WHEN** gg create runs from a Git subdirectory or managed worktree directory
- **THEN** the new session belongs to that exact directory's section while its managed worktree uses the existing project__sessions/branch path scheme

#### Scenario: Stopped record prevents section removal
- **WHEN** a section contains a stopped persistent session
- **THEN** removal is rejected without deleting the session, workspace or files

#### Scenario: Damaged section catalog
- **WHEN** section metadata is corrupt
- **THEN** the error is reported and the original catalog bytes remain unchanged
