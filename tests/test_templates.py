"""Startup templates tested with isolated real Git, tmux, and PTYs."""
import json
import os
from pathlib import Path
import shlex
import shutil
import time

from test_integration import SessionFixture, BIN, wait_for


class TemplateTests(SessionFixture):
    def template_file(self, name, commands, local=True):
        base = self.root / ".gg" if local else Path(self.env["XDG_CONFIG_HOME"]) / "grand-git"
        path = base / "templates" / (name + ".yaml")
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text(json.dumps({"commands": commands}))
        return path

    def send(self, pane, command):
        self.tmux("send-keys", "-t", pane, "-l", command)
        self.tmux("send-keys", "-t", pane, "Enter")

    def prepare_zsh(self, rc=""):
        # tmux captures the environment when the project's server starts.
        directory = Path(self.tmp.name) / "zsh"
        directory.mkdir()
        (directory / ".zshenv").write_text("unsetopt GLOBAL_RCS\n")
        (directory / ".zshrc").write_text(rc)
        self.env["ZDOTDIR"] = str(directory)

    def test_builtin_presets_launch_existing_executables_without_installing(self):
        shim = Path(self.tmp.name) / "programs"
        shim.mkdir()
        for command in ["codex", "claude", "npm"]:
            program = shim / command
            program.write_text("#!/bin/sh\nprintf '%s|%s' " + shlex.quote(command) + " \"$*\" > preset.log\n")
            program.chmod(0o700)
        self.env["PATH"] = str(shim) + os.pathsep + self.env["PATH"]
        for name, expected in [("codex", "codex|"), ("claude", "claude|"), ("npm-dev", "npm|run dev")]:
            self.gg("create", "preset-" + name, "--template", name)
            row = next(t for t in self.tasks() if t["branch"] == "preset-" + name)
            log = Path(row["path"]) / "preset.log"
            wait_for(lambda: log.exists())
            self.assertEqual(log.read_text(), expected)
            wait_for(lambda: "startup exited with status 0" in self.capture(self.terminal(row["id"])))

    def test_plain_shell_and_extra_terminal_receive_their_own_literal_environment(self):
        self.env.update(GG_BRANCH="wrong", GG_WORKTREE="wrong", GG_PROJECT_ROOT="wrong")
        branch = "feat/#{session_name}"
        self.create(branch)
        row = self.tasks()[0]
        worktree = Path(row["path"])
        command = "printf '%s|%s|%s' \"$GG_BRANCH\" \"$GG_WORKTREE\" \"$GG_PROJECT_ROOT\""
        self.send(self.terminal(row["id"]), command + " > plain-env")
        wait_for(lambda: (worktree / "plain-env").exists())
        expected = "|".join([branch, str(worktree), str(self.root)])
        self.assertEqual((worktree / "plain-env").read_text(), expected)
        proc, master = self.attach_session(cwd=worktree)
        wait_for(lambda: self.active() == str(row["id"]) + "|")
        existing_panes = {p[0] for p in self.panes(row["id"])}
        os.write(master, b"\x17t")
        wait_for(lambda: "Add terminal" in self.capture(self.sidebar(row["id"])))
        os.write(master, b"r")
        wait_for(lambda: len([p for p in self.geometry(row["id"]) if p[2] != "1"]) == 2)
        pane = next(p[0] for p in self.panes(row["id"])
                    if p[2] != "1" and p[0] not in existing_panes)
        self.send(pane, command + " > extra-env")
        wait_for(lambda: (worktree / "extra-env").exists())
        self.assertEqual((worktree / "extra-env").read_text(), expected)

    def test_templates_discovery_precedence_and_global_commands_outside_git(self):
        global_file = self.template_file("codex", ["printf global > startup"], local=False)
        local_file = self.template_file("codex", ["printf local > startup"])
        catalog = json.loads(self.gg("template", "list", "--json").stdout)
        self.assertEqual({"name", "source"}, set(catalog[0]))
        self.assertEqual(next(t for t in catalog if t["name"] == "codex")["source"], str(local_file))
        self.assertIn("printf local", self.gg("template", "show", "codex").stdout)
        self.assertIn("printf global", self.gg("template", "show", "codex", cwd=self.tmp.name).stdout)
        self.gg("set", "default_template", "codex", cwd=self.tmp.name)
        self.create("feat/local")
        row = self.tasks()[0]
        marker = Path(row["path"]) / "startup"
        wait_for(lambda: marker.exists())
        self.assertEqual(marker.read_text(), "local")
        self.assertEqual(row["template"], "codex")
        self.gg("stop", row["branch"])
        local_file.unlink()
        self.gg("start", row["branch"])
        wait_for(lambda: marker.read_text() == "global")
        self.assertTrue(global_file.exists())
        self.assertFalse((self.root / ".gg/config.yaml").exists())

    def test_defaults_apply_only_to_new_sessions_and_explicit_selection_wins(self):
        self.create("old")
        self.template_file("chosen", ["printf chosen > chosen"])
        self.gg("set", "default_template", "chosen", "--local")
        self.create("new")
        self.gg("create", "explicit", "--template", "shell")
        rows = {t["branch"]: t for t in self.public_tasks()}
        self.assertNotIn("main", rows)
        self.assertEqual(rows["old"]["template"], "shell")
        self.assertEqual(rows["new"]["template"], "chosen")
        self.assertEqual(rows["explicit"]["template"], "shell")
        self.gg("stop", "old")
        self.gg("start", "old")
        self.assertFalse((Path(rows["old"]["path"]) / "chosen").exists())
        proc, master = self.attach_session()
        wait_for(lambda: self.active() == "0|")
        self.assertFalse((self.root / "chosen").exists())

    def test_restart_uses_current_definition_and_active_start_does_not_repeat(self):
        path = self.template_file("versioned", ["printf 'one\\n' >> trace"])
        self.gg("create", "feat/versioned", "--template", "versioned")
        row = self.tasks()[0]
        trace = Path(row["path"]) / "trace"
        wait_for(lambda: trace.exists())
        self.gg("start", row["branch"])
        self.assertEqual(trace.read_text(), "one\n")
        self.gg("set", "default_template", "shell", "--local")
        self.gg("stop", row["branch"])
        self.template_file("versioned", ["printf 'two\\n' >> trace"])
        self.gg("start", row["branch"])
        wait_for(lambda: trace.read_text() == "one\ntwo\n")
        self.assertNotEqual(self.gg("start", row["branch"], "--template", "shell", check=False).returncode, 0)
        self.assertEqual(self.tasks()[0]["template"], "versioned")
        self.gg("stop", row["branch"])
        path.unlink()
        self.assertNotEqual(self.gg("start", row["branch"], check=False).returncode, 0)
        self.assertFalse(self.tasks()[0]["active"])
        self.gg("start", row["branch"], "--template", "shell")
        self.assertEqual(self.tasks()[0]["template"], "shell")
        self.assertEqual(trace.read_text(), "one\ntwo\n")

    def test_shared_config_reads_primary_and_ignored_gg_is_not_copied(self):
        (self.root / ".gitignore").write_text(".gg/\n.env\n")
        self.git("add", ".gitignore")
        self.commit("ignore gg")
        (self.root / ".env").write_text("local env")
        self.template_file("common", ["printf common > common"])
        self.gg("set", "default_template", "common", "--local")
        self.create("feat/one")
        one = Path(self.tasks()[0]["path"])
        self.assertFalse((one / ".gg").exists())
        self.assertEqual((one / ".env").read_text(), "local env")
        (one / ".gg").mkdir()
        (one / ".gg/config.yaml").write_text("default_template: missing\n")
        self.gg("create", "feat/two", cwd=one)
        two = next(t for t in self.tasks() if t["branch"] == "feat/two")
        self.assertEqual(two["template"], "common")
        self.gg("set", "language", "ru", "--local", cwd=one)
        self.assertIn("language: ru", (self.root / ".gg/config.yaml").read_text())
        self.assertNotIn("language", (one / ".gg/config.yaml").read_text())

    def test_migration_keeps_old_files_and_reset_does_not_revive_legacy(self):
        global_file = Path(self.env["XDG_CONFIG_HOME"]) / "grand-git/config.toml"
        global_file.parent.mkdir(parents=True)
        global_file.write_text('language = "ru"\nnavigation = "alt-arrows"\n')
        local_file = self.root / ".git/grand-git/config.toml"
        local_file.parent.mkdir(parents=True)
        local_file.write_text('language = "en"\n')
        self.assertIn("language = en", self.gg("config").stdout)
        self.gg("set", "default_template", "shell", cwd=self.tmp.name)
        self.gg("config", "set", "default_template", "codex", "--project")
        self.assertTrue(global_file.exists())
        self.assertTrue(local_file.exists())
        self.assertNotIn("navigation", (Path(self.env["XDG_CONFIG_HOME"]) / "grand-git/config.yaml").read_text())
        self.gg("reset", "language", "--local")
        self.gg("reset", "default_template", "--local")
        self.assertEqual((self.root / ".gg/config.yaml").read_text().strip(), "{}")
        self.assertIn("language = ru", self.gg("config").stdout)
        self.assertNotIn("navigation", self.gg("config").stdout)

    def test_commands_share_shell_state_fail_fast_and_environment_is_literal(self):
        branch = "feat/$(touch${IFS}injected)"
        commands = ["export GG_FIXTURE_VALUE='first value'", "mkdir inner", "cd inner",
                    "printf '%s|%s|%s|%s' \"$GG_FIXTURE_VALUE\" \"$GG_BRANCH\" \"$GG_WORKTREE\" \"$GG_PROJECT_ROOT\" > ../context",
                    "false", "touch ../must-not-run"]
        self.template_file("context", commands)
        self.gg("create", branch, "--template", "context")
        row = self.tasks()[0]
        worktree = Path(row["path"])
        wait_for(lambda: (worktree / "context").exists())
        self.assertEqual((worktree / "context").read_text(), "|".join(["first value", branch, str(worktree), str(self.root)]))
        pane = self.terminal(row["id"])
        wait_for(lambda: "startup exited with status 1" in self.capture(pane))
        self.send(pane, "printf fallback > ../fallback")
        wait_for(lambda: (worktree / "fallback").exists())
        self.assertFalse((worktree / "must-not-run").exists())
        self.assertFalse((worktree / "injected").exists())
        self.assertTrue(self.tasks()[0]["active"])

    def test_ctrl_c_returns_to_shell_for_sh_bash_and_zsh(self):
        self.prepare_zsh()
        for name in ["sh", "bash", "zsh"]:
            executable = shutil.which(name)
            if not executable:
                continue
            with self.subTest(shell=name):
                # Exercise native shell behavior without sourcing personal shell files.
                env = dict(self.env, HISTFILE="/dev/null")
                if name == "bash":
                    wrapper = Path(self.tmp.name) / "bash"
                    wrapper.write_text("#!/bin/sh\nexec " + shlex.quote(executable) + ' --noprofile --norc "$@"\n')
                    wrapper.chmod(0o700)
                    executable = str(wrapper)
                env["SHELL"] = executable
                self.template_file("interrupt", ["sleep 30", "touch must-not-run"])
                self.gg("create", "interrupt-" + name, "--template", "interrupt", env=env)
                row = next(t for t in self.tasks() if t["branch"] == "interrupt-" + name)
                pane = self.terminal(row["id"])
                wait_for(lambda: self.tmux("display-message", "-p", "-t", pane, "#{pane_current_command}").stdout.strip() == "sleep")
                self.tmux("send-keys", "-t", pane, "C-c")
                wait_for(lambda: "startup exited with status 130" in self.capture(pane))
                self.send(pane, "printf ready > fallback-ready")
                wait_for(lambda: (Path(row["path"]) / "fallback-ready").exists())
                self.assertFalse((Path(row["path"]) / "must-not-run").exists())
                self.gg("stop", row["branch"])

    def test_zsh_interactive_rc_is_loaded_before_template(self):
        executable = shutil.which("zsh")
        if not executable:
            self.skipTest("zsh unavailable")
        self.prepare_zsh("gg_fixture_function() { printf from-rc > rc-ready; }\n")
        self.env.update(SHELL=executable, HISTFILE="/dev/null")
        self.template_file("rc", ["gg_fixture_function"])
        self.gg("create", "feat/rc", "--template", "rc")
        path = Path(self.tasks()[0]["path"]) / "rc-ready"
        wait_for(lambda: path.exists())
        self.assertEqual(path.read_text(), "from-rc")

    def test_unknown_invalid_and_unsupported_templates_have_no_worktree_side_effects(self):
        self.template_file("invalid", ["echo ok"])
        (self.root / ".gg/templates/invalid.yaml").write_text("commands: [12]\n")
        for name in ["missing", "../escape", "invalid"]:
            result = self.gg("create", "feat/invalid", "--template", name, check=False)
            self.assertNotEqual(result.returncode, 0)
            self.assertFalse(Path(str(self.root) + "__sessions/feat/invalid").exists())
            self.assertNotEqual(self.git("show-ref", "--verify", "refs/heads/feat/invalid", check=False).returncode, 0)
        result = self.gg("create", "no-tty", "--choose-template", check=False)
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("interactive terminal", result.stderr)
        fake = Path(self.tmp.name) / "fish"
        fake.write_text('#!/bin/sh\nexec /bin/sh "$@"\n')
        fake.chmod(0o700)
        self.assertNotEqual(self.gg("create", "unsupported", "--template", "codex", env=dict(self.env, SHELL=str(fake)), check=False).returncode, 0)
        self.assertFalse((self.root / ".git/grand-git/state.json").exists())

    def test_commands_observe_committed_registry(self):
        script = "import json,os;from pathlib import Path;s=json.loads((Path(os.environ['GG_PROJECT_ROOT'])/'.git/grand-git/state.json').read_text());assert any(t['branch']==os.environ['GG_BRANCH'] for t in s['tasks']);Path('registered').write_text('yes')"
        self.template_file("registered", ["python3 -c " + shlex.quote(script)])
        self.gg("create", "feat/registered", "--template", "registered")
        row = self.tasks()[0]
        wait_for(lambda: (Path(row["path"]) / "registered").exists())
        wait_for(lambda: not list((self.root / ".git/grand-git/launches").glob("*.json")))

    def test_failed_registry_commit_never_runs_template(self):
        marker = Path(self.tmp.name) / "must-not-execute"
        self.template_file("guarded", ["touch " + shlex.quote(str(marker))])
        shim = Path(self.tmp.name) / "shim"
        shim.mkdir()
        registry = self.root / ".git/grand-git/state.json"
        real = shutil.which("tmux")
        script = "#!/usr/bin/python3\nimport pathlib,subprocess,sys\nargs=sys.argv[1:]\nresult=subprocess.run([" + repr(real) + "]+args)\nif len(args)>3 and args[2]=='select-pane' and '-T' not in args:\n    pathlib.Path(" + repr(str(registry)) + ").mkdir(exist_ok=True)\nsys.exit(result.returncode)\n"
        (shim / "tmux").write_text(script)
        (shim / "tmux").chmod(0o700)
        env = dict(self.env, PATH=str(shim) + os.pathsep + self.env["PATH"])
        result = self.gg("create", "feat/fail-commit", "--template", "guarded", env=env, check=False)
        self.assertNotEqual(result.returncode, 0)
        self.assertTrue(registry.is_dir(), result.stderr)
        self.assertFalse(marker.exists())
        self.assertFalse(Path(str(self.root) + "__sessions/feat/fail-commit").exists())
        self.assertNotEqual(self.git("show-ref", "--verify", "refs/heads/feat/fail-commit", check=False).returncode, 0)

    def test_cli_picker_selection_and_cancellation(self):
        self.template_file("zz-choice", ["printf selected > picked"])
        proc, master = self.start_client([str(BIN), "create", "feat/picked", "--choose-template"])
        wait_for(lambda: b"Choose template" in self.outputs[proc.pid])
        os.write(master, b"j\r")  # shell is selected initially; zz-choice follows.
        wait_for(lambda: proc.poll() is not None)
        self.assertEqual(proc.returncode, 0, bytes(self.outputs[proc.pid]))
        row = self.tasks()[0]
        self.assertEqual(row["template"], "zz-choice")
        wait_for(lambda: (Path(row["path"]) / "picked").exists())
        proc, master = self.start_client([str(BIN), "create", "feat/canceled", "--choose-template"])
        wait_for(lambda: b"Choose template" in self.outputs[proc.pid])
        os.write(master, b"\x1b")
        wait_for(lambda: proc.poll() is not None)
        self.assertEqual(len(self.tasks()), 1)

    def test_dashboard_picker_and_extra_terminal_do_not_repeat_template(self):
        self.template_file("zz-choice", ["printf 'once\\n' >> picked"])
        proc, master = self.attach_session()
        wait_for(lambda: self.active() == "0|")
        os.write(master, b"\x17n")
        wait_for(lambda: b"Create session" in self.outputs[proc.pid])
        os.write(master, b"feat/dashboard\t")
        wait_for(lambda: b"Choose template" in self.outputs[proc.pid])
        os.write(master, b"j\r\r")
        wait_for(lambda: len(self.tasks()) == 1)
        row = self.tasks()[0]
        wait_for(lambda: self.active() == str(row["id"]) + "|")
        trace = Path(row["path"]) / "picked"
        wait_for(lambda: trace.exists())
        self.assertEqual(row["template"], "zz-choice")
        os.write(master, b"\x17t")
        wait_for(lambda: "Add terminal" in self.capture(self.sidebar(row["id"])))
        os.write(master, b"r")
        wait_for(lambda: len([p for p in self.geometry(row["id"]) if p[2] != "1"]) == 2)
        self.assertEqual(trace.read_text(), "once\n")
