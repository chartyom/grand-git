"""Design interactions exercised through a real terminal in isolated tmux servers."""
import os
from pathlib import Path
import re
import shlex
import time

from test_integration import BIN, SessionFixture, wait_for


def background_cells(captured):
    rows = [[]]
    background = None
    for token in re.split(r"(\x1b\[[0-9;]*m)", captured):
        if token.startswith("\x1b["):
            codes = [int(code or 0) for code in token[2:-1].split(";")]
            i = 0
            while i < len(codes):
                code = codes[i]
                if code in (0, 49):
                    background = None
                elif 40 <= code <= 47:
                    background = ("indexed", code - 40)
                elif 100 <= code <= 107:
                    background = ("indexed", code - 100 + 8)
                elif code in (38, 48, 58):
                    mode = codes[i + 1]
                    count = 1 if mode == 5 else 3
                    color = ("indexed" if mode == 5 else "rgb", *codes[i + 2:i + 2 + count])
                    if code == 48:
                        background = color
                    i += count + 1
                i += 1
        else:
            for char in token:
                if char == "\n":
                    rows.append([])
                else:
                    rows[-1].append((char, background))
    return rows


class DashboardScreenFixture(SessionFixture):
    def setUp(self):
        super().setUp()
        self.env.pop("GG_TEST_LEGACY_DASHBOARD", None)
        self.outer = self.socket + "-outer"

    def outer_tmux(self, *args):
        return self.run_cmd("tmux", "-L", self.outer, *args)

    def launch(self, width=160, height=42):
        self.outer_tmux("-f", "/dev/null", "new-session", "-d", "-s", "preview", "-c", str(self.root))
        self.proc, self.master = self.start_client(["tmux", "-L", self.outer, "attach-session", "-t", "preview"])
        self.resize_client(self.proc, self.master, width, height)
        self.outer_tmux("send-keys", "-t", "preview:0", shlex.quote(str(BIN)) + " dashboard", "Enter")
        wait_for(lambda: "Add section" in self.screen())
        self.press(b"+")
        wait_for(lambda: "Ctrl+S" in self.screen())
        self.press(b"\x13")
        def selected_folder():
            screen = self.screen()
            return f"› {self.root.name}" in screen and "[T] Terminal" in screen and "Ctrl+S" not in screen
        wait_for(selected_folder)
        self.press(b"t")
        wait_for(lambda: any(row["kind"] == "terminal" and row["path"] == str(self.root) for row in self.public_tasks()))
        time.sleep(.15)
        wait_for(lambda: "Ctrl+W" in self.screen())
        wait_for(lambda: any(line[:40].strip() == self.root.name for line in self.screen().splitlines()))

    def screen(self):
        return self.outer_tmux("capture-pane", "-p", "-t", "preview:0").stdout

    def press(self, keys):
        os.write(self.master, keys)

    def click(self, text, button=0, sidebar=False):
        def target():
            for y, line in enumerate(self.screen().splitlines()):
                area = line[:40] if sidebar else line
                if text in area:
                    return area.index(text), y
        x, y = wait_for(target)
        sequence = f"\x1b[<{button};{x+1};{y+1}M\x1b[<{button};{x+1};{y+1}m".encode()
        # Deliver application mouse input without the outer server's click bindings.
        self.outer_tmux("send-keys", "-H", "-t", "preview:0", *(f"{byte:02x}" for byte in sequence))

    def folder(self):
        self.click(self.root.name, sidebar=True)
        wait_for(lambda: "[T] Terminal" in self.screen())


class DesignDashboardTests(DashboardScreenFixture):
    def test_named_worktrees_keep_terminals_across_branch_changes(self):
        self.launch()
        for name in ["clone1", "clone2", "clone3"]:
            self.folder()
            self.press(b"n")
            wait_for(lambda: "Worktree name" in self.screen())
            self.press(name.encode() + b"\r")
            wait_for(lambda: any(row["name"] == name for row in self.tasks()))
            wait_for(lambda: "Create session" not in self.screen())

        def open_worktree(name):
            self.click(name, sidebar=True)
            wait_for(lambda: f"▸ ● ⎇  {name}" in self.screen())

        def branch(name):
            return next(row["branch"] for row in self.tasks() if row["name"] == name)

        rows = self.tasks()
        self.assertEqual([row["branch"] for row in rows], ["", "", ""])
        panes = [self.terminal(row["id"]) for row in rows]
        def pids():
            return [self.tmux("display-message", "-p", "-t", pane, "#{pane_pid}").stdout for pane in panes]

        before = pids()
        open_worktree("clone1")
        self.press(b"git switch -c feat/login\r")
        wait_for(lambda: branch("clone1") == "feat/login")
        wait_for(lambda: any("feat/login" in line[:40] for line in self.screen().splitlines()))
        open_worktree("clone2")
        self.press(b"git switch release/demo\r")
        wait_for(lambda: branch("clone2") == "release/demo")
        open_worktree("clone1")
        self.press(b"git switch --detach\r")
        wait_for(lambda: branch("clone1") == "")
        self.assertEqual(pids(), before)
        self.assertEqual([(r["id"], r["name"], r["path"]) for r in self.tasks()],
                         [(r["id"], r["name"], r["path"]) for r in rows])
        self.assertIsNone(self.proc.poll())

    def test_terminal_backgrounds_stay_inside_each_pane(self):
        self.env.pop("NO_COLOR", None)
        self.env["COLORTERM"] = "truecolor"
        self.launch()
        launcher = next(row for row in self.public_tasks() if row["kind"] == "terminal" and row["path"] == str(self.root))
        task = int(launcher["id"].split(":")[1])
        first = self.terminal(task)
        colored_top = 3
        colored_rows = 8

        def sync_terminal(pane, marker):
            self.press(("printf '\\n" + marker + "\\n'\r").encode())
            wait_for(lambda: marker in self.capture(pane).splitlines())

        def paint(pane, color, label):
            width = int(self.tmux("display-message", "-p", "-t", pane, "#{pane_width}").stdout)
            output = "\\033[0m\\033[2J\\033[" + str(colored_top + 1) + ";1H\\033[48;5;" + str(color) + "m"
            output += ("%-" + str(width) + "s\\n") * colored_rows + "\\033[0m"
            command = "printf " + shlex.quote(output) + " " + " ".join(
                shlex.quote(label + " " + str(row)) for row in range(colored_rows))
            self.tmux("send-keys", "-t", pane, command, "Enter")
            wait_for(lambda: self.capture(pane).splitlines()[colored_top:colored_top + colored_rows] == [
                label + " " + str(row) for row in range(colored_rows)])
            def painted():
                lines = self.screen().splitlines()
                return len(lines) > colored_top + colored_rows and all(
                    label + " " + str(row) in lines[row + colored_top + 1] for row in range(colored_rows))
            wait_for(painted)

        def check_colors(expected, origin=41):
            rows = background_cells(self.outer_tmux("capture-pane", "-p", "-e", "-N", "-t", "preview:0").stdout)
            areas = []
            for pane, color in expected:
                bounds = self.tmux("display-message", "-p", "-t", pane,
                                   "#{pane_left} #{pane_top} #{pane_width}").stdout
                x, y, width = map(int, bounds.split())
                areas.append((x, y, width, ("indexed", color)))
            min_x, min_y = min(area[0] for area in areas), min(area[1] for area in areas)
            areas = [(origin + x - min_x, 1 + colored_top + y - min_y, width, color) for x, y, width, color in areas]
            for left, top, width, _ in areas:
                for y in range(top, top + colored_rows):
                    self.assertGreaterEqual(len(rows[y]), left + width)
            colors = {area[3] for area in areas}
            for y, row in enumerate(rows):
                for x, (_, background) in enumerate(row):
                    expected_color = next((color for left, top, width, color in areas
                                           if left <= x < left + width and top <= y < top + colored_rows), None)
                    if expected_color is not None:
                        self.assertEqual(background, expected_color, f"pane background at {x},{y}")
                    else:
                        self.assertNotIn(background, colors, f"terminal background leaked at {x},{y}")

        wait_for(lambda: self.tmux("display-message", "-p", "-t", first, "#{pane_width}").stdout.strip() == "119")
        paint(first, 22, "+ green")
        check_colors([(first, 22)])

        self.press(b"\x17t")
        wait_for(lambda: "Below" in self.screen())
        self.press(b"r")
        wait_for(lambda: len([pane for pane in self.panes(task) if pane[2] != "1"]) == 2)
        second = next(pane[0] for pane in self.panes(task) if pane[2] != "1" and pane[0] != first)
        # Wait for the dashboard to focus the right pane before typing into it.
        right_width = int(self.tmux("display-message", "-p", "-t", second, "#{pane_width}").stdout)
        wait_for(lambda: "▸" in self.screen().splitlines()[0][-right_width:])
        sync_terminal(second, "GG_COLOR_SPLIT_READY")
        paint(first, 22, "+ left")
        paint(second, 52, "- right")
        check_colors([(first, 22), (second, 52)])

        pids = {pane: self.tmux("display-message", "-p", "-t", pane, "#{pane_pid}").stdout
                for pane in (first, second)}
        for width, height, origin, terminal_width in ((99, 24, 0, 99), (160, 42, 41, 119)):
            self.resize_client(self.proc, self.master, width, height)
            def terminal_area_ready():
                widths = [int(row[6]) for row in self.geometry(task) if row[0] in (first, second)]
                return len(widths) == 2 and sum(widths) + 1 == terminal_width

            try:
                wait_for(terminal_area_ready)
            except AssertionError:
                outer = self.outer_tmux("display-message", "-p", "-t", "preview:0", "#{window_width}x#{window_height}")
                self.fail(f"Requested {width}x{height}, outer window {outer.stdout.strip()}, "
                          f"expected terminal area {terminal_width}, inner panes {self.geometry(task)}")
            sync_terminal(second, "GG_COLOR_RESIZE_READY_" + str(width))
            paint(first, 22, "+ left " + str(width))
            paint(second, 52, "- right " + str(width))
            check_colors([(first, 22), (second, 52)], origin)
            for pane, pid in pids.items():
                self.assertEqual(self.tmux("display-message", "-p", "-t", pane, "#{pane_pid}").stdout, pid)

    def test_folder_terminal_is_persistent_plain_shell_in_git(self):
        self.gg("set", "default_template", "codex")
        self.launch()
        before = self.git("branch", "--list").stdout
        self.folder()
        self.press(b"t")
        row = wait_for(lambda: next((r for r in self.public_tasks() if r["kind"] == "terminal"), None))
        self.assertEqual(row["workspace"]["type"], "git")
        self.assertEqual(row["path"], str(self.root))
        self.assertEqual((row["branch"], row["base"], row["template"]), ("", "", "shell"))
        self.assertEqual(self.git("branch", "--list").stdout, before)
        self.assertFalse(Path(str(self.root) + "__sessions").exists())
        self.folder()
        self.press(b"q")
        wait_for(lambda: "Ctrl+W" not in self.screen())
        self.assertTrue(next(r for r in self.public_tasks() if r["id"] == row["id"])["active"])
        self.gg("stop", row["id"])
        self.gg("start", row["id"])
        self.gg("delete", row["id"], "--yes")
        self.assertEqual((self.root / "README").read_text(), "main\n")

    def test_context_menu_does_not_start_but_single_click_does(self):
        self.create("feature")
        self.gg("stop", "feature")
        self.launch()
        self.click("feature", button=2, sidebar=True)
        wait_for(lambda: "[S] Start" in self.screen())
        self.assertFalse(self.tasks()[0]["active"])
        self.assertNotIn("[Q] Quit dashboard", self.screen())
        self.press(b"\x1b")
        wait_for(lambda: "[S] Start" not in self.screen())
        self.click("feature", sidebar=True)
        wait_for(lambda: self.tasks()[0]["active"])
        terminal = self.terminal(self.tasks()[0]["id"])
        self.press(b"echo GG_SINGLE_CLICK\r")
        wait_for(lambda: "GG_SINGLE_CLICK" in self.capture(terminal))
        self.click("feature", button=2, sidebar=True)
        wait_for(lambda: "[S] Stop" in self.screen())
        self.click("[S] Stop")
        wait_for(lambda: "Stop feature?" in self.screen())
        self.click("[Y] Stop")
        wait_for(lambda: not self.tasks()[0]["active"])
        wait_for(lambda: "No terminal open" in self.screen())

    def test_bulk_deletion_preflights_and_uses_surviving_frozen_targets(self):
        self.gg("create", "--branch", "alpha")
        self.gg("create", "--branch", "beta")
        dirty = Path(self.tasks()[0]["path"]) / "untracked"
        dirty.write_text("keep\n")
        self.launch()
        self.folder()
        self.press(b"d")
        wait_for(lambda: "Delete all sessions" in self.screen())
        self.create("later")
        self.gg("delete", "beta", "--yes")
        self.press(b"y")
        wait_for(lambda: "untracked" in self.screen())
        self.assertTrue(all(r["active"] for r in self.public_tasks()))
        self.assertEqual(dirty.read_text(), "keep\n")
        dirty.unlink()
        self.folder()
        self.press(b"d")
        wait_for(lambda: "Delete all sessions" in self.screen())
        self.create("new-after-confirmation")
        self.gg("delete", "later", "--yes")
        self.press(b"y")
        wait_for(lambda: [r["name"] for r in self.public_tasks()] == ["new-after-confirmation"])
        self.assertIn("alpha", self.git("branch", "--list").stdout)
        self.assertIn("beta", self.git("branch", "--list").stdout)
        self.assertEqual((self.root / "README").read_text(), "main\n")
        wait_for(lambda: "[T] Terminal" in self.screen())
        self.assertIsNone(self.proc.poll())

    def test_create_overlay_retains_error_and_picker_can_cancel(self):
        self.launch()
        self.folder()
        self.press(b"n")
        wait_for(lambda: "Create session" in self.screen())
        self.assertIn("Grand Git", self.screen())
        self.press(b"invalid name\r")
        wait_for(lambda: "invalid worktree name" in self.screen().lower())
        self.assertIn("Create session", self.screen())
        self.assertIn("invalid name", self.screen())
        self.click("[Esc] Cancel")
        wait_for(lambda: "Create session" not in self.screen())
        self.press(b"n")
        wait_for(lambda: "Create session" in self.screen())
        self.press(b"feat/from-design\t")
        wait_for(lambda: "Choose template" in self.screen())
        self.resize_client(self.proc, self.master, 100, 16)
        def resized_picker():
            lines = self.screen().splitlines()
            return len(lines) == 15 and "Ctrl+W" in lines[-1] and any("Choose template" in line for line in lines)
        wait_for(resized_picker)
        self.click("[Esc] Back")
        wait_for(lambda: "feat/from-design" in self.screen())
        self.click("[Enter] Create")
        row = wait_for(lambda: next((r for r in self.public_tasks() if r["name"] == "feat/from-design"), None))
        self.assertEqual(row["kind"], "worktree")
        self.assertEqual(row["template"], "shell")

    def test_delete_stopped_foreign_group_without_a_server_and_recreate_shell(self):
        other = Path(self.tmp.name) / "empty-folder"
        other.mkdir()
        (other / "keep").write_text("preserved\n")
        self.gg("create", "stopped", cwd=other)
        row = next(r for r in self.public_tasks() if r["name"] == "stopped")
        import hashlib
        socket = "gg-dir-" + hashlib.sha256(str(other).encode()).hexdigest()[:16]
        self.run_cmd("tmux", "-L", socket, "kill-server")
        self.launch()
        self.click("empty-folder", sidebar=True)
        wait_for(lambda: "[D] Delete all" in self.screen())
        self.press(b"d")
        wait_for(lambda: "Delete all sessions" in self.screen())
        self.press(b"y")
        wait_for(lambda: not any(r["id"] == row["id"] for r in self.public_tasks()))
        self.assertEqual((other / "keep").read_text(), "preserved\n")
        self.click("empty-folder", sidebar=True)
        self.press(b"t")
        created = wait_for(lambda: next((r for r in self.public_tasks() if r["path"] == str(other)), None))
        self.assertEqual((created["kind"], created["template"]), ("terminal", "shell"))

    def test_hover_preserves_terminal_focus_and_search_footer_is_exclusive(self):
        for i in range(10):
            self.create_stopped_worktree(f"find-{i}")
        self.launch()
        launcher = next(r for r in self.public_tasks() if r["kind"] == "terminal" and r["path"] == str(self.root))
        terminal = self.terminal(int(launcher["id"].split(":")[1]))
        lines = self.screen().splitlines()
        y = next(i for i, line in enumerate(lines) if "find-0" in line)
        self.press(f"\x1b[<35;5;{y+1}M".encode())
        wait_for(lambda: any("__sessions/find-0" in line for line in self.screen().splitlines()))
        self.press(b"echo GG_HOVER_FOCUS\r")
        wait_for(lambda: "GG_HOVER_FOCUS" in self.capture(terminal))
        self.press(b"\x1b[<35;80;10M")
        wait_for(lambda: "__sessions/find-0" not in self.screen())
        self.folder()
        self.press(b"k")
        wait_for(lambda: "Clear" in self.screen())
        self.press(b"no-match")
        wait_for(lambda: "No matching sessions" in self.screen())
        self.assertNotIn("[Q] Quit", self.screen())
        self.click("Clear")
        wait_for(lambda: "no-match" not in self.screen())
        self.assertIn("To list", self.screen())
        self.press(b"find-7\x1b[B")
        wait_for(lambda: "[Q] Quit" in self.screen())
        self.assertIn("find-7", self.screen())
        self.press(b"\x1b")
        wait_for(lambda: "▸ ● >_ " + self.root.name in self.screen())
        self.assertIn("[/] Search", self.screen())

    def test_settings_overlay_mouse_and_compact_resize(self):
        self.launch()
        self.folder()
        self.press(b",")
        wait_for(lambda: "Language: en" in self.screen())
        self.click("Language: en")
        wait_for(lambda: "Language: ru" in self.screen())
        self.click("Save to: Global")
        wait_for(lambda: "Save to: Local" in self.screen())
        self.click("[Enter] Save")
        wait_for(lambda: "Действия" in self.screen())
        self.assertIn("language: ru", (self.root / ".gg/config.yaml").read_text())
        self.resize_client(self.proc, self.master, 99, 24)
        wait_for(lambda: "Ctrl+W" in self.screen())
        self.press(b"?")
        wait_for(lambda: "Действия" in self.screen() and "[Esc]" in self.screen())
        self.press(b"\x1b")
        wait_for(lambda: "[Esc]" not in self.screen())
        self.press(b"q")
        wait_for(lambda: "Ctrl+W" not in self.screen())
