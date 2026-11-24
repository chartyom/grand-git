"""Background worktree preparation through the real dashboard, Git and tmux."""
from contextlib import contextmanager
import os
from pathlib import Path
import shlex
import shutil
import signal

from test_design_dashboard import DashboardScreenFixture
from test_integration import BIN, wait_for


class WorktreePreparationTests(DashboardScreenFixture):
    @contextmanager
    def gated_copy(self):
        control = Path(self.tmp.name) / "copy-control"
        control.mkdir()
        git = shlex.quote(shutil.which("git"))
        shim = control / "git"
        # The second ignored-file listing runs after regular files have been copied.
        shim.write_text(f"""#!/bin/sh
control={shlex.quote(str(control))}
if [ "$1" = ls-files ] && [ "$3" = --ignored ] && [ "$5" = --directory ]; then
    echo "$PPID" >> "$control/entered"
    while [ ! -e "$control/release" ] && [ ! -e "$control/release-$PPID" ]; do
        [ -d "$control" ] || exit 1
        sleep .02
    done
    if [ -e "$control/fail" ]; then
        echo copy-fixture-error >&2
        exit 1
    fi
fi
exec {git} "$@"
""")
        shim.chmod(0o755)
        self.env["PATH"] = str(control) + os.pathsep + self.env["PATH"]
        try:
            yield control
        finally:
            (control / "release").touch()

    def submit_worktree(self, name, branch=""):
        self.folder()
        self.press(b"n")
        wait_for(lambda: "Worktree name" in self.screen())
        self.press(name.encode() + (b"\x1b[B" + branch.encode() if branch else b"") + b"\r")

    def worktree_status_row(self, name, status):
        sidebar = (line[:40] for line in self.screen().splitlines())
        return next((line for line in sidebar if name in line and status in line), "")

    def crash_copy_worker(self, control):
        worker = int((control / "entered").read_text().strip())
        command = self.run_cmd("ps", "-p", str(worker), "-o", "command=").stdout
        self.assertIn("_prepare-worktree", command.split())
        os.kill(worker, signal.SIGKILL)
        (control / "release").touch()

    def test_worktree_copy_animates_while_sessions_and_sections_remain_usable(self):
        self.create_stopped_worktree("reader")
        other = Path(self.tmp.name) / "notes"
        other.mkdir()
        self.gg("create", "notes-shell", cwd=other)
        dep = Path("node_modules/pkg/index.js")
        (self.root / dep).parent.mkdir(parents=True)
        (self.root / dep).write_text("local dependency\n")
        (self.root / ".gitignore").write_text("node_modules/\n")
        template = self.root / ".gg/templates/copy-check.yaml"
        template.parent.mkdir(parents=True, exist_ok=True)
        template.write_text("commands:\n  - cat node_modules/pkg/index.js > startup-copy\n")
        self.gg("set", "default_template", "copy-check")
        with self.gated_copy() as control:
            self.launch()
            self.submit_worktree("deps")
            wait_for((control / "entered").exists)
            frame = wait_for(lambda: self.worktree_status_row("deps", "Copying"))
            wait_for(lambda: (row := self.worktree_status_row("deps", "Copying")) and row != frame)
            self.assertNotIn("Create session", self.screen())
            self.assertNotIn("deps", [row["name"] for row in self.public_tasks()])
            target = Path(str(self.root) + "__sessions") / "deps"
            self.assertEqual((target / dep).read_text(), "local dependency\n")
            self.assertFalse((target / "startup-copy").exists())

            self.click(other.name, sidebar=True)
            wait_for(lambda: "[T] Terminal" in self.screen())
            self.press(b"t")
            wait_for(lambda: len([row for row in self.public_tasks() if row["path"] == str(other)]) == 2)
            self.click("reader", sidebar=True)
            wait_for(lambda: self.worktree_status_row("reader", "▸ ●"))
            self.press(b"printf one > while-copying\r")
            reader = next(row for row in self.tasks() if row["name"] == "reader")
            received = Path(reader["path"]) / "while-copying"
            wait_for(received.exists)
            pane = self.terminal(reader["id"])
            pid = self.tmux("display-message", "-p", "-t", pane, "#{pane_pid}").stdout
            (control / "release").touch()
            wait_for(lambda: self.worktree_status_row("deps", "●"))
            wait_for(lambda: (target / "startup-copy").exists() and (target / "startup-copy").stat().st_size)
            self.assertEqual((target / "startup-copy").read_text(), "local dependency\n")
            completed = [row for row in self.tasks() if row["name"] == "deps"]
            self.assertEqual(len(completed), 1)
            self.assertEqual((completed[0]["copy_source"], completed[0]["copied_files"]), (str(self.root), 1))
            self.assertTrue(self.worktree_status_row("reader", "▸ ●"))
            self.press(b"printf two >> while-copying\r")
            wait_for(lambda: received.read_text() == "onetwo")
            self.assertEqual(self.tmux("display-message", "-p", "-t", pane, "#{pane_pid}").stdout, pid)

    def test_copy_survives_quit_and_reopened_dashboard_keeps_list_focus(self):
        with self.gated_copy() as control:
            self.launch()
            self.submit_worktree("portable")
            wait_for((control / "entered").exists)
            wait_for(lambda: self.worktree_status_row("portable", "Copying"))
            self.press(b"q")
            wait_for(lambda: "Ctrl+W" not in self.screen())
            self.outer_tmux("send-keys", "-t", "preview:0", shlex.quote(str(BIN)) + " dashboard", "Enter")
            wait_for(lambda: self.worktree_status_row("portable", "Copying"))
            self.click("portable", sidebar=True)
            self.press(b"\r")
            self.assertIn("No terminal open", self.screen())
            self.assertNotIn("portable", [row["name"] for row in self.public_tasks()])
            (control / "release").touch()
            wait_for(lambda: self.worktree_status_row("portable", "●"))
            self.assertIn("No terminal open", self.screen())
            self.assertTrue(next(row for row in self.public_tasks() if row["name"] == "portable")["active"])

    def test_copy_finishes_after_all_dashboards_disconnect(self):
        with self.gated_copy() as control:
            self.launch()
            self.submit_worktree("unattended")
            wait_for((control / "entered").exists)
            observer, master = self.start_client([str(BIN), "dashboard"])
            wait_for(lambda: b"Copying" in self.outputs[observer.pid] and b"unattended" in self.outputs[observer.pid])
            os.write(master, b"q")
            wait_for(lambda: observer.poll() is not None)
            client = self.outer_tmux("list-clients", "-F", "#{client_name}").stdout.strip()
            self.outer_tmux("detach-client", "-t", client)
            wait_for(lambda: self.proc.poll() is not None)
            wait_for(lambda: "Ctrl+W" not in self.screen())
            (control / "release").touch()
            wait_for(lambda: any(row["name"] == "unattended" and row["active"] for row in self.tasks()))

    def test_copy_failure_rolls_back_and_enter_retries_with_original_inputs(self):
        with self.gated_copy() as control:
            (control / "fail").touch()
            self.launch()
            self.submit_worktree("retry-me", "topic/deps")
            wait_for((control / "entered").exists)
            wait_for(lambda: self.worktree_status_row("retry-me", "Copying"))
            (control / "release").touch()
            wait_for(lambda: self.worktree_status_row("retry-me", "Failed"))
            self.assertNotIn("retry-me", [row["name"] for row in self.public_tasks()])
            self.assertFalse((Path(str(self.root) + "__sessions") / "retry-me").exists())
            self.assertEqual(self.git("branch", "--list", "topic/deps").stdout, "")
            self.press(b"\r")
            wait_for(lambda: "Worktree name" in self.screen() and "copy-fixture-error" in self.screen())
            self.assertIn("topic/deps", self.screen())
            (control / "fail").unlink()
            self.press(b"\r")
            wait_for(lambda: self.worktree_status_row("retry-me", "▸ ●"))
            tasks = self.tasks()
            self.assertEqual([(t["name"], t["branch"], t["template"]) for t in tasks],
                             [("retry-me", "topic/deps", "shell")])

    def test_copy_failure_preserves_external_uncommitted_work(self):
        with self.gated_copy() as control:
            (control / "fail").touch()
            self.launch()
            self.submit_worktree("external-edit", "topic/edited")
            wait_for((control / "entered").exists)
            target = Path(str(self.root) + "__sessions") / "external-edit"
            (target / "README").write_text("tracked work\n")
            (target / "new-file").write_text("untracked work\n")
            (control / "release").touch()
            wait_for(lambda: self.worktree_status_row("external-edit", "Failed"))
            self.assertEqual(self.tasks(), [])
            self.assertEqual((target / "README").read_text(), "tracked work\n")
            self.assertEqual((target / "new-file").read_text(), "untracked work\n")
            self.assertEqual(self.git("branch", "--show-current", cwd=target).stdout.strip(), "topic/edited")
            self.assertEqual((self.root / "README").read_text(), "main\n")

    def test_managed_copy_source_cannot_be_deleted_until_child_is_ready(self):
        (self.root / ".gitignore").write_text(".env\n")
        self.git("add", ".gitignore")
        self.commit("ignore local environment")
        self.create("base-copy")
        source = Path(self.tasks()[0]["path"])
        (source / ".env").write_text("only in the selected worktree\n")
        with self.gated_copy() as control:
            self.launch()
            self.click("base-copy", sidebar=True)
            wait_for(lambda: self.worktree_status_row("base-copy", "▸ ●"))
            self.press(b"\x17n")
            wait_for(lambda: "Worktree name" in self.screen())
            self.press(b"child\r")
            wait_for((control / "entered").exists)
            before = {row["id"]: row["active"] for row in self.public_tasks()}
            for target in ("base-copy", "--all"):
                result = self.gg("delete", target, "--yes", "--force", check=False, timeout=5)
                self.assertNotEqual(result.returncode, 0)
                self.assertIn("copy source", result.stderr.lower())
            self.assertEqual({row["id"]: row["active"] for row in self.public_tasks()}, before)
            self.assertTrue(source.is_dir())
            (control / "release").touch()
            wait_for(lambda: self.worktree_status_row("child", "▸ ●"))
            self.gg("delete", "base-copy", "--yes", "--force")
            self.assertFalse(source.exists())
            child, = self.tasks()
            self.assertEqual((child["name"], child["copy_source"]), ("child", str(source)))
            self.assertEqual((Path(child["path"]) / ".env").read_text(), "only in the selected worktree\n")

    def test_overlapping_worktree_copies_can_finish_in_reverse_order(self):
        with self.gated_copy() as control:
            self.launch()
            self.submit_worktree("first", "topic/first")
            entered = control / "entered"
            wait_for(entered.exists)
            wait_for(lambda: self.worktree_status_row("first", "Copying"))
            self.submit_worktree("second", "topic/second")
            wait_for(lambda: len(entered.read_text().splitlines()) == 2)
            wait_for(lambda: self.worktree_status_row("second", "Copying"))
            self.assertTrue(self.worktree_status_row("first", "Copying"))
            self.assertEqual(self.tasks(), [])
            duplicate = self.gg("create", "first", "--no-copy", check=False, timeout=5)
            self.assertNotEqual(duplicate.returncode, 0)

            second_worker = entered.read_text().splitlines()[1]
            (control / ("release-" + second_worker)).touch()
            wait_for(lambda: self.worktree_status_row("second", "▸ ●"))
            self.assertTrue(self.worktree_status_row("first", "Copying"))
            self.assertEqual([row["name"] for row in self.tasks()], ["second"])
            (control / "release").touch()
            wait_for(lambda: self.worktree_status_row("first", "●"))
            self.assertTrue(self.worktree_status_row("second", "▸ ●"))
            rows = self.tasks()
            self.assertCountEqual([(row["name"], row["branch"]) for row in rows],
                                  [("first", "topic/first"), ("second", "topic/second")])
            self.assertEqual(len({row["id"] for row in rows}), 2)

    def test_interrupted_copy_releases_its_name_and_branch_for_retry(self):
        with self.gated_copy() as control:
            self.launch()
            self.submit_worktree("interrupted", "topic/retry")
            wait_for((control / "entered").exists)
            wait_for(lambda: self.worktree_status_row("interrupted", "Copying"))
            self.crash_copy_worker(control)
            wait_for(lambda: self.worktree_status_row("interrupted", "Failed"))
            self.assertEqual(self.tasks(), [])
            self.assertFalse((Path(str(self.root) + "__sessions") / "interrupted").exists())
            self.assertEqual(self.git("branch", "--list", "topic/retry").stdout, "")
            self.press(b"\r")
            wait_for(lambda: "Worktree name" in self.screen() and "topic/retry" in self.screen())
            self.press(b"\r")
            wait_for(lambda: self.worktree_status_row("interrupted", "▸ ●"))
            self.assertEqual([(t["name"], t["branch"]) for t in self.tasks()], [("interrupted", "topic/retry")])

    def test_interrupted_copy_preserves_independently_committed_work(self):
        with self.gated_copy() as control:
            self.launch()
            self.submit_worktree("keep-commit", "topic/save")
            wait_for((control / "entered").exists)
            target = Path(str(self.root) + "__sessions") / "keep-commit"
            (target / "saved").write_text("user change\n")
            self.git("add", "saved", cwd=target)
            self.commit("save independent work", cwd=target)
            commit = self.git("rev-parse", "HEAD", cwd=target).stdout
            self.crash_copy_worker(control)
            wait_for(lambda: self.worktree_status_row("keep-commit", "Failed"))
            self.assertEqual(self.tasks(), [])
            self.assertEqual(self.git("rev-parse", "refs/heads/topic/save").stdout, commit)
            self.assertEqual(self.git("rev-parse", "HEAD", cwd=target).stdout, commit)
            self.assertEqual((target / "saved").read_text(), "user change\n")
