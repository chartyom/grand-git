"""Search, success feedback and source terminal ownership with real tmux clients."""
import json
import os
from pathlib import Path
import shlex
import shutil
import time

from test_integration import BIN, SessionFixture, wait_for


class DashboardLifecycleTests(SessionFixture):
    def setUp(self):
        super().setUp()
        self.env.pop("NO_COLOR", None)

    def origin(self):
        return next(row for row in self.public_tasks() if row["kind"] == "temporary")

    def launcher_id(self):
        return int(self.origin()["id"].split(":")[1])

    def focus_list(self, master):
        pid = next(proc.pid for proc, fd, _, _ in self.clients if fd == master)
        os.write(master, b"\x17")
        wait_for(lambda: any(row.startswith(f"{pid} ") and row.endswith(" 1") for row in
                             self.tmux("list-clients", "-F", "#{client_pid} #{@gg_sidebar}").stdout.splitlines()))

    def test_escape_clears_search_then_ctrl_w_returns_to_terminal(self):
        self.create("alpha")
        self.create("beta")
        self.search_population()
        proc, master = self.attach()
        self.focus_list(master)
        side = self.sidebar(self.launcher_id())
        wait_for(lambda: "[/] Search" in self.capture(side))
        for suffix in [b"", b"\r"]:
            os.write(master, b"/query" + suffix)
            wait_for(lambda: "No matching sessions" in self.capture(side))
            os.write(master, b"\x1b")
            wait_for(lambda: "alpha" in self.capture(side) and "beta" in self.capture(side))
            self.assertEqual(self.active(), f"{self.launcher_id()}|1")
        os.write(master, b"\x17")
        wait_for(lambda: self.active() == f"{self.launcher_id()}|")

    def test_last_q_stops_temporary_source_jobs_but_preserves_managed(self):
        self.create("persistent")
        proc, master = self.attach()
        wait_for(lambda: self.origin()["temporary"])
        marker = Path(self.tmp.name) / "source-heartbeat"
        self.tmux("send-keys", "-t", self.terminal(self.launcher_id()),
                  f"while true; do echo tick >> {shlex.quote(str(marker))}; sleep .05; done &", "Enter")
        wait_for(lambda: marker.exists() and len(marker.read_text().splitlines()) > 3)
        self.focus_list(master)
        os.write(master, b"q")
        wait_for(lambda: proc.poll() is not None)
        wait_for(lambda: not any(r["kind"] == "temporary" for r in self.public_tasks()))
        count = len(marker.read_text().splitlines())
        time.sleep(.2)
        self.assertEqual(count, len(marker.read_text().splitlines()))
        self.assertFalse(any(r["kind"] == "temporary" for r in self.public_tasks()))
        self.assertTrue(self.tasks()[0]["active"])
        self.assertTrue(self.root.exists())
        self.assertEqual(self.git("branch", "--show-current").stdout.strip(), "main")

    def test_last_temporary_shell_exit_removes_record_without_closing_dashboard(self):
        proc, master = self.attach()
        launcher = self.launcher_id()
        terminal = self.terminal(launcher)
        wait_for(lambda: any(line.split() == ["▸", "●", "dashboard"]
                             for line in self.capture(self.sidebar(launcher)).splitlines()))
        self.tmux("send-keys", "-t", terminal, "exit", "Enter")
        wait_for(lambda: not any(r["kind"] == "temporary" for r in self.public_tasks()))
        self.assertIsNone(proc.poll())
        self.assertTrue(self.root.exists())

    def test_closing_pty_removes_only_its_temporary_session(self):
        self.create("persistent")
        proc, master = self.attach()
        client = next(item for item in self.clients if item[0] is proc)
        self.clients.remove(client)
        _, _, stop, reader = client
        os.close(master)
        stop.set()
        reader.join(timeout=1)
        proc.wait(timeout=3)
        wait_for(lambda: not any(r["kind"] == "temporary" for r in self.public_tasks()))
        self.assertTrue(self.tasks()[0]["active"])

    def test_disconnect_closes_temporary_and_q_only_cleans_its_own_run(self):
        first, first_master = self.attach()
        wait_for(lambda: self.origin()["temporary"])
        for client in self.tmux("list-clients", "-F", "#{client_name}").stdout.splitlines():
            self.tmux("detach-client", "-t", client)
        wait_for(lambda: first.poll() is not None)
        wait_for(lambda: not any(r["kind"] == "temporary" for r in self.public_tasks()))
        first, first_master = self.attach()
        second, second_master = self.attach()
        wait_for(lambda: str(second.pid) in self.tmux("list-clients", "-F", "#{client_pid}").stdout.splitlines())
        os.write(first_master, b"\x17")
        os.write(first_master, b"q")
        wait_for(lambda: first.poll() is not None)
        self.assertIsNone(second.poll())
        wait_for(lambda: len([r for r in self.public_tasks() if r["kind"] == "temporary"]) == 1)
        os.write(second_master, b"\x17")
        os.write(second_master, b"q")
        wait_for(lambda: second.poll() is not None)
        wait_for(lambda: not any(r["kind"] == "temporary" for r in self.public_tasks()))

    def test_existing_source_is_persistent_and_dashboard_has_a_separate_launcher(self):
        self.attach_session()
        proc, master = self.attach()
        wait_for(lambda: self.origin()["temporary"])
        self.assertNotEqual(self.launcher_id(), 0)
        source = next(r for r in self.public_tasks() if r["branch"] == "main")
        self.assertTrue(source["active"])
        self.assertFalse(source["temporary"])
        self.focus_list(master)
        os.write(master, b"q")
        wait_for(lambda: proc.poll() is not None)
        wait_for(lambda: not any(r["kind"] == "temporary" for r in self.public_tasks()))
        self.assertTrue(next(r for r in self.public_tasks() if r["branch"] == "main")["active"])

    def test_stale_q_run_does_not_detach_or_clean_a_replacement_context(self):
        first, master = self.attach()
        context = Path(self.env["XDG_STATE_HOME"]) / "grand-git/clients" / f"{first.pid}.json"
        wait_for(context.exists)
        stale = json.loads(context.read_text())["token"]
        data = json.loads(context.read_text())
        data["token"] = "f" * 32
        context.write_text(json.dumps(data))
        identity = self.tmux("list-clients", "-F", "#{client_pid}:#{client_created}").stdout.strip()
        self.gg("_quit-run", "--client", identity, "--run-token", stale)
        self.assertIsNone(first.poll())
        self.assertTrue(self.origin()["active"])
        self.focus_list(master)
        os.write(master, b"q")
        wait_for(lambda: first.poll() is not None)
        wait_for(lambda: not any(r["kind"] == "temporary" for r in self.public_tasks()))

    def test_success_is_green_expires_and_error_remains(self):
        self.create("worker")
        self.search_population()
        proc, master = self.attach()
        self.focus_list(master)
        # Choose the managed task, then stop it through the asynchronous worker.
        os.write(master, b"/worker\r")
        wait_for(lambda: "/ worker" in self.capture(self.sidebar(self.launcher_id())) or "/worker" in self.capture(self.sidebar(self.launcher_id())) )
        os.write(master, b"s")
        wait_for(lambda: "Stop worker?" in self.capture(self.sidebar(self.launcher_id())))
        os.write(master, b"y")
        home = lambda: self.tmux("display-message", "-p", "#{pane_id}").stdout.strip()
        wait_for(lambda: "Completed: stop" in self.capture(home()))
        colored = self.tmux("capture-pane", "-p", "-e", "-t", home()).stdout
        success_line = next(line for line in colored.splitlines() if "Completed: stop" in line)
        self.assertTrue("38;5;149" in success_line or "38;2;158;206;106" in success_line, repr(success_line))
        wait_for(lambda: "Completed: stop" not in self.capture(home()), timeout=7)
        state_path = self.root / ".git/grand-git/state.json"
        state = json.loads(state_path.read_text())
        state.update(notice="fixture operation failed", notice_kind="error")
        state_path.write_text(json.dumps(state))
        wait_for(lambda: "fixture operation failed" in self.capture(home()))
        time.sleep(5.2)
        self.assertIn("fixture operation failed", self.capture(home()))

    def test_mouse_q_targets_its_client(self):
        first, master = self.attach()
        second, other = self.attach()
        wait_for(lambda: str(second.pid) in self.tmux("list-clients", "-F", "#{client_pid}").stdout.splitlines())
        self.focus_list(master)
        side = self.sidebar(self.launcher_id())
        wait_for(lambda: "[Q]" in self.capture(side))
        line = next(i for i, text in enumerate(self.capture(side).splitlines()) if "[Q]" in text)
        geometry = next(p for p in self.geometry(self.launcher_id()) if p[0] == side)
        column = self.capture(side).splitlines()[line].index("[Q]")
        x, y = int(geometry[4]) + column + 2, int(geometry[5]) + line + 1
        os.write(master, f"\x1b[<0;{x};{y}M\x1b[<0;{x};{y}m".encode())
        wait_for(lambda: first.poll() is not None)
        self.assertIsNone(second.poll())
        self.assertTrue(self.origin()["active"])
        os.write(other, b"\x17q")
        wait_for(lambda: second.poll() is not None)
        wait_for(lambda: not any(r["kind"] == "temporary" for r in self.public_tasks()))

    def test_compact_popup_search_clear_then_q(self):
        self.search_population()
        outer = self.socket + "-outer"
        self.run_cmd("tmux", "-L", outer, "-f", "/dev/null", "new-session", "-d", "-s", "preview",
                     "-x", "80", "-y", "36", "-c", str(self.root), shlex.join([str(BIN), "dashboard"]))
        proc, master = self.start_client(["tmux", "-L", outer, "attach-session", "-t", "preview"])
        self.resize_client(proc, master, 80)
        wait_for(lambda: any(r["kind"] == "temporary" for r in self.public_tasks()))
        wait_for(lambda: any(p[2] == "1" and p[3] == "_gg-park" for p in self.geometry(self.launcher_id())))
        def screen():
            return self.run_cmd("tmux", "-L", outer, "capture-pane", "-p", "-t", "preview:0").stdout
        os.write(master, b"\x17")
        wait_for(lambda: "[/] Search" in screen())
        os.write(master, b"/query\r")
        wait_for(lambda: "No matching sessions" in screen())
        os.write(master, b"\x1b")
        wait_for(lambda: "[/] Search" in screen() and "No matching sessions" not in screen())
        self.assertIn("dashboard", screen())
        self.assertIsNone(proc.poll())
        # If Escape had closed the popup this q would reach the source shell.
        os.write(master, b"q")
        wait_for(lambda: proc.poll() is not None)
        wait_for(lambda: not any(r["kind"] == "temporary" for r in self.public_tasks()))

    def test_q_preserves_terminal_during_prepared_attach(self):
        first, master = self.attach()
        wait_for(lambda: self.origin()["temporary"])
        first_id = self.origin()["id"]
        gate = Path(self.tmp.name) / "allow-attach"
        prepared = Path(self.tmp.name) / "attach-prepared"
        wrappers = Path(self.tmp.name) / "wrapper"
        wrappers.mkdir()
        real_tmux = shutil.which("tmux")
        wrapper = wrappers / "tmux"
        wrapper.write_text("#!/bin/sh\n" +
            'if [ "$3" = attach-session ]; then\n' +
            f"  touch {shlex.quote(str(prepared))}\n" +
            f"  while [ ! -e {shlex.quote(str(gate))} ]; do sleep .05; done\n" +
            'fi\n' + f'exec {shlex.quote(real_tmux)} "$@"\n')
        wrapper.chmod(0o755)
        self.env["PATH"] = str(wrappers) + os.pathsep + self.env["PATH"]
        second, other = self.start_client([str(Path(__file__).resolve().parents[1] / "bin/gg"), "dashboard"])
        wait_for(prepared.exists)
        self.focus_list(master)
        os.write(master, b"q")
        wait_for(lambda: first.poll() is not None)
        def remaining_origin():
            rows = [r for r in self.public_tasks() if r["kind"] == "temporary"]
            return rows[0] if len(rows) == 1 and rows[0]["id"] != first_id else None
        self.assertTrue(wait_for(remaining_origin)["active"])
        gate.touch()
        wait_for(lambda: str(second.pid) in self.tmux("list-clients", "-F", "#{client_pid}").stdout.splitlines())
        self.focus_list(other)
        os.write(other, b"q")
        wait_for(lambda: second.poll() is not None)
        wait_for(lambda: not any(r["kind"] == "temporary" for r in self.public_tasks()))
