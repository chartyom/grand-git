# Current verification

Date: 2026-11-16.

## Scope

Explicit directory sections in the Go dashboard and offline HTML prototype. Dashboard launches with list focus, no open terminal and no automatic session or calling-directory registration. Worktrees and plain terminals share a list. Section membership retains exact directory paths while Git workspaces, public IDs and CLI bulk scope remain unchanged. Existing catalogs migrate to root sections; legacy clients retain invocation-owned temporary cleanup.

## Requirements and evidence

| Requirements and scenarios | Evidence |
| --- | --- |
| dashboard / Persistent dashboard and terminal presentation; sessions / Legacy temporary session cleanup | `test_two_dashboards_launch_without_sessions_and_keep_list_accessible` checks two empty launches, no sessions or dashboard tmux server, compact Esc/Ctrl+W access and independent quit. `TestEmptyDashboardFocusAndDirectoryFormBounds` checks focus without a terminal. |
| dashboard / Explicit folder section management; Empty section removal | `TestFolderBrowserPathsHiddenDirectoriesAndCancellation` checks hidden folders, paths with spaces, launch-relative paths, retained invalid input and cancellation without state creation. `TestSectionMigrationRemovalAndDuplicatePaths` checks canonical symlink deduplication, persisted removal, retained workspace records and increasing IDs. |
| sessions / Persistent path sections and session membership; cli / Background creation result | `test_nested_sections_keep_cwd_and_bulk_deletion_inside_section` creates a worktree through CLI from a Git subdirectory, adds a plain terminal there through the dashboard, and checks exact shell cwd, shared workspace identity and saved section membership. |
| sessions / Persistent path sections and session membership, Existing catalog migration; Damaged section catalog | `TestSectionMigrationRemovalAndDuplicatePaths` migrates a catalog without section metadata and preserves workspace IDs. `TestFailedCreationAndCorruptSectionMetadataPreserveCatalog` checks removal of a newly allocated section after provisioning failure and byte-for-byte preservation of corrupt catalog data. |
| sessions / Persistent path sections and session membership, Stopped record prevents section removal | `TestConcurrentSectionsAndStoppedRecordProtection` runs eight concurrent additions, checks a single workspace and section, and rejects removal with a stopped record. Creation and removal both acquire the catalog lock before the registry lock. |
| dashboard / Alphabetical folder sections and safe transitions; Selected row and open terminal are distinct; Independent hover preview | `TestDesignSectionsFoldersAndHover` checks the mixed session list, selectable folders and hover without selection or focus changes. Existing grouped-scroll and selection tests cover ordering, duplicate names, empty folders and retained headings. Real-terminal hover and cross-project tests check input routing and unchanged dashboard/terminal PIDs. |
| dashboard / Folder lifecycle and creation actions; Surviving fixed bulk targets | `test_nested_sections_keep_cwd_and_bulk_deletion_inside_section` deletes only nested-section sessions, preserves the root terminal PID and Git branch, and removes the emptied section. `TestSectionActionsUseWorkspaceAndFixedSectionTargets` verifies backend workspace targeting even after the confirmed section disappears. `TestMissingSectionCannotResolveCurrentWorkspaceRecord` rejects fallback from a missing section to another record. |
| dashboard / Surviving fixed bulk targets; sessions / Protected deletion with branch preservation | `test_bulk_deletion_preflights_and_uses_surviving_frozen_targets` checks dirty refusal before process termination, disappeared targets, exclusion of later records and preserved branches/files. Existing bulk CLI tests retain workspace-wide scope and protected origin behavior. |
| sessions / Persistent plain terminals in Git groups; startup-templates / Directory templates and explicit folder shells | `test_folder_terminal_is_persistent_plain_shell_in_git` checks the reserved shell template, terminal kind, empty branch/base, persistence after Q, restart and file preservation. The nested-section integration case verifies plain Git terminals outside the root path. |
| dashboard / Persistent dashboard and terminal presentation; Explicit keyboard command mode; Explicit terminal placement | Continuous-dashboard tests check cross-project PID continuity, Q, PTY disconnect, outer-tmux detach, compact navigation, BackTab/application mouse input, split focus and preservation of the original terminal process. Existing legacy suites verify invocation-owned cleanup and service-only upgrades. |
| dashboard / Target-bound context menus and overlays; Bounded interactive dashboard dialogs; Required-only dashboard creation | Unit and real-tmux cases cover one-click cold start, target-bound menus, retained create errors, template cancellation, mouse settings and compact resize. `TestEmptyDashboardFocusAndDirectoryFormBounds` checks the directory dialog and hit targets in English/Russian at 160×42, 99×24, 100×16 and 22×8. |
| dashboard / Conditional search availability; Complete search cancellation; Search editing controls | Go and real-terminal tests cover the ten/eleven-row boundary, activity filtering, Up-to-search, exclusive Clear/Down controls, retained editing focus, cancellation and reveal of the open session. Section headings and Add section do not enter the candidate count. |
| dashboard / Terminal style isolation; Responsive compact sidebar | `test_terminal_backgrounds_stay_inside_each_pane` checks colored rows and trailing spaces in adjacent panes, resizing, retained PIDs and clean sidebar/title/status cells. The complete real-tmux design suite passes with explicit section and terminal creation. |
| cli / Global public session JSON; sessions / Stable project registry; configuration; startup-templates; codex-hooks | The full suite checks public group:session IDs and JSON fields, shared configuration, templates, copies, literal environments, project-local hooks, legacy records and protected worktrees. Internal section IDs remain absent from public session JSON. |
| development-workflow / Local interactive terminal design; Prototype folder and session navigation; Prototype folder browser and empty sections | The 24-test Chromium suite covers all fixture scenarios, list-only launch/relaunch, mixed rows, exact-path additions, invalid paths, duplicates, empty removal, section-local deletion, search, menus, forms, settings, splits and responsive layouts. Stable unique selectors are checked through transitions. Dashboard and folder-browser screenshots were visually inspected. |
| development-workflow / Dedicated usage skill maintenance | Independent `/root/section_usage_review`, using the project `skill-maintainer` role, updated only `docs/skills/grand-git/SKILL.md`. Its fresh isolated build passed 14 public help routes, 30 command examples in help mode without config/state writes, 20 focused Go tests, skill validation and scoped diff checks. It found no additional public-interface mismatch. |
| installation / Factual public onboarding; development-workflow / Verification after every completed plan | English/Russian READMEs, both usage references, CLI help, the prototype guide, usage skill and OpenSpec context describe explicit section creation and session-free dashboard launch. The installer fixture suite passes. No change proposal or archive was created. |

## Checks

- `go build -o bin/gg ./cmd/gg`: PASS.
- `go test ./...` and `go vet ./...`: PASS.
- `env -u TMUX -u TMUX_PANE -u GG_DASHBOARD_CLIENT sh scripts/test.sh`: PASS; final run completed 108 integration tests in 210.971 seconds, with Go tests and vet passing.
- Direct continuous-dashboard suite: PASS, 11 tests; real-tmux design suite: PASS, 8 tests, also included in the full run.
- `npm test --prefix design`: PASS, 24 Chromium tests, 0 failures. Chromium used environment-provided shared libraries.
- `sh scripts/check-specs.sh`: PASS, 8 current specifications, 0 failures.
- `git diff --check`: PASS.
- Independent usage-skill review: PASS; skill validator, command examples and focused interface tests passed.

## README media verification

Requirements: installation / Repository-local workflow demo; cli / Background creation result; dashboard / Required-only dashboard creation, Explicit keyboard command mode, Revisit running tasks.

- `go build -o bin/gg ./cmd/gg`: PASS.
- `scripts/record-demo.py` with agg 1.9.0 and Pillow 13.0.0: PASS. A real tmux recording creates two worktrees, checks that only `feat/login/app.py` changes, preserves terminal PIDs across task switching and dashboard exit, and confirms both sessions remain active after Q. The fixture uses temporary config/state directories and dedicated tmux servers.
- Media decoding: PASS. The GIF contains 94 frames at 1060×583, loops over 28.47 seconds and occupies 208,502 bytes. Its first frame previews the completed dashboard; the PNG matches the decoded final frame. The creation form, second worktree and restored terminal frames were visually inspected.
- Both README image links, static PNG links, alternative text and release archive asset paths: PASS. The files use repository-relative paths and exist at the referenced locations.
- `sh scripts/check-specs.sh`: PASS, 8 current specifications. `git diff --check`: PASS.
