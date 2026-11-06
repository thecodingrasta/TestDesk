---
name: testdesk-projects
description: Register, select and configure TestDesk projects without changing another agent session's default or test work.
---

# Multiple projects

List projects with `testdesk_projects`; use `limit` and `offset` for additional pages. Target a unique name or absolute root with the `project` argument on project-scoped tools. `testdesk_project_activate` changes this MCP session's default only.

Use `testdesk_project_add` with an absolute `projectRoot` to register a trusted repository. It creates `testdesk.json` only if absent. Review detected commands before running them.

For an idle project, `testdesk_project_configure` merges detected providers while preserving custom providers. Use `replaceProviders` only for an authorized full reset. Configuration is rejected while work is queued or running.

Queues and history are separate for each project. Avoid duplicate providers for the same test scope; inspect overlap warnings in execution plans. `testdesk_project_forget_missing` removes one explicitly selected missing, unloaded root from the registry without deleting files.
