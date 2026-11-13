"""Installer checks against a local HTTP release fixture; no public release is needed."""
import hashlib
import http.server
import io
import os
from pathlib import Path
import shutil
import subprocess
import tarfile
import tempfile
import threading
import unittest

APP = Path(__file__).resolve().parents[1]
INSTALLER = APP / "scripts/install.sh"


class InstallerTests(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory(prefix="gg-install-test-")
        self.root = Path(self.tmp.name)
        self.tools = self.root / "tools"
        self.tools.mkdir()
        for name in ["curl", "tar", "gzip", "awk", "mktemp", "chmod", "cp", "mv", "ln", "rm", "mkdir", "uname", "sha256sum", "shasum"]:
            source = shutil.which(name)
            if source:
                (self.tools / name).symlink_to(source)
        self.program("git", "echo 'git version 2.43.0'")
        self.program("tmux", "echo 'tmux 3.4'")
        self.destination = self.root / "bin with spaces"
        self.files = {}
        owner = self

        class Handler(http.server.BaseHTTPRequestHandler):
            def do_GET(self):
                if self.path == "/releases/latest":
                    self.send_response(302)
                    self.send_header("Location", "/releases/tag/v1.2.3")
                    self.end_headers()
                    return
                if self.path == "/releases/tag/v1.2.3":
                    self.send_response(200)
                    self.end_headers()
                    return
                data = owner.files.get(self.path)
                self.send_response(200 if data is not None else 404)
                self.end_headers()
                if data is not None:
                    self.wfile.write(data)

            def log_message(self, *args):
                pass

        self.server = http.server.ThreadingHTTPServer(("127.0.0.1", 0), Handler)
        self.thread = threading.Thread(target=self.server.serve_forever, daemon=True)
        self.thread.start()
        self.env = dict(os.environ, HOME=str(self.root), PATH=str(self.tools), GG_REPOSITORY="fixture/grand-git",
                        GG_DOWNLOAD_BASE_URL=f"http://127.0.0.1:{self.server.server_port}/releases")
        self.archive("linux", "amd64")
        self.archive("linux", "arm64")
        self.archive("darwin", "amd64")
        self.archive("darwin", "arm64")

    def tearDown(self):
        self.server.shutdown()
        self.server.server_close()
        self.thread.join()
        self.tmp.cleanup()

    def program(self, name, body):
        path = self.tools / name
        if path.is_symlink():
            path.unlink()
        path.write_text("#!/bin/sh\n" + body + "\n")
        path.chmod(0o755)

    def archive(self, platform, arch):
        name = f"grand-git_1.2.3_{platform}_{arch}.tar.gz"
        contents = f"#!/bin/sh\necho 'Grand Git 1.2.3 {platform}/{arch}'\n".encode()
        output = io.BytesIO()
        with tarfile.open(fileobj=output, mode="w:gz") as archive:
            entry = tarfile.TarInfo("gg")
            entry.mode = 0o755
            entry.size = len(contents)
            archive.addfile(entry, io.BytesIO(contents))
        data = output.getvalue()
        prefix = "/releases/download/v1.2.3/"
        self.files[prefix + name] = data
        checksum = f"{hashlib.sha256(data).hexdigest()}  {name}\n".encode()
        self.files[prefix + "checksums.txt"] = self.files.get(prefix + "checksums.txt", b"") + checksum

    def run_installer(self, *args):
        return subprocess.run(["/bin/sh", str(INSTALLER), "--bin-dir", str(self.destination), *args],
                              env=self.env, text=True, capture_output=True, timeout=15)

    def test_each_platform_architecture_latest_and_explicit_version(self):
        for platform, arch in [("Linux", "x86_64"), ("Linux", "aarch64"), ("Darwin", "x86_64"), ("Darwin", "arm64")]:
            with self.subTest(platform=platform, arch=arch):
                self.program("uname", f'case "$1" in -s) echo {platform} ;; -m) echo {arch} ;; esac')
                args = ["--version", "v1.2.3"] if arch == "arm64" else []
                result = self.run_installer(*args)
                self.assertEqual(result.returncode, 0, result.stderr)
                installed = self.destination / "gg"
                self.assertTrue(os.access(installed, os.X_OK))
                expected_arch = "amd64" if arch == "x86_64" else "arm64"
                self.assertIn(f"{platform.lower()}/{expected_arch}", installed.read_text())
                self.assertIn("Add to PATH", result.stdout)
                installed.unlink()
                self.assertFalse((self.root / ".zshrc").exists())
                self.assertEqual(list(self.destination.glob(".gg-install-*")), [])

    def test_overwrite_requires_force_and_checksum_failure_preserves_installation(self):
        self.destination.mkdir()
        target = self.destination / "gg"
        target.write_text("keep my binary")
        result = self.run_installer()
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("--force", result.stderr)
        result = self.run_installer("--force")
        self.assertEqual(result.returncode, 0, result.stderr)
        target.write_text("keep my updated binary")
        key = "/releases/download/v1.2.3/checksums.txt"
        self.files[key] = self.files[key].replace(self.files[key][:64], b"0" * 64)
        # Force uname so the corrupted checksum is the selected archive on every runner.
        self.program("uname", 'case "$1" in -s) echo Linux ;; -m) echo x86_64 ;; esac')
        result = self.run_installer("--force")
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("SHA-256 mismatch", result.stderr)
        self.assertEqual(target.read_text(), "keep my updated binary")

    def test_missing_old_dependencies_and_download_failure_do_not_install(self):
        (self.tools / "tmux").unlink()
        result = self.run_installer()
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("tmux not found", result.stderr)
        self.program("tmux", "echo 'tmux 3.3a'")
        result = self.run_installer()
        self.assertIn("tmux 3.4+ required", result.stderr)
        self.program("tmux", "echo 'tmux 3.5a'")
        self.program("git", "echo 'git version 2.30.9'")
        result = self.run_installer()
        self.assertIn("git 2.31+ required", result.stderr)
        self.program("git", "echo 'git version 2.43.0'")
        result = self.run_installer("--version", "v99.0.0")
        self.assertIn("download failed", result.stderr)
        self.assertFalse((self.destination / "gg").exists())

    def test_unsupported_platform_missing_checksum_and_invalid_arguments(self):
        self.program("uname", 'echo Windows_NT')
        self.assertIn("WSL2", self.run_installer().stderr)
        self.program("uname", 'case "$1" in -s) echo Linux ;; -m) echo x86_64 ;; esac')
        self.files["/releases/download/v1.2.3/checksums.txt"] = b""
        self.assertIn("checksum is missing", self.run_installer().stderr)
        self.assertIn("requires a value", self.run_installer("--version").stderr)
        self.assertIn("invalid release tag", self.run_installer("--version", "../../bad").stderr)
        self.env["GG_REPOSITORY"] = "invalid-repository"
        self.assertIn("GG_REPOSITORY", self.run_installer().stderr)

    def test_default_repository_and_explicit_override_select_download_urls(self):
        self.program("uname", 'case "$1" in -s) echo Linux ;; -m) echo x86_64 ;; esac')
        self.program("curl", 'printf "%s\\n" "$*" >&2; exit 1')
        del self.env["GG_DOWNLOAD_BASE_URL"]
        for repository, expected in [(None, "chartyom/grand-git"), ("", "chartyom/grand-git"),
                                     ("fixture/grand-git", "fixture/grand-git")]:
            with self.subTest(repository=repository):
                if repository is None:
                    self.env.pop("GG_REPOSITORY", None)
                else:
                    self.env["GG_REPOSITORY"] = repository
                result = self.run_installer("--version", "v1.2.3")
                self.assertNotEqual(result.returncode, 0)
                self.assertIn(f"https://github.com/{expected}/releases/download/v1.2.3/checksums.txt", result.stderr)
                self.assertFalse((self.destination / "gg").exists())

    def test_missing_repository_override_installs_from_fixture(self):
        self.env.pop("GG_REPOSITORY", None)
        self.program("uname", 'case "$1" in -s) echo Linux ;; -m) echo x86_64 ;; esac')
        result = self.run_installer()
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertTrue(os.access(self.destination / "gg", os.X_OK))
