# Installation and release distribution

[English quick start](../README.md) · [Russian quick start](../README.ru.md)

## Build from source

Clone [chartyom/grand-git](https://github.com/chartyom/grand-git) with `git clone https://github.com/chartyom/grand-git.git` and enter `grand-git`. Build with Go 1.26+:

```sh
go build -o bin/gg ./cmd/gg
./bin/gg --version
./bin/gg --help
```

For a permanent source installation, copy `bin/gg` to a directory on PATH, such as `~/.local/bin`, after checking for an existing executable; do not overwrite one unintentionally.

tmux 3.4+ must be on PATH for terminal operations; Git 2.31+ is additionally required for Git worktrees. Ordinary directory sessions work without Git, although the installer checks both dependencies. Install them using `sudo apt install git tmux` on Ubuntu/WSL, `sudo dnf install git tmux` on Fedora, `sudo pacman -S git tmux` on Arch, or `brew install git tmux` on macOS. Check versions afterward; older distribution packages may be too old. [Upstream tmux installation instructions](https://github.com/tmux/tmux/wiki/Installing) include other package managers and source builds.

Use an interactive terminal. Windows uses WSL2 and the Linux build; no native Windows terminal backend is provided. Runtime installations do not need Go, Node.js or Python. Startup applications have their own dependencies.

## Release downloads

Choose a version from [chartyom/grand-git releases](https://github.com/chartyom/grand-git/releases). Replace `vX.Y.Z` in the examples with the selected tag.

The release pipeline packages:

- `grand-git_<version>_linux_amd64.tar.gz`
- `grand-git_<version>_linux_arm64.tar.gz`
- `grand-git_<version>_darwin_amd64.tar.gz`
- `grand-git_<version>_darwin_arm64.tar.gz`
- `checksums.txt` and `install.sh`

Use `amd64` for Intel/AMD x86-64, `arm64` for Apple Silicon/aarch64, `darwin` for macOS and `linux` inside WSL2.

### Installer

```sh
curl -fsSL "https://github.com/chartyom/grand-git/releases/latest/download/install.sh" -o install-gg.sh
less install-gg.sh
sh install-gg.sh --version vX.Y.Z
```

Omitting `--version` selects the latest stable published release. Prereleases require an explicit version. The script needs a POSIX shell, curl, tar/gzip, standard Unix file tools, and sha256sum or shasum. It validates Git/tmux first, verifies the selected archive against `checksums.txt`, extracts the binary to a temporary directory, then installs atomically. HTTPS is required except for local HTTP test fixtures.

The installer defaults to `chartyom/grand-git`; `GG_REPOSITORY=another-owner/another-repo` overrides the repository, while an unset or empty value uses the default. The default destination is `~/.local/bin`. Use `--bin-dir /absolute/path` to change it. Existing files require `--force`; directory destinations are rejected. Failed downloads, invalid archives and checksum mismatches preserve an existing installation. The installer does not run downloaded executables, install system packages, edit shell startup files or grant hook trust.

Set PATH for a terminal with `export PATH="$HOME/.local/bin:$PATH"`. For persistence, add it using your shell's normal configuration mechanism. The script prints a hint without editing those files. `GG_DOWNLOAD_BASE_URL` can select an HTTPS mirror exposing the same `/latest` redirect and `/download/<tag>/...` layout.

Update by rerunning the installer with `--force`; pin a version to select an older release. An update refreshes service panels on the next dashboard entry and preserves running user terminals. To uninstall a default install, remove `~/.local/bin/gg`. This does not stop live tmux tasks or remove worktrees/settings; use `gg stop --all --yes` first if you also want to end processes.

### Manual archive

Download the matching archive and `checksums.txt` from the same release. For example, on Linux amd64 after selecting a real version:

```sh
GG_REPOSITORY=chartyom/grand-git
GG_TAG=vX.Y.Z
GG_ARCHIVE=grand-git_${GG_TAG#v}_linux_amd64.tar.gz
curl -fLO "https://github.com/$GG_REPOSITORY/releases/download/$GG_TAG/$GG_ARCHIVE"
curl -fLO "https://github.com/$GG_REPOSITORY/releases/download/$GG_TAG/checksums.txt"
awk -v archive="$GG_ARCHIVE" '$2==archive {print}' checksums.txt > selected-checksum.txt
# Require exactly one matching checksum before proceeding.
test "$(wc -l < selected-checksum.txt | tr -d ' ')" = 1
sha256sum -c selected-checksum.txt   # macOS: shasum -a 256 -c selected-checksum.txt
```

Only after a successful checksum check, extract `gg` and place it in your chosen PATH directory. Inspect an existing executable before replacing it. The installer automates these steps and refuses replacement without `--force`.

## Install with Go

```sh
go install github.com/chartyom/grand-git/cmd/gg@latest
```

This requires Go 1.26+ and the runtime dependencies. The binary is installed in `GOBIN`, or `$(go env GOPATH)/bin` when `GOBIN` is unset, and reports `dev` without release linker metadata.

## If gg runs git gui

If `gg` opens Git GUI or reports `git: 'gui' is not a git command`, run `type -a gg` in Bash or Zsh. An alias such as `gg='git gui citool'` takes precedence over the Grand Git executable on PATH.

With Grand Git on PATH, `command gg --version` bypasses the alias. You can also use the executable's path directly, for example `./bin/gg --version` from the source checkout. If `type -a gg` shows only the alias, add the directory containing the executable to PATH.

Run `unalias gg` to remove the alias in the current shell. For future shells, remove or rename the alias in your shell configuration, or put `unalias gg` after the line that loads it.

## Maintainer release procedure

1. Use `https://github.com/chartyom/grand-git` as the source and release repository. The module path, installer default and GoReleaser target use this identity.
2. Run `sh scripts/test.sh` and `sh scripts/check-specs.sh`. Native Linux amd64/arm64 and macOS Intel/Apple Silicon behavior jobs are defined in `.github/workflows/verify.yml`.
3. For a local packaging check, use pinned GoReleaser **v2.10.2**: `goreleaser check`, then `goreleaser release --snapshot --clean`. This requires a Git checkout with commits and a configured remote. Snapshot builds do not publish releases.
4. Push the chosen `vX.Y.Z` tag when a release is intended. `release.yml` first runs the verification workflow, then prepares a **draft** GitHub release containing archives, SHA-256 checksums and the installer. It uses the repository's normal `GITHUB_TOKEN` with contents permission; no additional publication token is required. Manual workflow runs from a branch create snapshot artifacts only.
5. Inspect the draft, verify the installer against its artifacts and publish it. The installer selects the latest stable published release by default.

GoReleaser and actions are pinned in the workflow. Release binaries embed version, commit and build date through linker flags; local builds report `dev`. No application dependencies or tmux binary are bundled.
