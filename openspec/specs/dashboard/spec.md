# Dashboard Specification

## Purpose

Describe Grand Git's interactive session list and real terminals with panel cycling.

## Requirements

### Requirement: Persistent dashboard and terminal presentation
New dashboard invocations SHALL keep one gg TUI process across project switches and present real tmux terminal panes beside the sidebar. Input SHALL reach the selected pane. Launch SHALL show saved folder sections with list focus, without creating a session, opening a terminal, adding the calling directory or starting a tmux server for an empty screen. It SHALL work inside or outside tmux. Existing legacy dashboards SHALL continue until exit. Updates SHALL preserve user processes.

#### Scenario: Attach from an ordinary terminal
- **WHEN** dashboard launches outside tmux
- **THEN** it shows saved sections and an Add section action without opening a shell

#### Scenario: Existing tmux context
- **WHEN** dashboard launches inside its own or foreign tmux
- **THEN** the list opens without creating a second interactive tmux client

#### Scenario: Exact calling directory
- **WHEN** dashboard launches from a project subdirectory or outside Git
- **THEN** the calling directory is available as the folder browser starting path but is not automatically registered

#### Scenario: Upgrade a legacy dashboard
- **WHEN** a previous version left old panes
- **THEN** service panes update without stopping user processes

#### Scenario: Cross-project process continuity
- **WHEN** a new dashboard opens tasks in different project servers and returns
- **THEN** its gg PID, list state and user terminal PIDs remain unchanged

#### Scenario: Typing in a split terminal
- **WHEN** the user types a command into the focused pane while several terminal panes are visible
- **THEN** the dashboard forwards the input in order without recapturing all panes for each keystroke
- **AND** live terminal output, pane titles and the cursor continue to refresh

#### Scenario: Unavailable target server
- **WHEN** opening a task in another project fails
- **THEN** the dashboard remains open on the previous task and shows the error

### Requirement: Session list startup and background refresh
The first session list SHALL be usable before fresh Git status checks complete. Reading existing folder sections SHALL use the committed catalog during concurrent session creation. Each sidebar SHALL have at most one periodic refresh in flight. Detached worktree panes SHALL resume session and Git refreshes when a client displays their window.

#### Scenario: Launch while worktrees are active in the background
- **WHEN** a user opens a dashboard with existing folder sections and detached worktrees
- **THEN** the saved sessions are available for selection while Git status loads
- **AND** detached worktree sidebars do not each scan the Git projects

### Requirement: Terminal pane titles and status line
The dashboard SHALL show each tmux pane title above its contents and a green status line with the current project and activity count at the bottom. Both SHALL remain visible at compact width.

#### Scenario: Terminal title and status line
- **WHEN** the dashboard opens a task or changes to compact width
- **THEN** the pane title remains above the terminal and the green project/activity status line remains visible below it

### Requirement: Terminal style isolation
Terminal text attributes and background colors SHALL remain within their pane's content area without coloring the session list, separators, pane titles, status line or neighboring terminals. Consecutive styled rows and colored trailing spaces SHALL retain their appearance within each pane, including after splits and resizing.

#### Scenario: Colored terminal rows beside the session list
- **WHEN** an application displays consecutive rows with a colored background, including trailing spaces
- **THEN** each row keeps its background only inside the terminal content area and the session list keeps its own colors

#### Scenario: Independently styled terminal panes
- **WHEN** adjacent terminal panes display different background colors or the dashboard is resized
- **THEN** each pane retains its own row colors without extending them into neighboring panes or dashboard controls

### Requirement: Explicit keyboard command mode
List commands SHALL receive input only with list focus. Ctrl+W SHALL move focus through the list and terminal panes in screen order, wrapping after the last pane. Ctrl+W SHALL be consumed; Ctrl+G, ordinary arrows, Tab, Shift+Tab and Ctrl+P SHALL reach terminal applications. Esc SHALL cancel forms, confirmations and search, and SHALL close a compact list.

#### Scenario: Switch tasks
- **WHEN** the user focuses the list, selects a record and presses Enter
- **THEN** its terminal opens; stopped records start

#### Scenario: Revisit running tasks
- **WHEN** the user switches between running tasks without changing terminal size or task labels
- **THEN** terminal processes and application titles remain, and unchanged pane metadata and layout do not cause repeated full-screen redraws

#### Scenario: Cancel command mode
- **WHEN** Esc is pressed in a form, confirmation, search or compact list
- **THEN** the interaction is canceled without changing the open terminal

#### Scenario: Cycle without leaking input
- **WHEN** Ctrl+W focuses the list and another Ctrl+W returns to the terminal
- **THEN** the terminal application receives neither key
- **AND** separate Ctrl+G and Ctrl+P keys reach the application normally

#### Scenario: Multiple terminal panes
- **WHEN** Ctrl+W is pressed repeatedly in a window with multiple terminal panes
- **THEN** focus visits each pane and the list once per cycle

#### Scenario: Reverse tab in the terminal
- **WHEN** Shift+Tab is pressed while a terminal pane has focus
- **THEN** the application receives the BackTab key sequence without the key name being printed as text

### Requirement: Contextual task actions
The list SHALL offer N to create, Q to detach, T for an extra terminal, S to start/stop, D to delete unprotected records and comma for settings. stop/delete SHALL confirm a specific internal record; source sessions SHALL also support stopping.

#### Scenario: Empty dashboard
- **WHEN** there are no created tasks
- **THEN** Add section remains available and no automatic session row appears

#### Scenario: Additional terminal
- **WHEN** the user presses T for an active record
- **THEN** they choose Below or Right before pane creation and the new shell receives focus

### Requirement: Required-only dashboard creation
N SHALL create within the selected section, using its directory or the selected session as base. Git SHALL require a worktree name and offer a separate optional branch. Ordinary directories SHALL offer an optional name. The form SHALL show a template: Tab opens its list, arrows/Enter select, and Esc restores the choice or cancels. Ordinary creation SHALL open its terminal. Description SHALL be absent. Without a selection, users SHALL add or select a section first.

#### Scenario: Git creation
- **WHEN** N is pressed on a Git project session
- **THEN** Enter creates a detached worktree from the selected base with default_template when the branch is empty
- **AND** Up, Down or a click selects the optional branch field

#### Scenario: Branch changes in a live terminal
- **WHEN** the user switches to another branch or detaches HEAD in a managed worktree
- **THEN** the refreshed details and search reflect the new checkout while the row keeps its name and open terminal

#### Scenario: Directory creation
- **WHEN** N is pressed on an ordinary group and the form is confirmed with the default name
- **THEN** an ordinary terminal is created there without a branch question

#### Scenario: Optional picker cancellation
- **WHEN** the user opens Tab and cancels selection
- **THEN** the name and previous template remain

### Requirement: Background Git worktree preparation
The dashboard SHALL select an animated preparation row on accepting a Git worktree and show the copying phase during file transfer. Compact terminals SHALL show progress in the status line. Other sessions SHALL remain usable. Completion SHALL replace the row and open its terminal only if the initiating dashboard stayed on it with list focus. Session and bulk actions SHALL target completed sessions.

#### Scenario: Continue working during a large copy
- **WHEN** a worktree is copying ignored dependencies and the user opens another session of the same project
- **THEN** that terminal accepts input during copying and keeps focus when the worktree becomes ready

#### Scenario: Leave and return
- **WHEN** a dashboard quits or disconnects during worktree preparation
- **THEN** creation continues and another dashboard can see its pending row or completed session

#### Scenario: Correct a background error
- **WHEN** the copy fails
- **THEN** the animation stops, the row reports its error and Enter restores the submitted name, branch and template

### Requirement: Target-bound deletion confirmation
Confirmations SHALL stay bound to the selected record regardless of list changes. Y SHALL apply only to it; N/Esc SHALL cancel.

#### Scenario: Cancel deletion
- **WHEN** Esc is pressed in a confirmation
- **THEN** the task remains and the interface stays on the list

#### Scenario: Target changed externally
- **WHEN** the confirmation target disappears before Y
- **THEN** the neighboring task is unaffected

### Requirement: Searchable mouse-enabled session list
One row SHALL show each name and live activity without IDs or status suffixes. Details SHALL show path, terminal count, agent, Git status, current branch or detached HEAD and open state when space permits; Git SHALL be inapplicable to plain terminals. / SHALL search names, branches and groups when eligible; A SHALL filter activity across accessible groups. A single click or Enter SHALL open/start a session; clicking a folder SHALL select it and the wheel SHALL scroll.

#### Scenario: Session presentation
- **WHEN** active and stopped sessions of different types exist
- **THEN** circles reflect live user terminals regardless of agent status or focus, while selected details show the count and other statuses in words

#### Scenario: Search and mouse opening
- **WHEN** name/branch/group search or activity filtering is applied
- **THEN** matching records from all groups appear; the mouse opens the exact selected record

#### Scenario: Shared sidebar mouse input
- **WHEN** a client clicks a session in a shared legacy sidebar
- **THEN** that single click opens the session only for its initiating client

#### Scenario: Background Git status
- **WHEN** Git data refreshes
- **THEN** each project retains its own shared cache with at least two seconds between refreshes and unknown on error

### Requirement: Selected row and open terminal are distinct
The left arrow SHALL mark the terminal open in the current dashboard. The selected row SHALL use a distinct highlight and a separate selection mark when it is not the open row. Entering the list from a terminal SHALL select that terminal's row, clearing search and activity filtering only if needed to reveal it.

#### Scenario: Browse without opening
- **WHEN** keyboard selection moves to another row or the pointer previews it
- **THEN** the highlight follows keyboard selection, hover only updates details, and the arrow remains on the open terminal

#### Scenario: Return from a filtered terminal
- **WHEN** the open terminal's row is hidden by search or activity filtering as the list gains focus
- **THEN** the row is revealed and selected

### Requirement: Responsive compact sidebar
The sidebar SHALL occupy 25% of the window within 28–56 columns. Below 100 columns or 16 rows it SHALL hide; enlarging SHALL restore it without restarting user processes or changing terminal layout. Ctrl+W after the last terminal pane in compact mode SHALL open a temporary list.

#### Scenario: Wide narrow wide
- **WHEN** the window widens, narrows and widens again
- **THEN** the list returns to its full-height column on the left
- **AND** terminal panes keep their PIDs and their positions relative to each other as the available area changes

#### Scenario: Compact session access
- **WHEN** the list is hidden and the user presses Ctrl+W after the last terminal pane
- **THEN** a full-screen list is available; Ctrl+W or Esc closes it and Enter opens a session

### Requirement: Explicit terminal placement
Adding a second or later terminal SHALL always offer Below/Right. Esc SHALL cancel without a pane. The first terminal SHALL fill the right area without a placement question.

#### Scenario: Cancel placement
- **WHEN** the user cancels placement selection
- **THEN** the terminal count stays unchanged

#### Scenario: Horizontal and vertical placement
- **WHEN** Right or Below is selected
- **THEN** the pane appears right of or below that task's last user terminal, respectively

### Requirement: Conditional search availability
Search controls and hints SHALL be absent at ten or fewer candidate rows, and / SHALL do nothing. Candidates SHALL include dashboard rows across accessible groups after All/Active filtering, without group headings. Typing a query SHALL NOT change eligibility. Falling to ten rows SHALL clear the query, blur the field and retain list focus; growing to eleven SHALL restore availability without restoring an old query.

#### Scenario: Ten and eleven rows
- **WHEN** the candidate list grows from ten to eleven rows and then shrinks to ten
- **THEN** search becomes available and then clears and disappears without leaving the list

#### Scenario: No matches versus activity filtering
- **WHEN** an eligible query matches nothing
- **THEN** search remains available; changing to an activity filter with ten or fewer candidates clears it

### Requirement: Complete search cancellation
Esc SHALL clear editing and applied search, reveal and select the open terminal when present, and keep list focus. The activity filter SHALL remain unless it hides that terminal. Compact lists SHALL remain open until another Esc when a terminal is open; without a terminal, list access SHALL remain. Search SHALL filter while typing; Down or Enter SHALL apply the query and return to the list.

#### Scenario: Editing and applied search
- **WHEN** Esc is pressed while typing or after applying search with Enter
- **THEN** the query and search field clear, the open terminal is selected when present and list focus remains

#### Scenario: Search with no matches and active filter
- **WHEN** the query matches nothing and active filtering is enabled
- **THEN** Esc shows active records, selects the open terminal when present and does not close the compact list

### Requirement: Expiring successful operation feedback
Successful background stop/delete operations SHALL show green for five seconds from completion. Refresh, resize and re-entry SHALL NOT extend that deadline. Errors SHALL remain visible without automatic expiry. Colors SHALL respect terminal capabilities and NO_COLOR. Legacy Completed without timestamp metadata SHALL be expired; agent turn completion done SHALL NOT change behavior.

#### Scenario: Successful stop or delete
- **WHEN** a worker reports success
- **THEN** the message is green and hides after five seconds without reappearing on attach

#### Scenario: New feedback replaces old feedback
- **WHEN** a new message appears before the first expires
- **THEN** the first deadline does not hide the new message and errors do not disappear automatically

### Requirement: Legacy temporary source presentation
Existing legacy temporary records SHALL remain visible with dashboard as their TUI name, temporary lifetime in details and no public ID. They SHALL count toward search eligibility. New dashboards SHALL create no temporary records and SHALL NOT claim ownership of legacy records. Legacy cleanup SHALL remain bound to its original invocation token.

#### Scenario: New dashboard beside a legacy client
- **WHEN** a new dashboard opens while a legacy temporary terminal is running
- **THEN** the existing row remains visible without another temporary row
- **AND** quitting the new dashboard preserves the legacy terminal

### Requirement: Alphabetical folder sections and safe transitions
Dashboard and compact lists SHALL sort selectable folder sections alphabetically without counts or trailing markers. Paths SHALL distinguish duplicate names. Worktrees and terminals SHALL share creation order with ⎇ and >_ marks. Exact section paths SHALL remain independent of workspace roots. Empty unfiltered sections SHALL support creation. Scrolling SHALL retain selection and its heading; browsing SHALL start nothing.

#### Scenario: Equal branch names
- **WHEN** two groups contain the same branch and one record is confirmed for deletion
- **THEN** only the fixed group/session pair is affected

#### Scenario: Scrolling and compact mode
- **WHEN** the selected record lies below several groups or a compact list is open
- **THEN** the record and its header are visible; IDs are hidden and folder headers can be selected without opening a terminal

#### Scenario: Cross-server transition
- **WHEN** Enter opens another group's record
- **THEN** the same gg process shows the target server's terminal; existing terminal PIDs remain

#### Scenario: Unavailable group
- **WHEN** one project is unavailable
- **THEN** its group shows an error while other sessions stay accessible

### Requirement: Compact contextual controls
The sidebar SHALL omit its title and total count, showing Active only when filtered. Session footers SHALL show Open/New/Actions/Quit and Delete for unprotected selections. Folder and search footers SHALL show their contextual controls. ? SHALL open actions; Esc SHALL close them before clearing search. Focus SHALL remain visible without color. Details SHALL identify the open session. Refresh SHALL retain selection or choose the nearest remaining item.

#### Scenario: Contextual actions
- **WHEN** ? opens for a selected protected session or a short list
- **THEN** delete or search respectively is absent; shortcuts and visible mouse targets remain usable

#### Scenario: Refresh while browsing
- **WHEN** rows refresh or the selected record disappears
- **THEN** selection remains on that record or its nearest remaining neighbor rather than jumping to the first row

### Requirement: Content-aware sidebar layout
The list viewport SHALL reserve actual header, details, footer and notice heights. Scrolling SHALL retain the selected row and its group heading. Text and hit targets SHALL stay within terminal bounds; action menus SHALL scroll in short windows. English, Russian and NO_COLOR SHALL remain supported.

#### Scenario: Short localized window
- **WHEN** the sidebar or compact list is resized with long names and Russian labels
- **THEN** the selected row remains visible, actions stay accessible and invisible controls cannot receive mouse clicks

### Requirement: Background stop preserves focus
Stopping or deleting a background session SHALL preserve the current terminal and list focus. Closing the currently open session SHALL first move to the service list so the dashboard remains available.

#### Scenario: Stop a different task
- **WHEN** CLI stops a task while the dashboard is browsing or running another session
- **THEN** the open window and focus remain unchanged

### Requirement: Target-bound context menus and overlays
Right-click SHALL select a folder or session and open its actions beside the pointer without starting or opening it. ? and Shift+F10 SHALL open selected-target actions. Targets SHALL stay bound to group/session identities across refresh. Pointer menus SHALL contain target actions; keyboard menus SHALL also offer available search, filtering, settings and quit. Esc or an outside click SHALL dismiss a pointer menu.

#### Scenario: Inspect a stopped session
- **WHEN** a stopped session is right-clicked
- **THEN** its menu offers Start without starting the session
- **AND** a subsequent action applies to that exact target even if list selection changes

#### Scenario: Short localized dialogs
- **WHEN** a menu or form is opened or resized in a short Russian or colorless window
- **THEN** its selected control and cancel action remain visible and all hit targets fit the rendered dialog

#### Scenario: Correct invalid creation input
- **WHEN** creation fails validation
- **THEN** the form keeps its name and template, displays the error and permits correction or cancellation

### Requirement: Folder lifecycle and creation actions
Sections SHALL offer T for a persistent shell in their exact directory and N for worktree creation in Git or a directory session elsewhere. Enter SHALL open folder actions. S SHALL confirm stopping running sessions; D SHALL confirm deleting only that section's sessions. Confirmation SHALL capture the full target set independently of filtering. Protected directories, ordinary files and Git branches SHALL remain.

#### Scenario: Plain terminal in a Git folder
- **WHEN** T is pressed on a Git folder
- **THEN** a persistent shell opens at the section path without a branch, worktree or copied files
- **AND** it appears in the shared session list with a terminal type mark and Git inapplicable

#### Scenario: Changing bulk targets
- **WHEN** a target disappears and another session is created while a folder deletion confirmation is open
- **THEN** only surviving original targets are considered
- **AND** a modified worktree blocks the entire operation before any terminal stops

#### Scenario: Empty folder after deletion
- **WHEN** the last session in a folder is deleted
- **THEN** the unfiltered folder remains selectable and supports creation

### Requirement: Enable relative Git worktree paths
Only available Git sections SHALL offer R. Confirmation SHALL bind to the selected project and ask whether all clients, including the host, use Git 2.48+. After checking local Git, gg SHALL set worktree.useRelativePaths in the repository, repair existing links and verify each worktree with git status. Errors SHALL identify the cause and allow retry.

#### Scenario: Share an existing and a new worktree
- **WHEN** a user confirms on a Git section with Git 2.48+ and the host and VM share the main repository and its sibling worktrees in the same layout
- **THEN** the project's existing links are converted and future worktrees use relative links
- **AND** Git can report local changes in both environments while the existing gg sessions and their terminals stay in place

#### Scenario: Cancel or use an older Git
- **WHEN** the user cancels the confirmation or confirms with a local Git older than 2.48
- **THEN** the repository configuration and worktree links remain unchanged

### Requirement: Independent hover preview
Hover SHALL preview session or folder details without changing selection, focus or the open terminal. Leaving the list SHALL restore selected-item details. Motion SHALL use the displayed list's cached data without starting terminals.

#### Scenario: Preview another folder
- **WHEN** the pointer moves over another folder while a terminal has focus
- **THEN** that folder's path and counts appear in details without moving focus or opening a session
- **AND** leaving the list restores the selected item's details

### Requirement: Search editing controls
Up above the first item SHALL focus available search. Clicking an applied field SHALL resume editing. Editing SHALL show only Clear and Down-to-list in the footer. Clear SHALL empty the query while retaining input focus; Down, Enter and Down-to-list SHALL apply the query and restore list controls. Section headings and the Add section control SHALL NOT count toward search eligibility.

#### Scenario: Clear and resume browsing
- **WHEN** Clear is clicked during search editing
- **THEN** the query empties and input retains focus
- **AND** Down returns to the list without changing the open terminal

### Requirement: Bounded interactive dashboard dialogs
New dashboards SHALL show creation, template selection, placement, settings, menus and confirmations as overlays above terminal content. Keyboard and mouse controls SHALL remain inside visible bounds, with scrolling when needed. Modal input, including Ctrl+W, SHALL NOT reach terminal applications. Creation errors SHALL retain entered values. The project/activity status line SHALL remain visible.

#### Scenario: Resize a creation form
- **WHEN** the terminal is resized while creation or its template picker is open
- **THEN** the name and selected template remain, controls fit the new bounds and terminal processes continue

### Requirement: Surviving fixed bulk targets
Dashboard bulk workers SHALL skip disappeared confirmed targets and exclude records created later. They SHALL preflight every surviving deletion target before stopping any process. If no confirmed targets survive, they SHALL report that without affecting other records. A failed preflight SHALL preserve all processes and files. Later operational failures SHALL report partial results accurately.

#### Scenario: Confirm after all targets disappear
- **WHEN** every confirmed target was removed externally before confirmation
- **THEN** the operation reports that no targets remain and does not affect later records

### Requirement: Explicit folder section management
The dashboard SHALL provide + Add section in empty and compact lists. Its bounded folder dialog SHALL support path input, hidden directories, parent navigation, mouse, keyboard and Esc cancellation. Relative paths SHALL use launch cwd; ~/ SHALL expand home. Ctrl+S or Add SHALL confirm an accessible directory, select its section and open no terminal. Browsing SHALL create nothing. Errors SHALL retain input; duplicate canonical paths within a workspace SHALL select the existing section.

#### Scenario: Root and subdirectory sections
- **WHEN** the user adds /project and /project/src from one Git repository
- **THEN** both sections remain independently selectable and T opens a persistent shell at the selected exact path

#### Scenario: Invalid path and cancellation
- **WHEN** an inaccessible path is submitted or the folder dialog is canceled
- **THEN** no section or session is created; submission errors retain the input for correction

#### Scenario: Remove an empty section
- **WHEN** an empty section is removed and the dashboard is reopened
- **THEN** it stays removed, files and workspace IDs remain, and concurrent creation cannot leave a session attached to a removed section

#### Scenario: Empty compact dashboard
- **WHEN** no terminal is open and Esc or Ctrl+W is pressed in a compact list
- **THEN** the list and Add section action remain accessible

### Requirement: Empty section removal
X and Remove section SHALL remove only sections without registered sessions, including stopped records. Removal SHALL preserve files and workspace records, recheck emptiness against concurrent creation and persist across dashboard restarts. Unavailable sections SHALL NOT be removed without a reliable emptiness check.

#### Scenario: Concurrent creation and removal
- **WHEN** another process creates a session while an empty section is being removed
- **THEN** removal is rejected or creation reports that its section disappeared; no committed session references a removed section
