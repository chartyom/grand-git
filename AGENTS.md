# Working on Grand Git

- Write repository text in English; preserve Russian localization and `README.ru.md`.
- Read `openspec/README.md`, `openspec/project.md` and affected main specifications before planning or changing behavior. The user's latest instructions take precedence.
- Keep plans in chat. Create `openspec/changes/` materials only when the user explicitly requests them.
- Keep code, tests, help, documentation and main specifications consistent. Check affected SHALL requirements and scenarios; record actual evidence in `openspec/verification.md`.
- Document implemented behavior. Keep assistant opinions, speculative caveats and work-status commentary in terminal/chat responses.
- Keep OpenSpec current: no completed archives or deferred wish lists. Consolidate and remove completed change materials if any were explicitly requested.
- Build with `go build -o bin/gg ./cmd/gg`. After CLI/dashboard changes run `sh scripts/test.sh`; after specification changes run `sh scripts/check-specs.sh` (pinned npx, no global install).
- After public commands, flags, JSON, settings or dashboard/session behavior change, delegate usage-skill review to the [skill-maintainer](.codex/agents/skill-maintainer.toml). Its write scope is [the skill directory](docs/skills/grand-git/). Record the actual review result; mark it incomplete if delegation is unavailable.
- Use temporary repositories and isolated tmux servers for tests; never use personal projects as fixtures.
- Do not change global Codex, Claude, shell or tmux settings without a separate instruction. Never grant automatic hook trust.
- Deleting modified or untracked task files requires explicit `--force`; preserve Git branches. Never delete source/external protected worktrees, even with `--force`.
- Dashboard upgrades restart only service panes and preserve user processes. Report the result, checks and material limitations briefly in the terminal/chat response.
