"""Recover a detached worker while its foreground Git checkout is in flight."""
import os
from pathlib import Path
import shlex
import shutil
import signal

from test_design_dashboard import DashboardScreenFixture
from test_integration import wait_for


class WorktreeCheckoutInterruptionTests(DashboardScreenFixture):
    def test_worker_dies_while_git_is_adding_its_checkout(self):
        control = Path(self.tmp.name) / "checkout-control"
        control.mkdir()
        git = shlex.quote(shutil.which("git"))
        shim = control / "git"
        shim.write_text(f"""#!/bin/sh
control={shlex.quote(str(control))}
if [ "$1" = worktree ] && [ "$2" = add ] && [ ! -f "$control/release" ]; then
    echo "$$" > "$control/entered"
    while [ ! -f "$control/release" ]; do
        [ -d "$control" ] || exit 1
        sleep .02
    done
    {git} "$@"
    result=$?
    echo "$result" > "$control/finished"
    exit "$result"
fi
exec {git} "$@"
""")
        shim.chmod(0o755)
        self.env["PATH"] = str(control) + os.pathsep + self.env["PATH"]
        name = "checkout-gate"
        target = Path(str(self.root) + "__sessions") / name
        self.launch()
        self.folder()
        self.press(b"n")
        wait_for(lambda: "Worktree name" in self.screen())
        self.press(name.encode() + b"\r")
        try:
            entered = control / "entered"
            child = int(wait_for(lambda: entered.exists() and entered.read_text().strip()))
            worker = child
            while worker > 1:
                fields = self.run_cmd("ps", "-p", str(worker), "-o", "ppid=", "-o", "command=").stdout.split()
                if "_prepare-worktree" in fields:
                    break
                worker = int(fields[0])
            self.assertGreater(worker, 1, "the gated Git process needs a live creation worker")
            os.kill(worker, signal.SIGKILL)

            frames = set()

            def observer_running():
                row = next((line[:40] for line in self.screen().splitlines() if name in line[:40]), "")
                if "Preparing" in row:
                    frames.add(row)
                return "Failed" in row or len(frames) >= 8

            # Keep the checkout blocked across the dashboard's recovery scans.
            # A worker can also cancel its Git child and report failure sooner.
            wait_for(observer_running)
        finally:
            (control / "release").touch()

        def checkout_finished():
            if (control / "finished").exists():
                return True
            state = self.run_cmd("ps", "-p", str(child), "-o", "stat=", check=False).stdout.strip()
            return not state or state.startswith("Z")

        wait_for(checkout_finished)
        wait_for(lambda: any(name in line[:40] and "Failed" in line[:40] for line in self.screen().splitlines()))
        self.assertFalse(os.path.lexists(target))
        self.assertNotIn("worktree " + str(target) + "\n", self.git("worktree", "list", "--porcelain").stdout)
        self.assertEqual(self.tasks(), [])
        self.click(name, sidebar=True)
        wait_for(lambda: "Worktree name" in self.screen() and name in self.screen())
        self.press(b"\r")
        wait_for(lambda: any("▸ ●" in line[:40] and name in line[:40] for line in self.screen().splitlines()))
        self.assertEqual([row["name"] for row in self.tasks()], [name])
