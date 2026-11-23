"""Global groups, ordinary directories, and per-client launchers with real tmux."""
import hashlib
import json
import os
from pathlib import Path
import shlex
import shutil
import time

from test_integration import BIN, SessionFixture, wait_for


class GlobalDashboardTests(SessionFixture):
    def socket_for(self, row):
        group = row["workspace"]
        key = group["path"] if group["type"] == "directory" else str(Path(group["path"]) / ".git")
        return ("gg-dir-" if group["type"] == "directory" else "gg-") + hashlib.sha256(key.encode()).hexdigest()[:16]

    def mux_for(self, row, *args):
        return self.run_cmd("tmux", "-L", self.socket_for(row), *args)

    def local_id(self, row):
        return int(row["id"].split(":")[1])

    def launchers(self):
        return [r for r in self.public_tasks() if r["kind"] == "temporary"]

    def focus_launcher(self, master, row):
        pid = next(proc.pid for proc, fd, _, _ in self.clients if fd == master)
        wait_for(lambda: str(pid) in self.mux_for(row, "list-clients", "-F", "#{client_pid}").stdout.splitlines())
        os.write(master, b"\x17")
        def sidebar_focused():
            clients = self.mux_for(row, "list-clients", "-F", "#{client_pid} #{pane_id}").stdout.splitlines()
            pane = next((line.split()[1] for line in clients if line.startswith(f"{pid} ")), None)
            return pane is not None and self.mux_for(row, "display-message", "-p", "-t", pane, "#{@gg_sidebar}").stdout.strip() == "1"
        wait_for(sidebar_focused)

    def select(self, master, query):
        self.search_population()
        os.write(master, b"/" + query.encode() + b"\r")

    def test_global_listing_ids_duplicate_branches_and_safe_plain_deletion(self):
        self.create("1")
        other = Path(self.tmp.name) / "other"
        other.mkdir()
        self.git("init", "-b", "main", cwd=other)
        (other / "file").write_text("other")
        self.git("add", ".", cwd=other)
        self.commit("other", cwd=other)
        self.gg("create", "1", cwd=other)
        plain = Path(self.tmp.name) / "plain"
        plain.mkdir()
        (plain / "keep").write_text("user data")
        self.gg("create", cwd=plain)
        self.gg("create", cwd=plain)
        rows = json.loads(self.gg("list", "--json", cwd=plain).stdout)
        self.assertEqual(len({r["id"] for r in rows}), len(rows))
        self.assertEqual(len([r for r in rows if r["branch"] == "1"]), 2)
        terminals = [r for r in rows if r["kind"] == "terminal"]
        self.assertEqual([r["name"] for r in terminals], ["plain", "plain"])
        self.assertTrue(all(r["branch"] == r["base"] == "" for r in terminals))
        self.gg("delete", terminals[0]["id"], "--yes")
        self.assertEqual((plain / "keep").read_text(), "user data")
        self.gg("stop", terminals[1]["id"])
        self.gg("start", terminals[1]["id"], cwd=plain)
        self.gg("delete", "--all", "--yes", cwd=plain)
        self.assertTrue((plain / "keep").exists())
        self.assertEqual(len([r for r in self.public_tasks() if r["branch"] == "1"]), 2)
        self.gg("delete", "1", "--yes")
        survivors = self.public_tasks()
        self.assertEqual(len([r for r in survivors if r["branch"] == "1"]), 1)
        self.assertEqual(next(r for r in survivors if r["branch"] == "1")["workspace"]["path"], str(other))

    def test_exact_cwd_and_independent_launcher_cleanup(self):
        child = self.root / "subdirectory"
        child.mkdir()
        self.create("persistent")
        first, master = self.attach(cwd=child)
        wait_for(lambda: len(self.launchers()) == 1)
        origin = self.launchers()[0]
        self.assertEqual(origin["path"], str(child))
        self.assertEqual(origin["template"], "shell")
        pid = self.mux_for(origin, "display-message", "-p", "#{pane_pid}").stdout.strip()
        second, other = self.attach(cwd=child)
        wait_for(lambda: len(self.launchers()) == 2)
        own_rows = lambda task_id: sum("● dashboard" in line for line in self.capture(self.sidebar(task_id)).splitlines())
        wait_for(lambda: own_rows(self.local_id(origin)) == 2 and own_rows(self.local_id(self.launchers()[-1])) == 2)
        self.assertNotEqual(self.sidebar(self.local_id(origin)), self.sidebar(self.local_id(self.launchers()[-1])))
        self.focus_launcher(master, self.launchers()[-1])
        os.write(master, b"q")
        wait_for(lambda: first.poll() is not None)
        wait_for(lambda: len(self.launchers()) == 1)
        self.assertNotEqual(self.launchers()[0]["id"], origin["id"])
        self.assertIsNone(second.poll())
        self.assertTrue(next(r for r in self.public_tasks() if r["branch"] == "persistent")["active"])
        self.focus_launcher(other, self.launchers()[0])
        os.write(other, b"q")
        wait_for(lambda: second.poll() is not None)
        wait_for(lambda: not self.launchers())
        self.assertTrue(child.exists())

    def test_two_clients_select_windows_independently(self):
        self.create("persistent")
        first, master = self.attach()
        second, other = self.attach()
        def window(pid):
            rows = self.tmux("list-clients", "-F", "#{client_pid} #{session_name} #{window_id}").stdout.splitlines()
            return next(line.split()[1:] for line in rows if line.startswith(f"{pid} "))
        first_before, second_before = window(first.pid), window(second.pid)
        self.assertNotEqual(first_before[0], second_before[0])
        self.assertNotEqual(first_before[1], second_before[1])
        self.focus_launcher(master, next(r for r in self.launchers() if self.local_id(r) == 2))
        os.write(master, b"k\r")
        wait_for(lambda: window(first.pid)[1] != first_before[1])
        self.assertEqual(window(second.pid), second_before)

    def test_cross_server_enter_preserves_processes_and_q_removes_original_launcher(self):
        plain = Path(self.tmp.name) / "aaa-other"
        plain.mkdir()
        self.gg("create", "foreign", cwd=plain)
        foreign = next(r for r in self.public_tasks() if r["name"] == "foreign")
        before = set(self.mux_for(foreign, "list-panes", "-a", "-F", "#{pane_pid}").stdout.splitlines())
        proc, master = self.attach()
        wait_for(lambda: len(self.launchers()) == 1)
        origin = self.launchers()[0]
        origin_id = self.local_id(origin)
        self.focus_launcher(master, origin)
        side = self.sidebar(origin_id)
        wait_for(lambda: "aaa-other" in self.capture(side) and "repo with spaces" in self.capture(side))
        self.select(master, "foreign")
        wait_for(lambda: "foreign" in self.capture(side))
        os.write(master, b"\r")
        wait_for(lambda: str(proc.pid) in self.mux_for(foreign, "list-clients", "-F", "#{client_pid}").stdout.splitlines())
        self.assertIsNone(proc.poll())
        self.assertEqual(before, set(self.mux_for(foreign, "list-panes", "-a", "-F", "#{pane_pid}").stdout.splitlines()))
        self.assertTrue(self.launchers()[0]["active"])
        self.focus_launcher(master, foreign)
        os.write(master, b"q")
        wait_for(lambda: proc.poll() is not None)
        wait_for(lambda: not self.launchers())
        self.assertTrue(next(r for r in self.public_tasks() if r["id"] == foreign["id"])["active"])

    def test_single_click_foreign_row_recovers_stale_mouse_client(self):
        plain = Path(self.tmp.name) / "aaa-other"
        plain.mkdir()
        self.gg("create", "foreign", cwd=plain)
        foreign = next(r for r in self.public_tasks() if r["name"] == "foreign")
        before = set(self.mux_for(foreign, "list-panes", "-a", "-F", "#{pane_pid}").stdout.splitlines())
        proc, master = self.attach()
        origin = self.launchers()[0]
        side = self.sidebar(self.local_id(origin))
        wait_for(lambda: "foreign" in self.capture(side))
        line = next(i for i, text in enumerate(self.capture(side).splitlines()) if "foreign" in text)
        left, top = map(int, self.mux_for(origin, "display-message", "-p", "-t", side,
                                          "#{pane_left} #{pane_top}").stdout.split())
        self.mux_for(origin, "set-option", "-p", "-t", side, "@gg_mouse_clients", "0:0,0:0,")
        click = f"\x1b[<0;{left + 5};{top + line + 1}M\x1b[<0;{left + 5};{top + line + 1}m".encode()
        os.write(master, click)
        wait_for(lambda: str(proc.pid) in self.mux_for(foreign, "list-clients", "-F", "#{client_pid}").stdout.splitlines())
        self.assertIsNone(proc.poll())
        self.assertEqual(before, set(self.mux_for(foreign, "list-panes", "-a", "-F", "#{pane_pid}").stdout.splitlines()))

    def test_single_clicks_open_only_for_the_initiating_client(self):
        plain = Path(self.tmp.name) / "aaa-other"
        plain.mkdir()
        self.gg("create", "foreign", cwd=plain)
        foreign = next(r for r in self.public_tasks() if r["name"] == "foreign")
        first, first_master = self.attach()
        second, second_master = self.attach()
        origin = self.launchers()[-1]
        side = self.sidebar(self.local_id(origin))
        wait_for(lambda: "foreign" in self.capture(side))
        line = next(i for i, text in enumerate(self.capture(side).splitlines()) if "foreign" in text)
        left, top = map(int, self.mux_for(origin, "display-message", "-p", "-t", side,
                                          "#{pane_left} #{pane_top}").stdout.split())
        click = f"\x1b[<0;{left + 5};{top + line + 1}M\x1b[<0;{left + 5};{top + line + 1}m".encode()
        os.write(first_master, click)
        time.sleep(.08)
        os.write(second_master, click)
        wait_for(lambda: str(second.pid) in self.mux_for(foreign, "list-clients", "-F", "#{client_pid}").stdout.splitlines())
        wait_for(lambda: str(first.pid) in self.mux_for(foreign, "list-clients", "-F", "#{client_pid}").stdout.splitlines())

    def test_directory_dashboard_create_form_and_disconnect(self):
        plain = Path(self.tmp.name) / "ordinary"
        plain.mkdir()
        proc, master = self.start_client([str(BIN), "dashboard"], cwd=plain)
        wait_for(lambda: len(self.launchers()) == 1)
        origin = self.launchers()[0]
        self.focus_launcher(master, origin)
        os.write(master, b"n")
        wait_for(lambda: b"Name (optional)" in self.outputs[proc.pid])
        os.write(master, b"\r")
        wait_for(lambda: any(r["kind"] == "terminal" for r in self.public_tasks()))
        terminal = next(r for r in self.public_tasks() if r["kind"] == "terminal")
        self.assertEqual(terminal["path"], str(plain))
        self.assertEqual(terminal["name"], "ordinary")
        client = self.mux_for(origin, "list-clients", "-F", "#{client_name}").stdout.strip()
        self.mux_for(origin, "detach-client", "-t", client)
        wait_for(lambda: proc.poll() is not None)
        wait_for(lambda: not self.launchers())
        self.assertTrue(next(r for r in self.public_tasks() if r["kind"] == "terminal")["active"])

    def test_plain_template_ignores_local_gg_and_rejects_git_flags(self):
        plain = Path(self.tmp.name) / "ordinary"
        plain.mkdir()
        (plain / ".gg").mkdir()
        (plain / ".gg/config.yaml").write_text("unknown: broken\n")
        templates = Path(self.env["XDG_CONFIG_HOME"]) / "grand-git/templates"
        templates.mkdir(parents=True)
        (templates / "mark.yaml").write_text("commands:\n  - echo started > marker\n")
        for args in [("--base", "main"), ("--no-copy",)]:
            self.assertNotEqual(self.gg("create", *args, cwd=plain, check=False).returncode, 0)
        self.gg("create", "--template", "mark", cwd=plain)
        wait_for(lambda: (plain / "marker").exists())
        self.assertEqual((plain / "marker").read_text().strip(), "started")

    def test_unavailable_group_reports_partial_json_without_hiding_healthy_sessions(self):
        self.create("healthy")
        plain = Path(self.tmp.name) / "missing"
        plain.mkdir()
        self.gg("create", cwd=plain)
        plain.rmdir()
        result = self.gg("list", "--json", check=False)
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("healthy", [r["branch"] for r in json.loads(result.stdout)])
        self.assertIn("missing", result.stderr)

    def test_init_does_not_install_hooks_in_launcher_subdirectory(self):
        child = self.root / "child"
        child.mkdir()
        proc, master = self.attach(cwd=child)
        wait_for(lambda: len(self.launchers()) == 1)
        self.gg("init")
        self.assertTrue((self.root / ".codex/hooks.json").exists())
        self.assertFalse((child / ".codex").exists())
        state = json.loads((self.root / ".git/grand-git/state.json").read_text())
        launcher = next(t for t in state["tasks"] if t.get("kind") == "temporary")
        self.assertNotIn("generated_files", launcher)

    def test_plain_terminal_needs_no_git_and_deletion_after_git_init_preserves_files(self):
        plain = Path(self.tmp.name) / "plain"
        plain.mkdir()
        programs = Path(self.tmp.name) / "programs"
        programs.mkdir()
        (programs / "tmux").symlink_to(shutil.which("tmux"))
        env = dict(self.env, PATH=str(programs))
        self.gg("create", cwd=plain, env=env)
        terminal = next(r for r in self.public_tasks() if r["kind"] == "terminal")
        self.git("init", "-b", "main", cwd=plain)
        (plain / "precious").write_text("keep")
        self.gg("delete", terminal["id"], "--yes")
        self.assertEqual((plain / "precious").read_text(), "keep")
        self.assertTrue((plain / ".git").is_dir())

    def test_create_popup_is_shown_to_the_initiating_client(self):
        first, master = self.attach()
        second, other = self.attach()
        wait_for(lambda: len(self.launchers()) == 2)
        self.focus_launcher(master, self.launchers()[-1])
        before_first = len(self.outputs[first.pid])
        before_second = len(self.outputs[second.pid])
        os.write(master, b"n")
        wait_for(lambda: b"Branch name" in self.outputs[first.pid][before_first:])
        self.assertNotIn(b"Branch name", self.outputs[second.pid][before_second:])
        os.write(master, b"\x1b")
        self.assertIsNone(first.poll())
        self.assertIsNone(second.poll())

    def test_compact_popup_can_switch_groups_and_quit_its_original_run(self):
        plain = Path(self.tmp.name) / "other"
        plain.mkdir()
        self.gg("create", "foreign", cwd=plain)
        foreign = next(r for r in self.public_tasks() if r["name"] == "foreign")
        proc, master = self.attach()
        wait_for(lambda: len(self.launchers()) == 1)
        origin = self.launchers()[0]
        self.resize_client(proc, master, 80)
        wait_for(lambda: any(p[3] == "_gg-park" for p in self.geometry(self.local_id(origin))))
        offset = len(self.outputs[proc.pid])
        os.write(master, b"\x17")
        wait_for(lambda: b"[?] Actions" in self.outputs[proc.pid][offset:])
        self.select(master, "foreign")
        wait_for(lambda: b"foreign" in self.outputs[proc.pid][offset:])
        os.write(master, b"\r")
        wait_for(lambda: str(proc.pid) in self.mux_for(foreign, "list-clients", "-F", "#{client_pid}").stdout.splitlines())
        wait_for(lambda: "1|_gg-park" in self.mux_for(foreign, "list-panes", "-a", "-F", "#{@gg_sidebar}|#{session_name}").stdout)
        offset = len(self.outputs[proc.pid])
        os.write(master, b"\x17")
        wait_for(lambda: b"[Q]" in self.outputs[proc.pid][offset:])
        os.write(master, b"q")
        wait_for(lambda: proc.poll() is not None)
        wait_for(lambda: not self.launchers())
        self.assertTrue(next(r for r in self.public_tasks() if r["id"] == foreign["id"])["active"])

    def test_new_uses_the_selected_foreign_directory(self):
        plain = Path(self.tmp.name) / "target"
        plain.mkdir()
        self.gg("create", "foreign", cwd=plain)
        proc, master = self.attach()
        wait_for(lambda: len(self.launchers()) == 1)
        origin = self.launchers()[0]
        self.focus_launcher(master, origin)
        self.select(master, "foreign")
        wait_for(lambda: "foreign" in self.capture(self.sidebar(self.local_id(origin))))
        os.write(master, b"n")
        wait_for(lambda: b"Name (optional)" in self.outputs[proc.pid])
        os.write(master, b"\r")
        wait_for(lambda: any(r["name"] == "target" and r["kind"] == "terminal" for r in self.public_tasks()))
        created = next(r for r in self.public_tasks() if r["name"] == "target" and r["kind"] == "terminal")
        self.assertEqual(created["path"], str(plain))
        wait_for(lambda: str(proc.pid) in self.mux_for(created, "list-clients", "-F", "#{client_pid}").stdout.splitlines())
