#!/usr/bin/env python3
"""Record the README walkthrough using a temporary Git project and tmux servers."""

import argparse
import fcntl
import hashlib
import json
import os
from pathlib import Path
import pty
import select
import shutil
import struct
import subprocess
import tempfile
import termios
import threading
import time


REPO = Path(__file__).resolve().parents[1]
COLS, ROWS = 108, 26


class Recording:
    def __init__(self, directory):
        self.directory = directory
        self.root = directory / "atlas"
        self.root.mkdir()
        (directory / "tmux").mkdir()
        self.env = {key: os.environ[key] for key in ("PATH", "HOME") if key in os.environ}
        self.env.update(
            PATH=str(REPO / "bin") + os.pathsep + self.env.get("PATH", os.defpath),
            XDG_CONFIG_HOME=str(directory / "config"),
            XDG_STATE_HOME=str(directory / "state"),
            TMUX_TMPDIR=str(directory / "tmux"),
            GIT_CONFIG_GLOBAL="/dev/null",
            GIT_CONFIG_SYSTEM="/dev/null",
            SHELL="/bin/sh", TERM="xterm-256color", COLORTERM="truecolor",
            LANG="C.UTF-8", LC_ALL="C.UTF-8", PS1="${GG_BRANCH:-atlas} $ ",
        )
        self.socket = "gg-" + hashlib.sha256(str(self.root / ".git").encode()).hexdigest()[:16]
        self.client = None
        self.master = None
        self.reader = None
        self.events = []
        self.previous = None
        self.started = 0

    def run(self, *args, check=True):
        return subprocess.run(args, cwd=self.root, env=self.env, check=check,
                              capture_output=True, text=True, timeout=15).stdout

    def tmux(self, *args, check=True):
        return self.run("tmux", "-L", "recorder", *args, check=check)

    def setup(self):
        self.run("git", "init", "-b", "main")
        (self.root / "app.py").write_text("login_enabled = False\n")
        self.run("git", "add", "app.py")
        self.run("git", "-c", "user.name=Demo", "-c", "user.email=demo@example.invalid",
                 "commit", "-m", "Initial project")
        self.tmux("-f", "/dev/null", "new-session", "-d", "-s", "demo",
                  "-x", str(COLS), "-y", str(ROWS), "-c", str(self.root), "/bin/sh")
        self.tmux("set-option", "-g", "status", "off")
        self.master, slave = pty.openpty()
        try:
            fcntl.ioctl(slave, termios.TIOCSWINSZ, struct.pack("HHHH", ROWS, COLS, 0, 0))
            self.client = subprocess.Popen(
                ["tmux", "-L", "recorder", "attach-session", "-t", "demo"],
                cwd=self.root, env=self.env, stdin=slave, stdout=slave, stderr=slave,
                start_new_session=True,
            )
        finally:
            os.close(slave)
        self.reader = threading.Thread(target=self.drain, daemon=True)
        self.reader.start()
        self.started = time.monotonic()
        self.wait_for(lambda: "atlas $" in self.screen())

    def drain(self):
        while self.client.poll() is None:
            try:
                if select.select([self.master], [], [], 0.1)[0]:
                    if not os.read(self.master, 65536):
                        break
            except OSError:
                break

    def screen(self):
        return self.tmux("capture-pane", "-p", "-t", "demo:0")

    def capture(self):
        screen = self.tmux("capture-pane", "-p", "-e", "-N", "-t", "demo:0")
        if screen != self.previous:
            frame = "\x1b[?25l\x1b[0m\x1b[2J" + "".join(
                f"\x1b[{row};1H{line}" for row, line in enumerate(screen.splitlines(), 1)
            )
            self.events.append([round(time.monotonic() - self.started, 3), "o", frame])
            self.previous = screen

    def hold(self, seconds):
        end = time.monotonic() + seconds
        while time.monotonic() < end:
            self.capture()
            time.sleep(0.08)

    def wait_for(self, predicate):
        end = time.monotonic() + 12
        while time.monotonic() < end:
            if predicate():
                self.capture()
                return
            self.hold(0.08)
        raise RuntimeError("Demo did not reach the expected state:\n" + self.screen())

    def press(self, *keys):
        self.tmux("send-keys", "-t", "demo:0", *keys)
        self.hold(0.15)

    def type(self, text):
        for char in text:
            self.tmux("send-keys", "-l", "-t", "demo:0", char)
            self.hold(0.04)
        self.hold(0.25)

    def command(self, text):
        self.type(text)
        self.press("Enter")

    def sessions(self):
        return json.loads(self.run(str(REPO / "bin/gg"), "list", "--json"))

    def terminal_pids(self):
        return self.run("tmux", "-L", self.socket, "list-panes", "-a", "-F",
                        "#{@gg_task}:#{pane_pid}")

    def walkthrough(self):
        self.hold(0.6)
        self.command("gg create feat/login")
        self.wait_for(lambda: "Session created: feat/login" in self.screen())
        self.hold(1.2)
        self.command("gg dashboard")
        self.wait_for(lambda: "No terminal open" in self.screen())
        self.hold(0.8)
        self.press("j", "Enter")
        self.wait_for(lambda: "feat/login $" in self.screen())
        self.hold(0.5)
        self.command("echo 'login_enabled = True' > app.py")
        self.command("git diff")
        self.wait_for(lambda: "+login_enabled = True" in self.screen())
        self.hold(1.5)
        self.press("C-w", "n")
        self.wait_for(lambda: "Template" in self.screen())
        self.hold(0.6)
        self.type("fix/header")
        self.hold(0.5)
        self.press("Enter")
        self.wait_for(lambda: "fix/header $" in self.screen())
        self.command("git status -sb")
        self.wait_for(lambda: "## fix/header" in self.screen())
        self.command("cat app.py")
        self.wait_for(lambda: "login_enabled = False" in self.screen())
        self.hold(1.8)
        pids = self.terminal_pids()
        self.press("C-w")
        self.hold(0.5)
        self.press("k")
        self.hold(0.5)
        self.press("Enter")
        self.wait_for(lambda: "+login_enabled = True" in self.screen())
        self.command("cat app.py")
        self.hold(2)
        assert self.terminal_pids() == pids, "Switching restarted a terminal"
        worktrees = Path(str(self.root) + "__sessions")
        assert (worktrees / "feat/login/app.py").read_text() == "login_enabled = True\n"
        assert (worktrees / "fix/header/app.py").read_text() == "login_enabled = False\n"
        assert (self.root / "app.py").read_text() == "login_enabled = False\n"

    def verify_quit(self):
        pids = self.terminal_pids()
        self.tmux("send-keys", "-t", "demo:0", "C-w", "q")
        end = time.monotonic() + 5
        while time.monotonic() < end and "Grand Git" in self.screen():
            time.sleep(0.1)
        assert "Grand Git" not in self.screen(), "Dashboard did not quit"
        sessions = self.sessions()
        assert len(sessions) == 2 and all(row["active"] for row in sessions)
        assert self.terminal_pids() == pids, "Quitting restarted a terminal"

    def close(self):
        self.run("tmux", "-L", self.socket, "kill-server", check=False)
        self.tmux("kill-server", check=False)
        if self.client is not None:
            self.client.wait(timeout=5)
        if self.reader is not None:
            self.reader.join(timeout=1)
        if self.master is not None:
            os.close(self.master)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--agg", default="agg", help="Path to asciinema agg 1.9.0+")
    args = parser.parse_args()
    from PIL import Image

    if not (REPO / "bin/gg").is_file():
        parser.error("Build gg first: go build -o bin/gg ./cmd/gg")
    if not shutil.which(args.agg):
        parser.error("Install asciinema agg, or pass its path with --agg")
    output = REPO / "docs/assets"
    output.mkdir(parents=True, exist_ok=True)
    with tempfile.TemporaryDirectory(prefix="gg-demo-") as temporary:
        recording = Recording(Path(temporary).resolve())
        try:
            recording.setup()
            recording.walkthrough()
            recording.verify_quit()
            cast = Path(temporary) / "demo.cast"
            header = {"version": 2, "width": COLS, "height": ROWS,
                      "title": "Grand Git: parallel tasks", "env": {"TERM": "xterm-256color"}}
            # Show the finished dashboard when a viewer only displays the first frame.
            events = [[0, "o", recording.events[-1][2]]]
            events.extend([round(at + 1.5, 3), kind, frame] for at, kind, frame in recording.events)
            cast.write_text("\n".join(json.dumps(item) for item in [header, *events]) + "\n")
            gif = Path(temporary) / "gg-demo.gif"
            subprocess.run([args.agg, "--quiet", "--font-family", "DejaVu Sans Mono",
                            "--theme", "github-dark", "--font-size", "16", "--line-height", "1.35",
                            "--fps-cap", "12", "--last-frame-duration", "3", str(cast), str(gif)], check=True)
            assert gif.stat().st_size < 2 * 1024 * 1024, "README GIF exceeds 2 MiB"
            with Image.open(gif) as animation:
                assert animation.is_animated and animation.info.get("loop") == 0
                animation.seek(animation.n_frames - 1)
                animation.convert("RGB").save(output / "gg-demo.png", optimize=True)
                print(f"GIF: {animation.width}x{animation.height}, {animation.n_frames} frames")
            shutil.copyfile(gif, output / gif.name)
            print(f"Saved {output / gif.name} ({gif.stat().st_size:,} bytes) and gg-demo.png")
            print("Verified separate worktrees, unchanged terminal PIDs and active sessions after quit.")
        finally:
            recording.close()


if __name__ == "__main__":
    main()
