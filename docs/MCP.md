# MCP integration

TestDesk exposes a local stdio MCP server:

```text
testdesk mcp
```

`--project <path>` is optional. Without it, TestDesk serves every registered project.

Use **Settings → MCP / agentic AI** to detect targets, install with one click, or copy a manual snippet. The installer knows Claude (Desktop and Code), Antigravity and Cursor, Windsurf, Cline / Roo Code / VS Code, Codex and OpenCode, and it can show the universal `mcpServers` JSON that most other hosts accept. From a terminal, `testdesk-cli mcp-targets --project <path>` lists the targets it detects and `mcp-install --target <id>` installs one.

The packaged desktop's `mcp` command connects to the running desktop or starts it in the tray, then proxies the client's requests to a desktop-owned session. Startup fails if the tray-backed service does not become ready. Each client has its own project default and disconnecting it cancels its own work. The standalone CLI looks for a separate desktop executable beside it; set `TESTDESK_DESKTOP_EXECUTABLE` to the desktop executable when it is installed elsewhere. See [installation](INSTALL.md).

New desktop versions use a private startup handoff. Before the desktop is ready to serve clients, a cancelled launcher can reclaim its own verified child; after the handoff, the desktop remains available to other clients. For compatibility, a launcher does not forcibly reclaim a desktop that never acknowledges the protocol, since it may be an older shared version.

Install is always explicit. Before TestDesk changes an existing target configuration, it writes a timestamped `.testdesk-backup-*` file beside it. Restore that file if an install needs to be reverted.

Key v1 tools:

- `testdesk_capabilities`, `testdesk_projects`, `testdesk_project_activate`, `testdesk_project_add`, `testdesk_project_forget_missing`, `testdesk_project_configure`;
- `testdesk_snapshot`, `testdesk_tests`, `testdesk_discover`, `testdesk_discovery_status`, `testdesk_plan`, `testdesk_run`, `testdesk_run_status`, `testdesk_control`;
- `testdesk_diagnostics`, `testdesk_artifacts`, `testdesk_settings`, `testdesk_history`, `testdesk_test_details`, `testdesk_rerun_failed`, `testdesk_skills_install`;
- `testdesk_mcp_install_targets`, `testdesk_mcp_install`, `testdesk_mcp_manual_config`.

Standard MCP resources & prompts:
- **Resources**: `testdesk://guide`, `testdesk://capabilities` and `testdesk://projects`.
- **Prompts**: `testdesk-verify-changes`, `testdesk-debug-failure` and `testdesk-plan-suite`.

Responses are bounded by default. Use IDs and filters, paginate catalogs, plan broad runs and request diagnostics only for a selected failing run.

`testdesk_test_details` accepts a test ID or a uniquely matching name in the selected project. If a name matches more than one test, use its `testId` from the catalog. When both ID and name are supplied, they must identify the same test; an unknown ID does not fall back to the name. Failure details are included only when the recorded test identity can be matched safely.

For discovery progress, include a string or integer `progressToken` in the tool call's `_meta`. TestDesk sends `notifications/progress` with the active provider in the message and completed provider scans as the progress units. Tokens encoded in more than 256 bytes do not receive progress notifications. A client can cancel its own pending discovery or wait with `notifications/cancelled` and the original request ID; cancellation does not stop another client's request. A cancelled request need not receive a response.

Poll `testdesk_discovery_status` for the active or latest discovery in a project. It reports the provider, phase, start time and time since the last complete stdout or stderr line observed; it never includes the line itself. Before any line is observed, `lastOutputAt` and `quietMillis` are absent. Output can be buffered, so silence does not establish that a process is stalled or healthy.

Plan and run items can include `warnings` when selected .NET providers may overlap. Matching names alone do not prove that tests are identical, so TestDesk keeps each requested item; inspect the provider IDs and select the intended scope. Explicitly configured providers retain their IDs when new providers are detected. Detection avoids adding an equivalent .NET provider for the same scope and commands.

A provider-only request plans discovered cases for each requested provider and a provider-wide item for each requested provider without discovered cases. Inspect the plan before running: these scopes can have different granularity. Unknown or disabled provider IDs are rejected, including in mixed lists.

When test IDs are combined with provider IDs or other filters, every requested test must satisfy those constraints; a contradictory request is rejected rather than partially executed. Provider IDs also constrain filtered requests that do not specify test IDs.

`testdesk_rerun_failed` retains individual-test and provider-wide items from the selected run. When a `runId` is supplied it must identify an existing run; an invalid or missing ID does not fall back to the latest run.

Wait and discovery requests can run alongside status and control requests on the same connection. Responses can arrive out of order; match them by JSON-RPC ID and wait for a discovery response before requesting its updated catalog. Each connection accepts up to 16 pending wait or discovery requests; further requests receive an error. Concurrent discoveries for the same project are serialized, while different projects remain independent. A request without an explicit project keeps the session default selected when the request arrived.

## Project selection and concurrent runs

Call `testdesk_projects` to list the configured projects, including each display name and stable absolute root. The default page has 50 entries; use `limit` (at most 100) and `offset` with `total` and `hasMore` to navigate. `testdesk_project_forget_missing` removes one explicitly named absolute root from the registry only when it is missing and not loaded in this session; it does not delete project files. Use `testdesk_project_activate` with `project` (a unique display name) or `projectRoot` to change this MCP session's default project.

Every project-scoped MCP tool also accepts an optional `project` argument. It may be a unique configured project name or an absolute project root. An explicit argument targets only that call and does not change the default project. For example:

```json
{"project":"My API","providerIds":["unit"]}
```

TestDesk retains a separate manager, queue, catalog, run history, diagnostics and artifacts for each project. Agents can therefore run tests in different projects concurrently; select a project explicitly, or activate it, to inspect that project's status and controls. If two configured projects share a display name, use the absolute root to remove ambiguity.

For a project with an empty or stale provider list, call `testdesk_project_configure` with its project name or root. It detects the built-in provider set and merges it into the existing configuration without replacing custom providers. Set `replaceProviders` only when an intentional full reset is required. Configuration is rejected while the project has queued or running work.

## Diagnostics and controls

Diagnostics default to 32 KiB and are capped at 512000 bytes per call. Set `maxBytes: 4096` for a focused failure excerpt. This cap does not bound reports or on-disk log sizes. Run IDs must be single local path components.

Pause holds future queue work. Stop and kill share the manager cancellation path and cancel active work plus queued runs. They are not distinct graceful/forceful signal protocols.
