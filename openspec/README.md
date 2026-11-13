# Grand Git specifications

`specs/` is the source of truth for current product behavior. Requirements use SHALL and concrete WHEN/THEN scenarios in OpenSpec format.

| Specification | Scope |
| --- | --- |
| [cli](specs/cli/spec.md) | Commands, global list/JSON, public IDs and creation in Git/ordinary directories |
| [sessions](specs/sessions/spec.md) | Worktrees, branch bases, registry, process persistence and safe deletion |
| [dashboard](specs/dashboard/spec.md) | Shared grouped list, panel cycling, compact mode and terminals |
| [configuration](specs/configuration/spec.md) | YAML settings, shared .gg, supported TOML migration and default_template |
| [startup-templates](specs/startup-templates/spec.md) | Templates, selection and commands after session registration |
| [codex-hooks](specs/codex-hooks/spec.md) | Project hooks, trust and agent statuses |
| [installation](specs/installation/spec.md) | Supported platforms, installation and release artifacts |
| [development-workflow](specs/development-workflow/spec.md) | Current specifications, verification and usage skill maintenance |

[project.md](project.md) describes the current architecture and environment. [verification.md](verification.md) records verification evidence.

## Maintenance

1. Read affected main specifications and the user's latest instructions.
2. Keep plans in chat. Create `changes/<change-id>/` and proposal/design/tasks/delta materials only on an explicit user request. Ordinary implementation updates main specifications and a compact `verification.md` record directly.
3. Implement the agreed change and verify affected requirements and scenarios.
4. Update main specifications and documentation, then consolidate evidence in `verification.md`. Report unperformed checks and work-status notes in the terminal/chat response.
5. Remove completed or superseded change directories and obsolete references. Keep current compatibility requirements and active work without preserving old plans.

Agent instructions are in [AGENTS.md](../AGENTS.md). This process requires no skill or hook installation into home directories.

## Validation

```sh
sh scripts/test.sh         # product behavior
sh scripts/check-specs.sh  # current specifications and active changes
```

OpenSpec validation uses `@fission-ai/openspec@2.0.0` through npx and requires Node.js 22.12+. The first run may populate the npm cache; nothing is installed globally.
