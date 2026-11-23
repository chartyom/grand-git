"""Compact UI and source retirement against isolated Git/tmux/PTY fixtures."""
import os
import re
import time
from pathlib import Path

from test_integration import SessionFixture, wait_for


class CompactDashboardTests(SessionFixture):
    def launcher(self):
        return next(t for t in self.public_tasks() if t["kind"] == "temporary")

    def test_ctrl_w_opens_and_closes_compact_list(self):
        proc, master = self.attach()
        task_id = int(self.launcher()["id"].split(":")[1])
        terminal = self.terminal(task_id)
        self.resize_client(proc, master, 80)
        wait_for(lambda: any(p[2] == "1" and p[3] == "_gg-park" for p in self.geometry(task_id)))
        offset = len(self.outputs[proc.pid])
        os.write(master, b"\x17")
        wait_for(lambda: b"[?] Actions" in self.outputs[proc.pid][offset:])
        os.write(master, b"\x17")
        time.sleep(.2)
        os.write(master, b"echo compact-cycle-ok\r")
        wait_for(lambda: "compact-cycle-ok" in self.capture(terminal).splitlines())

    def test_normal_commands_never_register_a_source(self):
        self.assertEqual(self.public_tasks(), [])
        result = self.gg("start", "main", check=False)
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("gg dashboard", result.stderr)
        for command in [("stop", "--all", "--yes"), ("delete", "--all", "--yes"), ("init",)]:
            self.gg(*command)
            self.assertEqual(self.public_tasks(), [])
        self.create("work")
        self.assertEqual([t["kind"] for t in self.public_tasks()], ["worktree"])
        proc, master = self.attach()
        rows = self.public_tasks()
        self.assertEqual([t["kind"] for t in rows], ["worktree", "temporary"])
        self.assertEqual(self.git("branch", "--show-current").stdout.strip(), "main")

    def test_external_worktree_and_hooks_do_not_create_source_records(self):
        external = Path(self.tmp.name) / "external"
        self.git("worktree", "add", str(external), "release/demo")
        self.assertEqual(self.gg("list", "--json", cwd=external).stdout.strip(), "[]")
        self.gg("init", cwd=external)
        self.assertTrue((external / ".codex/hooks.json").is_file())
        self.assertEqual(self.public_tasks(), [])
        child = external / "child"
        child.mkdir()
        proc, master = self.attach(cwd=child)
        row = self.launcher()
        self.assertEqual(row["path"], str(child))
        self.assertEqual(len(self.public_tasks()), 1)
        self.gg("delete", row["id"], "--yes", "--force")
        self.assertTrue((external / "README").exists())
        self.assertTrue(child.is_dir())
        self.git("show-ref", "--verify", "refs/heads/release/demo")

    def test_running_legacy_source_retires_only_after_stopping(self):
        proc, master = self.attach_session()
        self.create("work")
        pid = self.tmux("display-message", "-p", "-t", self.terminal(0), "#{pane_pid}").stdout
        for _ in range(3):
            source = next(t for t in self.public_tasks() if t["kind"] == "source")
            self.assertEqual(source["id"], "1:0")
            self.assertTrue(source["active"])
        self.assertEqual(pid, self.tmux("display-message", "-p", "-t", self.terminal(0), "#{pane_pid}").stdout)
        self.gg("stop", "main")
        wait_for(lambda: not any(t["kind"] == "source" for t in self.public_tasks()))
        self.assertTrue((self.root / "README").is_file())
        self.git("show-ref", "--verify", "refs/heads/main")
        self.assertTrue(self.tasks()[0]["active"])
        self.assertEqual(self.tasks()[0]["id"], 1)
        self.assertNotEqual(self.gg("start", "main", check=False).returncode, 0)

    def test_ten_eleven_and_active_filter_search_threshold(self):
        for i in range(8):
            self.create_stopped_worktree(f"task-{i}")
        self.create("task-8")
        proc, master = self.attach()
        launcher_id = int(self.launcher()["id"].split(":")[1])
        side = self.sidebar(launcher_id)
        os.write(master, b"\x17")
        wait_for(lambda: self.active().endswith("|1"))
        self.assertNotIn("[/]", self.capture(side))
        os.write(master, b"/")
        os.write(master, b"?")
        wait_for(lambda: "[S]" in self.capture(side))  # slash did not enter text editing
        self.assertNotIn("[/]", self.capture(side))
        os.write(master, b"\x1b")
        self.create("last")
        wait_for(lambda: "[/]" in self.capture(side))
        os.write(master, b"/missing\r")
        wait_for(lambda: "No matching sessions" in self.capture(side))
        os.write(master, b"\x1b")
        wait_for(lambda: "[/]" in self.capture(side))
        os.write(master, b"/last\r")
        wait_for(lambda: "/ last" in self.capture(side) or "/last" in self.capture(side))
        self.gg("stop", "task-8")
        os.write(master, b"a")
        wait_for(lambda: any(line.strip() == "Active" for line in self.capture(side).splitlines())
                 and "[/]" not in self.capture(side))
        self.assertFalse(any(line.strip() in ("/last", "/ last") for line in self.capture(side).splitlines()))
        self.assertIn("dashboard", self.capture(side))
        self.assertEqual(self.active(), f"{launcher_id}|1")

    def test_focus_and_actions_are_readable_without_color(self):
        self.env["NO_COLOR"] = "1"
        proc, master = self.attach()
        task_id = int(self.launcher()["id"].split(":")[1])
        side = self.sidebar(task_id)
        wait_for(lambda: "▸ " in self.capture(side))
        os.write(master, b"\x17")
        wait_for(lambda: self.active() == f"{task_id}|1")
        wait_for(lambda: re.search(r"Open: +yes", self.capture(side)))
        self.assertIn("[?] Actions", self.capture(side))
        self.assertNotIn("GRAND GIT", self.capture(side))
        self.assertRegex(self.capture(side), r"Terminals: +1")
        self.assertRegex(self.capture(side), r"Open: +yes")
        screen = self.tmux("capture-pane", "-p", "-e", "-t", side).stdout
        for attributes in re.findall(r"\x1b\[([0-9;]*)m", screen):
            self.assertFalse(any(30 <= int(code) <= 49 or 90 <= int(code) <= 107 for code in attributes.split(";") if code), attributes)
        self.assertNotIn("[S]", screen)
        os.write(master, b"?")
        wait_for(lambda: "[S]" in self.capture(side))
        os.write(master, b"\x1b")
        wait_for(lambda: "[?]" in self.capture(side))
        self.assertEqual(self.active(), f"{task_id}|1")
