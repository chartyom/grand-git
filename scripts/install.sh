#!/bin/sh
# Grand Git user installation. Requires no Go toolchain and never invokes sudo.
set -eu
fail() { printf '%s\n' "gg install: $*" >&2; exit 1; }
usage() {
    cat <<'HELP'
Usage: sh install.sh [--version vX.Y.Z|latest] [--bin-dir DIR] [--force]
Repository: chartyom/grand-git. Set GG_REPOSITORY to override it with another owner/name.
Default: latest stable release, ~/.local/bin. Existing files require --force.
Git 2.31+ and tmux 3.4+ must be installed separately. Shell configuration is unchanged.
HELP
}
tag=latest
bin_dir=${HOME:?HOME is required}/.local/bin
force=false
while [ "$#" -gt 0 ]; do
    case "$1" in
        --version|--bin-dir)
            [ "$#" -ge 2 ] && [ -n "$2" ] || fail "$1 requires a value"
            if [ "$1" = --version ]; then tag=$2; else bin_dir=$2; fi
            shift 2 ;;
        --force) force=true; shift ;;
        --help|-h) usage; exit 0 ;;
        *) fail "unknown argument: $1" ;;
    esac
done
case "$bin_dir" in /*) ;; *) fail '--bin-dir must be an absolute path' ;; esac
repo=${GG_REPOSITORY:-chartyom/grand-git}
printf '%s\n' "$repo" | awk '/^[A-Za-z0-9_.-]+\/[A-Za-z0-9_.-]+$/ {ok=1} END {exit !ok}' || fail 'invalid GG_REPOSITORY; expected owner/name'
for tool in curl tar gzip awk mktemp chmod cp mv ln; do command -v "$tool" >/dev/null 2>&1 || fail "$tool is required"; done
case "$(uname -s)" in
    Linux) platform=linux; hint='Ubuntu/WSL: sudo apt update && sudo apt install git tmux; Fedora: sudo dnf install git tmux; Arch: sudo pacman -S git tmux' ;;
    Darwin) platform=darwin; hint='brew install git tmux' ;;
    *) fail 'supported platforms are Linux, macOS, and Windows through WSL2' ;;
esac
case "$(uname -m)" in
    x86_64|amd64) arch=amd64 ;;
    aarch64|arm64) arch=arm64 ;;
    *) fail 'supported architectures are amd64 and arm64' ;;
esac
check_version() {
    tool=$1 flag=$2 required_major=$3 required_minor=$4
    command -v "$tool" >/dev/null 2>&1 || fail "$tool not found. Install: $hint"
    detected=$("$tool" "$flag" 2>&1) || fail "cannot inspect $tool. Install or upgrade: $hint"
    printf '%s\n' "$detected" | awk -v major="$required_major" -v minor="$required_minor" '
        match($0, /[0-9]+\.[0-9]+/) {split(substr($0,RSTART,RLENGTH),v,"."); found=1}
        END {exit !(found && (v[1]>major || v[1]==major && v[2]>=minor))}' ||
        fail "$tool $required_major.$required_minor+ required (found $detected). Install or upgrade: $hint"
}
check_version git --version 2 31
check_version tmux -V 3 4
if command -v sha256sum >/dev/null 2>&1; then checksum_tool=sha256sum
elif command -v shasum >/dev/null 2>&1; then checksum_tool=shasum
else fail 'sha256sum or shasum is required'; fi
if [ -e "$bin_dir/gg" ] || [ -L "$bin_dir/gg" ]; then
    [ ! -d "$bin_dir/gg" ] || fail "$bin_dir/gg is a directory; choose another --bin-dir"
    [ "$force" = true ] || fail "$bin_dir/gg already exists; use --force to replace it"
fi
base=${GG_DOWNLOAD_BASE_URL:-https://github.com/$repo/releases}
case "$base" in
    https://*) protocols='=https' ;;
    http://127.0.0.1:*|http://localhost:*) protocols='=http,https' ;;
    *) fail 'download base must use HTTPS (local test servers may use HTTP)' ;;
esac
work_dir=$(mktemp -d "${TMPDIR:-/tmp}/gg-install.XXXXXX")
stage_path=
trap 'rm -rf "$work_dir"; if [ -n "$stage_path" ]; then rm -f "$stage_path"; fi' EXIT
trap 'exit 1' HUP INT TERM
fetch() { curl --fail --silent --show-error --location --proto "$protocols" --proto-redir "$protocols" "$1" --output "$2" || fail "download failed: $1"; }
if [ "$tag" = latest ]; then
    effective=$(curl --fail --silent --show-error --location --proto "$protocols" --proto-redir "$protocols" --output /dev/null --write-out '%{url_effective}' "${base%/}/latest") || fail 'cannot resolve latest stable release'
    tag=${effective##*/}
fi
printf '%s\n' "$tag" | awk '/^v[0-9]+\.[0-9]+\.[0-9]+(-[A-Za-z0-9][A-Za-z0-9.-]*)?$/ {ok=1} END {exit !ok}' || fail "invalid release tag: $tag"
version=${tag#v}
archive_name=grand-git_${version}_${platform}_${arch}.tar.gz
fetch "${base%/}/download/$tag/checksums.txt" "$work_dir/checksums.txt"
fetch "${base%/}/download/$tag/$archive_name" "$work_dir/$archive_name"
expected=$(awk -v name="$archive_name" '$2==name {print $1}' "$work_dir/checksums.txt")
printf '%s\n' "$expected" | awk 'length($0)==64 && $0 !~ /[^0-9a-f]/ {ok++} END {exit !(ok==1 && NR==1)}' || fail 'release checksum is missing or invalid'
if [ "$checksum_tool" = sha256sum ]; then actual=$(sha256sum "$work_dir/$archive_name" | awk '{print $1}')
else actual=$(shasum -a 256 "$work_dir/$archive_name" | awk '{print $1}'); fi
[ "$expected" = "$actual" ] || fail 'SHA-256 mismatch; existing installation preserved'
mkdir "$work_dir/extract"
tar -xzf "$work_dir/$archive_name" -C "$work_dir/extract" gg || fail 'cannot extract gg from archive'
[ -f "$work_dir/extract/gg" ] && [ ! -L "$work_dir/extract/gg" ] || fail 'archive must contain a regular gg binary'
mkdir -p "$bin_dir"
stage_path=$(mktemp "$bin_dir/.gg-install-XXXXXX")
cp "$work_dir/extract/gg" "$stage_path"
chmod 755 "$stage_path"
# Repeat the overwrite check after downloading; do not replace a newly installed file.
if [ -e "$bin_dir/gg" ] || [ -L "$bin_dir/gg" ]; then
    [ "$force" = true ] && [ ! -d "$bin_dir/gg" ] || fail 'destination changed during installation; existing file preserved'
fi
if [ "$force" = true ]; then
    mv -f "$stage_path" "$bin_dir/gg"
else
    ln "$stage_path" "$bin_dir/gg" || fail 'destination appeared during installation; existing file preserved'
    rm -f "$stage_path"
fi
stage_path=
printf 'Installed Grand Git %s: %s/gg\n' "$tag" "$bin_dir"
printf 'Check: "%s/gg" --version\n' "$bin_dir"
case ":${PATH:-}:" in *":$bin_dir:"*) ;; *) printf 'Add to PATH for this terminal: export PATH="%s:$PATH"\n' "$bin_dir" ;; esac
