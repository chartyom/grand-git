"""Named managed worktrees exercised with the public CLI and native Git."""
from pathlib import Path
import json
import os
import shlex
import shutil

from test_integration import SessionFixture


class NamedWorktreeTests(SessionFixture):
    def make_commit(self, path, message):
        self.git("-c", "user.name=Test", "-c", "user.email=test@localhost",
                 "commit", "--allow-empty", "-m", message, cwd=path)
        return self.git("rev-parse", "HEAD", cwd=path).stdout.strip()

    def test_detached_slots_keep_identity_during_native_switch(self):
        self.git("branch", "clone1")
        self.make_commit(self.root, "caller ahead of clone1")
        for name in ("clone1", "clone2", "clone3"):
            self.gg("create", name)
        rows = self.public_tasks()
        self.assertEqual([row["name"] for row in rows], ["clone1", "clone2", "clone3"])
        root_head = self.git("rev-parse", "HEAD").stdout.strip()
        for row in rows:
            self.assertEqual(row["branch"], "")
            self.assertEqual(row["base"], root_head)
            self.assertEqual(Path(row["path"]), Path(str(self.root) + "__sessions") / row["name"])
            self.assertEqual(self.git("symbolic-ref", "-q", "HEAD", cwd=row["path"], check=False).returncode, 1)
        self.assertNotEqual(self.git("rev-parse", "clone1").stdout.strip(), root_head)
        self.assertNotEqual(self.git("show-ref", "--verify", "refs/heads/clone2", check=False).returncode, 0)

        first, second, third = rows
        pane = self.terminal(int(first["id"].split(":")[1]))
        pid = self.tmux("display-message", "-p", "-t", pane, "#{pane_pid}").stdout
        self.git("switch", "-c", "feat/login", cwd=first["path"])
        self.git("switch", "release/demo", cwd=second["path"])
        self.assertNotEqual(self.git("switch", "feat/login", cwd=third["path"], check=False).returncode, 0)
        updated = self.public_tasks()
        self.assertEqual([row["branch"] for row in updated], ["feat/login", "release/demo", ""])
        self.assertEqual([row["id"] for row in rows], [row["id"] for row in updated])
        self.assertEqual([row["name"] for row in rows], [row["name"] for row in updated])
        self.assertEqual([row["path"] for row in rows], [row["path"] for row in updated])
        self.assertEqual(self.tmux("display-message", "-p", "-t", pane, "#{pane_pid}").stdout, pid)
        self.assertNotEqual(self.gg("stop", "feat/login", check=False).returncode, 0)
        self.gg("stop", "clone1")
        self.assertNotEqual(self.gg("create", "clone1", check=False).returncode, 0)
        self.gg("start", first["id"])
        self.gg("delete", "clone1", "--yes")
        self.git("show-ref", "--verify", "refs/heads/feat/login")
        self.git("show-ref", "--verify", "refs/heads/clone1")

    def test_branch_is_optional_and_distinct_from_name(self):
        self.make_commit(self.root, "new main")
        self.gg("create", "clone1", "--branch", "feat/login", "--base", "release/demo")
        self.gg("create", "--branch", "release/demo")
        self.gg("create", "clone2", "--base", "release/demo")
        rows = self.public_tasks()
        base = self.git("rev-parse", "release/demo").stdout.strip()
        self.assertEqual([(r["name"], r["branch"], r["base"]) for r in rows],
                         [("clone1", "feat/login", base), ("release/demo", "release/demo", base),
                          ("clone2", "", base)])
        for args in [("--branch", "feat/login"), ("--branch", "feat/login", "--base", "main")]:
            self.assertNotEqual(self.gg("create", "clone3", *args, check=False).returncode, 0)
        self.assertEqual(self.public_tasks(), rows)

    def test_unreferenced_commit_blocks_bulk_and_single_deletion(self):
        self.gg("create", "first")
        self.gg("create", "second")
        first, second = self.public_tasks()
        pid = self.tmux("display-message", "-p", "-t", self.terminal(1), "#{pane_pid}").stdout
        commit = self.make_commit(second["path"], "detached only")
        for target in ("second", "--all"):
            result = self.gg("delete", target, "--yes", check=False)
            self.assertNotEqual(result.returncode, 0)
            self.assertIn("git switch -c", result.stderr)
            self.assertIn("--force", result.stderr)
        self.assertEqual(self.public_tasks(), [first, second])
        self.assertEqual(self.tmux("display-message", "-p", "-t", self.terminal(1), "#{pane_pid}").stdout, pid)
        self.assertEqual(self.git("rev-parse", "HEAD", cwd=second["path"]).stdout.strip(), commit)
        self.gg("delete", "second", "--force", "--yes")
        self.gg("delete", "first", "--yes")
        self.assertEqual(self.public_tasks(), [])

    def test_detached_history_preserved_by_branch_remote_ref_or_tag(self):
        for index, ref in enumerate(["refs/heads/kept", "refs/remotes/origin/kept", "refs/tags/kept"]):
            with self.subTest(ref=ref):
                name = f"clone{index}"
                self.gg("create", name)
                path = self.public_tasks()[0]["path"]
                ancestor = self.make_commit(path, f"ancestor {index}")
                tip = self.make_commit(path, f"tip {index}")
                if ref.startswith("refs/tags/"):
                    self.git("-c", "user.name=Test", "-c", "user.email=test@localhost",
                             "tag", "-a", "kept", "-m", "keep history", tip)
                else:
                    self.git("update-ref", ref, tip)
                self.git("switch", "--detach", ancestor, cwd=path)
                self.gg("delete", name, "--yes")
                self.assertFalse(Path(path).exists())
                self.git("merge-base", "--is-ancestor", ancestor, ref)

    def test_failed_creation_preserves_existing_refs_and_paths(self):
        self.git("branch", "clone1")
        hook = self.root / ".git/hooks/post-checkout"
        hook.write_text("#!/bin/sh\necho checkout-failed >&2\nexit 128\n")
        hook.chmod(0o755)
        path = Path(str(self.root) + "__sessions") / "clone1"
        for args in [(), ("--branch", "new/task"), ("--branch", "release/demo")]:
            with self.subTest(args=args):
                result = self.gg("create", "clone1", *args, check=False)
                self.assertNotEqual(result.returncode, 0)
                self.assertIn("checkout-failed", result.stderr)
                self.assertFalse(path.exists())
                self.assertEqual(self.public_tasks(), [])
                self.git("show-ref", "--verify", "refs/heads/clone1")
                self.git("show-ref", "--verify", "refs/heads/release/demo")
                self.assertNotEqual(self.git("show-ref", "--verify", "refs/heads/new/task", check=False).returncode, 0)
        hook.unlink()
        self.git("worktree", "add", "--detach", str(path), "HEAD")
        shutil.rmtree(path)  # Simulate a missing external checkout with live Git registration.
        self.assertNotEqual(self.gg("create", "clone1", "--branch", "new/task", check=False).returncode, 0)
        self.assertNotEqual(self.git("show-ref", "--verify", "refs/heads/new/task", check=False).returncode, 0)
        self.assertIn(str(path), self.git("worktree", "list", "--porcelain").stdout)

    def test_add_race_preserves_native_worktree_with_identical_detached_head(self):
        real_git = shutil.which("git", path=self.env["PATH"])
        shim_dir = Path(self.tmp.name) / "shim"
        shim_dir.mkdir()
        shim = shim_dir / "git"
        path = Path(str(self.root) + "__sessions") / "contested"
        winner = path / "external-file"
        # Insert a native writer just before our worktree add and give it the
        # identical commit. Only the competing invocation reached checkout.
        shim.write_text(
            "#!/bin/sh\n"
            'if [ "$1 $2" = "worktree add" ]; then\n'
            f"  {shlex.quote(real_git)} worktree add --detach -- {shlex.quote(str(path))} HEAD || exit 1\n"
            f"  touch {shlex.quote(str(winner))}\n"
            "fi\n"
            f'exec {shlex.quote(real_git)} "$@"\n'
        )
        shim.chmod(0o755)
        hook = self.root / ".git/hooks/post-checkout"
        hook.write_text("#!/bin/sh\nexit 0\n")
        hook.chmod(0o755)
        env = dict(self.env, PATH=str(shim_dir) + os.pathsep + self.env["PATH"])
        result = self.gg("create", "contested", check=False, env=env)
        self.assertNotEqual(result.returncode, 0)
        self.assertTrue(winner.is_file())
        self.assertIn(str(path), self.git("worktree", "list", "--porcelain").stdout)
        self.assertEqual(self.public_tasks(), [])

    def test_explicit_local_branch_selection_uses_exact_ref(self):
        self.git("tag", "shared")
        self.git("tag", "@")
        branch_head = self.make_commit(self.root, "local branches")
        self.git("branch", "shared")
        self.git("branch", "@")
        self.make_commit(self.root, "caller")
        for name, branch in (("clone1", "shared"), ("@", "@")):
            if name == branch:
                self.gg("create", "--branch", branch)
            else:
                self.gg("create", name, "--branch", branch)
            row = next(row for row in self.public_tasks() if row["name"] == name)
            self.assertEqual(row["branch"], branch)
            self.assertEqual(row["base"], branch_head)
            self.assertEqual(self.git("rev-parse", "HEAD", cwd=row["path"]).stdout.strip(), branch_head)

    def test_delete_rechecks_detached_commit_after_stopping_the_terminal(self):
        self.gg("create", "clone1")
        row = self.public_tasks()[0]
        path = Path(row["path"])
        marker = Path(self.tmp.name) / "preflight-checked"
        shim_dir = Path(self.tmp.name) / "tools"
        shim_dir.mkdir()
        real_git = shlex.quote(shutil.which("git", path=self.env["PATH"]))
        shim = shim_dir / "git"
        shim.write_text(
            "#!/bin/sh\n"
            'if [ "$1 $2 $3 $4" = "status --porcelain -z --untracked-files=all" ]; then\n'
            f"  if [ -f {shlex.quote(str(marker))} ]; then\n"
            f"    {real_git} -c user.name=Test -c user.email=test@localhost commit --allow-empty -qm late || exit 1\n"
            f"  else touch {shlex.quote(str(marker))}; fi\n"
            "fi\n"
            f'exec {real_git} "$@"\n'
        )
        shim.chmod(0o755)
        env = dict(self.env, PATH=str(shim_dir) + os.pathsep + self.env["PATH"])
        result = self.gg("delete", "clone1", "--yes", env=env, check=False)
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("git switch -c", result.stderr)
        self.assertEqual(self.git("log", "-1", "--format=%s", cwd=path).stdout.strip(), "late")
        remaining = self.public_tasks()
        self.assertEqual([task["id"] for task in remaining], [row["id"]])
        self.assertFalse(remaining[0]["active"])

    def test_post_checkout_matches_native_git_and_preserves_user_tracing(self):
        hooks = self.root / "custom hooks"
        hooks.mkdir()
        self.git("config", "core.hooksPath", hooks.name)
        hook = hooks / "post-checkout"
        hook.write_text(
            "#!/usr/bin/env python3\n"
            "import json, os, sys\n"
            "from pathlib import Path\n"
            "keys = ['GIT_DIR', 'GIT_WORK_TREE', 'GIT_EXEC_PATH', 'GIT_PREFIX', 'PATH', 'GIT_TRACE2_EVENT']\n"
            "event = {'argv': sys.argv, 'cwd': os.getcwd(), 'stdin': sys.stdin.read(),"
            " 'env': {k: os.getenv(k) for k in keys}, 'readme': Path('README').read_text()}\n"
            "Path(os.environ['HOOK_REPORT']).write_text(json.dumps(event))\n"
        )
        hook.chmod(0o755)
        target = Path(str(self.root) + "__sessions") / "clone"
        target.parent.mkdir()
        trace = Path(self.tmp.name) / "user-trace.jsonl"
        report = Path(self.tmp.name) / "hook-report.json"
        temp = Path(self.tmp.name) / "temporary files"
        temp.mkdir()
        env = dict(self.env, GIT_TRACE2_EVENT=str(trace), HOOK_REPORT=str(report), TMPDIR=str(temp))
        self.git("worktree", "add", "--detach", str(target), "HEAD", env=env)
        expected = json.loads(report.read_text())
        self.git("worktree", "remove", str(target))
        report.unlink()
        trace.unlink()
        self.gg("create", "clone", env=env)
        self.assertEqual(json.loads(report.read_text()), expected)
        records = [json.loads(line) for line in trace.read_text().splitlines()]
        self.assertTrue(any(event.get("event") == "start" and
                            event.get("argv", [])[1:3] == ["worktree", "add"] for event in records))
        self.assertTrue(any(event.get("event") == "child_start" and
                            event.get("hook_name") == "post-checkout" for event in records))
        self.assertEqual(list(temp.glob("gg-add-*")), [])

    def test_rollback_preserves_replacement_at_the_owned_path(self):
        hook = self.root / ".git/hooks/post-checkout"
        hook.write_text(
            "#!/bin/sh\nset -eu\n"
            "original=$(pwd -P)\n"
            f"cd {shlex.quote(str(self.root))}\n"
            'git worktree move "$original" "$MOVED_TO"\n'
            'git -c core.hooksPath=/dev/null worktree add --detach -- "$original" HEAD\n'
            'touch "$original/external-file"\nexit "$HOOK_STATUS"\n'
        )
        hook.chmod(0o755)
        for code in (0, 3):
            with self.subTest(hook_status=code):
                name = f"clone{code}"
                original = Path(str(self.root) + "__sessions") / name
                moved = Path(self.tmp.name) / f"moved-{name}"
                env = dict(self.env, MOVED_TO=str(moved), HOOK_STATUS=str(code))
                self.assertNotEqual(self.gg("create", name, check=False, env=env).returncode, 0)
                self.assertTrue((original / "external-file").is_file())
                self.assertTrue((moved / ".git").is_file())
                self.assertEqual(self.public_tasks(), [])
