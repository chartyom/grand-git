# Installation Specification

## Purpose

Define consumer installation, supported build targets and release packaging.

## Requirements

### Requirement: Canonical source and release identity
The canonical repository SHALL be https://github.com/chartyom/grand-git. The Go module SHALL be github.com/chartyom/grand-git, and imports and release linker flags SHALL use that module path. GoReleaser SHALL target GitHub owner chartyom and repository grand-git.

#### Scenario: Consistent build and repository identity
- **WHEN** source builds, release packaging or installation examples identify the project
- **THEN** they use the canonical repository and module path, and release binaries retain embedded version, commit and build date

### Requirement: Portable runtime targets
Grand Git SHALL build for Linux and macOS on amd64 and arm64. Windows SHALL use the Linux build through WSL2 rather than a native binary. Runtime SHALL require Git 2.31+ and tmux 3.4+, without Go or Node.js. macOS SHALL enumerate process ownership without /proc and preserve start-time checks before signals; ordinary copying SHALL preserve independent files and metadata. Native platform validation SHALL be reported separately from cross-compilation.

#### Scenario: macOS build and process lifecycle
- **WHEN** built for either macOS architecture
- **THEN** no Linux-only cloning syscall is required, and stopping terminals uses macOS process identities

#### Scenario: Git 2.31 through 2.35
- **WHEN** `gg create` runs from the main worktree or a linked worktree with Git 2.31 through 2.35
- **THEN** Grand Git discovers the same project from both worktrees and creates an active session using that Git version
- **AND** `--base` still selects the checked-out base branch's worktree as the environment copy source when available

### Requirement: Versioned release artifacts
The release pipeline SHALL build four grand-git_<version>_<os>_<arch>.tar.gz archives with gg and documentation, checksums.txt using SHA-256, and install.sh. Builds SHALL embed version, commit and date. Tagged releases SHALL depend on successful behavior/spec checks, including native Linux and macOS runners. The pipeline SHALL stage a draft for explicit publication; branch workflow runs SHALL only produce snapshots.

#### Scenario: Packaging a version
- **WHEN** a version tag passes verification
- **THEN** a draft contains the four named archives, checksums and installer

### Requirement: Verified user installation
The POSIX installer SHALL select OS/architecture, default to latest stable and ~/.local/bin, and accept --version, --bin-dir and --force. It SHALL check dependencies, verify SHA-256 before atomic installation and preserve existing executables on failure. Replacement SHALL require --force. It SHALL use HTTPS except local fixtures, and SHALL not invoke sudo or alter shell configuration, trust or project files.

#### Scenario: Successful and pinned installation
- **WHEN** a matching release archive passes validation
- **THEN** its executable is installed into the selected directory and PATH guidance is printed

#### Scenario: Invalid or unavailable artifact
- **WHEN** download, checksum or archive validation fails
- **THEN** installation fails and an existing executable is preserved

### Requirement: Installer repository selection
The installer SHALL default to chartyom/grand-git. GG_REPOSITORY SHALL allow a validated owner/name override; an unset or empty value SHALL use the default. Invalid values SHALL fail before downloading.

#### Scenario: Default repository and explicit override
- **WHEN** GG_REPOSITORY is unset or empty, or an explicit owner/name is provided
- **THEN** release URLs use chartyom/grand-git or the explicit override respectively; invalid repository values fail before downloading

### Requirement: Factual public onboarding
Both READMEs SHALL summarize the session workflow, prerequisites, source builds, first use, safeguards and explicit folder sections, linking installation details. Clone/download examples SHALL use chartyom/grand-git and version placeholders. Documentation SHALL describe installation steps without assistant commentary or publication-status caveats. Update/uninstall guidance SHALL distinguish executables from project files and running sessions.

#### Scenario: Choosing an installation method
- **WHEN** a user reads the installation guide
- **THEN** source builds, release archives and the installer have concrete instructions, with version placeholders explained

### Requirement: Shell alias troubleshooting
Both READMEs SHALL link to installation troubleshooting for a shell alias named `gg` that invokes `git gui`. The installation guide SHALL explain how to inspect command resolution, run the executable despite the alias and remove the alias for current and future shells.

#### Scenario: An alias shadows the installed executable
- **WHEN** Bash or Zsh resolves `gg` to an alias for `git gui` and Grand Git is on PATH
- **THEN** `type -a gg` reports the alias and executable, `command gg --version` runs Grand Git, and `unalias gg` restores direct invocation of `gg` in the current shell

### Requirement: Repository-local workflow demo
Both READMEs SHALL embed a repository-local animated GIF of the actual CLI/dashboard workflow using relative paths and descriptive alternative text, with a link to a static PNG. The demo SHALL use a temporary project and isolated tmux servers and SHALL include reproducible recording instructions.

#### Scenario: Viewing the workflow on GitHub
- **WHEN** a reader opens either README on GitHub
- **THEN** the embedded GIF shows creating tasks and switching between independent worktrees while retaining edits and terminals
- **AND** both media files resolve within the same repository without an external video player

### Requirement: Concise alternatives and installation references
READMEs SHALL link detailed archive, installer, update and uninstall instructions instead of duplicating the installation guide. Their short Alternatives table SHALL link primary documentation and describe use cases without unsupported superiority claims.

#### Scenario: Choosing a workflow
- **WHEN** a reader compares Grand Git with manual Git/tmux, Worktrunk or workmux
- **THEN** the README explains its simple session workflow and links the alternatives' own documentation
