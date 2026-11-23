"""A dashboard invocation keeps its Go process while changing projects."""

import os
import sys
from pathlib import Path
import json
import shlex
import time

from test_integration import BIN, SessionFixture, wait_for


def executable_path(pid):
    if sys.platform == "darwin":
        import ctypes

        libproc = ctypes.CDLL("/usr/lib/libproc.dylib", use_errno=True)
        path = ctypes.create_string_buffer(4096)  # PROC_PIDPATHINFO_MAXSIZE
        if libproc.proc_pidpath(pid, path, len(path)) <= 0:
            raise OSError(ctypes.get_errno(), "proc_pidpath")
        return Path(os.fsdecode(path.value)).resolve()
    return Path(f"/proc/{pid}/exe").resolve()


class ContinuousDashboardTests(SessionFixture):
    def setUp(self):
        super().setUp()
        self.env.pop("GG_TEST_LEGACY_DASHBOARD", None)

    def launch_terminal(self):
        proc, master = self.start_client([str(BIN), "dashboard"])
        self.add_terminal(proc, master)
        return proc, master

    def add_terminal(self, proc, master, cwd=None):
        wait_for(lambda: b"Add section" in self.outputs[proc.pid])
        opened_at = len(self.outputs[proc.pid])
        os.write(master, b"+")
        wait_for(lambda: b"Ctrl+S" in self.outputs[proc.pid][opened_at:])
        if cwd is not None:
            os.write(master, b"\x15" + os.path.relpath(cwd, self.root).encode())
        submitted_at = len(self.outputs[proc.pid])
        os.write(master, b"\x13")
        target = str(cwd or self.root)
        catalog = Path(self.env["XDG_STATE_HOME"]) / "grand-git/workspaces.json"
        wait_for(lambda: catalog.exists() and any(s["path"] == target for s in json.loads(catalog.read_text()).get("sections", [])))
        selected = ("› " + Path(target).name).encode()
        wait_for(lambda: selected in self.outputs[proc.pid][submitted_at:])
        os.write(master, b"t")
        wait_for(lambda: any(row["kind"] == "terminal" and row["path"] == target for row in self.public_tasks()))
        time.sleep(.15)

    def local_terminal(self):
        return next(row for row in self.public_tasks() if row["kind"] == "terminal" and row["path"] == str(self.root))

    def test_tmux_status_and_pane_title_are_visible(self):
        self.env.pop("NO_COLOR", None)
        proc, master = self.launch_terminal()
        wait_for(lambda: bool(self.local_terminal()))
        wait_for(lambda: b"Grand Git" in self.outputs[proc.pid])
        wait_for(lambda: "repo with spaces".encode() in self.outputs[proc.pid])
        self.assertIn(b"Ctrl+W Next", self.outputs[proc.pid])

        task = int(self.local_terminal()["id"].split(":")[1])
        self.resize_client(proc, master, 80, 24)
        before = len(self.outputs[proc.pid])
        wait_for(lambda: b"Grand Git" in self.outputs[proc.pid][before:])
        wait_for(lambda: any(pane[6:8] == ["80", "22"] for pane in self.geometry(task) if pane[2] != "1"))
        os.write(master, b"\x17q")
        wait_for(lambda: proc.poll() is not None)

    def test_cross_project_switch_keeps_dashboard_pid_and_terminal_processes(self):
        other = Path(self.tmp.name) / "aaa-other"
        other.mkdir()
        self.gg("create", "foreign", cwd=other)
        foreign = next(row for row in self.public_tasks() if row["name"] == "foreign")
        foreign_pid = self.run_cmd("tmux", "-L", self.socket_for(foreign), "list-panes", "-a", "-F", "#{pane_pid}").stdout.splitlines()

        proc, master = self.launch_terminal()
        wait_for(lambda: bool(self.local_terminal()))
        self.assertEqual(executable_path(proc.pid), BIN.resolve())

        switched_at = len(self.outputs[proc.pid])
        os.write(master, b"\x17kk\r")
        wait_for(lambda: b"foreign" in self.outputs[proc.pid][switched_at:])
        time.sleep(.2)
        before = len(self.outputs[proc.pid])
        os.write(master, b"printf 'GG_%s\\n' CONTINUOUS_MARKER\r")
        wait_for(lambda: b"GG_CONTINUOUS_MARKER" in self.outputs[proc.pid][before:])
        self.assertIsNone(proc.poll())

        os.write(master, b"\x17jj\r")
        time.sleep(.3)
        self.assertIsNone(proc.poll())
        self.assertEqual(executable_path(proc.pid), BIN.resolve())
        self.assertEqual(foreign_pid, self.run_cmd("tmux", "-L", self.socket_for(foreign), "list-panes", "-a", "-F", "#{pane_pid}").stdout.splitlines())

        os.write(master, b"\x17q")
        wait_for(lambda: proc.poll() is not None)
        self.assertTrue(self.local_terminal()["active"])
        self.assertTrue(next(row for row in self.public_tasks() if row["id"] == foreign["id"])["active"])

    def socket_for(self, row):
        import hashlib
        group = row["workspace"]
        key = group["path"] if group["type"] == "directory" else str(Path(group["path"]) / ".git")
        return ("gg-dir-" if group["type"] == "directory" else "gg-") + hashlib.sha256(key.encode()).hexdigest()[:16]

    def test_compact_list_escape_and_quit(self):
        proc, master = self.launch_terminal()
        wait_for(lambda: bool(self.local_terminal()))
        self.resize_client(proc, master, 80, 24)
        os.write(master, b"\x17")
        wait_for(lambda: b"[Q] Quit" in self.outputs[proc.pid])
        os.write(master, b"\x1b")
        self.assertIsNone(proc.poll())
        os.write(master, b"\x17q")
        wait_for(lambda: proc.poll() is not None)

    def test_outer_tmux_detach_preserves_its_terminal(self):
        outer = self.socket + "-outer"
        self.run_cmd("tmux", "-L", outer, "new-session", "-d", "-s", "outer", "-c", str(self.root))
        proc, master = self.start_client(["tmux", "-L", outer, "attach-session", "-t", "outer"])
        wait_for(lambda: bool(self.run_cmd("tmux", "-L", outer, "list-clients", "-F", "#{client_name}").stdout.strip()))
        self.run_cmd("tmux", "-L", outer, "send-keys", "-t", "outer:0", str(BIN) + " dashboard", "Enter")
        self.add_terminal(proc, master)
        client = self.run_cmd("tmux", "-L", outer, "list-clients", "-F", "#{client_name}").stdout.strip()
        self.run_cmd("tmux", "-L", outer, "detach-client", "-t", client)
        self.assertTrue(self.local_terminal()["active"])
        self.assertIsNotNone(proc.poll())

    def test_terminal_disconnect_preserves_persistent_shell(self):
        proc, master = self.launch_terminal()
        wait_for(lambda: bool(self.local_terminal()))
        client = next(item for item in self.clients if item[0] is proc)
        self.clients.remove(client)
        os.close(master)
        client[2].set()
        client[3].join(timeout=1)
        proc.wait(timeout=3)
        self.assertTrue(self.local_terminal()["active"])

    def test_two_dashboards_launch_without_sessions_and_keep_list_accessible(self):
        first, first_master = self.start_client([str(BIN), "dashboard"])
        second, second_master = self.start_client([str(BIN), "dashboard"])
        for proc in (first, second):
            wait_for(lambda: b"Add section" in self.outputs[proc.pid])
        self.assertEqual(self.public_tasks(), [])
        catalog = Path(self.env["XDG_STATE_HOME"]) / "grand-git/workspaces.json"
        self.assertEqual(json.loads(catalog.read_text())["sections"], [])
        self.assertNotEqual(self.tmux("has-session", "-t", "=dashboard", check=False).returncode, 0)
        self.resize_client(first, first_master, 80, 24)
        os.write(first_master, b"\x1b\x17q")
        wait_for(lambda: first.poll() is not None)
        self.assertIsNone(second.poll())
        os.write(second_master, b"q")
        wait_for(lambda: second.poll() is not None)
        self.assertEqual(self.public_tasks(), [])

    def test_nested_sections_keep_cwd_and_bulk_deletion_inside_section(self):
        child = self.root / "src"
        child.mkdir()
        proc, master = self.launch_terminal()
        root_row = self.local_terminal()
        root_pane = self.terminal(int(root_row["id"].split(":")[1]))
        root_pid = self.tmux("display-message", "-p", "-t", root_pane, "#{pane_pid}").stdout
        self.gg("create", "feat/nested", "--no-copy", cwd=child)
        catalog_path = Path(self.env["XDG_STATE_HOME"]) / "grand-git/workspaces.json"
        catalog = json.loads(catalog_path.read_text())
        src = next(section for section in catalog["sections"] if section["path"] == str(child))
        self.assertEqual(src["workspace"], root_row["workspace"]["id"])
        os.write(master, b"\x17")
        self.add_terminal(proc, master, cwd=child)
        row = next(row for row in self.public_tasks() if row["kind"] == "terminal" and row["path"] == str(child))
        pane = self.terminal(int(row["id"].split(":")[1]))
        self.assertEqual(self.tmux("display-message", "-p", "-t", pane, "#{pane_current_path}").stdout.strip(), str(child))
        state = json.loads((self.root / ".git/grand-git/state.json").read_text())
        self.assertEqual([task["section_id"] for task in state["tasks"] if task["id"] != int(root_row["id"].split(":")[1])], [src["id"], src["id"]])
        before = len(self.outputs[proc.pid])
        os.write(master, b"\x17kkd")
        wait_for(lambda: b"Delete all sessions" in self.outputs[proc.pid][before:])
        os.write(master, b"y")
        wait_for(lambda: [row["id"] for row in self.public_tasks()] == [root_row["id"]])
        self.assertEqual(self.tmux("display-message", "-p", "-t", root_pane, "#{pane_pid}").stdout, root_pid)
        self.assertTrue(child.is_dir())
        self.assertIn("feat/nested", self.git("branch", "--list").stdout)
        time.sleep(.5)
        os.write(master, b"x")
        wait_for(lambda: not any(section["id"] == src["id"] for section in json.loads(catalog_path.read_text())["sections"]))
        os.write(master, b"q")
        wait_for(lambda: proc.poll() is not None)
        self.assertTrue(self.local_terminal()["active"])

    def test_terminal_mouse_input_reaches_selected_pane(self):
        proc, master = self.launch_terminal()
        wait_for(lambda: bool(self.local_terminal()))
        row = self.local_terminal()
        pane = self.terminal(int(row["id"].split(":")[1]))
        marker = Path(self.tmp.name) / "mouse.bin"
        command = "stty raw -echo; printf '\\033[?1000h\\033[?1006h'; dd bs=1 count=9 of=" + shlex.quote(str(marker)) + " 2>/dev/null; printf '\\033[?1000l\\033[?1006l'; stty sane"
        self.tmux("send-keys", "-t", pane, command, "Enter")
        wait_for(lambda: self.tmux("display-message", "-p", "-t", pane, "#{mouse_sgr_flag}").stdout.strip() == "1")
        os.write(master, b"\x1b[<0;36;6M")
        wait_for(lambda: marker.exists() and marker.stat().st_size == 9)
        self.assertEqual(marker.read_bytes(), b"\x1b[<0;5;5M")
        wait_for(lambda: self.tmux("display-message", "-p", "-t", pane, "#{mouse_sgr_flag}").stdout.strip() == "0")
        os.write(master, b"\x1b[<0;2;4M\x1b[<0;36;5M")
        os.write(master, b"echo GG_MOUSE_FOCUS\r")
        wait_for(lambda: "GG_MOUSE_FOCUS" in self.tmux("capture-pane", "-p", "-t", pane).stdout)

    def test_shift_tab_reaches_terminal_as_backtab(self):
        proc, master = self.launch_terminal()
        wait_for(lambda: bool(self.local_terminal()))
        task = int(self.local_terminal()["id"].split(":")[1])
        pane = self.terminal(task)
        ready = Path(self.tmp.name) / "backtab-ready"
        marker = Path(self.tmp.name) / "backtab.bin"
        command = (
            "stty raw -echo; : > " + shlex.quote(str(ready)) +
            "; dd bs=1 count=3 of=" + shlex.quote(str(marker)) + " 2>/dev/null; stty sane"
        )
        self.tmux("send-keys", "-t", pane, command, "Enter")
        wait_for(lambda: ready.exists())
        os.write(master, b"\x1b[Z")
        wait_for(lambda: marker.exists() and marker.stat().st_size == 3)
        self.assertEqual(marker.read_bytes(), b"\x1b[Z")
        self.assertIsNone(proc.poll())

    def test_split_focuses_new_terminal_and_preserves_first(self):
        proc, master = self.launch_terminal()
        wait_for(lambda: bool(self.local_terminal()))
        task = int(self.local_terminal()["id"].split(":")[1])
        first = self.terminal(task)

        def pane_pid(pane):
            rows = self.tmux("list-panes", "-a", "-F", "#{pane_pid} #{@gg_task} #{pane_id}").stdout.splitlines()
            return next(row.split()[0] for row in rows if row.split()[1:] == [str(task), pane])

        first_pid = pane_pid(first)
        before = len(self.outputs[proc.pid])
        os.write(master, b"\x17t")
        wait_for(lambda: b"Below" in self.outputs[proc.pid][before:])
        os.write(master, b"r")
        wait_for(lambda: len([pane for pane in self.panes(task) if pane[2] != "1"]) == 2)
        second = next(pane[0] for pane in self.panes(task) if pane[2] != "1" and pane[0] != first)
        os.write(master, b"echo GG_SPLIT_SELECTED\r")
        wait_for(lambda: "GG_SPLIT_SELECTED" in self.tmux("capture-pane", "-p", "-t", second).stdout)
        self.assertEqual(first_pid, pane_pid(first))
        os.write(master, b"\x17\x17echo GG_FIRST_SELECTED\r")
        wait_for(lambda: "GG_FIRST_SELECTED" in self.tmux("capture-pane", "-p", "-t", first).stdout)
        os.write(master, b"\x17\x17q")
        wait_for(lambda: proc.poll() is not None)

    def test_failed_foreign_open_keeps_dashboard_available(self):
        other = Path(self.tmp.name) / "aaa-other"
        other.mkdir()
        self.gg("create", "foreign", cwd=other)
        foreign = next(row for row in self.public_tasks() if row["name"] == "foreign")
        self.gg("stop", foreign["id"])
        state_path = Path(self.env["XDG_STATE_HOME"]) / "grand-git/directories" / str(foreign["workspace"]["id"]) / "state.json"
        state = json.loads(state_path.read_text())
        state["tasks"][0]["template"] = "missing-template"
        state_path.write_text(json.dumps(state))

        proc, master = self.launch_terminal()
        wait_for(lambda: bool(self.local_terminal()))
        before = len(self.outputs[proc.pid])
        os.write(master, b"\x17kk\r")
        wait_for(lambda: b"missing-template" in self.outputs[proc.pid][before:])
        self.assertIsNone(proc.poll())
        self.assertFalse(next(row for row in self.public_tasks() if row["id"] == foreign["id"])["active"])
        os.write(master, b"q")
        wait_for(lambda: proc.poll() is not None)
