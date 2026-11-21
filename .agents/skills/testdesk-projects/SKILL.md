---
name: testdesk-projects
description: Register, switch, configure, inspect, and clean up TestDesk projects across concurrent repositories without interrupting other clients. Use when working in multi-repo setups or adding/reconfiguring workspaces.
license: GPL-3.0
---

# Manage multiple projects

1. Call `testdesk_projects` (`limit` <= 100, `offset`) to see registered workspaces, their display names, and absolute roots.
2. Pass an explicit `project` (unique name or absolute root) on project-scoped tool calls so you never alter another client's active session default.
3. Call `testdesk_project_activate` only when you intentionally want to change your own MCP session default.
4. Register a trusted repository with `testdesk_project_add` (`projectRoot`, optional `name`, `activate: false` to keep your current default). Review detected commands in `testdesk.json` or `testdesk_settings` before running them.
5. Update provider detection on an idle project with `testdesk_project_configure`. Existing custom providers are preserved unless `replaceProviders: true` is explicitly passed.
6. Provision or update repository skill/instruction files cleanly with `testdesk_skills_install`.

Read [multi-project routing](references/multi-project-routing.md) for registry cleanup (`testdesk_project_forget_missing`), project isolation guarantees, and `testdesk.json` trust checks.
