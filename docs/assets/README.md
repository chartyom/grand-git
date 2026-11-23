# README demo

`gg-demo.gif` records the built `gg` CLI and dashboard in a real tmux terminal. `gg-demo.png` is its final frame. Both READMEs reference these files using relative paths, so the media stays with the repository on GitHub and in clones.

The walkthrough creates `feat/login`, edits `app.py`, creates `fix/header` through the dashboard, and returns to the original terminal. The second worktree retains the original file while the first retains its uncommitted edit.

## Regenerate

The recorder runs from a source checkout. In addition to the project's build and runtime dependencies, it uses Python 3.11+ with venv and pip, the DejaVu Sans Mono font, and [agg 1.9.0](https://github.com/asciinema/agg/releases/tag/v1.9.0) on PATH.

From the repository root:

```sh
go build -o bin/gg ./cmd/gg
python3 -m venv /tmp/grand-git-demo-venv
/tmp/grand-git-demo-venv/bin/python -m pip install Pillow==13.0.0
/tmp/grand-git-demo-venv/bin/python scripts/record-demo.py
```

Use `--agg /path/to/agg` if the renderer is outside PATH. The script replaces both media files. It uses a temporary Git repository, isolated config/state directories and dedicated tmux servers; cleanup stops only those servers. Shell and tmux startup use no user configuration.

The capture uses 108 columns and 26 rows, a 16-pixel font and a 12 FPS cap. The GIF opens with a dashboard preview, loops and holds the final frame for three seconds. The script checks worktree isolation, terminal PID continuity, running sessions after dashboard exit, the GIF's animation/loop metadata and a 2 MiB size limit before saving the assets.
