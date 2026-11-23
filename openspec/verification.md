# Current verification

## v1.0.1 release preparation (2026-11-23)

- The delegated usage-skill review checked the Git requirements for v1.0.1 and v1.0.0 against the worktree discovery implementation, updated the stable download link and pinned installer example, and confirmed the CLI and installer help. The legacy Git worktree regression test and all six installer fixture tests passed.
- OpenSpec validation passed for all eight specifications. On Linux amd64, the build, Go tests, go vet and all 109 Python integration tests passed. GoReleaser v2.10.2 validated the configuration and built snapshot archives for all four targets.
- Four Go fixture failures reproduced with a symlinked temporary directory. With canonical fixture roots, the full Go suite passed in that environment. Four Linux integration checks exercised the revised process fixtures. Six dashboard synchronization scenarios passed twice, including on a single CPU. Both zsh tests passed with an empty home directory and a writable completion directory.
- Linux amd64 and arm64 CI passed the 109 integration tests after the focus checks selected the open terminal. On Linux, the revised shell checks passed with both sh and Bash. Ten search scenarios passed with stopped worktree fixtures, and a separate CLI test verified eleven simultaneously active worktrees with one terminal each. All nineteen continuous and design dashboard tests passed.

## Public release documentation for v1.0.0 (2026-11-23)

Requirements: installation / Factual public onboarding, Portable runtime targets, Verified user installation, Versioned release artifacts.

- GitHub listed [v1.0.0](https://github.com/chartyom/grand-git/releases/tag/v1.0.0) as the latest stable release, published November 16, 2026. All four published archives matched `checksums.txt` and contained the same eight paths. The downloaded Linux amd64 executable reported version 1.0.0 and commit `3f8b9d4`.
- On Linux amd64, the latest and pinned installers, manual checksum and extraction instructions, tagged source build, and `go install` with both `@v1.0.0` and `@latest` succeeded. Replacing a disposable installation with `--force` and then removing the executable preserved the live terminal PID and worktree. The six installer fixture tests passed.
- The published executable failed to create a worktree with native Git 2.31.0, 2.34.1 and 2.35.1 because `git worktree list` rejected `-z`. It created, listed and deleted worktrees with Git 2.36.0 and 2.43.0. A build of `fcf9d51` also passed that sequence with Git 2.35.1.
- The README walkthrough exercised ignored-file copying, stop/start, deletion and branch preservation in a temporary repository. Eleven targeted CLI/dashboard integration tests passed on a build of the release tag. All fifteen template tests passed with the published Linux binary and test executables for the preset applications. The binary read all seven documented template examples through `gg template show`.
- Running the demo recorder with the release binary verified independent worktrees and stable terminal PIDs after switching sessions and quitting. The published PNG matches the final frame of the shipped 94-frame GIF.
- Checked all 36 relative links, images and heading fragments in the revised consumer guides in a fresh checkout with the documentation patch applied. All 22 such references in the bundled guides also resolved using the published archive layout. All nine distinct external documentation links returned HTTP 200.

## Directory sections (2026-11-16)

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

## Shell alias troubleshooting verification

Requirement: installation / Shell alias troubleshooting.

- Bash 5.2 and Zsh 5.9 with an isolated PATH and Git without GUI helpers: `gg='git gui citool'` reproduced `git: 'gui' is not a git command` even with Grand Git on PATH. `type -a gg` showed the alias and binary, and showed only the alias when the binary was removed from PATH. `command gg --version`, the direct executable path and plain `gg --version` after `unalias gg` all returned the Grand Git version.
- Both README links resolve to the [installation troubleshooting section](../docs/installation.md#if-gg-runs-git-gui).
- `go build -o bin/gg ./cmd/gg`, `python3 -m unittest discover -s tests -p test_installation.py -v` (6 tests), and `sh scripts/check-specs.sh` (8 specifications): PASS.

## Git worktree compatibility verification

Requirements: installation / Portable runtime targets, Git 2.31 through 2.35; sessions / Ignored environment discovery and source, Protected origin worktrees.

- Native Git 2.31.0 and 2.34.1 on Linux: the preceding build failed at `git worktree list --porcelain -z`. With the fix, `test_create_without_worktree_list_z`, `test_copy_source_follows_base_worktree_or_calling_worktree` and `test_existing_external_worktree_is_registered_without_ownership` passed on both versions. The regression test checks creation from the main and linked worktrees, copy sources for checked-out and unchecked branches, and a worktree path containing spaces, quotes, a Unicode character and a newline.
- Native Git 2.31.0 with isolated tmux: `test_nested_sections_keep_cwd_and_bulk_deletion_inside_section` and `test_create_popup_is_shown_to_the_initiating_client` passed.
- An independent `skill-maintainer` review confirmed that the usage skill already documents Git 2.31+ and the base and calling-worktree copy rules. Fresh `gg create --help` and three focused native Git 2.31.0 integration checks passed. No skill edits were needed.
- `go build -o bin/gg ./cmd/gg` and `sh scripts/check-specs.sh` (8 specifications): PASS.
- `sh scripts/test.sh` on Linux with Git 2.43.0: Go tests, `go vet` and 109 integration tests passed, including the regression test with a Git shim that rejects `worktree list --porcelain -z`.
- macOS amd64 and arm64 cross-builds produced Mach-O executables. `git diff --check`: PASS.
