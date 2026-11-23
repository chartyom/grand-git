"""Real Git/tmux/PTY tests. Every test owns an isolated repo and tmux socket."""
import concurrent.futures
import fcntl
import hashlib
import json
import os
from pathlib import Path
import pty
import secrets
import shutil
import signal
import select
import shlex
import struct
import subprocess
import tempfile
import termios
import time
import threading
import unittest

APP = Path(__file__).resolve().parents[1]
BIN = APP / "bin/gg"


def wait_for(predicate, timeout=8):
    end = time.monotonic() + timeout
    while time.monotonic() < end:
        result = predicate()
        if result:
            return result
        time.sleep(0.08)
    raise AssertionError("condition did not become true")


class SessionFixture(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory(prefix="gg-test-")
        self.tmp.name = str(Path(self.tmp.name).resolve())
        self.root = Path(self.tmp.name) / "repo with spaces"
        self.root.mkdir()
        self.env = dict(os.environ, TERM="xterm-256color", SHELL="/bin/sh",
                        GG_TEST_LEGACY_DASHBOARD="1",
                        GIT_CONFIG_GLOBAL="/dev/null", GIT_CONFIG_SYSTEM="/dev/null",
                        XDG_CONFIG_HOME=str(Path(self.tmp.name) / "config"),
                        XDG_STATE_HOME=str(Path(self.tmp.name) / "state"))
        self.env.pop("TMUX", None)
        self.env.pop("TMUX_PANE", None)
        self.git("init", "-b", "main")
        (self.root / "README").write_text("main\n")
        self.git("add", ".")
        self.git("-c", "user.name=Test", "-c", "user.email=test@localhost", "commit", "-m", "main")
        self.git("branch", "release/demo")
        self.socket = "gg-" + hashlib.sha256(str(self.root / ".git").encode()).hexdigest()[:16]
        self.clients = []
        self.outputs = {}

    def tearDown(self):
        catalog = Path(self.env["XDG_STATE_HOME"]) / "grand-git/workspaces.json"
        if catalog.exists():
            try:
                for group in json.loads(catalog.read_text())["workspaces"]:
                    prefix = "gg-dir-" if group.get("directory") else "gg-"
                    key = group["root"] if group.get("directory") else group["common"]
                    socket = prefix + hashlib.sha256(key.encode()).hexdigest()[:16]
                    self.run_cmd("tmux", "-L", socket, "kill-server", check=False, cwd=self.tmp.name)
            except (ValueError, KeyError):
                pass
        self.tmux("kill-server", check=False)
        self.run_cmd("tmux", "-L", self.socket + "-outer", "kill-server", check=False)
        for proc, master, stop, reader in self.clients:
            if proc.poll() is None:
                proc.terminate()
                try:
                    proc.wait(timeout=2)
                except subprocess.TimeoutExpired:
                    proc.kill()
                    proc.wait(timeout=2)
            stop.set()
            reader.join(timeout=1)
            os.close(master)
        shutil.rmtree(str(self.root) + "__sessions", ignore_errors=True)
        self.tmp.cleanup()

    def run_cmd(self, *args, check=True, cwd=None, env=None, input=None):
        p = subprocess.run(args, cwd=cwd or self.root, env=env or self.env,
                           text=True, input=input, capture_output=True, timeout=20)
        if check and p.returncode:
            self.fail(f"{args}: {p.stdout}\n{p.stderr}")
        return p

    def git(self, *args, **kwargs):
        return self.run_cmd("git", *args, **kwargs)

    def commit(self, message, cwd=None):
        return self.git("-c", "user.name=Test", "-c", "user.email=test@localhost", "commit", "-m", message, cwd=cwd)

    def gg(self, *args, **kwargs):
        return self.run_cmd(str(BIN), *args, **kwargs)

    def tmux(self, *args, **kwargs):
        return self.run_cmd("tmux", "-L", self.socket, *args, **kwargs)

    def create(self, name="task", **kwargs):
        return self.gg("create", name, **kwargs)

    def tasks(self, cwd=None):
        return [t for t in self.all_tasks(cwd) if t["kind"] == "worktree"]

    def all_tasks(self, cwd=None):
        public = json.loads(self.gg("list", "--json", cwd=cwd).stdout)
        return [dict(t, id=int(t["id"].split(":")[1])) for t in public
                if t["workspace"]["path"] == str(self.root) and t["kind"] in ("source", "worktree")]

    def delete(self, task_id, *args, **kwargs):
        task = next(t for t in self.all_tasks() if t["id"] == task_id)
        return self.gg("delete", task["branch"], *args, **kwargs)

    def panes(self, task=1):
        data = self.tmux("list-panes", "-a", "-F",
                         "#{pane_id}|#{@gg_task}|#{@gg_sidebar}|#{pane_active}|#{pane_current_path}|#{pane_width}").stdout
        rows = {}
        for line in data.splitlines():
            row = line.split("|")
            if row[1] == str(task):
                rows.setdefault(row[0], row)
        return list(rows.values())

    def terminal(self, task=1):
        return next(p[0] for p in self.panes(task) if p[2] != "1")

    def sidebar(self, task=1):
        return next(p[0] for p in self.panes(task) if p[2] == "1")

    def start_client(self, args, cwd=None):
        master, slave = pty.openpty()
        fcntl.ioctl(slave, termios.TIOCSWINSZ, struct.pack("HHHH", 36, 120, 0, 0))
        proc = subprocess.Popen(args, cwd=cwd or self.root, env=self.env,
                                stdin=slave, stdout=slave, stderr=slave, start_new_session=True)
        os.close(slave)
        # A real terminal continuously consumes output. Without draining the PTY,
        # the client eventually blocks on output instead of reading hotkeys.
        stop = threading.Event()
        output = bytearray()
        self.outputs[proc.pid] = output
        def drain():
            while not stop.is_set():
                try:
                    if select.select([master], [], [], 0.1)[0]:
                        chunk = os.read(master, 65536)
                        if not chunk:
                            break
                        output.extend(chunk)
                except OSError:
                    break
        reader = threading.Thread(target=drain, daemon=True)
        reader.start()
        self.clients.append((proc, master, stop, reader))
        return proc, master

    def attach(self, cwd=None):
        proc, master = self.start_client([str(BIN), "dashboard"], cwd=cwd)
        wait_for(lambda: str(proc.pid) in self.tmux("list-clients", "-F", "#{client_pid}", check=False).stdout.splitlines())
        return proc, master

    def seed_legacy_source(self, cwd=None, running=True):
        """Explicitly model a pre-v6 registry; normal gg calls never create this row."""
        cwd = cwd or self.root
        self.gg("list", cwd=cwd)
        top = self.git("rev-parse", "--show-toplevel", cwd=cwd).stdout.strip()
        directory = self.root / ".git/grand-git"
        directory.mkdir(exist_ok=True)
        with (directory / "lock").open("a+") as lock:
            fcntl.flock(lock, fcntl.LOCK_EX)
            path = directory / "state.json"
            state = json.loads(path.read_text()) if path.exists() else {"next_id": 1, "tasks": []}
            task = next((t for t in state["tasks"] if t["path"] == top and t.get("kind") != "temporary"), None)
            if task is None:
                task_id = 0 if top == str(self.root) else state["next_id"]
                state["next_id"] = max(state["next_id"], task_id + 1)
                task = {"id": task_id, "path": top, "protected": True,
                        "branch": self.git("branch", "--show-current", cwd=cwd).stdout.strip(),
                        "base": self.git("rev-parse", "HEAD", cwd=cwd).stdout.strip(),
                        "created_at": "2026-01-01T00:00:00Z"}
                state["tasks"].insert(0, task)
                path.write_text(json.dumps(state))
            if running:
                # Establish the old terminal while holding the registry lock, so
                # already-running sidebars cannot retire a half-built fixture.
                panes = self.tmux("list-panes", "-a", "-F", "#{pane_id}|#{@gg_task}|#{@gg_sidebar}", check=False).stdout
                existing = [r.split("|") for r in panes.splitlines() if r.split("|")[1] == str(task["id"])]
                if not any(r[2] != "1" for r in existing):
                    if self.tmux("has-session", "-t", "=dashboard", check=False).returncode:
                        window = self.tmux("-f", "/dev/null", "new-session", "-d", "-s", "dashboard", "-x", "120", "-y", "36", "-c", top, "-P", "-F", "#{window_id}", "/bin/sh").stdout.strip()
                    else:
                        window = self.tmux("new-window", "-d", "-t", "dashboard:", "-c", top, "-P", "-F", "#{window_id}", "/bin/sh").stdout.strip()
                    self.tmux("set-window-option", "-t", window, "@gg_task", str(task["id"]))
                    self.tmux("set-window-option", "-t", window, "remain-on-exit", "on")
                    command = shlex.join([str(BIN), "_sidebar", "--root", str(self.root), "--task", str(task["id"])])
                    side = self.tmux("split-window", "-d", "-h", "-b", "-l", "30", "-t", window, "-P", "-F", "#{pane_id}", command).stdout.strip()
                    self.tmux("set-option", "-p", "-t", side, "@gg_sidebar", "1")
        catalog = json.loads((Path(self.env["XDG_STATE_HOME"]) / "grand-git/workspaces.json").read_text())
        group = next(g for g in catalog["workspaces"] if g["root"] == str(self.root))
        return task["id"], group["id"]

    def attach_session(self, cwd=None):
        """Open a managed task, or an explicitly seeded legacy source fixture."""
        cwd = cwd or self.root
        top = self.git("rev-parse", "--show-toplevel", cwd=cwd).stdout.strip()
        row = next((t for t in self.public_tasks() if t["path"] == top and t["kind"] != "temporary"), None)
        if row is None:
            task_id, group_id = self.seed_legacy_source(cwd)
        else:
            task_id, group_id = int(row["id"].split(":")[1]), row["workspace"]["id"]
        client_path = Path(self.env["XDG_STATE_HOME"]) / "grand-git/clients"
        client_path.mkdir(parents=True, exist_ok=True)
        gate = client_path / f"connect-ready-{secrets.token_hex(8)}"
        proc, master = self.start_client([
            "/bin/sh", "-c", 'while [ ! -f "$1" ]; do sleep 0.01; done; shift; exec "$@"',
            "sh", str(gate), str(BIN), "_connect", "--workspace", str(group_id), "--task", str(task_id)
        ], cwd=cwd)
        stat = Path(f"/proc/{proc.pid}/stat").read_text()
        start = stat[stat.rfind(")") + 1:].split()[19]
        (client_path / f"{proc.pid}.json").write_text(json.dumps({
            "pid": proc.pid, "start": start, "token": secrets.token_hex(16), "origins": []
        }))
        gate.touch()
        wait_for(lambda: str(proc.pid) in self.tmux("list-clients", "-F", "#{client_pid}", check=False).stdout.splitlines())
        return proc, master

    def search_population(self):
        """Provide the eleven real rows needed by search-focused scenarios."""
        for index in range(max(0, 11 - len(self.public_tasks()))):
            self.create(f"zz-search-{index}")

    def active(self):
        clients = self.tmux("list-clients", "-F", "#{client_pid}|#{@gg_task}|#{@gg_sidebar}", check=False).stdout
        by_pid = {int(row.split("|", 1)[0]): row for row in clients.splitlines()}
        for proc, _, _, _ in reversed(self.clients):
            if proc.pid in by_pid and proc.poll() is None:
                _, task, sidebar = by_pid[proc.pid].split("|", 2)
                return task + "|" + sidebar
        return self.tmux("display-message", "-p", "#{@gg_task}|#{@gg_sidebar}").stdout.strip()

    def capture(self, pane):
        return self.tmux("capture-pane", "-p", "-t", pane).stdout

    def public_tasks(self):
        return json.loads(self.gg("list", "--json").stdout)

    def geometry(self, task):
        fields = "#{pane_id}|#{@gg_task}|#{@gg_sidebar}|#{session_name}|#{pane_left}|#{pane_top}|#{pane_width}|#{pane_height}|#{pane_pid}|#{pane_dead}"
        rows = self.tmux("list-panes", "-a", "-F", fields).stdout.splitlines()
        unique = {}
        for line in rows:
            row = line.split("|")
            if row[1] == str(task):
                unique.setdefault(row[0], row)
        return list(unique.values())

    def resize_client(self, proc, master, width, height=36):
        fcntl.ioctl(master, termios.TIOCSWINSZ, struct.pack("HHHH", height, width, 0, 0))
        os.kill(proc.pid, signal.SIGWINCH)


class SessionsTest(SessionFixture):
    def test_branch_identity_activity_and_named_paths(self):
        self.create("feat/feat1")
        row = next(t for t in self.public_tasks() if t["branch"] == "feat/feat1")
        self.assertEqual(row["path"], str(Path(str(self.root) + "__sessions") / "feat/feat1"))
        self.assertTrue(row["active"])
        self.assertEqual(row["terminals"], 1)
        self.assertRegex(row["id"], r"^\d+:\d+$")
        self.assertNotIn("description", row)
        before = self.terminal()
        pid = self.tmux("display-message", "-p", "-t", before, "#{pane_pid}").stdout.strip()
        self.assertNotEqual(self.create("feat/feat1", check=False).returncode, 0)
        self.gg("stop", "feat/feat1")
        self.assertFalse(self.tasks()[0]["active"])
        self.assertTrue(Path(row["path"]).is_dir())
        self.assertNotEqual(self.create("feat/feat1", check=False).returncode, 0)
        self.assertNotEqual(self.gg("start", "1", check=False).returncode, 0)
        self.gg("start", "feat/feat1")
        self.gg("start", "feat/feat1")
        self.assertEqual(self.tasks()[0]["terminals"], 1)
        self.assertNotEqual(pid, self.tmux("display-message", "-p", "-t", self.terminal(), "#{pane_pid}").stdout.strip())
        self.gg("stop", "--all", "--yes")
        self.assertFalse(any(t["active"] for t in self.public_tasks()))
        self.assertEqual(len(self.public_tasks()), 1)
        # A branch literally named "1" is a branch, never an internal ID.
        self.create("1")
        self.gg("delete", "1", "--yes")
        self.assertEqual([t["branch"] for t in self.tasks()], ["feat/feat1"])

    def test_named_path_rejects_symlink_parents_and_occupied_directories(self):
        base = Path(str(self.root) + "__sessions")
        base.mkdir()
        outside = Path(self.tmp.name) / "outside"
        outside.mkdir()
        (base / "feat").symlink_to(outside, target_is_directory=True)
        self.assertNotEqual(self.create("feat/escape", check=False).returncode, 0)
        self.assertFalse((outside / "escape").exists())
        self.assertEqual(self.tasks(), [])
        (base / "feat").unlink()
        (base / "occupied").mkdir()
        (base / "occupied/keep").write_text("keep")
        self.assertNotEqual(self.create("occupied", check=False).returncode, 0)
        self.assertEqual((base / "occupied/keep").read_text(), "keep")

    def test_bulk_delete_preflight_preserves_running_tasks_and_protected_origin(self):
        self.create("first")
        self.create("second")
        self.attach_session()
        first, second = self.tasks()
        (Path(second["path"]) / "keep").write_text("unsaved")
        pids = [self.tmux("display-message", "-p", "-t", self.terminal(i), "#{pane_pid}").stdout.strip() for i in [0, 1, 2]]
        self.assertNotEqual(self.gg("delete", "--all", "--yes", check=False).returncode, 0)
        self.assertEqual(pids, [self.tmux("display-message", "-p", "-t", self.terminal(i), "#{pane_pid}").stdout.strip() for i in [0, 1, 2]])
        self.assertTrue(all(t["active"] for t in self.public_tasks()))
        self.assertNotEqual(self.gg("stop", "--all", check=False).returncode, 0)
        self.gg("delete", "--all", "--force", "--yes")
        self.assertEqual(self.tasks(), [])
        self.assertEqual(self.public_tasks(), [])
        self.assertTrue((self.root / "README").is_file())
        self.assertFalse(Path(first["path"]).exists())
        self.assertFalse(Path(second["path"]).exists())
        self.git("show-ref", "--verify", "refs/heads/first")
        self.git("show-ref", "--verify", "refs/heads/second")

    def test_stop_all_from_own_terminal_leaves_dashboard_and_records(self):
        self.create("feat/self")
        path = self.tasks()[0]["path"]
        proc, master = self.attach_session(cwd=path)
        wait_for(lambda: self.active() == "1|")
        os.write(master, (shlex.quote(str(BIN)) + " stop --all --yes\r").encode())
        wait_for(lambda: not any(t["active"] for t in self.public_tasks()))
        wait_for(lambda: self.active() == "-1|1")
        self.assertTrue(Path(path).is_dir())
        self.assertIsNone(proc.poll())
        wait_for(lambda: "Active 0/1" in self.tmux("show-options", "-g", "-v", "status-right").stdout)
        os.write(master, b"j\r")
        wait_for(lambda: self.active() == "1|")
        self.assertTrue(self.tasks()[0]["active"])

    def test_delete_all_from_own_terminal_preserves_source_directory(self):
        self.create("feat/self")
        path = self.tasks()[0]["path"]
        proc, master = self.attach_session(cwd=path)
        wait_for(lambda: self.active() == "1|")
        os.write(master, (shlex.quote(str(BIN)) + " delete --all --yes\r").encode())
        wait_for(lambda: not self.tasks())
        self.assertFalse(Path(path).exists())
        self.assertEqual(self.public_tasks(), [])
        self.assertTrue((self.root / "README").is_file())
        self.assertIsNone(proc.poll())

    def test_settings_layers_and_live_navigation_language(self):
        self.gg("config", "set", "language", "ru", cwd=self.tmp.name)
        self.assertIn("Доступные команды", self.gg("--help").stdout)
        self.gg("config", "set", "language", "en", "--project")
        self.assertIn("Available Commands", self.gg("--help").stdout)
        self.create("feat/settings")
        self.assertIn("language = en", self.gg("config", cwd=self.tasks()[0]["path"]).stdout)
        proc, master = self.attach_session()
        pid = self.tmux("display-message", "-p", "-t", self.terminal(0), "#{pane_pid}").stdout.strip()
        os.write(master, b"\x17")
        wait_for(lambda: self.active() == "0|1")
        self.gg("config", "reset", "language", "--project")
        wait_for(lambda: "[?] Действия" in self.capture(self.sidebar(0)))
        keys = self.tmux("list-keys", "-T", "root").stdout
        self.assertIn("C-w", keys)
        self.assertNotIn(" C-g ", keys)
        self.assertNotIn(" C-Left ", keys)
        self.assertNotIn(" M-Left ", keys)
        self.assertNotIn(" C-p ", keys)
        self.assertEqual(pid, self.tmux("display-message", "-p", "-t", self.terminal(0), "#{pane_pid}").stdout.strip())
        before = (self.root / ".gg/config.yaml").read_bytes()
        self.assertNotEqual(self.gg("config", "set", "navigation", "alt-arrows", "--project", check=False).returncode, 0)
        self.assertEqual(before, (self.root / ".gg/config.yaml").read_bytes())

    def test_responsive_sidebar_preserves_split_processes_and_compact_popup(self):
        proc, master = self.attach_session()
        wait_for(lambda: self.active() == "0|")
        os.write(master, b"\x17t")
        wait_for(lambda: "Add terminal" in self.capture(self.sidebar(0)))
        os.write(master, b"r")
        wait_for(lambda: len([p for p in self.geometry(0) if p[2] != "1"]) == 2)
        terms = [p for p in self.geometry(0) if p[2] != "1"]
        self.assertEqual(terms[0][5], terms[1][5])
        pids = {p[0]: p[8] for p in terms}
        self.resize_client(proc, master, 200)
        wait_for(lambda: any(p[2] == "1" and p[3] == "dashboard" and p[6] == "50" for p in self.geometry(0)))
        self.resize_client(proc, master, 80)
        wait_for(lambda: any(p[2] == "1" and p[3] == "_gg-park" for p in self.geometry(0)))
        # Cycle through the user panes, then open the compact list.
        self.tmux("select-pane", "-t", terms[0][0])
        output_start = len(self.outputs[proc.pid])
        os.write(master, b"\x17\x17")
        wait_for(lambda: b"[?] Actions" in self.outputs[proc.pid][output_start:])
        os.write(master, b"\x1b")
        time.sleep(0.15)
        self.resize_client(proc, master, 160)
        wait_for(lambda: any(p[2] == "1" and p[3] == "dashboard" and p[6] == "40" for p in self.geometry(0)))
        now = self.geometry(0)
        self.assertEqual(pids, {p[0]: p[8] for p in now if p[2] != "1"})
        side = next(p for p in now if p[2] == "1")
        user = [p for p in now if p[2] != "1"]
        self.assertTrue(all(int(p[4]) > int(side[4]) for p in user))
        self.assertEqual(user[0][5], user[1][5])

    def test_search_filter_mouse_and_split_cancel(self):
        self.create("feat/alpha")
        self.create("fix/beta")
        proc, master = self.attach_session()
        os.write(master, b"\x17")
        wait_for(lambda: self.active() == "0|1")
        self.search_population()
        os.write(master, b"/beta\r")
        wait_for(lambda: "fix/beta" in self.capture(self.sidebar(0)) and "feat/alpha" not in self.capture(self.sidebar(0)))
        os.write(master, b"\r")
        wait_for(lambda: self.active() == "2|")
        os.write(master, b"\x17t")
        wait_for(lambda: "Add terminal" in self.capture(self.sidebar(2)))
        os.write(master, b"\x1b")
        self.assertEqual(self.tasks()[1]["terminals"], 1)
        # Fresh per-window list has no query. A single click opens alpha.
        side = next(p for p in self.geometry(2) if p[2] == "1")
        line = next(i for i, text in enumerate(self.capture(side[0]).splitlines()) if "feat/alpha" in text)
        y = int(side[5]) + line + 1
        click = f"\x1b[<0;4;{y}M\x1b[<0;4;{y}m".encode()
        os.write(master, click)
        wait_for(lambda: self.active() == "1|")
        os.write(master, b"\x17s")
        wait_for(lambda: "Stop feat/alpha?" in self.capture(self.sidebar(1)))
        os.write(master, b"y")
        wait_for(lambda: not self.tasks()[0]["active"])
        wait_for(lambda: self.active() == "-1|1")
        os.write(master, b"a")
        home = self.tmux("display-message", "-p", "#{pane_id}").stdout.strip()
        wait_for(lambda: "feat/alpha" not in self.capture(home))

    def test_small_dashboard_can_create_and_open_another_session(self):
        proc, master = self.attach_session()
        root_pid = self.tmux("display-message", "-p", "-t", self.terminal(0), "#{pane_pid}").stdout.strip()
        self.resize_client(proc, master, 24, 10)
        wait_for(lambda: any(p[2] == "1" and p[3] == "_gg-park" for p in self.geometry(0)))
        self.create("tiny")
        self.assertTrue(self.tasks()[0]["active"])
        self.assertTrue(any(p[2] == "1" and p[3] == "_gg-park" for p in self.geometry(1)))
        offset = len(self.outputs[proc.pid])
        os.write(master, b"\x17")
        wait_for(lambda: b"[?] Actions" in self.outputs[proc.pid][offset:])
        os.write(master, b"j\r")
        wait_for(lambda: self.active() == "1|")
        self.resize_client(proc, master, 130)
        wait_for(lambda: any(p[2] == "1" and p[3] == "dashboard" for p in self.geometry(1)))
        self.assertEqual(root_pid, self.tmux("display-message", "-p", "-t", self.terminal(0), "#{pane_pid}").stdout.strip())

    def test_switching_active_tasks_avoids_repeated_full_redraws(self):
        self.create("first")
        self.create("second")
        proc, master = self.attach_session(cwd=self.tasks()[0]["path"])
        first, second = self.terminal(1), self.terminal(2)
        pids = [self.tmux("display-message", "-p", "-t", pane, "#{pane_pid}").stdout.strip()
                for pane in (first, second)]
        self.tmux("send-keys", "-t", first, "printf '\\033]2;App title\\007'", "Enter")
        title = lambda: self.tmux("display-message", "-p", "-t", first, "#{pane_title}").stdout.strip()
        wait_for(lambda: title() == "App title")

        for source, target, key in ((1, 2, b"j"), (2, 1, b"k")):
            os.write(master, b"\x17")
            wait_for(lambda: self.active() == f"{source}|1")
            os.write(master, key)
            name = "second" if target == 2 else "first"
            wait_for(lambda: any(line.endswith("/" + name) for line in self.capture(self.sidebar(source)).splitlines())
                     and any(line.split() == ["Open:", "no"] for line in self.capture(self.sidebar(source)).splitlines()))
            time.sleep(.2)
            offset = len(self.outputs[proc.pid])
            os.write(master, b"\r")
            wait_for(lambda: self.active() == f"{target}|")
            time.sleep(.15)
            output = bytes(self.outputs[proc.pid][offset:])
            self.assertLessEqual(output.count(b"\x1b[H"), 2)

        self.assertEqual(title(), "App title")
        self.assertEqual(pids, [self.tmux("display-message", "-p", "-t", pane, "#{pane_pid}").stdout.strip()
                                for pane in (first, second)])

    def test_legacy_panel_does_not_respawn_itself_during_switch(self):
        self.create("first")
        self.create("second")
        proc, master = self.attach()
        temporary = next(row for row in self.public_tasks() if row["kind"] == "temporary")
        temporary_id = int(temporary["id"].split(":")[1])
        os.write(master, b"\x17")
        wait_for(lambda: self.active() == f"{temporary_id}|1")
        side = self.sidebar(temporary_id)
        pid = self.tmux("display-message", "-p", "-t", side, "#{pane_pid}").stdout.strip()
        self.tmux("set-option", "-p", "-t", side, "@gg_ui_revision", "stale")
        os.write(master, b"k\r")
        wait_for(lambda: self.active() == "2|")
        self.assertIsNone(proc.poll())
        self.assertEqual(pid, self.tmux("display-message", "-p", "-t", side, "#{pane_pid}").stdout.strip())

    def test_settings_form_save_scope_and_cancel(self):
        proc, master = self.attach_session()
        os.write(master, b"\x17,")
        wait_for(lambda: "Save to:" in self.capture(self.sidebar(0)))
        os.write(master, b"\x1b[C")
        os.write(master, b"\x1b")
        wait_for(lambda: "Save to:" not in self.capture(self.sidebar(0)))
        self.assertFalse((self.root / ".gg/config.yaml").exists())
        os.write(master, b",")
        wait_for(lambda: "Save to:" in self.capture(self.sidebar(0)))
        # Template -> claude; scope -> project; save atomically.
        os.write(master, b"\x1b[B\x1b[C\x1b[B\x1b[C\r")
        wait_for(lambda: (self.root / ".gg/config.yaml").exists())
        self.assertIn("default_template = claude", self.gg("config").stdout)
        self.assertIn("language = en", self.gg("config").stdout)
        os.write(master, b"\x17")
        wait_for(lambda: self.active() == "0|")

    def test_stop_ends_background_jobs_and_start_opens_a_new_shell(self):
        self.create("worker")
        marker = Path(self.tmp.name) / "worker-heartbeat"
        terminal = self.terminal(1)
        self.tmux("send-keys", "-t", terminal,
                  f"while true; do echo tick >> {shlex.quote(str(marker))}; sleep .05; done &", "Enter")
        wait_for(lambda: marker.exists() and len(marker.read_text().splitlines()) > 3)
        self.gg("stop", "worker")
        n = len(marker.read_text().splitlines())
        time.sleep(.3)
        self.assertEqual(n, len(marker.read_text().splitlines()))
        self.gg("start", "worker")
        time.sleep(.2)
        self.assertEqual(n, len(marker.read_text().splitlines()), "Start unexpectedly resumed the old background job")
        self.assertEqual(self.tasks()[0]["terminals"], 1)

    def test_ctrl_w_cycles_list_and_terminal_panes(self):
        proc, master = self.attach_session()
        top = self.terminal(0)
        os.write(master, b"\x17t")
        wait_for(lambda: "Add terminal" in self.capture(self.sidebar(0)))
        os.write(master, b"b")
        wait_for(lambda: len([p for p in self.geometry(0) if p[2] != "1"]) == 2)
        bottom = next(p[0] for p in self.geometry(0) if p[2] != "1" and p[0] != top)
        current = lambda: self.tmux("display-message", "-p", "#{pane_id}").stdout.strip()
        wait_for(lambda: current() == bottom)
        os.write(master, b"\x17")
        wait_for(lambda: self.active() == "0|1")
        os.write(master, b"\x17")
        wait_for(lambda: current() == top)
        os.write(master, b"\x17")
        wait_for(lambda: current() == bottom)

    def test_create_bases_list_and_reuse_branch(self):
        # Make release genuinely different so --base isn't tested against identical commits.
        self.git("checkout", "release/demo")
        (self.root / "README").write_text("release\n")
        self.git("commit", "-am", "release", "--author=Test <test@localhost>",
                 env=dict(self.env, GIT_COMMITTER_NAME="Test", GIT_COMMITTER_EMAIL="test@localhost"))
        release = self.git("rev-parse", "HEAD").stdout.strip()
        self.git("checkout", "main")
        self.gg("create", "task/main", "--base", "main")
        self.gg("create", "task/release", "--base", "release/demo")
        tasks = self.tasks()
        self.assertEqual([t["id"] for t in tasks], [1, 2])
        self.assertEqual(tasks[1]["base"], release)
        self.assertEqual((Path(tasks[1]["path"]) / "README").read_text(), "release\n")
        self.assertEqual(len(self.tasks(cwd=tasks[0]["path"])), 2)
        self.assertNotEqual(self.gg("create", "main", check=False).returncode, 0)
        self.assertEqual(len(self.tasks()), 2)
        self.delete(1, "--yes")
        self.assertEqual(self.git("show-ref", "--verify", "refs/heads/task/main").returncode, 0)
        self.create("task/main")
        self.assertEqual(self.tasks()[-1]["id"], 3)
        self.delete(2, "--yes")
        self.create("task/release")
        self.assertEqual(self.tasks()[-1]["base"], release)

    def test_concurrent_creation_allocates_unique_ids(self):
        with concurrent.futures.ThreadPoolExecutor(max_workers=4) as pool:
            futures = [pool.submit(self.create, f"task/{i}") for i in range(4)]
            for f in futures:
                f.result()
        self.assertEqual([t["id"] for t in self.tasks()], [1, 2, 3, 4])

    def test_deletion_protects_user_files_and_keeps_branch(self):
        self.create()
        path = Path(self.tasks()[0]["path"])
        (path / "untracked.txt").write_text("do not lose")
        self.assertNotEqual(self.delete(1, "--yes", check=False).returncode, 0)
        self.assertTrue((path / "untracked.txt").exists())
        (path / "untracked.txt").unlink()
        (path / "README").write_text("edited")
        self.assertNotEqual(self.delete(1, "--yes", check=False).returncode, 0)
        self.delete(1, "--force", "--yes")
        self.assertFalse(path.exists())
        self.assertEqual(self.tasks(), [])
        self.git("show-ref", "--verify", "refs/heads/task")

    def test_hooks_are_local_idempotent_and_preserve_existing(self):
        global_files = [Path.home() / ".codex/config.toml", Path.home() / ".codex/hooks.json"]
        before_global = {p: p.read_bytes() if p.exists() else None for p in global_files}
        folder = self.root / ".codex"
        folder.mkdir()
        original = {"description": "mine", "hooks": {"Stop": [{"hooks": [{"type": "command", "command": "echo existing"}]}]}}
        (folder / "hooks.json").write_text(json.dumps(original))
        (folder / "config.toml").write_text('model = "keep-me"\n[features]\nhooks = false\n')
        self.create()
        self.gg("init")
        first = (folder / "hooks.json").read_bytes()
        self.gg("init")
        self.assertEqual(first, (folder / "hooks.json").read_bytes())
        doc = json.loads(first)
        self.assertEqual(doc["hooks"]["Stop"][0]["hooks"][0]["command"], "echo existing")
        for groups in doc["hooks"].values():
            managed = [h for g in groups for h in g["hooks"] if h.get("statusMessage") == "grand-git"]
            self.assertEqual(len(managed), 1)
        backups = self.root / ".git/grand-git/hook-backups"
        self.assertTrue(any(json.loads(p.read_text()) == original for p in backups.glob("*-hooks.json")))
        self.assertIn('model = \'keep-me\'', (folder / "config.toml").read_text())
        path = Path(self.tasks()[0]["path"])
        self.assertTrue((path / ".codex/hooks.json").is_file())
        self.assertFalse((path / ".codex").is_symlink())
        for event, status in [("SessionStart", "ready"), ("UserPromptSubmit", "working"),
                              ("PermissionRequest", "waiting"), ("Stop", "done")]:
            self.gg("_hook", "--event", event, input=json.dumps({"cwd": str(path), "session_id": "test-root"}))
            record = json.loads((self.root / ".git/grand-git/agent-1.json").read_text())
            self.assertEqual(record["status"], status)
        self.gg("_hook", "--event", "Stop", input=json.dumps({"cwd": str(path), "session_id": "other-root"}))
        self.assertEqual(json.loads((self.root / ".git/grand-git/agent-1.json").read_text())["session_id"], "test-root")
        for p, content in before_global.items():
            self.assertEqual(content, p.read_bytes() if p.exists() else None)
        # Only our untouched generated files are exempt from dirty protection.
        self.delete(1, "--yes")
        self.create("next")
        path = Path(self.tasks()[0]["path"])
        with (path / ".codex/config.toml").open("a") as f:
            f.write("\n# user edit\n")
        self.assertNotEqual(self.delete(2, "--yes", check=False).returncode, 0)

    def test_creation_rolls_back_when_tmux_is_unavailable(self):
        only_git = Path(self.tmp.name) / "only-git"
        only_git.mkdir()
        (only_git / "git").symlink_to(shutil.which("git"))
        p = self.create("failed", check=False, env=dict(self.env, PATH=str(only_git)))
        self.assertNotEqual(p.returncode, 0)
        self.assertEqual(self.tasks(), [])
        self.assertFalse((Path(str(self.root) + "__sessions") / "failed").exists())
        self.assertNotEqual(self.git("show-ref", "--verify", "refs/heads/failed", check=False).returncode, 0)

    def test_failed_checkout_hook_rolls_back_worktree_and_new_branch(self):
        hook = self.root / ".git/hooks/post-checkout"
        hook.write_text("#!/bin/sh\nexit 1\n")
        hook.chmod(0o755)
        result = self.create("failed-hook", check=False)
        self.assertNotEqual(result.returncode, 0)
        self.assertEqual(self.tasks(), [])
        self.assertFalse((Path(str(self.root) + "__sessions") / "failed-hook").exists())
        self.assertNotEqual(self.git("show-ref", "--verify", "refs/heads/failed-hook", check=False).returncode, 0)

    def test_dashboard_modal_keys_resize_and_background_survival(self):
        self.create("first")
        self.create("second")
        proc, master = self.attach_session()
        # The original worktree immediately opens its normal shell.
        wait_for(lambda: self.active() == "0|")
        os.write(master, b"\x17")
        wait_for(lambda: self.active() == "0|1")
        wait_for(lambda: "[?] Actions" in self.capture(self.sidebar(0)))
        os.write(master, b"j\r")
        wait_for(lambda: self.active() == "1|")
        marker = Path(self.tmp.name) / "heartbeat"
        line = f"while true; do echo tick >> '{marker}'; sleep 0.1; done\r".encode()
        os.write(master, line)
        wait_for(lambda: marker.exists() and len(marker.read_text().splitlines()) > 2)
        os.write(master, b"\x17")
        wait_for(lambda: self.active() == "1|1")
        os.write(master, b"j\r")
        wait_for(lambda: self.active() == "2|")
        n = len(marker.read_text().splitlines())
        wait_for(lambda: len(marker.read_text().splitlines()) > n + 2)
        os.write(master, b"\x17t")
        wait_for(lambda: "Add terminal" in self.capture(self.sidebar(2)))
        os.write(master, b"b")
        wait_for(lambda: len([p for p in self.panes(2) if p[2] != "1"]) == 2)
        wait_for(lambda: self.active() == "2|")
        os.write(master, b"\x17")
        wait_for(lambda: "[D] Delete" in self.capture(self.sidebar(2)))
        os.write(master, b"d")
        wait_for(lambda: "Delete second?" in self.capture(self.sidebar(2)))
        os.write(master, b"\x1b")
        wait_for(lambda: "Delete second?" not in self.capture(self.sidebar(2)))
        os.write(master, b"\x17")
        wait_for(lambda: self.active() == "2|")
        # Escape and Ctrl+C go to the shell/agent outside command mode.
        os.write(master, b"echo mode-off-ok\r")
        wait_for(lambda: "mode-off-ok" in self.capture(self.tmux("display-message", "-p", "#{pane_id}").stdout.strip()))
        os.write(master, b"sleep 30\r")
        wait_for(lambda: self.tmux("display-message", "-p", "#{pane_current_command}").stdout.strip() == "sleep")
        os.write(master, b"\x03")
        wait_for(lambda: self.tmux("display-message", "-p", "#{pane_current_command}").stdout.strip() == "sh")
        os.write(master, b"echo after-interrupt\r")
        wait_for(lambda: "after-interrupt" in self.capture(self.tmux("display-message", "-p", "#{pane_id}").stdout.strip()).splitlines())
        fcntl.ioctl(master, termios.TIOCSWINSZ, struct.pack("HHHH", 28, 100, 0, 0))
        os.kill(proc.pid, signal.SIGWINCH)
        wait_for(lambda: all(p[5] == "28" for p in self.panes(2) if p[2] == "1"))
        os.write(master, b"\x17\x17q")
        wait_for(lambda: proc.poll() is not None)
        n = len(marker.read_text().splitlines())
        wait_for(lambda: len(marker.read_text().splitlines()) > n + 2)

    def test_stopped_server_preserves_registry(self):
        self.create()
        self.tmux("kill-server")
        self.assertEqual(len(self.tasks()), 1)
        self.assertIn("stopped", self.gg("list").stdout)
        proc, master = self.attach_session()
        os.write(master, b"\x17j\r")
        wait_for(lambda: self.active() == "1|")
        self.assertEqual(len(self.tasks()), 1)

    def test_interactive_create_prompts_and_cancellation(self):
        proc, master = self.start_client([str(BIN), "create"])
        wait_for(lambda: "Branch name".encode() in self.outputs[proc.pid])
        os.write(master, b"interactive\r")
        wait_for(lambda: proc.poll() is not None)
        self.assertEqual(proc.returncode, 0)
        self.assertEqual(self.tasks()[0]["branch"], "interactive")
        self.assertNotIn("description", self.tasks()[0])
        self.assertNotIn("Description".encode(), self.outputs[proc.pid])
        proc, master = self.start_client([str(BIN), "create"])
        wait_for(lambda: "Branch name".encode() in self.outputs[proc.pid])
        os.write(master, b"\x1b")
        wait_for(lambda: proc.poll() is not None)
        self.assertEqual(len(self.tasks()), 1)

    def test_dashboard_create_and_delete_from_menu(self):
        proc, master = self.attach_session()
        os.write(master, b"\x17n")
        wait_for(lambda: "Branch name".encode() in self.outputs[proc.pid])
        os.write(master, b"from-menu\r")
        wait_for(lambda: len(self.tasks()) == 1)
        self.assertNotIn("description", self.tasks()[0])
        wait_for(lambda: self.active() == "1|")
        os.write(master, b"\x17d")
        wait_for(lambda: "Delete from-menu?" in self.capture(self.sidebar(1)))
        os.write(master, b"y")
        wait_for(lambda: not self.tasks())
        self.assertFalse((Path(str(self.root) + "__sessions") / "from-menu").exists())
        self.git("show-ref", "--verify", "refs/heads/from-menu")

    def test_create_with_branch_never_prompts_for_optional_fields(self):
        proc, master = self.start_client([str(BIN), "create", "no-questions"])
        wait_for(lambda: proc.poll() is not None)
        self.assertEqual(proc.returncode, 0)
        self.assertNotIn("description", self.tasks()[0])
        self.assertNotIn("Branch name".encode(), self.outputs[proc.pid])
        self.assertNotIn("Description".encode(), self.outputs[proc.pid])
        proc, master = self.start_client([str(BIN), "create", "with-description", "--description", "Explicit description"])
        wait_for(lambda: proc.poll() is not None)
        self.assertNotEqual(proc.returncode, 0)
        self.assertEqual(len(self.tasks()), 1)

    def test_help_is_available_without_repository_or_terminal(self):
        outside = self.tmp.name
        for args in [(), ("--help",), ("help",), ("create", "--help")]:
            with self.subTest(args=args):
                result = self.gg(*args, cwd=outside)
                self.assertIn("gg", result.stdout)
                self.assertIn("--help", result.stdout)
        bare = self.gg(cwd=outside).stdout
        for command in ["create", "list", "delete", "dashboard", "init"]:
            self.assertIn(command, bare)
        self.assertFalse((self.root / ".git/grand-git").exists())
        self.assertNotEqual(self.gg("create", check=False).returncode, 0)

    def test_dashboard_from_foreign_and_own_tmux(self):
        self.create()
        outer = self.socket + "-outer"
        self.run_cmd("tmux", "-L", outer, "-f", "/dev/null", "new-session", "-d", "-s", "outer", "-c", str(self.root), "/bin/sh")
        proc, master = self.start_client(["tmux", "-L", outer, "attach-session", "-t", "outer"])
        wait_for(lambda: self.run_cmd("tmux", "-L", outer, "list-clients", "-F", "#{client_pid}").stdout.strip())
        os.write(master, (shlex.quote(str(BIN)) + " dashboard\r").encode())
        wait_for(lambda: self.tmux("list-clients", "-F", "#{client_pid}", check=False).stdout.strip())
        self.search_population()
        os.write(master, b"\x17/task\r\r")
        wait_for(lambda: self.active() == "1|")
        # Starting dashboard again from its own terminal must switch, not nest.
        os.write(master, (shlex.quote(str(BIN)) + " dashboard; echo own-switch-ok\r").encode())
        wait_for(lambda: "own-switch-ok" in self.capture(self.terminal()).splitlines())
        self.assertEqual(len(self.tmux("list-clients", "-F", "#{client_pid}").stdout.splitlines()), 1)
        self.assertNotIn("no current client", self.capture(self.terminal()))
        self.assertIsNone(proc.poll())

    def test_delete_from_its_own_terminal_completes(self):
        self.create()
        proc, master = self.attach_session()
        os.write(master, b"\x17j\r")
        wait_for(lambda: self.active() == "1|")
        os.write(master, (shlex.quote(str(BIN)) + " delete task --yes\r").encode())
        wait_for(lambda: not self.tasks())
        self.assertFalse((Path(str(self.root) + "__sessions") / "task").exists())
        self.assertIsNone(proc.poll())

    def test_ignored_files_are_copied_independently_and_can_be_skipped(self):
        (self.root / ".gitignore").write_text(".env\n.agents/\nnode_modules/\ncache/*\n!cache/keep.txt\nempty/\n")
        (self.root / "nested").mkdir()
        (self.root / "nested/.gitignore").write_text("*.local\n")
        self.git("add", ".gitignore", "nested/.gitignore")
        self.commit("ignore local environment")
        files = {".env": "local env", ".agents/skills/task.md": "skill", ".agents/new\nline.md": "newline",
                 "node_modules/pkg/index.js": "original dependency", "node_modules/tool.js": "#!/bin/sh\n",
                 "cache/data": "ignored cache", "nested/settings.local": "nested ignored"}
        for name, contents in files.items():
            path = self.root / name
            path.parent.mkdir(parents=True, exist_ok=True)
            path.write_text(contents)
        (self.root / "node_modules/.bin").mkdir()
        (self.root / "node_modules/.bin/tool").symlink_to("../tool.js")
        (self.root / "node_modules/tool.js").chmod(0o751)
        os.utime(self.root / "node_modules/tool.js", (1_600_000_000, 1_600_000_000))
        (self.root / "cache/keep.txt").write_text("not ignored")
        (self.root / "notes.txt").write_text("ordinary untracked file")
        (self.root / "empty/nested").mkdir(parents=True)
        os.mkfifo(self.root / ".agents/pipe")
        nested_repo = self.root / ".agents/nested-repo"
        nested_repo.mkdir()
        self.git("init", "-b", "main", cwd=nested_repo)
        (nested_repo / "private.txt").write_text("nested repository content")
        self.create("copy-env")
        task = self.tasks()[0]
        dest = Path(task["path"])
        self.assertEqual(task["copy_source"], str(self.root))
        for name, contents in files.items():
            self.assertEqual((dest / name).read_text(), contents)
        self.assertFalse((dest / "cache/keep.txt").exists())
        self.assertFalse((dest / "notes.txt").exists())
        self.assertFalse((dest / ".agents/pipe").exists())
        self.assertFalse((dest / ".agents/nested-repo").exists())
        self.assertTrue((dest / "empty/nested").is_dir())
        self.assertEqual(os.readlink(dest / "node_modules/.bin/tool"), "../tool.js")
        self.assertEqual((dest / "node_modules/tool.js").stat().st_mode & 0o777, 0o751)
        self.assertEqual((dest / "node_modules/tool.js").stat().st_mtime, 1_600_000_000)
        self.assertFalse(os.path.samefile(dest / "node_modules/pkg/index.js", self.root / "node_modules/pkg/index.js"))
        (dest / "node_modules/pkg/index.js").write_text("task-only edit")
        self.assertEqual((self.root / "node_modules/pkg/index.js").read_text(), "original dependency")
        self.assertTrue((dest / ".git").is_file())
        self.gg("create", "without-copy", "--no-copy")
        skipped = self.tasks()[1]
        self.assertNotIn("copy_source", skipped)
        self.assertFalse((Path(skipped["path"]) / ".env").exists())

    def test_copy_source_follows_base_worktree_or_calling_worktree(self):
        (self.root / ".gitignore").write_text(".env\n")
        self.git("add", ".gitignore")
        self.commit("ignore env")
        self.git("branch", "-f", "release/demo", "main")
        release = Path(self.tmp.name) / "release worktree"
        self.git("worktree", "add", str(release), "release/demo")
        (self.root / ".env").write_text("main environment")
        (release / ".env").write_text("release environment")
        self.gg("create", "fix-release", "--base", "release/demo")
        task = self.tasks()[0]
        self.assertEqual(task["copy_source"], str(release))
        self.assertEqual((Path(task["path"]) / ".env").read_text(), "release environment")
        subdir = Path(task["path"]) / "subdir"
        subdir.mkdir()
        self.gg("create", "child", cwd=subdir)
        child = self.tasks()[1]
        self.assertEqual(child["copy_source"], task["path"])
        self.assertEqual((Path(child["path"]) / ".env").read_text(), "release environment")

    def test_create_without_worktree_list_z(self):
        if self.git("worktree", "list", "--porcelain", "-z", check=False).returncode == 0:
            git = shutil.which("git", path=self.env["PATH"])
            tools = Path(self.tmp.name) / "tools"
            tools.mkdir()
            shim = tools / "git"
            shim.write_text(f"""#!/bin/sh
case "$*" in
    --version) echo 'git version 2.31.0'; exit 0 ;;
    'worktree list --porcelain -z') echo 'error: unknown switch z' >&2; exit 129 ;;
esac
exec {shlex.quote(git)} "$@"
""")
            shim.chmod(0o755)
            self.env["PATH"] = str(tools) + os.pathsep + self.env["PATH"]

        (self.root / ".gitignore").write_text(".env\n")
        self.git("add", ".gitignore")
        self.commit("ignore env")
        self.git("branch", "-f", "release/demo", "main")
        self.git("branch", "unchecked", "main")
        release = Path(self.tmp.name) / 'release "é"\nworktree'
        self.git("worktree", "add", str(release), "release/demo")
        (self.root / ".env").write_text("main environment")
        (release / ".env").write_text("release environment")

        scenarios = [
            ("from-release", "release/demo", self.root, release, "release environment"),
            ("from-main", "main", release, self.root, "main environment"),
            ("from-unchecked", "unchecked", release, release, "release environment"),
        ]
        for name, base, cwd, source, contents in scenarios:
            with self.subTest(base=base):
                self.gg("create", name, "--base", base, cwd=cwd)
                task = next(t for t in self.tasks(cwd) if t["branch"] == name)
                self.assertEqual(task["copy_source"], str(source))
                self.assertEqual((Path(task["path"]) / ".env").read_text(), contents)
                self.assertTrue(task["active"])

    def test_copy_preserves_destination_files_and_does_not_follow_target_links(self):
        (self.root / ".gitignore").write_text(".env\nlocal/\nnode_modules/\n")
        self.git("add", ".gitignore")
        self.commit("ignore environment")
        self.git("branch", "-f", "release/demo", "main")
        release = Path(self.tmp.name) / "release"
        outside = Path(self.tmp.name) / "outside"
        outside.mkdir()
        self.git("worktree", "add", str(release), "release/demo")
        (release / ".env").write_text("tracked release env")
        (release / "local").symlink_to(outside, target_is_directory=True)
        self.git("add", "-f", ".env", "local", cwd=release)
        self.commit("tracked conflicts", cwd=release)
        commit = self.git("rev-parse", "HEAD", cwd=release).stdout.strip()
        (self.root / ".env").write_text("source env")
        (self.root / "local").mkdir()
        (self.root / "local/escape.txt").write_text("must not escape")
        (self.root / "node_modules").mkdir()
        (self.root / "node_modules/tracked.js").write_text("source branch code")
        self.git("add", "-f", "node_modules/tracked.js")
        self.commit("source-only tracked code")
        self.gg("create", "conflicts", "--base", commit)
        target = Path(self.tasks()[0]["path"])
        self.assertEqual((target / ".env").read_text(), "tracked release env")
        self.assertEqual(os.readlink(target / "local"), str(outside))
        self.assertFalse((outside / "escape.txt").exists())
        self.assertFalse((target / "node_modules/tracked.js").exists())

    def test_copy_failure_rolls_back_new_task(self):
        if os.geteuid() == 0:
            self.skipTest("root can read files without permission")
        (self.root / ".gitignore").write_text(".env\n")
        self.git("add", ".gitignore")
        self.commit("ignore env")
        blocked = self.root / ".env"
        blocked.write_text("unreadable")
        blocked.chmod(0)
        try:
            self.assertNotEqual(self.gg("create", "failed-copy", check=False).returncode, 0)
        finally:
            blocked.chmod(0o600)
        self.assertEqual(self.tasks(), [])
        self.assertFalse((Path(str(self.root) + "__sessions") / "failed-copy").exists())
        self.assertNotEqual(self.git("show-ref", "--verify", "refs/heads/failed-copy", check=False).returncode, 0)

    def test_explicit_origin_and_managed_task_attachment(self):
        self.assertEqual(self.all_tasks(), [])
        proc, master = self.attach_session()
        self.assertTrue(self.all_tasks()[0]["protected"])
        wait_for(lambda: self.active() == "0|")
        self.assertEqual(self.tmux("display-message", "-p", "-t", self.terminal(0), "#{pane_current_path}").stdout.strip(), str(self.root))
        self.assertEqual(self.tmux("display-message", "-p", "-t", self.terminal(0), "#{pane_current_command}").stdout.strip(), "sh")
        os.write(master, b"echo origin-shell-ok\r")
        wait_for(lambda: "origin-shell-ok" in self.capture(self.terminal(0)).splitlines())
        os.write(master, b"\x17")
        wait_for(lambda: "[?] Actions" in self.capture(self.sidebar(0)))
        self.assertNotIn("Delete", self.capture(self.sidebar(0)))
        self.assertNotEqual(self.delete(0, "--yes", "--force", check=False).returncode, 0)
        self.assertTrue((self.root / "README").exists())
        self.create()
        task = self.tasks()[0]
        nested, _ = self.attach_session(cwd=task["path"])
        wait_for(lambda: self.active() == "1|")
        self.assertEqual(self.tmux("display-message", "-p", "#{pane_current_path}").stdout.strip(), task["path"])
        self.assertIsNone(nested.poll())

    def test_existing_external_worktree_is_registered_without_ownership(self):
        release = Path(self.tmp.name) / "release"
        self.git("worktree", "add", str(release), "release/demo")
        self.assertEqual(self.all_tasks(cwd=release), [])
        proc, master = self.attach_session(cwd=release)
        origin = next(t for t in self.all_tasks() if t["path"] == str(release))
        self.assertTrue(origin["protected"])
        wait_for(lambda: self.active() == f'{origin["id"]}|')
        self.assertEqual(self.tmux("display-message", "-p", "#{pane_current_path}").stdout.strip(), str(release))
        self.assertNotEqual(self.gg("delete", origin["branch"], "--yes", "--force", check=False).returncode, 0)
        self.assertTrue((release / "README").exists())

    def test_navigation_does_not_leak_and_ctrl_p_reaches_terminal(self):
        proc, master = self.attach_session()
        ready = Path(self.tmp.name) / "raw-ready"
        result = Path(self.tmp.name) / "raw-result"
        script = ("import os,sys,termios,tty; from pathlib import Path; "
                  "fd=sys.stdin.fileno(); old=termios.tcgetattr(fd); tty.setraw(fd); "
                  f"Path({str(ready)!r}).write_text('ready'); "
                  f"data=os.read(fd,1); Path({str(result)!r}).write_text(data.hex()); "
                  "termios.tcsetattr(fd,termios.TCSANOW,old)")
        os.write(master, ("python3 -c " + shlex.quote(script) + "\r").encode())
        wait_for(ready.exists)
        os.write(master, b"\x17")
        wait_for(lambda: self.active() == "0|1")
        os.write(master, b"\x17")
        wait_for(lambda: self.active() == "0|")
        time.sleep(0.15)
        self.assertFalse(result.exists(), "Navigation leaked into the raw application")
        os.write(master, b"\x10")
        wait_for(result.exists)
        self.assertEqual(result.read_text(), "10")

    def test_removed_navigation_keys_reach_application(self):
        proc, master = self.attach_session()
        ready = Path(self.tmp.name) / "prefix-ready"
        result = Path(self.tmp.name) / "prefix-result"
        script = ("import os,sys,termios,tty; from pathlib import Path; "
                  "fd=sys.stdin.fileno(); old=termios.tcgetattr(fd); tty.setraw(fd); "
                  f"Path({str(ready)!r}).write_text('ready'); "
                  "data=b''.join(os.read(fd,1) for _ in range(3)); "
                  f"Path({str(result)!r}).write_text(data.hex()); "
                  "termios.tcsetattr(fd,termios.TCSANOW,old)")
        os.write(master, ("python3 -c " + shlex.quote(script) + "\r").encode())
        wait_for(ready.exists)
        os.write(master, b"\x07x\t")
        wait_for(result.exists)
        self.assertEqual(result.read_text(), "077809")

    def test_legacy_registry_and_welcome_upgrade_preserve_running_terminals(self):
        self.create()
        task_path = self.tasks()[0]["path"]
        self.gg("stop", "task")
        legacy_path = Path(str(self.root) + "__sessions") / "1"
        self.git("worktree", "move", task_path, str(legacy_path))
        state_file = self.root / ".git/grand-git/state.json"
        state = json.loads(state_file.read_text())
        task = next(t for t in state["tasks"] if t["id"] == 1)
        task["path"] = str(legacy_path)
        task["description"] = "old description"
        task.pop("template", None)
        task.pop("launch_token", None)
        state_file.write_text(json.dumps(state))
        self.gg("start", "task")
        self.attach_session()
        marker = Path(self.tmp.name) / "migration-heartbeat"
        terminal = self.terminal(1)
        worker_pid = self.tmux("display-message", "-p", "-t", terminal, "#{pane_pid}").stdout.strip()
        self.tmux("send-keys", "-t", terminal,
                  f"while true; do echo tick >> {shlex.quote(str(marker))}; sleep 0.1; done", "Enter")
        wait_for(lambda: marker.exists() and len(marker.read_text().splitlines()) > 2)
        root_terminal = self.terminal(0)
        self.tmux("respawn-pane", "-k", "-t", root_terminal,
                  shlex.quote(str(BIN)) + " '_welcome'")
        sidebar = self.sidebar(1)
        before_pid = self.tmux("display-message", "-p", "-t", sidebar, "#{pane_pid}").stdout.strip()
        self.tmux("set-option", "-p", "-u", "-t", sidebar, "@gg_ui_revision")
        state_file = self.root / ".git/grand-git/state.json"
        state = json.loads(state_file.read_text())
        state["tasks"] = [t for t in state["tasks"] if t["id"] != 0]
        state_file.write_text(json.dumps(state))
        proc, master = self.attach_session()
        wait_for(lambda: self.active() == "0|")
        wait_for(lambda: self.tmux("display-message", "-p", "-t", root_terminal,
                                  "#{pane_current_command}").stdout.strip() == "sh")
        after_pid = self.tmux("display-message", "-p", "-t", sidebar, "#{pane_pid}").stdout.strip()
        self.assertNotEqual(before_pid, after_pid)
        self.assertEqual(worker_pid, self.tmux("display-message", "-p", "-t", terminal, "#{pane_pid}").stdout.strip())
        count = len(marker.read_text().splitlines())
        wait_for(lambda: len(marker.read_text().splitlines()) > count + 2)
        self.assertTrue(self.all_tasks()[0]["protected"])
        self.assertEqual(self.tasks()[0]["id"], 1)
        self.assertEqual(self.tasks()[0]["path"], str(legacy_path))
        self.assertNotIn("description", self.public_tasks()[-1])
        self.assertEqual(self.tasks()[0]["template"], "shell")


if __name__ == "__main__":
    unittest.main(verbosity=2)
