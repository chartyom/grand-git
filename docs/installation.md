# Installation and release distribution

[English quick start](../README.md) · [Russian quick start](../README.ru.md)

## Requirements

Grand Git runs on Linux and macOS on amd64 or arm64. On Windows, run the Linux build inside WSL2. Use an interactive terminal for the dashboard.

- Terminal sessions require **tmux 3.4+** on PATH.
- For a source build containing the worktree fix (`fcf9d51`), install **Git 2.31+**. The **v1.0.0 release requires Git 2.36+** even though its installer accepts Git 2.31 through 2.35. With those older Git versions, v1.0.0 fails during worktree discovery because `git worktree list` rejects `-z`.
- Ordinary directory sessions can run without Git if you install the binary manually. The installer checks for both Git and tmux.
- Release binaries run without Go, Node.js or Python. Install any programs you select in a startup template separately.

Install the dependencies with `sudo apt install git tmux` on Ubuntu/WSL, `sudo dnf install git tmux` on Fedora, `sudo pacman -S git tmux` on Arch, or `brew install git tmux` on macOS. Check the installed versions with `git --version` and `tmux -V`. For distributions that package an older tmux, see the [upstream installation instructions](https://github.com/tmux/tmux/wiki/Installing).

## Release downloads

The following filenames are from v1.0.0. That release includes an installer, a SHA-256 checksum list and these four binary archives. [Grand Git v1.0.0 downloads](https://github.com/chartyom/grand-git/releases/tag/v1.0.0)

| Operating system | CPU | Archive |
| --- | --- | --- |
| Linux / WSL2 | Intel or AMD x86-64 | `grand-git_1.0.0_linux_amd64.tar.gz` |
| Linux / WSL2 | ARM64 / aarch64 | `grand-git_1.0.0_linux_arm64.tar.gz` |
| macOS | Intel | `grand-git_1.0.0_darwin_amd64.tar.gz` |
| macOS | Apple Silicon | `grand-git_1.0.0_darwin_arm64.tar.gz` |

### Installer

To install the latest stable release in `~/.local/bin`, download and inspect the installer, then run it.

```sh
curl -fsSL https://github.com/chartyom/grand-git/releases/latest/download/install.sh -o install-gg.sh
less install-gg.sh
sh install-gg.sh
"$HOME/.local/bin/gg" --version
```

For a pinned installation, download the installer from the same tag and pass the tag to the script. For another version, replace `v1.0.0` in both commands. Prereleases require an explicit tag. [All releases](https://github.com/chartyom/grand-git/releases)

```sh
curl -fsSL https://github.com/chartyom/grand-git/releases/download/v1.0.0/install.sh -o install-gg.sh
sh install-gg.sh --version v1.0.0
```

The installer needs a POSIX shell, curl, tar/gzip, standard Unix file tools and either `sha256sum` or `shasum`. It selects the archive for the host, verifies it against `checksums.txt` and installs `gg` atomically. An existing `gg` file requires `--force`. A directory named `gg` at the destination is rejected. Failed downloads, invalid archives and checksum mismatches preserve the existing executable.

Use `--bin-dir /absolute/path` to change the destination. For the default destination, run `export PATH="$HOME/.local/bin:$PATH"` in your terminal. For future shells, add the export to your shell configuration.

The installer defaults to releases from `chartyom/grand-git`. Set `GG_REPOSITORY=owner/name` to select another repository, or set `GG_DOWNLOAD_BASE_URL` to an HTTPS mirror exposing the same `/latest` redirect and `/download/<tag>/...` layout.

### Manual archive

In an empty download directory, use the following example for Linux amd64. Set `archive` to the matching filename from the table for another platform. The commands extract the executable after its checksum matches.

```sh
(
  set -eu
  tag=v1.0.0
  archive=grand-git_1.0.0_linux_amd64.tar.gz
  base="https://github.com/chartyom/grand-git/releases/download/$tag"
  curl -fsSLO "$base/$archive"
  curl -fsSLO "$base/checksums.txt"
  awk -v file="$archive" '$2 == file {print; n++} END {exit n != 1}' checksums.txt > selected-checksum.txt
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum -c selected-checksum.txt
  else
    shasum -a 256 -c selected-checksum.txt
  fi
  tar -xzf "$archive" gg
  ./gg --version
)
```

After successful verification, copy `gg` to a directory on PATH. For example, `mkdir -p "$HOME/.local/bin" && cp -i gg "$HOME/.local/bin/gg"` prompts before replacing an existing file.

### Update and uninstall

Download the latest stable installer as above and run `sh install-gg.sh --force` to update. Supply the same `--bin-dir` if you installed in a custom location. Use `--version v1.0.0 --force` to reinstall that version. Reopen the dashboard to use the new executable. Running user terminals survive the update.

To uninstall the default binary, run `rm "$HOME/.local/bin/gg"`. Sessions, worktrees, configuration and running terminals remain in place. To stop running terminals first, run `gg stop --all` in each project or ordinary directory group you want to stop.

## Build from source

To build the released source with **Go 1.26+**, clone its tag and build the CLI.

```sh
git clone --branch v1.0.0 https://github.com/chartyom/grand-git.git
cd grand-git
go build -o bin/gg ./cmd/gg
./bin/gg --version
./bin/gg --help
```

Local builds report `dev` by default. For a permanent source installation, copy `bin/gg` to a directory on PATH.

## Install with Go

```sh
go install github.com/chartyom/grand-git/cmd/gg@v1.0.0
```

Use `@latest` to select the latest module release. This method requires Go 1.26+ and builds from the tagged module source. The binary is installed in `GOBIN`, or the first GOPATH entry's `bin` directory when `GOBIN` is unset. It reports `dev` because the release linker flags are applied when packaging the binary archives. Run `go version -m /path/to/gg` with the installed executable's path to see the module version.

## If gg runs git gui

If `gg` opens Git GUI or reports `git: 'gui' is not a git command`, run `type -a gg` in Bash or Zsh. An alias such as `gg='git gui citool'` takes precedence over the Grand Git executable on PATH.

With Grand Git on PATH, `command gg --version` bypasses the alias. You can also use the executable's path directly, for example `./bin/gg --version` from the source checkout. If `type -a gg` shows only the alias, add the directory containing the executable to PATH.

Run `unalias gg` to remove the alias in the current shell. For future shells, remove or rename the alias in your shell configuration, or put `unalias gg` after the line that loads it.

## Maintainer release procedure

1. Use `https://github.com/chartyom/grand-git` as the source and release repository. The module path, installer default and GoReleaser target use this identity.
2. Run `sh scripts/test.sh` and `sh scripts/check-specs.sh`. Native Linux amd64/arm64 and macOS Intel/Apple Silicon behavior jobs are defined in `.github/workflows/verify.yml`.
3. For a local packaging check with GoReleaser **v2.10.2**, run `goreleaser check`, then `goreleaser release --snapshot --clean`. This requires a Git checkout with commits and a configured remote. Snapshot builds do not publish releases.
4. Push the chosen `vX.Y.Z` tag when a release is intended. After the verification jobs pass, `release.yml` prepares a **draft** GitHub release containing archives, SHA-256 checksums and the installer. It uses the repository's `GITHUB_TOKEN` with contents write permission. Manual workflow runs from a branch create snapshot artifacts only.
5. Inspect the draft, verify the installer against its artifacts and publish it. The installer selects the latest stable published release by default.

Release binaries embed version, commit and build date through linker flags.
