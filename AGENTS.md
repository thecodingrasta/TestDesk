<!-- testdesk:start -->
# TestDesk

Use TestDesk MCP for configured project tests. Inspect `testdesk_capabilities` or `testdesk_snapshot` before queueing unfamiliar work. A known native test selector can run directly when orchestration adds no value.

- Find test IDs with `testdesk_tests`; narrow by project, provider, layer, module or query.
- Pass an explicit `project` name or absolute root when working across repositories.
- Preview broad suites with `testdesk_plan`; prefer explicit IDs for focused checks.
- Queue with `testdesk_run`, wait with `testdesk_wait`, then inspect status.
- On failure, request `testdesk_diagnostics` with `maxBytes: 4096` and use `testdesk_test_details` for identity and failure evidence. Redact sensitive output before sharing.
- After a fix, use `testdesk_rerun_failed` for that project. Do not stop another client's work without authorization.
- Provider configuration runs local commands with user permissions. Review untrusted configurations before running or installing instructions.

Read only the skill needed:

| Task | Skill |
|---|---|
| Setup and workflow | `.agents/skills/testdesk-guide/SKILL.md` |
| Plan, run and control | `.agents/skills/testdesk-running/SKILL.md` |
| Diagnose failures | `.agents/skills/testdesk-debugging/SKILL.md` |
| Configure multiple projects | `.agents/skills/testdesk-projects/SKILL.md` |

<!-- testdesk:end -->
