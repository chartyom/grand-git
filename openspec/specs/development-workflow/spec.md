# Development Workflow Specification

## Purpose

Enforce the user's requirement: after every completed plan, verify implementation compliance, update agreed specifications and preserve verification evidence.

## Requirements

### Requirement: Specification-aware planning
Agents SHALL read context and affected main specs before planning or behavior changes. Plans SHALL stay in chat without repository artifacts. OpenSpec change directories and proposal/design/tasks/delta materials SHALL require an explicit user request. Ordinary implementation SHALL update main specs and a compact verification report directly. Latest user instructions SHALL override older requirements; deferred features SHALL NOT count as implemented.

#### Scenario: Planning without scaffolding
- **WHEN** the user requests a plan or ordinary implementation without explicitly requesting OpenSpec change materials
- **THEN** no change directory, proposal, design, tasks or delta files are generated

#### Scenario: User changes CLI behavior
- **WHEN** the user changes startup behavior or required fields
- **THEN** the agent updates code, tests, help and affected requirements as one change

### Requirement: Verification after every completed plan
After every completed plan, the agent SHALL compare affected requirements and scenarios with implementation and tests, fix discrepancies and update main specifications. Actual results SHALL be consolidated in openspec/verification.md without substituting format validation for behavioral checks. Unperformed checks and work-status notes SHALL be reported in the terminal/chat response.

#### Scenario: Implementation completed
- **WHEN** plan tasks are implemented
- **THEN** the report links affected requirements, tests and check results
- **AND** main specifications reflect the agreed new behavior

#### Scenario: Manual verification unavailable
- **WHEN** verification needs user trust for live Codex hooks or an unavailable C compiler
- **THEN** the terminal/chat response states which check did not run, without recording it as passed

### Requirement: Honest completion and consolidation
A finished change SHALL be consolidated into the main specifications and current verification report only after appropriate checks. Checked tasks SHALL reflect work actually completed. Completed change directories SHALL be removed after consolidation; unfinished work and unperformed checks SHALL NOT be marked complete.

#### Scenario: Change ready to consolidate
- **WHEN** behavior is verified and main specifications are synchronized
- **THEN** completed work and evidence are consolidated and its change directory is removed

### Requirement: Current-only specification workspace
OpenSpec SHALL contain current behavior, architecture, verification evidence and actively pursued changes. Superseded proposals, completed change archives and deferred wish lists SHALL be removed. Implemented compatibility requirements SHALL remain. Assistant opinions, speculative caveats and work-status commentary SHALL stay in terminal/chat responses rather than production code, UI text or repository documentation.

#### Scenario: Superseded change cleanup
- **WHEN** old change materials describe behavior already replaced by the main specifications
- **THEN** those materials and their index links are removed without deleting current requirements

#### Scenario: Unperformed platform verification
- **WHEN** a native platform check has not run
- **THEN** the terminal/chat response reports it and the verification report lists only actual check results

### Requirement: Dedicated usage skill maintenance
The usage skill SHALL live in docs/skills/grand-git/SKILL.md. The project SHALL provide a dedicated Codex skill-maintainer profile. AGENTS.md SHALL require delegating skill review and updates after changes to public commands, flags, JSON, settings or dashboard. The current verification report SHALL distinguish profile creation from a review actually performed.

#### Scenario: Public API changes
- **WHEN** the public interface changes
- **THEN** a separate agent checks the actual interface and updates the skill within its limited write scope

#### Scenario: Delegation unavailable
- **WHEN** a separate agent is unavailable
- **THEN** the review is marked incomplete without claiming a successful independent review

### Requirement: English repository text with Russian localization
Repository prose, including documentation, agent instructions, specifications and active change records, SHALL use English except README.ru.md. Internal error messages SHALL use English while retaining Russian interface resources, localization tests and legacy data compatibility. The configurable ru language SHALL remain available.

#### Scenario: Translating project text
- **WHEN** Russian repository prose is translated
- **THEN** README.ru.md remains unchanged, current requirements retain their meaning, and Russian text remains only where needed for the configurable Russian interface or legacy compatibility

#### Scenario: English internal errors with Russian presentation
- **WHEN** an internal error is presented to a user
- **THEN** the default interface uses English and language ru continues to use its Russian localization

### Requirement: Local interactive terminal design
The repository SHALL provide an offline, buildless HTML prototype in design/ with terminal-compatible layout, fictional in-memory data and repeatable scenarios for dashboard actions. Demonstration controls SHALL be separate from the modeled terminal. Actions SHALL NOT execute Git, tmux or shell commands.

#### Scenario: Inspect actions without a live project
- **WHEN** index.html is opened locally and the user selects a scenario and performs actions
- **THEN** the prototype shows the resulting states without executing Git, tmux or shell commands
- **AND** reset restores the scenario's initial state

#### Scenario: Demonstrate browser-reserved terminal input
- **WHEN** the user activates the prototype's Next panel control
- **THEN** it models Ctrl+W panel cycling without taking over the browser's close-tab shortcut

### Requirement: Prototype folder and session navigation
The prototype SHALL show worktree and terminal marks in one list per exact-path section. Launch SHALL focus the list without creating or opening a session. A single click SHALL open/start a session; arrows SHALL browse section headers and sessions. Hover SHALL preview details without changing selection or terminal focus; leaving SHALL restore selected details. Empty unfiltered sections SHALL support creation.

#### Scenario: Open and inspect independently
- **WHEN** the user hovers another session and then clicks a stopped session
- **THEN** hover previews that session's path and statuses without opening it
- **AND** the click starts and opens the exact clicked session

#### Scenario: Select a folder
- **WHEN** keyboard navigation reaches a folder header
- **THEN** the folder has visible focus, directory details and its own actions
- **AND** the current terminal remains open

### Requirement: Prototype target-bound context menus
Right-clicking a prototype session, terminal or folder SHALL open actions beside the pointer without opening a terminal. Menus SHALL fit terminal bounds, support keyboard operation and close with Esc or an outside click. Session menus SHALL offer creation, opening, additional terminals, Start/Stop according to activity and deletion for unprotected records.

#### Scenario: Menu for a stopped session
- **WHEN** a stopped session is right-clicked
- **THEN** its menu offers Start without starting it until the action is selected
- **AND** a running session offers Stop instead

### Requirement: Prototype group lifecycle actions
Folder actions SHALL offer a plain shell in that directory, stopping running sessions, deleting all sessions and worktree creation only for Git folders. Bulk confirmations SHALL bind to fixed session identities, exclude later records and preflight surviving targets before stopping processes. Deletion SHALL preserve Git branches, ordinary directory files and protected worktree directories.

#### Scenario: Relative worktree paths
- **WHEN** the user selects R on a Git folder and confirms host compatibility
- **THEN** the prototype models enabling relative links for that workspace

#### Scenario: Group operation with changing records
- **WHEN** a group deletion confirmation is open and a target disappears or another record is created
- **THEN** confirmation affects only the surviving original targets in that group
- **AND** a modified worktree or unreferenced detached commit prevents any stop or deletion

#### Scenario: Named worktree
- **WHEN** a user creates a worktree in the prototype
- **THEN** the tree starts at detached HEAD unless the optional branch is filled, and details show the checkout separately from its name

#### Scenario: Worktree preparation in the prototype
- **WHEN** a user submits a worktree and moves to another terminal during simulated copying
- **THEN** the pending row animates, completion preserves that terminal and a failed copy can be retried with its original form values

#### Scenario: Plain terminal in a Git folder
- **WHEN** New terminal here is selected for a Git folder
- **THEN** a plain shell opens in that folder without creating a branch or worktree
- **AND** it appears in the shared session list

### Requirement: Prototype dedicated search controls
Up above the first prototype list item SHALL focus eligible search. Down, Enter and Down-to-list SHALL return to the list with the query applied. Esc during editing or an applied query SHALL clear search state, reveal and select the open terminal's row, and retain list focus without closing a compact list. Clicking an applied field SHALL resume editing. Section headers and Add section SHALL NOT count toward eligibility.

#### Scenario: Search navigation and cancellation
- **WHEN** Up enters search, text filters the list and Down leaves the field
- **THEN** the query remains applied and session controls return
- **AND** Esc clears the query and selects the open terminal's row

### Requirement: Prototype search-only hints
Search editing SHALL show only Clear and Down-to-list footer controls, hiding session and folder actions. Clear SHALL empty the query while retaining search focus. Leaving search editing SHALL restore contextual list controls.

#### Scenario: Search-only hints
- **WHEN** the prototype search field has focus
- **THEN** the footer contains Clear and Down-to-list controls without session or folder actions
- **AND** clearing keeps input focus while Esc restores the open terminal's list selection

### Requirement: Complete stable prototype selectors
Every DOM element, including metadata and dynamically created elements, SHALL have a nonempty document-wide unique data-testid. Entity selectors SHALL remain stable across filtering, reordering and localization. Browser tests SHALL check this invariant after transitions.

#### Scenario: Verify selectors through transitions
- **WHEN** browser tests open forms, switch sessions, filter records, change language or resize the terminal
- **THEN** every DOM element retains a unique nonempty data-testid and unchanged entities retain their selectors

### Requirement: Prototype folder browser and empty sections
The prototype SHALL model + Add section using a fictional directory tree with path input, browsing, parent navigation and Ctrl+S confirmation. It SHALL model exact paths, duplicate selection, invalid paths and empty-section removal without reading the host filesystem. Root and nested Git sections SHALL share workspace identity and worktree name uniqueness while retaining independent session membership. Q and relaunch SHALL preserve persistent sessions without creating a temporary shell.

#### Scenario: Add a nested Git section
- **WHEN** the user adds /workspace/atlas/src and creates a terminal there
- **THEN** its path is exactly /workspace/atlas/src and the root section's sessions remain unchanged

#### Scenario: Empty startup and relaunch
- **WHEN** a fresh dashboard is opened or an existing dashboard is relaunched
- **THEN** Add section is available, no terminal is opened automatically and existing session identities remain
