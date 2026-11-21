# MCP integration

TestDesk exposes a local stdio MCP server:

```text
testdesk mcp
```

`--project <path>` is optional. Without it, TestDesk serves every registered project.

Open **Settings**, switch to **Agent Integrations**, and use **Install** beside a detected workspace or pick a host under **Manual MCP setup** to copy a ready-made snippet. Supported installers cover Claude (Desktop and Code), Antigravity and Cursor, Windsurf, VS Code / GitHub Copilot (`.vscode/mcp.json` using the `servers` schema), Codex, and OpenCode, alongside universal `mcpServers` JSON and direct CLI commands. From a terminal, `testdesk-cli mcp-targets --project <path>` lists detected hosts and `testdesk-cli mcp-install --project <path> --target <id>` writes a configuration and project skills.

The packaged desktop's `mcp` command connects to the running desktop or starts it in the background, then proxies the client's requests to the desktop-owned session. Startup admits MCP once the UI/lifecycle owner is ready, even if tray registration falls back on restricted environments. Each client keeps its own session project default, and disconnecting closes/cancels only work initiated on that client's context. Standalone `testdesk-cli mcp` looks for the desktop executable beside it; set `TESTDESK_DESKTOP_EXECUTABLE` to the desktop executable's absolute path when installed elsewhere. See [installation](INSTALL.md).

Install actions always back up any existing host target file as `<config>.testdesk-backup-*` before writing changes.

## Live MCP tools (24)

Call `tools/list` on the server for current JSON Schemas and exact descriptions:

- **Discovery & catalog:** `testdesk_capabilities`, `testdesk_guide`, `testdesk_snapshot`, `testdesk_catalog`, `testdesk_tests`, `testdesk_discover`, `testdesk_discovery_status`
- **Planning, running & controls:** `testdesk_plan`, `testdesk_run`, `testdesk_run_status`, `testdesk_wait`, `testdesk_control`, `testdesk_rerun_failed`
- **Diagnostics, history & settings:** `testdesk_diagnostics`, `testdesk_artifacts`, `testdesk_history`, `testdesk_test_details`, `testdesk_settings`
- **Projects & skill provisioning:** `testdesk_projects`, `testdesk_project_activate`, `testdesk_project_add`, `testdesk_project_configure`, `testdesk_project_forget_missing`, `testdesk_skills_install`

Note: Host MCP config management (`mcp-targets`, `mcp-install`) is exposed through **Settings -> Agent Integrations** and the `testdesk` / `testdesk-cli` CLI, not as MCP tools.

## Resources and prompts

- **Resources:** `testdesk://guide`, `testdesk://capabilities`, and `testdesk://projects`
- **Prompts:** `testdesk-verify-changes`, `testdesk-debug-failure`, and `testdesk-plan-suite`

## Request behavior, progress, and scoping

Responses are bounded by default. Use `testdesk_tests` filters (`limit`, `offset`), preview broad runs with `testdesk_plan`, and pass `maxBytes: 4096` to `testdesk_diagnostics` when investigating failures.

`testdesk_test_details` accepts `testId` or a uniquely matching `name` in the selected project. If a name matches more than one test, use its `testId`. When both ID and name are supplied, they must identify the same test; an unknown ID does not fall back to name lookup. Failure details are attached only when test identity matches unambiguously.

For discovery progress, include a string or integer `progressToken` in `_meta` (encoded in at most 256 bytes). TestDesk emits `notifications/progress` with the active provider detail and completed provider count. Clients can cancel pending `testdesk_discover` or `testdesk_wait` calls with `notifications/cancelled` referencing the request ID without interrupting other clients. Concurrent discovery requests for one project serialize while different projects run in parallel; up to 16 concurrent long requests are allowed per connection.

`testdesk_discovery_status` reports the active or latest provider, phase, start timestamp, and `quietMillis` since the last complete stdout or stderr line (absent before any line arrives). Runner buffering means silence alone does not prove a stall.

When `testIds` are combined with filters (`providerIds`, `layers`, etc.), every requested test must satisfy all constraints; contradictory selections fail validation cleanly rather than running a subset. Provider-only runs queue discovered tests for providers with a populated catalog and one provider-wide item for enabled providers without discovered cases. Unknown or disabled provider IDs fail validation even in mixed lists.

`testdesk_rerun_failed` preserves mixed individual-test and provider-wide failure items from the selected `runId` (or latest failed run if omitted). Supplying an unknown explicit `runId` returns an error rather than falling back.

## Project selection and concurrent runs

Call `testdesk_projects` (`limit` up to 100, `offset`) to list registered projects with display names and absolute roots. `testdesk_project_forget_missing` unregisters one explicitly named missing, unloaded root without deleting files on disk. `testdesk_project_activate` changes only your MCP connection's default project.

All project-scoped tools also accept an optional `project` argument (unique display name or absolute root). Passing `project` scopes only that request and leaves the session default untouched. If two registered projects share a name, pass the absolute root.

For a workspace needing updated built-in detection, `testdesk_project_configure` detects built-in providers and merges them cleanly without replacing custom providers unless `replaceProviders: true` is passed. Configuration rejects updates while a run is queued or active in that project.

## Diagnostics and controls

`testdesk_diagnostics` defaults to 32,768 bytes and enforces a 512,000-byte ceiling per call. `runId` must be a single local path component. `pause` holds future queue items without suspending running subprocesses; `stop` and `kill` share the manager cancellation path and cancel active plus queued runs cleanly.
