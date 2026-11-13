# Codex Hooks Specification

## Purpose

Define local Codex status integration that preserves existing project settings without changing global configuration or user hook trust.

## Requirements

### Requirement: Project-local idempotent initialization
`init` SHALL add handlers with statusMessage=grand-git to project Codex configuration in the primary directory and managed worktrees, preserving other settings and hooks. Backups SHALL use <git-common-dir>/grand-git/hook-backups. Repeated runs SHALL NOT duplicate handlers. New tasks after init SHALL receive local integration automatically; global settings SHALL NOT change.

#### Scenario: Existing user configuration
- **WHEN** init runs with an existing third-party Stop hook and model setting
- **THEN** both remain and Grand Git handlers are added separately

#### Scenario: Repeated initialization
- **WHEN** init runs again
- **THEN** handlers are not duplicated and existing settings remain

### Requirement: Explicit Codex trust
The tool SHALL instruct users to inspect and confirm hooks through `/hooks`. It SHALL NOT grant trust automatically. Tasks SHALL remain usable without trusted hooks.

#### Scenario: Hooks not trusted
- **WHEN** the user has not confirmed project hooks in Codex
- **THEN** the task terminal is available but exact agent state is not guaranteed

### Requirement: Event-based agent status
For one primary Codex per task, events SHALL set states: SessionStart — ready; UserPromptSubmit/PreToolUse/PostToolUse — working; PermissionRequest — waiting for permission; Stop — turn completed; Interrupt — interrupted; SessionEnd — shell. Agent state SHALL differ from terminal lifecycle. Stored/public codes SHALL be ready, working, waiting, done, interrupted, shell, stopped and unknown; the interface SHALL localize them and read legacy Russian values.

#### Scenario: Permission request and completed turn
- **WHEN** PermissionRequest and Stop arrive sequentially for a task
- **THEN** state becomes waiting for permission, then turn completed, which does not mean the whole task is complete

#### Scenario: Unknown or stopped agent
- **WHEN** exact events are unavailable or no live terminals exist
- **THEN** the interface shows shell/unknown or stopped without hiding the task

### Requirement: Ignore unrelated hook events
Events SHALL bind through working directory and session identity. Explicit subagent events, unknown events and events outside managed tasks SHALL be ignored; handler failure SHALL NOT interrupt Codex.

#### Scenario: Event from a different root session
- **WHEN** Stop arrives with another session_id after primary session registration
- **THEN** it does not replace the primary session's state

### Requirement: Exclude terminal-only paths from initialization
init SHALL install hooks only in the primary directory, the calling real Git worktree and registered real Git worktrees, without registering a source session. terminal/temporary paths SHALL NOT become separate installation roots, and their records SHALL NOT receive worktree Generated metadata.

#### Scenario: Launcher from a subdirectory
- **WHEN** init runs after dashboard from repo/subdir
- **THEN** hooks are installed at the repository root; subdir/.codex is not created
