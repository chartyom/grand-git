# Configuration Specification

## Purpose

Define global and shared project YAML settings, migration and default template selection for Grand Git.

## Requirements

### Requirement: Layered shared configuration
Settings SHALL apply in this order: built-ins → global grand-git/config.yaml in absolute XDG_CONFIG_HOME or ~/.config → shared primary .gg/config.yaml. config SHALL show values and sources. set/reset and config set/reset SHALL default to global scope; --local and compatible --project select local scope. Reading SHALL NOT create files.

#### Scenario: Override and reset
- **WHEN** the project overrides language and reset language --local removes the key
- **THEN** the global language applies and other settings remain

#### Scenario: Shared primary configuration
- **WHEN** commands run from another task with its own .gg
- **THEN** reads and local writes use the primary directory's .gg

#### Scenario: Invalid input or malformed configuration
- **WHEN** a setting or YAML is invalid
- **THEN** an error is reported and the file remains unchanged

### Requirement: Localized interface and stable machine codes
Language SHALL default to en; ru SHALL switch native CLI/dashboard messages, help, forms and confirmations. Machine keys, commands and status codes SHALL remain English. Legacy Russian statuses SHALL map to corresponding codes.

#### Scenario: Language switching
- **WHEN** language changes to ru
- **THEN** help and running panes show Russian without restarting user terminals

### Requirement: Dashboard panel cycling
The isolated project server SHALL bind Ctrl+W for panel cycling and remove the former Ctrl+G prefix, direct Ctrl/Alt arrow and Alt+H/J/K/L bindings. Existing user processes SHALL continue running.

#### Scenario: Existing server keymap update
- **WHEN** an existing dashboard is upgraded
- **THEN** Ctrl+W cycles panels, the old direct keys and Ctrl+G reach applications, and terminal processes remain

#### Scenario: Removed navigation setting
- **WHEN** config displays settings or set/reset targets navigation
- **THEN** config omits navigation and set/reset rejects it without modifying files

### Requirement: Dashboard settings form
The settings form SHALL select language, default_template and Global/Local scope through keyboard or mouse controls; ordinary directory groups SHALL offer only Global. Enter SHALL save values in one atomic update; Esc SHALL cancel. Settings SHALL apply without restarting user terminals.

#### Scenario: Save or cancel settings
- **WHEN** the user changes values in the form
- **THEN** files stay unchanged until save; Enter applies the selected scope

### Requirement: Legacy TOML layer migration
If a level has no YAML, the system SHALL read legacy global config.toml or project <git-common-dir>/grand-git/config.toml. The first write SHALL migrate active values at the selected level, apply the change and atomically save YAML without removing TOML. Deprecated navigation values in YAML or TOML SHALL be accepted but ignored and omitted on the next write. YAML SHALL completely replace that level's old source; resetting the last key SHALL leave an empty YAML object.

#### Scenario: First write migrates a complete layer
- **WHEN** legacy TOML has language and navigation and the user changes default_template
- **THEN** YAML retains language and default_template but omits navigation; TOML is unchanged

#### Scenario: Reset never revives obsolete values
- **WHEN** all local YAML keys are reset while old TOML remains
- **THEN** global values apply without rereading old local TOML

### Requirement: Default startup template setting
default_template SHALL default to shell and accept valid template names. It SHALL select templates only for create without --template; later changes SHALL NOT change existing sessions' templates.

#### Scenario: Default applies only to new records
- **WHEN** default_template changes to codex
- **THEN** new sessions select codex while legacy and source records retain shell

### Requirement: Strict YAML settings validation
Unknown fields, invalid types, duplicate keys, multiple YAML documents and invalid values SHALL be rejected without overwriting the file.

#### Scenario: Reject and preserve malformed settings
- **WHEN** saving is attempted with malformed YAML
- **THEN** the command reports an error and preserves the original bytes

### Requirement: Primary worktree configuration authority
Local .gg SHALL be read and modified in the primary working directory regardless of the calling worktree. Other worktrees' .gg versions SHALL NOT replace the shared source.

#### Scenario: Local write from another task
- **WHEN** set --local runs from a task with its own .gg
- **THEN** the primary directory's configuration changes and the task's file is preserved

### Requirement: Directory workspace settings
Ordinary groups SHALL use only built-in and global settings. Their .gg SHALL NOT be read or modified. --local/--project SHALL be rejected outside Git; ordinary group forms SHALL offer only global saving. Dashboard settings SHALL belong to the open context, not merely a selected row from another group.

#### Scenario: Ignore directory local configuration
- **WHEN** an ordinary directory contains malformed .gg/config.yaml
- **THEN** a terminal session is created using global settings

#### Scenario: Local scope outside Git
- **WHEN** --local is requested in an ordinary directory
- **THEN** the operation is rejected without changing files
