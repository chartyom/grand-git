# Grand Git terminal design

Open [index.html](index.html) directly in a browser. HTML, CSS and JavaScript are local, with no runtime packages, build step, network requests or server requirement. An optional HTTP preview is available with `python3 -m http.server 8080 --directory design` from the repository root.

The prototype models the existing dashboard in a character grid: grouped sessions and contextual details on the left, terminal panes on the right, and a project/activity status line below. Its controls use text, rectangular boundaries and terminal colors. The scenario toolbar and guide outside the terminal are demonstration controls.

## Interaction

Click a session once to open its terminal; stopped sessions start immediately. Up/Down browse sessions and folder headers without changing the open terminal. Worktree sessions (⎇) and plain terminals (>_) share a list in creation order inside each folder section. Folder names have no trailing indicator. Hover over a session or folder to preview its details without changing selection or the open terminal. Leaving the list restores details for the selected item. Session commands apply with list focus:

| Input | Action |
| --- | --- |
| Up/Down or J/K | Select a folder or session; Up above the first folder enters available search |
| Enter | Open/start a session, or show the selected folder’s actions |
| + / X | Add a folder section / remove an empty section |
| N | Create a Git worktree session or ordinary directory session |
| T | Add a plain terminal Below or Right; start a stopped session; on a folder, open a new shell there |
| S | Start or confirm stop; on a folder, confirm stopping all running sessions |
| D | Confirm deletion for an unprotected session, or all sessions in the selected folder |
| A | Switch All/Active filtering |
| / | Search when more than ten activity-filtered candidates exist |
| Right-click / ? / Shift+F10 | Actions for the selected session or folder |
| , | Settings for the open project |
| Q | Detach while persistent sessions remain |
| Esc | Cancel the current dialog, fully cancel search and select the open terminal’s row, or close the compact list |

Right-click opens a menu beside the pointer without opening or starting the target. Worktree and terminal menus offer creation, opening, additional terminals, Start/Stop according to live terminals, and Delete for unprotected records. Folder menus offer a new plain shell in that directory, Stop all and Delete all when applicable, and worktree creation for Git folders. Bulk confirmations freeze the target identities across filtering and external changes; modified worktrees block deletion before any terminal stops. Protected folders and Git branches remain. Empty folders stay accessible for creation.

Search editing shows only **Clear** and **↓ To list** in the sidebar footer. Clear empties the query and keeps input focus. Down, Enter or the To list button returns to the list with the query applied. Esc clears both editing and applied search, reveals the row whose terminal is open and keeps list focus, including in compact mode. Clicking an applied search field resumes editing. Search remains conditional on more than ten activity-filtered session rows; folder headers and add-section controls do not count.

Use **Next panel** in the demonstration toolbar to emulate Ctrl+W. The browser retains its own Ctrl+W shortcut. Native Tab navigation is available for controls; Tab from the creation name field opens the template picker. Esc returns from the picker without changing the name or saved choice. Forms accept both mouse and keyboard input. Terminal input appends to an illustrative transcript and does not execute commands.

The + action opens a folder browser backed by a fictional directory tree, with a path field, parent navigation and Ctrl+S confirmation. Existing sections are selected without duplication. `/workspace/atlas` and `/workspace/atlas/src` share one Git workspace but have separate session membership; shells start at the section path. X removes only empty sections. All browser actions remain in memory and do not access the host filesystem.

Settings default to English and the `shell` template. Russian, local Git-project overrides and global settings are modeled in memory. Ordinary directory settings offer only global scope. Existing sessions keep their saved template name.

## Scenarios and state

The initial fixture contains three groups and twelve sessions: managed worktrees, a protected running source, ordinary terminals, stopped records, and persistent plain shells in Git. All paths and terminal output are fictional fixtures; the prototype never reads real repositories or starts processes.

The scenario selector loads reproducible starting states for creation, directory sessions, splits, stop/start/delete, modified and protected worktrees, the eleven-to-ten search boundary, no search matches, settings, unavailable groups/templates, disappearing confirmation targets, empty/stopped dashboards, persistent shell exit, notification deadlines, duplicate groups, compact windows, long Russian labels, agent state transitions and colorless rendering.

The guide describes the steps and last action. **Simulate event** supplies external removal, last persistent-shell exit or notification events in the corresponding scenarios. **Reset** reloads the current scenario, including its default settings and terminal size. Reloading the page returns to the initial fixture. **Launch dashboard** after detaching restores list focus without creating a session or opening a terminal. The fresh-dashboard scenario has no sections or sessions.

Search eligibility counts candidates after activity filtering and before text filtering. Stop preserves managed and ordinary records. Protected worktrees cannot be deleted; modified worktrees refuse deletion. Successful managed deletion retains its branch, and ordinary deletion retains directory files. Confirmation holds a fixed group/session identity. Success notifications expire five seconds after completion; replacement notices have independent deadlines and errors do not expire automatically.

The size selector offers a measured fit-to-window grid and fixed 160×42, 100×16, 99×24 and 120×15 terminals. The sidebar uses 25% of the columns, bounded to 28–56. Below 100 columns or 16 rows, panel cycling opens a full-screen list. Terminal pane identity, output and relative splits survive resizing and project switches.

## DOM selector contract

Every element has a nonempty, document-wide unique `data-testid`, including document metadata, containers, labels, options, hidden controls and dynamically created elements. Text nodes do not require attributes. The `el` creation helper requires an explicit ID; HTML is rendered as individual DOM elements rather than a canvas or screenshot.

Selectors use an area and stable entity identity, for example `gg-sidebar-group-1-session-2-row`, `gg-sidebar-group-1-session-2-name`, `gg-pane-2-title` and `gg-confirm-yes`. Group/session/pane IDs do not depend on labels, language, ordering or filtering. A new shell gets a new pane ID. Repeated representations use distinct area prefixes. Terminal transcript line indices are stable because lines are append-only.

## Verification

Node.js 24 or later is used for the test runner. Install the pinned browser-test dependency with `npm ci --prefix design`. Install its Chromium browser and required OS libraries using Playwright's installation instructions; for a suitable development environment, `cd design && npx playwright install --with-deps chromium` installs both.

Run `npm test --prefix design`. Tests open the local HTML file in Chromium, exercise scenarios through their controls, and inspect model snapshots for lifecycle invariants. The DOM audit checks every element for missing, empty and duplicate selectors after transitions. Browser tests also check notification deadlines, localization, keyboard focus, compact boundaries and retained terminal identity. Browser automation is a development dependency only.

The prototype interaction requirements are in the development-workflow specification. The Go dashboard implements these terminal interactions with real sessions and tmux panes. Production behavior is defined in the dashboard, sessions, configuration and startup-template specifications; demonstration controls and fictional data belong only to this prototype.
