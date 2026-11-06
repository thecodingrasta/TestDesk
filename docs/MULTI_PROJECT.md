# Multi-project operation

TestDesk stores a per-user list of recent projects under the OS configuration directory. Each project retains independent `.testdesk` catalogs, run history, diagnostics and artifacts.

Use the active-project dropdown for a fast switch. The **Projects** screen supports:

- see provider/test counts and the last-opened time;
- activate or refresh a configured project;
- add a new project folder and create/detect configuration;
- refresh discovery;
- forget a project without deleting project files.

The MCP server also provides `testdesk_projects`, `testdesk_project_add` and `testdesk_project_activate`. Project activation changes the MCP session default only; it does not alter, cancel or delete other projects. All project-scoped MCP calls accept an optional `project` name or absolute root, so agents may target multiple projects concurrently without switching the default session context.
